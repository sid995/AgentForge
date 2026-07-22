#!/usr/bin/env bash

set -euo pipefail

kind_bin="${KIND_BIN:?KIND_BIN must be configured}"
kubectl_bin="${KUBECTL_BIN:?KUBECTL_BIN must be configured}"
node_image="${KIND_NODE_IMAGE:?KIND_NODE_IMAGE must be configured}"
cluster_name="agentforge-phase6-${RANDOM}-${RANDOM}"
kubeconfig_path="$(mktemp)"
manager_log="$(mktemp)"
manager_pid=""

cleanup() {
    if [[ -n "${manager_pid}" ]] && kill -0 "${manager_pid}" 2>/dev/null; then
        kill "${manager_pid}"
        wait "${manager_pid}" || true
    fi
    "${kind_bin}" delete cluster --name "${cluster_name}" >/dev/null 2>&1 || true
    rm -f "${kubeconfig_path}" "${manager_log}"
}
trap cleanup EXIT

"${kind_bin}" create cluster \
    --name "${cluster_name}" \
    --image "${node_image}" \
    --config test/kind/cluster.yaml \
    --kubeconfig "${kubeconfig_path}" \
    --wait 180s

export KUBECONFIG="${kubeconfig_path}"
worker_node="$("${kubectl_bin}" get nodes --selector='!node-role.kubernetes.io/control-plane' -o jsonpath='{.items[0].metadata.name}')"
if [[ -z "${worker_node}" ]]; then
    printf '%s\n' 'kind lifecycle gate could not resolve a worker node' >&2
    exit 1
fi
"${kubectl_bin}" label node "${worker_node}" agentforge.dev/node-pool=agents topology.kubernetes.io/zone=kind-zone --overwrite
"${kubectl_bin}" taint node "${worker_node}" agentforge.dev/node-pool=agents:NoSchedule --overwrite

"${kubectl_bin}" apply -f config/crd/bases/execution.agentforge.dev_agentruns.yaml
"${kubectl_bin}" wait --for=condition=Established customresourcedefinition/agentruns.execution.agentforge.dev --timeout=60s
"${kubectl_bin}" create namespace agentforge-kind-lifecycle
"${kubectl_bin}" create configmap artifact-store --namespace agentforge-kind-lifecycle --from-literal=destination=kind-test
"${kubectl_bin}" apply -f - <<'YAML'
apiVersion: scheduling.k8s.io/v1
kind: PriorityClass
metadata:
  name: agentforge-agent
value: 100000
globalDefault: false
description: AgentForge integration-test execution priority.
YAML

./bin/manager \
    --metrics-bind-address=0 \
    --health-probe-bind-address=0 \
    --leader-elect=false >"${manager_log}" 2>&1 &
manager_pid="$!"

"${kubectl_bin}" apply -f test/kind/agentrun.yaml
deadline=$((SECONDS + 300))
observed_phase=""
while (( SECONDS < deadline )); do
    observed_phase="$("${kubectl_bin}" get agentrun lifecycle-missing-evidence \
        --namespace agentforge-kind-lifecycle \
        -o jsonpath='{.status.phase}' 2>/dev/null || true)"
    if [[ "${observed_phase}" == "Failed" ]]; then
        break
    fi
    if ! kill -0 "${manager_pid}" 2>/dev/null; then
        break
    fi
    sleep 2
done
if [[ "${observed_phase}" != "Failed" ]]; then
    printf '%s\n' 'operator manager log:' >&2
    sed -n '1,240p' "${manager_log}" >&2
    "${kubectl_bin}" get agentrun,jobs,pods,persistentvolumeclaims --namespace agentforge-kind-lifecycle -o yaml >&2 || true
    exit 1
fi

failure_category="$("${kubectl_bin}" get agentrun lifecycle-missing-evidence --namespace agentforge-kind-lifecycle -o jsonpath='{.status.failureCategory}')"
ready_reason="$("${kubectl_bin}" get agentrun lifecycle-missing-evidence --namespace agentforge-kind-lifecycle -o jsonpath='{.status.conditions[?(@.type=="Ready")].reason}')"
attempt_phase="$("${kubectl_bin}" get agentrun lifecycle-missing-evidence --namespace agentforge-kind-lifecycle -o jsonpath='{.status.attempts[0].phase}')"
artifact_reference="$("${kubectl_bin}" get agentrun lifecycle-missing-evidence --namespace agentforge-kind-lifecycle -o jsonpath='{.status.artifactManifestRef}')"
job_count="$("${kubectl_bin}" get jobs --namespace agentforge-kind-lifecycle -l execution.agentforge.dev/run-id=019b0000-0000-7000-8000-000000000013 -o name | wc -l | tr -d ' ')"
pod_phase="$("${kubectl_bin}" get pods --namespace agentforge-kind-lifecycle -l execution.agentforge.dev/run-id=019b0000-0000-7000-8000-000000000013 -o jsonpath='{.items[0].status.phase}')"

if [[ "${failure_category}" != "EXECUTION" || "${ready_reason}" != "ResultEvidenceInvalid" ||
      "${attempt_phase}" != "Failed" || -n "${artifact_reference}" || "${job_count}" != "1" || "${pod_phase}" != "Succeeded" ]]; then
    printf 'unexpected lifecycle result: category=%s reason=%s attempt=%s artifact=%s jobs=%s pod=%s\n' \
        "${failure_category}" "${ready_reason}" "${attempt_phase}" "${artifact_reference}" "${job_count}" "${pod_phase}" >&2
    exit 1
fi

sleep 6
job_count_after_reconcile="$("${kubectl_bin}" get jobs --namespace agentforge-kind-lifecycle -l execution.agentforge.dev/run-id=019b0000-0000-7000-8000-000000000013 -o name | wc -l | tr -d ' ')"
if [[ "${job_count_after_reconcile}" != "1" ]]; then
    printf 'duplicate reconciliation created %s Jobs\n' "${job_count_after_reconcile}" >&2
    exit 1
fi

printf '%s\n' 'kind lifecycle integration gate passed'

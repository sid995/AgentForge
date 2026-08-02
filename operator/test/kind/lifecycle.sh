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
crd_deadline=$((SECONDS + 60))
until [[ "$("${kubectl_bin}" get customresourcedefinition/agentruns.execution.agentforge.dev -o jsonpath='{.status.conditions[?(@.type=="Established")].status}' 2>/dev/null || true)" == "True" ]]; do
    if (( SECONDS >= crd_deadline )); then
        printf '%s\n' 'AgentRun CRD did not become Established within 60 seconds' >&2
        exit 1
    fi
    sleep 1
done
"${kubectl_bin}" create namespace agentforge-kind-lifecycle
"${kubectl_bin}" create configmap artifact-store --namespace agentforge-kind-lifecycle --from-literal='config.json={"schemaVersion":1,"backend":"filesystem","root":"/workspace/artifacts"}'
"${kubectl_bin}" create configmap runner-trust --namespace agentforge-kind-lifecycle --from-file=task-trust.json=test/kind/runner-task-trust.json
"${kubectl_bin}" create secret generic runner-task --namespace agentforge-kind-lifecycle --from-file=envelope.json=test/kind/runner-task.json --from-file=envelope.json.sig=test/kind/runner-task.json.sig
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

(cd .. && docker build --tag agentforge/runner:kind --file agent-runner/Dockerfile .)
"${kind_bin}" load docker-image --name "${cluster_name}" agentforge/runner:kind
runner_image="$(
    docker exec "${cluster_name}-control-plane" ctr --namespace k8s.io images list |
        awk '$1 == "docker.io/agentforge/runner:kind" && $3 ~ /^sha256:[a-f0-9]{64}$/ { print "docker.io/agentforge/runner@" $3; exit }'
)"
if [[ ! "${runner_image}" =~ ^docker\.io/agentforge/runner@sha256:[a-f0-9]{64}$ ]]; then
    printf '%s\n' 'runner kind gate could not resolve a digest-qualified image' >&2
    exit 1
fi
while IFS= read -r node; do
    docker exec "${node}" ctr --namespace k8s.io images tag docker.io/agentforge/runner:kind "${runner_image}"
done < <("${kind_bin}" get nodes --name "${cluster_name}")

"${kubectl_bin}" apply -f - <<YAML
apiVersion: execution.agentforge.dev/v1alpha1
kind: AgentRun
metadata:
  name: lifecycle-runner-success
  namespace: agentforge-kind-lifecycle
spec:
  tenantId: 019b0000-0000-7000-8000-000000000041
  projectId: 019b0000-0000-7000-8000-000000000042
  runId: 019b0000-0000-7000-8000-000000000043
  attemptId: 019b0000-0000-7000-8000-000000000044
  attempt: 1
  runnerImage: ${runner_image}
  runtime: python-3.12
  executionProfile: standard
  taskRef: secret://runner-task/envelope.json
  timeoutSeconds: 120
  retryPolicy:
    maxAttempts: 1
    initialBackoffSeconds: 5
    maxBackoffSeconds: 30
  resources:
    requests:
      cpuMillis: 100
      memoryMiB: 128
    limits:
      cpuMillis: 200
      memoryMiB: 256
  workspace:
    sizeGiB: 1
    storageClassName: standard
    retentionPolicy: Retain
  network:
    profile: Isolated
  artifactDestinationRef:
    name: artifact-store
  configurationRefs:
    - name: runner-trust
  secretRefs:
    - name: runner-task
  desiredState: Running
YAML

deadline=$((SECONDS + 300))
runner_phase=""
while (( SECONDS < deadline )); do
    runner_phase="$("${kubectl_bin}" get agentrun lifecycle-runner-success --namespace agentforge-kind-lifecycle -o jsonpath='{.status.phase}' 2>/dev/null || true)"
    if [[ "${runner_phase}" == "Succeeded" ]]; then
        break
    fi
    if ! kill -0 "${manager_pid}" 2>/dev/null; then
        break
    fi
    sleep 2
done
runner_manifest="$("${kubectl_bin}" get agentrun lifecycle-runner-success --namespace agentforge-kind-lifecycle -o jsonpath='{.status.artifactManifestRef}' 2>/dev/null || true)"
runner_pod="$("${kubectl_bin}" get pods --namespace agentforge-kind-lifecycle -l execution.agentforge.dev/run-id=019b0000-0000-7000-8000-000000000043 -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
runner_pvc="$("${kubectl_bin}" get persistentvolumeclaims --namespace agentforge-kind-lifecycle -l execution.agentforge.dev/run-id=019b0000-0000-7000-8000-000000000043 -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
if [[ "${runner_phase}" != "Succeeded" || "${runner_manifest}" != file://*result-manifest.json || -z "${runner_pod}" || -z "${runner_pvc}" ]]; then
    printf 'runner result invalid: phase=%s manifest=%s pod=%s pvc=%s\n' "${runner_phase}" "${runner_manifest}" "${runner_pod}" "${runner_pvc}" >&2
    sed -n '1,240p' "${manager_log}" >&2
    exit 1
fi
"${kubectl_bin}" delete pod "${runner_pod}" --namespace agentforge-kind-lifecycle --wait=true
"${kubectl_bin}" apply -f - <<YAML
apiVersion: v1
kind: Pod
metadata:
  name: artifact-reader
  namespace: agentforge-kind-lifecycle
spec:
  restartPolicy: Never
  nodeSelector:
    agentforge.dev/node-pool: agents
  tolerations:
    - key: agentforge.dev/node-pool
      operator: Equal
      value: agents
      effect: NoSchedule
  containers:
    - name: reader
      image: docker.io/library/busybox@sha256:9532d8c39891ca2ecde4d30d7710e01fb739c87a8b9299685c63704296b16028
      command: ["sh", "-c", "test -f /workspace/artifacts/tenants/019b0000-0000-7000-8000-000000000041/projects/019b0000-0000-7000-8000-000000000042/runs/019b0000-0000-7000-8000-000000000043/attempts/1/result-manifest.json"]
      volumeMounts:
        - name: workspace
          mountPath: /workspace
  volumes:
    - name: workspace
      persistentVolumeClaim:
        claimName: ${runner_pvc}
YAML
if ! "${kubectl_bin}" wait --for=jsonpath='{.status.phase}'=Succeeded pod/artifact-reader --namespace agentforge-kind-lifecycle --timeout=120s; then
    "${kubectl_bin}" describe pod artifact-reader --namespace agentforge-kind-lifecycle >&2 || true
    "${kubectl_bin}" logs artifact-reader --namespace agentforge-kind-lifecycle >&2 || true
    exit 1
fi

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

deadline=$((SECONDS + 300))
retry_phase=""
retry_attempt=""
while (( SECONDS < deadline )); do
    retry_phase="$("${kubectl_bin}" get agentrun lifecycle-transient-retry \
        --namespace agentforge-kind-lifecycle \
        -o jsonpath='{.status.phase}' 2>/dev/null || true)"
    retry_attempt="$("${kubectl_bin}" get agentrun lifecycle-transient-retry \
        --namespace agentforge-kind-lifecycle \
        -o jsonpath='{.status.attempt}' 2>/dev/null || true)"
    if [[ "${retry_phase}" == "Failed" && "${retry_attempt}" == "2" ]]; then
        break
    fi
    if ! kill -0 "${manager_pid}" 2>/dev/null; then
        break
    fi
    sleep 2
done
retry_category="$("${kubectl_bin}" get agentrun lifecycle-transient-retry --namespace agentforge-kind-lifecycle -o jsonpath='{.status.failureCategory}' 2>/dev/null || true)"
retry_job_count="$("${kubectl_bin}" get jobs --namespace agentforge-kind-lifecycle -l execution.agentforge.dev/run-id=019b0000-0000-7000-8000-000000000023 -o name | wc -l | tr -d ' ')"
retry_job_names="$("${kubectl_bin}" get jobs --namespace agentforge-kind-lifecycle -l execution.agentforge.dev/run-id=019b0000-0000-7000-8000-000000000023 -o name)"
if [[ "${retry_phase}" != "Failed" || "${retry_attempt}" != "2" ||
      "${retry_category}" != "TRANSIENT_DEPENDENCY" || "${retry_job_count}" != "2" ||
      "${retry_job_names}" != *"-a1-job"* || "${retry_job_names}" != *"-a2-job"* ]]; then
    printf 'unexpected retry result: phase=%s attempt=%s category=%s jobs=%s names=%s\n' \
        "${retry_phase}" "${retry_attempt}" "${retry_category}" "${retry_job_count}" "${retry_job_names}" >&2
    printf '%s\n' 'operator manager log:' >&2
    sed -n '1,240p' "${manager_log}" >&2
    "${kubectl_bin}" get agentrun,jobs,pods,persistentvolumeclaims --namespace agentforge-kind-lifecycle -o yaml >&2 || true
    exit 1
fi

deadline=$((SECONDS + 300))
retained_phase=""
while (( SECONDS < deadline )); do
    retained_phase="$("${kubectl_bin}" get agentrun lifecycle-retained-cleanup \
        --namespace agentforge-kind-lifecycle \
        -o jsonpath='{.status.phase}' 2>/dev/null || true)"
    if [[ "${retained_phase}" == "Failed" ]]; then
        break
    fi
    if ! kill -0 "${manager_pid}" 2>/dev/null; then
        break
    fi
    sleep 2
done
if [[ "${retained_phase}" != "Failed" ]]; then
    printf 'retained cleanup fixture did not complete execution: phase=%s\n' "${retained_phase}" >&2
    exit 1
fi
"${kubectl_bin}" delete agentrun lifecycle-retained-cleanup --namespace agentforge-kind-lifecycle --wait=false
deadline=$((SECONDS + 120))
retention_state=""
while (( SECONDS < deadline )); do
    retention_state="$("${kubectl_bin}" get persistentvolumeclaims \
        --namespace agentforge-kind-lifecycle \
        -l execution.agentforge.dev/run-id=019b0000-0000-7000-8000-000000000033 \
        -o jsonpath='{.items[0].metadata.annotations.execution\.agentforge\.dev/retention-state}' 2>/dev/null || true)"
    if ! "${kubectl_bin}" get agentrun lifecycle-retained-cleanup --namespace agentforge-kind-lifecycle >/dev/null 2>&1 &&
       [[ "${retention_state}" == "Released" ]]; then
        break
    fi
    sleep 2
done
retained_pvc_count="$("${kubectl_bin}" get persistentvolumeclaims --namespace agentforge-kind-lifecycle -l execution.agentforge.dev/run-id=019b0000-0000-7000-8000-000000000033 -o name | wc -l | tr -d ' ')"
if "${kubectl_bin}" get agentrun lifecycle-retained-cleanup --namespace agentforge-kind-lifecycle >/dev/null 2>&1 ||
   [[ "${retention_state}" != "Released" || "${retained_pvc_count}" != "1" ]]; then
    printf 'unexpected retained cleanup result: state=%s pvcs=%s\n' "${retention_state}" "${retained_pvc_count}" >&2
    printf '%s\n' 'operator manager log:' >&2
    sed -n '1,240p' "${manager_log}" >&2
    "${kubectl_bin}" get agentrun,jobs,pods,persistentvolumeclaims --namespace agentforge-kind-lifecycle -o yaml >&2 || true
    exit 1
fi

printf '%s\n' 'kind lifecycle integration gate passed'

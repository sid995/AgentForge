/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package resources

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"

	executionv1alpha1 "github.com/sid995/agentforge/operator/api/v1alpha1"
)

func TestIsolatedExecutionResourcesGolden(t *testing.T) {
	resources := buildTestResources(t, NewDefaultBuilder(), validResourceAgentRun())
	actual := marshalResources(t, resources)
	if os.Getenv("AGENTFORGE_PRINT_GOLDEN") == "1" {
		fmt.Print(actual)
		return
	}
	expected, err := os.ReadFile(filepath.Join("testdata", "isolated.golden.yaml"))
	if err != nil {
		t.Fatalf("read golden manifest: %v", err)
	}
	if actual != string(expected) {
		t.Fatalf("resource manifest differs from golden file\n--- expected\n%s\n--- actual\n%s", expected, actual)
	}
}

func TestExecutionJobSecurityPolicy(t *testing.T) {
	run := validResourceAgentRun()
	resources := buildTestResources(t, NewDefaultBuilder(), run)
	if err := ValidateSecurity(resources); err != nil {
		t.Fatalf("validate secure resources: %v", err)
	}
	pod := resources.Job.Spec.Template.Spec
	if pod.HostNetwork || pod.HostPID || pod.HostIPC {
		t.Fatal("host namespaces are enabled")
	}
	if pod.AutomountServiceAccountToken == nil || *pod.AutomountServiceAccountToken {
		t.Fatal("Pod API token automount is not disabled")
	}
	if resources.ServiceAccount.AutomountServiceAccountToken == nil || *resources.ServiceAccount.AutomountServiceAccountToken {
		t.Fatal("ServiceAccount API token automount is not disabled")
	}
	if pod.SecurityContext == nil || pod.SecurityContext.RunAsUser == nil || *pod.SecurityContext.RunAsUser == 0 {
		t.Fatal("Pod does not have a concrete non-root identity")
	}
	container := pod.Containers[0]
	if !container.Resources.Requests.Cpu().Equal(*container.Resources.Limits.Cpu()) && container.Resources.Limits.Cpu().Cmp(*container.Resources.Requests.Cpu()) < 0 {
		t.Fatal("CPU limit is below its request")
	}
	if got := container.Resources.Requests.Cpu().MilliValue(); got != int64(run.Spec.Resources.Requests.CPUMillis) {
		t.Fatalf("CPU request: want %dm, got %dm", run.Spec.Resources.Requests.CPUMillis, got)
	}
	if got := container.Resources.Limits.Memory().Value() / (1024 * 1024); got != int64(run.Spec.Resources.Limits.MemoryMiB) {
		t.Fatalf("memory limit: want %dMi, got %dMi", run.Spec.Resources.Limits.MemoryMiB, got)
	}
	workspaceMounts := 0
	secretMounts := 0
	for _, mount := range container.VolumeMounts {
		switch {
		case mount.Name == workspaceVolumeName && mount.MountPath == workspaceMountPath:
			workspaceMounts++
		case strings.HasPrefix(mount.Name, "external-secret-"):
			secretMounts++
			if !mount.ReadOnly || !strings.HasPrefix(mount.MountPath, secretMountRoot+"/") {
				t.Fatalf("secret mount is not read-only and bounded: %#v", mount)
			}
		}
	}
	if workspaceMounts != 1 || secretMounts != len(run.Spec.SecretRefs) {
		t.Fatalf("workspace/secret mounts: workspace=%d secrets=%d", workspaceMounts, secretMounts)
	}
	for _, environment := range container.Env {
		if environment.ValueFrom != nil {
			t.Fatalf("runner environment unexpectedly references a secret or field: %#v", environment)
		}
	}
	if resources.NetworkPolicy.Spec.Ingress == nil || len(resources.NetworkPolicy.Spec.Ingress) != 0 || len(resources.NetworkPolicy.Spec.Egress) != 0 {
		t.Fatalf("isolated NetworkPolicy is not deny-all: %#v", resources.NetworkPolicy.Spec)
	}
	assertControllerOwners(t, run, resources)
}

func TestTrustedProfileAndRestrictedEgress(t *testing.T) {
	profile := StandardProfile()
	profile.Name = "sandboxed"
	profile.RuntimeClassName = "gvisor"
	profile.PriorityClassName = "agentforge-sandboxed"
	policy := approvedEgressPolicy()
	builder, err := NewBuilder([]ExecutionProfile{profile}, []EgressPolicy{policy})
	if err != nil {
		t.Fatalf("create builder: %v", err)
	}
	run := validResourceAgentRun()
	run.Spec.ExecutionProfile = profile.Name
	run.Spec.Network = executionv1alpha1.NetworkSpec{
		Profile:                executionv1alpha1.NetworkProfileRestrictedEgress,
		AllowedDestinationsRef: &executionv1alpha1.LocalObjectReference{Name: policy.Name},
	}
	resources := buildTestResources(t, builder, run)
	pod := resources.Job.Spec.Template.Spec
	if pod.RuntimeClassName == nil || *pod.RuntimeClassName != "gvisor" {
		t.Fatalf("runtime class was not applied: %v", pod.RuntimeClassName)
	}
	if !reflect.DeepEqual(pod.NodeSelector, profile.NodeSelector) || !reflect.DeepEqual(pod.Tolerations, profile.Tolerations) {
		t.Fatal("trusted node selector or tolerations were not applied")
	}
	if len(pod.TopologySpreadConstraints) != len(profile.TopologyKeys) {
		t.Fatalf("topology constraints: want %d, got %d", len(profile.TopologyKeys), len(pod.TopologySpreadConstraints))
	}
	if !reflect.DeepEqual(resources.NetworkPolicy.Spec.Egress, policy.Rules) {
		t.Fatalf("approved egress was not applied: %#v", resources.NetworkPolicy.Spec.Egress)
	}
}

func TestRetainedWorkspaceHasNoGarbageCollectionOwner(t *testing.T) {
	run := validResourceAgentRun()
	run.Spec.Workspace.RetentionPolicy = executionv1alpha1.WorkspaceRetentionRetain
	resources := buildTestResources(t, NewDefaultBuilder(), run)
	if len(resources.Workspace.OwnerReferences) != 0 {
		t.Fatalf("retained PVC would be garbage collected: %v", resources.Workspace.OwnerReferences)
	}
	if resources.Workspace.Annotations["execution.agentforge.dev/retention-policy"] != "Retain" {
		t.Fatal("retained PVC is missing its explicit retention annotation")
	}
}

func TestSecurityMutationDetection(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ExecutionResources)
	}{
		{name: "privileged", mutate: func(resources *ExecutionResources) {
			resources.Job.Spec.Template.Spec.Containers[0].SecurityContext.Privileged = ptr.To(true)
		}},
		{name: "privilege escalation", mutate: func(resources *ExecutionResources) {
			resources.Job.Spec.Template.Spec.Containers[0].SecurityContext.AllowPrivilegeEscalation = ptr.To(true)
		}},
		{name: "writable root", mutate: func(resources *ExecutionResources) {
			resources.Job.Spec.Template.Spec.Containers[0].SecurityContext.ReadOnlyRootFilesystem = ptr.To(false)
		}},
		{name: "host network", mutate: func(resources *ExecutionResources) { resources.Job.Spec.Template.Spec.HostNetwork = true }},
		{name: "automounted token", mutate: func(resources *ExecutionResources) {
			resources.Job.Spec.Template.Spec.AutomountServiceAccountToken = ptr.To(true)
		}},
		{name: "host path", mutate: func(resources *ExecutionResources) {
			resources.Job.Spec.Template.Spec.Volumes = append(resources.Job.Spec.Template.Spec.Volumes, corev1.Volume{
				Name: "host", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/var/run/docker.sock"}},
			})
		}},
		{name: "Docker socket mount", mutate: func(resources *ExecutionResources) {
			resources.Job.Spec.Template.Spec.Containers[0].VolumeMounts = append(resources.Job.Spec.Template.Spec.Containers[0].VolumeMounts,
				corev1.VolumeMount{Name: "tmp", MountPath: "/var/run/docker.sock"})
		}},
		{name: "wildcard egress", mutate: func(resources *ExecutionResources) {
			resources.NetworkPolicy.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{{}}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resources := buildTestResources(t, NewDefaultBuilder(), validResourceAgentRun())
			test.mutate(resources)
			if err := ValidateSecurity(resources); err == nil {
				t.Fatal("security mutation was not rejected")
			}
		})
	}
}

func TestInvalidBuilderConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		profiles []ExecutionProfile
		policies []EgressPolicy
	}{
		{name: "no profiles"},
		{name: "missing dedicated node pool", profiles: []ExecutionProfile{func() ExecutionProfile {
			profile := StandardProfile()
			delete(profile.NodeSelector, requiredNodePoolLabel)
			return profile
		}()}},
		{name: "control-plane toleration", profiles: []ExecutionProfile{func() ExecutionProfile {
			profile := StandardProfile()
			profile.Tolerations = append(profile.Tolerations, corev1.Toleration{Key: "node-role.kubernetes.io/control-plane", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule})
			return profile
		}()}},
		{name: "invalid runtime class", profiles: []ExecutionProfile{func() ExecutionProfile {
			profile := StandardProfile()
			profile.RuntimeClassName = "Invalid"
			return profile
		}()}},
		{name: "wildcard destination", profiles: []ExecutionProfile{StandardProfile()}, policies: []EgressPolicy{{
			Name: "wildcard", Rules: []networkingv1.NetworkPolicyEgressRule{{To: []networkingv1.NetworkPolicyPeer{{}}, Ports: approvedEgressPolicy().Rules[0].Ports}},
		}}},
		{name: "empty namespace selector", profiles: []ExecutionProfile{StandardProfile()}, policies: []EgressPolicy{{
			Name: "all-namespaces", Rules: []networkingv1.NetworkPolicyEgressRule{{
				To: []networkingv1.NetworkPolicyPeer{{NamespaceSelector: &metav1.LabelSelector{}}}, Ports: approvedEgressPolicy().Rules[0].Ports,
			}},
		}}},
		{name: "internet-wide destination", profiles: []ExecutionProfile{StandardProfile()}, policies: []EgressPolicy{{
			Name: "internet", Rules: egressRulesForCIDR("0.0.0.0/0"),
		}}},
		{name: "overly broad destination", profiles: []ExecutionProfile{StandardProfile()}, policies: []EgressPolicy{{
			Name: "half-internet", Rules: egressRulesForCIDR("128.0.0.0/1"),
		}}},
		{name: "metadata destination", profiles: []ExecutionProfile{StandardProfile()}, policies: []EgressPolicy{{
			Name: "metadata", Rules: egressRulesForCIDR("169.254.169.254/32"),
		}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewBuilder(test.profiles, test.policies); err == nil {
				t.Fatal("invalid builder configuration was accepted")
			}
		})
	}
}

func TestBuilderCopiesTrustedConfiguration(t *testing.T) {
	profile := StandardProfile()
	policy := approvedEgressPolicy()
	builder, err := NewBuilder([]ExecutionProfile{profile}, []EgressPolicy{policy})
	if err != nil {
		t.Fatalf("create builder: %v", err)
	}
	profile.NodeSelector[requiredNodePoolLabel] = "control-plane"
	policy.Rules[0].To[0].IPBlock.CIDR = "0.0.0.0/0"
	run := validResourceAgentRun()
	run.Spec.Network = executionv1alpha1.NetworkSpec{
		Profile:                executionv1alpha1.NetworkProfileRestrictedEgress,
		AllowedDestinationsRef: &executionv1alpha1.LocalObjectReference{Name: "approved-egress"},
	}
	resources := buildTestResources(t, builder, run)
	if resources.Job.Spec.Template.Spec.NodeSelector[requiredNodePoolLabel] != "agents" ||
		resources.NetworkPolicy.Spec.Egress[0].To[0].IPBlock.CIDR != "203.0.113.0/24" {
		t.Fatal("builder retained mutable aliases to trusted configuration")
	}
}

func TestInvalidAgentRunBuildInput(t *testing.T) {
	builder := NewDefaultBuilder()
	tests := []struct {
		name   string
		mutate func(*executionv1alpha1.AgentRun)
	}{
		{name: "missing UID", mutate: func(run *executionv1alpha1.AgentRun) { run.UID = "" }},
		{name: "unknown profile", mutate: func(run *executionv1alpha1.AgentRun) { run.Spec.ExecutionProfile = "unknown" }},
		{name: "mutable image", mutate: func(run *executionv1alpha1.AgentRun) { run.Spec.RunnerImage = "ghcr.io/agentforge/runner:latest" }},
		{name: "invalid resources", mutate: func(run *executionv1alpha1.AgentRun) { run.Spec.Resources.Limits.CPUMillis = 1 }},
		{name: "isolated with destination", mutate: func(run *executionv1alpha1.AgentRun) {
			run.Spec.Network.AllowedDestinationsRef = &executionv1alpha1.LocalObjectReference{Name: "approved-egress"}
		}},
		{name: "unconfigured restricted destination", mutate: func(run *executionv1alpha1.AgentRun) {
			run.Spec.Network = executionv1alpha1.NetworkSpec{
				Profile:                executionv1alpha1.NetworkProfileRestrictedEgress,
				AllowedDestinationsRef: &executionv1alpha1.LocalObjectReference{Name: "missing"},
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			run := validResourceAgentRun()
			test.mutate(run)
			if _, err := builder.BuildAll(run); err == nil {
				t.Fatal("invalid AgentRun was accepted")
			}
		})
	}
}

func validResourceAgentRun() *executionv1alpha1.AgentRun {
	return &executionv1alpha1.AgentRun{
		ObjectMeta: metav1.ObjectMeta{
			Name: "run-example", Namespace: "tenant-example", UID: types.UID("11111111-2222-3333-4444-555555555555"),
		},
		Spec: executionv1alpha1.AgentRunSpec{
			TenantID:         "019b0000-0000-7000-8000-000000000002",
			ProjectID:        "019b0000-0000-7000-8000-000000000003",
			RunID:            "019b0000-0000-7000-8000-000000000004",
			AttemptID:        "019b0000-0000-7000-8000-000000000005",
			Attempt:          1,
			RunnerImage:      "ghcr.io/agentforge/runner@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Runtime:          "python-3.12",
			ExecutionProfile: "standard",
			TaskRef:          "vault://tasks/run-example",
			TimeoutSeconds:   1800,
			RetryPolicy: executionv1alpha1.RetryPolicy{
				MaxAttempts: 3, InitialBackoffSeconds: 5, MaxBackoffSeconds: 300,
			},
			Resources: executionv1alpha1.ResourceRequirements{
				Requests: executionv1alpha1.ResourceValues{CPUMillis: 1000, MemoryMiB: 2048},
				Limits:   executionv1alpha1.ResourceValues{CPUMillis: 2000, MemoryMiB: 4096},
			},
			Workspace: executionv1alpha1.WorkspaceSpec{
				SizeGiB: 20, StorageClassName: "standard", RetentionPolicy: executionv1alpha1.WorkspaceRetentionDelete,
			},
			Network:                executionv1alpha1.NetworkSpec{Profile: executionv1alpha1.NetworkProfileIsolated},
			ArtifactDestinationRef: executionv1alpha1.LocalObjectReference{Name: "artifact-store"},
			ConfigurationRefs: []executionv1alpha1.LocalObjectReference{
				{Name: "z-runner-config"}, {Name: "a-policy-config"},
			},
			SecretRefs: []executionv1alpha1.LocalObjectReference{
				{Name: "source-credential"}, {Name: "artifact-credential"},
			},
			DesiredState: executionv1alpha1.DesiredStateRunning,
		},
		Status: executionv1alpha1.AgentRunStatus{Attempt: 1},
	}
}

func approvedEgressPolicy() EgressPolicy {
	return EgressPolicy{Name: "approved-egress", Rules: egressRulesForCIDR("203.0.113.0/24")}
}

func egressRulesForCIDR(cidr string) []networkingv1.NetworkPolicyEgressRule {
	port := intstr.FromInt32(443)
	return []networkingv1.NetworkPolicyEgressRule{{
		To:    []networkingv1.NetworkPolicyPeer{{IPBlock: &networkingv1.IPBlock{CIDR: cidr}}},
		Ports: []networkingv1.NetworkPolicyPort{{Protocol: ptr.To(corev1.ProtocolTCP), Port: &port}},
	}}
}

func buildTestResources(t *testing.T, builder *Builder, run *executionv1alpha1.AgentRun) *ExecutionResources {
	t.Helper()
	resources, err := builder.BuildAll(run)
	if err != nil {
		t.Fatalf("build resources: %v", err)
	}
	return resources
}

func marshalResources(t *testing.T, resources *ExecutionResources) string {
	t.Helper()
	objects := []any{resources.ServiceAccount, resources.Configuration, resources.Workspace, resources.NetworkPolicy, resources.Job}
	var output strings.Builder
	for index, object := range objects {
		manifest, err := yaml.Marshal(object)
		if err != nil {
			t.Fatalf("marshal %T: %v", object, err)
		}
		if index > 0 {
			output.WriteString("---\n")
		}
		output.Write(manifest)
	}
	return output.String()
}

func assertControllerOwners(t *testing.T, run *executionv1alpha1.AgentRun, resources *ExecutionResources) {
	t.Helper()
	objects := []metav1.Object{resources.ServiceAccount, resources.Configuration, resources.Workspace, resources.NetworkPolicy, resources.Job}
	for _, object := range objects {
		owners := object.GetOwnerReferences()
		if len(owners) != 1 || owners[0].UID != run.UID || owners[0].Controller == nil || !*owners[0].Controller {
			t.Errorf("%T has invalid controller ownership: %#v", object, owners)
		}
	}
}

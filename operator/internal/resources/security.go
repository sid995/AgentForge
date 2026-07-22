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
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
)

// ValidateSecurity applies defense-in-depth checks to built workload resources.
func ValidateSecurity(resources *ExecutionResources) error {
	if resources == nil || resources.ServiceAccount == nil || resources.NetworkPolicy == nil || resources.Job == nil {
		return fmt.Errorf("service account, network policy, and Job are required for security validation")
	}
	if resources.ServiceAccount.AutomountServiceAccountToken == nil || *resources.ServiceAccount.AutomountServiceAccountToken {
		return fmt.Errorf("workload ServiceAccount token automount must be disabled")
	}
	if err := validateNetworkPolicySecurity(resources.NetworkPolicy); err != nil {
		return err
	}
	pod := &resources.Job.Spec.Template.Spec
	if pod.HostNetwork || pod.HostPID || pod.HostIPC {
		return fmt.Errorf("host namespace access is prohibited")
	}
	if pod.AutomountServiceAccountToken == nil || *pod.AutomountServiceAccountToken {
		return fmt.Errorf("pod service account token automount must be disabled")
	}
	if pod.SecurityContext == nil || pod.SecurityContext.RunAsNonRoot == nil || !*pod.SecurityContext.RunAsNonRoot ||
		pod.SecurityContext.SeccompProfile == nil || pod.SecurityContext.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		return fmt.Errorf("pod must run as non-root with RuntimeDefault seccomp")
	}
	if len(pod.Containers) != 1 || len(pod.InitContainers) != 0 || len(pod.EphemeralContainers) != 0 {
		return fmt.Errorf("execution Pod must contain exactly one validated runner container")
	}
	container := &pod.Containers[0]
	securityContext := container.SecurityContext
	if securityContext == nil || securityContext.RunAsNonRoot == nil || !*securityContext.RunAsNonRoot ||
		securityContext.Privileged == nil || *securityContext.Privileged ||
		securityContext.AllowPrivilegeEscalation == nil || *securityContext.AllowPrivilegeEscalation ||
		securityContext.ReadOnlyRootFilesystem == nil || !*securityContext.ReadOnlyRootFilesystem ||
		securityContext.SeccompProfile == nil || securityContext.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		return fmt.Errorf("runner security context violates the restricted policy")
	}
	if securityContext.Capabilities == nil || len(securityContext.Capabilities.Add) != 0 ||
		len(securityContext.Capabilities.Drop) != 1 || securityContext.Capabilities.Drop[0] != "ALL" {
		return fmt.Errorf("runner must drop all Linux capabilities")
	}
	if resources.Job.Spec.ActiveDeadlineSeconds == nil || *resources.Job.Spec.ActiveDeadlineSeconds < 1 ||
		resources.Job.Spec.BackoffLimit == nil || *resources.Job.Spec.BackoffLimit != 0 {
		return fmt.Errorf("job must have an active deadline and Kubernetes retries disabled")
	}
	for _, volume := range pod.Volumes {
		if volume.HostPath != nil {
			return fmt.Errorf("hostPath volumes are prohibited")
		}
		if volume.Projected != nil {
			for _, source := range volume.Projected.Sources {
				if source.ServiceAccountToken != nil {
					return fmt.Errorf("projected Kubernetes API tokens are prohibited")
				}
			}
		}
	}
	for _, mount := range container.VolumeMounts {
		if strings.Contains(strings.ToLower(mount.MountPath), "docker.sock") || mount.MountPath == "/var/run/docker" {
			return fmt.Errorf("docker socket mounts are prohibited")
		}
	}
	return nil
}

func validateNetworkPolicySecurity(policy *networkingv1.NetworkPolicy) error {
	if len(policy.Spec.PolicyTypes) != 2 || !containsPolicyType(policy.Spec.PolicyTypes, networkingv1.PolicyTypeIngress) ||
		!containsPolicyType(policy.Spec.PolicyTypes, networkingv1.PolicyTypeEgress) {
		return fmt.Errorf("NetworkPolicy must govern ingress and egress")
	}
	if policy.Spec.Ingress == nil || len(policy.Spec.Ingress) != 0 {
		return fmt.Errorf("workload ingress must be denied")
	}
	for _, rule := range policy.Spec.Egress {
		if len(rule.To) == 0 || len(rule.Ports) == 0 {
			return fmt.Errorf("wildcard egress is prohibited")
		}
		for _, peer := range rule.To {
			if err := validateNetworkPeer(peer); err != nil {
				return err
			}
		}
	}
	return nil
}

func containsPolicyType(values []networkingv1.PolicyType, want networkingv1.PolicyType) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

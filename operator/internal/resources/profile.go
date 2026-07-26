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
	"net/netip"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8svalidation "k8s.io/apimachinery/pkg/util/validation"

	executionv1alpha1 "github.com/sid995/agentforge/operator/api/v1alpha1"
)

const requiredNodePoolLabel = "agentforge.dev/node-pool"

var prohibitedEgressPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/0"),
	netip.MustParsePrefix("::/0"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("fe80::/10"),
}

// ExecutionProfile is trusted Operator configuration selected by spec.executionProfile.
type ExecutionProfile struct {
	Name                          string
	NodeSelector                  map[string]string
	Tolerations                   []corev1.Toleration
	TopologyKeys                  []string
	RuntimeClassName              string
	PriorityClassName             string
	TerminationGracePeriodSeconds int64
}

// EgressPolicy is trusted, name-addressed NetworkPolicy configuration.
type EgressPolicy struct {
	Name  string
	Rules []networkingv1.NetworkPolicyEgressRule
}

// StandardProfile returns the default dedicated agent-node scheduling policy.
func StandardProfile() ExecutionProfile {
	return ExecutionProfile{
		Name: "standard",
		NodeSelector: map[string]string{
			requiredNodePoolLabel: "agents",
		},
		Tolerations: []corev1.Toleration{{
			Key:      requiredNodePoolLabel,
			Operator: corev1.TolerationOpEqual,
			Value:    "agents",
			Effect:   corev1.TaintEffectNoSchedule,
		}},
		TopologyKeys: []string{
			"topology.kubernetes.io/zone",
			"kubernetes.io/hostname",
		},
		PriorityClassName:             "agentforge-agent",
		TerminationGracePeriodSeconds: 30,
	}
}

func validateProfile(profile ExecutionProfile) error {
	if profile.Name == "" || len(k8svalidation.IsDNS1123Subdomain(profile.Name)) != 0 {
		return fmt.Errorf("execution profile name must be a DNS subdomain")
	}
	if profile.NodeSelector[requiredNodePoolLabel] != "agents" {
		return fmt.Errorf("execution profile must select the dedicated agents node pool")
	}
	for key, value := range profile.NodeSelector {
		if len(k8svalidation.IsQualifiedName(key)) != 0 || len(k8svalidation.IsValidLabelValue(value)) != 0 {
			return fmt.Errorf("execution profile contains an invalid node selector")
		}
		if isControlPlaneKey(key) {
			return fmt.Errorf("execution profile cannot select control-plane nodes")
		}
	}
	for _, toleration := range profile.Tolerations {
		if toleration.Key == "" || toleration.Operator == corev1.TolerationOpExists || isControlPlaneKey(toleration.Key) {
			return fmt.Errorf("execution profile contains a broad or control-plane toleration")
		}
		if len(k8svalidation.IsQualifiedName(toleration.Key)) != 0 || len(k8svalidation.IsValidLabelValue(toleration.Value)) != 0 {
			return fmt.Errorf("execution profile contains an invalid toleration")
		}
		if toleration.Effect != corev1.TaintEffectNoSchedule && toleration.Effect != corev1.TaintEffectNoExecute {
			return fmt.Errorf("execution profile toleration effect is not approved")
		}
	}
	if profile.TerminationGracePeriodSeconds < 1 || profile.TerminationGracePeriodSeconds > 300 {
		return fmt.Errorf("termination grace period must be between 1 and 300 seconds")
	}
	if len(profile.TopologyKeys) > 8 {
		return fmt.Errorf("execution profile has too many topology constraints")
	}
	seenTopologyKeys := make(map[string]struct{}, len(profile.TopologyKeys))
	for _, key := range profile.TopologyKeys {
		if len(k8svalidation.IsQualifiedName(key)) != 0 {
			return fmt.Errorf("execution profile contains an invalid topology key")
		}
		if _, exists := seenTopologyKeys[key]; exists {
			return fmt.Errorf("execution profile contains a duplicate topology key")
		}
		seenTopologyKeys[key] = struct{}{}
	}
	for field, value := range map[string]string{
		"runtime class":  profile.RuntimeClassName,
		"priority class": profile.PriorityClassName,
	} {
		if value != "" && len(k8svalidation.IsDNS1123Subdomain(value)) != 0 {
			return fmt.Errorf("%s must be a DNS subdomain", field)
		}
	}
	return nil
}

func validateEgressPolicy(policy EgressPolicy) error {
	if policy.Name == "" || len(k8svalidation.IsDNS1123Subdomain(policy.Name)) != 0 {
		return fmt.Errorf("egress policy name must be a DNS subdomain")
	}
	if len(policy.Rules) == 0 || len(policy.Rules) > 32 {
		return fmt.Errorf("restricted egress policy must contain 1 through 32 rules")
	}
	for _, rule := range policy.Rules {
		if len(rule.To) == 0 || len(rule.Ports) == 0 {
			return fmt.Errorf("egress rules require bounded destinations and ports")
		}
		for _, peer := range rule.To {
			if err := validateNetworkPeer(peer); err != nil {
				return err
			}
		}
		for _, port := range rule.Ports {
			if port.Port == nil || (port.Protocol != nil && *port.Protocol != corev1.ProtocolTCP && *port.Protocol != corev1.ProtocolUDP) {
				return fmt.Errorf("egress rules require explicit TCP or UDP ports")
			}
			if port.EndPort != nil && port.Port.Type != intstr.Int {
				return fmt.Errorf("egress endPort requires a numeric port")
			}
		}
	}
	return nil
}

func validateNetworkPeer(peer networkingv1.NetworkPolicyPeer) error {
	if peer.IPBlock == nil && peer.NamespaceSelector == nil && peer.PodSelector == nil {
		return fmt.Errorf("wildcard egress peers are prohibited")
	}
	if peer.IPBlock != nil && (peer.NamespaceSelector != nil || peer.PodSelector != nil) {
		return fmt.Errorf("IPBlock cannot be combined with selector peers")
	}
	if peer.IPBlock == nil {
		for _, selector := range []*metav1.LabelSelector{peer.NamespaceSelector, peer.PodSelector} {
			if selector == nil {
				continue
			}
			if len(selector.MatchLabels) == 0 && len(selector.MatchExpressions) == 0 {
				return fmt.Errorf("empty egress selectors are prohibited")
			}
			if _, err := metav1.LabelSelectorAsSelector(selector); err != nil {
				return fmt.Errorf("egress selector is invalid")
			}
		}
		return nil
	}
	prefix, err := netip.ParsePrefix(peer.IPBlock.CIDR)
	if err != nil || prefix.String() != peer.IPBlock.CIDR {
		return fmt.Errorf("egress IPBlock must use canonical CIDR notation")
	}
	if (prefix.Addr().Is4() && prefix.Bits() < 8) || (prefix.Addr().Is6() && prefix.Bits() < 32) {
		return fmt.Errorf("egress IPBlock is broader than the approved minimum prefix")
	}
	for _, prohibited := range prohibitedEgressPrefixes {
		if prohibited.Bits() == 0 && prefix.Bits() == 0 && prohibited.Addr().BitLen() == prefix.Addr().BitLen() {
			return fmt.Errorf("internet-wide egress is prohibited")
		}
		if prohibited.Bits() != 0 && prefix.Overlaps(prohibited) {
			return fmt.Errorf("link-local and metadata-service egress is prohibited")
		}
	}
	for _, exception := range peer.IPBlock.Except {
		exceptPrefix, exceptErr := netip.ParsePrefix(exception)
		if exceptErr != nil || !prefix.Contains(exceptPrefix.Addr()) || exceptPrefix.Bits() < prefix.Bits() {
			return fmt.Errorf("egress IPBlock exception must be contained by its CIDR")
		}
	}
	return nil
}

func profileForRun(profiles map[string]ExecutionProfile, run *executionv1alpha1.AgentRun) (ExecutionProfile, error) {
	profile, exists := profiles[run.Spec.ExecutionProfile]
	if !exists {
		return ExecutionProfile{}, fmt.Errorf("execution profile %q is not configured", run.Spec.ExecutionProfile)
	}
	return profile, nil
}

func egressForRun(policies map[string]EgressPolicy, run *executionv1alpha1.AgentRun) (*EgressPolicy, error) {
	if run.Spec.Network.Profile == executionv1alpha1.NetworkProfileIsolated {
		if run.Spec.Network.AllowedDestinationsRef != nil {
			return nil, fmt.Errorf("isolated network cannot reference allowed destinations")
		}
		return nil, nil
	}
	if run.Spec.Network.Profile != executionv1alpha1.NetworkProfileRestrictedEgress || run.Spec.Network.AllowedDestinationsRef == nil {
		return nil, fmt.Errorf("network profile is not supported")
	}
	policy, exists := policies[run.Spec.Network.AllowedDestinationsRef.Name]
	if !exists {
		return nil, fmt.Errorf("allowed destination policy %q is not configured", run.Spec.Network.AllowedDestinationsRef.Name)
	}
	return &policy, nil
}

func copyProfile(profile ExecutionProfile) ExecutionProfile {
	profile.NodeSelector = mapsClone(profile.NodeSelector)
	profile.Tolerations = slices.Clone(profile.Tolerations)
	profile.TopologyKeys = slices.Clone(profile.TopologyKeys)
	return profile
}

func copyEgressPolicy(policy EgressPolicy) EgressPolicy {
	sourceRules := policy.Rules
	policy.Rules = make([]networkingv1.NetworkPolicyEgressRule, len(sourceRules))
	for index := range sourceRules {
		policy.Rules[index] = *sourceRules[index].DeepCopy()
	}
	return policy
}

func mapsClone(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func isControlPlaneKey(key string) bool {
	return strings.Contains(key, "node-role.kubernetes.io/control-plane") || strings.Contains(key, "node-role.kubernetes.io/master")
}

func selectorForRun(run *executionv1alpha1.AgentRun) *metav1.LabelSelector {
	return &metav1.LabelSelector{MatchLabels: workloadSelectorLabels(run)}
}

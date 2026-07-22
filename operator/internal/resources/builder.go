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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8svalidation "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/utils/ptr"

	executionv1alpha1 "github.com/sid995/agentforge/operator/api/v1alpha1"
	"github.com/sid995/agentforge/operator/internal/naming"
)

const (
	runnerContainerName = "runner"
	workspaceVolumeName = "workspace"
	workspaceMountPath  = "/workspace"
	runtimeConfigPath   = "/etc/agentforge/runtime"
	secretMountRoot     = "/var/run/agentforge/secrets"
	configMountRoot     = "/etc/agentforge/config"
	nonRootUID          = int64(65532)
)

// Builder resolves trusted profiles and creates pure Kubernetes objects without API writes.
type Builder struct {
	profiles       map[string]ExecutionProfile
	egressPolicies map[string]EgressPolicy
}

// ExecutionResources contains every object needed for one execution attempt.
type ExecutionResources struct {
	ServiceAccount *corev1.ServiceAccount
	Configuration  *corev1.ConfigMap
	Workspace      *corev1.PersistentVolumeClaim
	NetworkPolicy  *networkingv1.NetworkPolicy
	Job            *batchv1.Job
}

type buildContext struct {
	run     *executionv1alpha1.AgentRun
	names   naming.ResourceNames
	profile ExecutionProfile
	egress  *EgressPolicy
	labels  map[string]string
	owner   metav1.OwnerReference
}

// NewBuilder validates and copies trusted cluster-side configuration.
func NewBuilder(profiles []ExecutionProfile, egressPolicies []EgressPolicy) (*Builder, error) {
	if len(profiles) == 0 {
		return nil, fmt.Errorf("at least one execution profile is required")
	}
	builder := &Builder{
		profiles:       make(map[string]ExecutionProfile, len(profiles)),
		egressPolicies: make(map[string]EgressPolicy, len(egressPolicies)),
	}
	for _, profile := range profiles {
		if err := validateProfile(profile); err != nil {
			return nil, fmt.Errorf("profile %q: %w", profile.Name, err)
		}
		if _, exists := builder.profiles[profile.Name]; exists {
			return nil, fmt.Errorf("duplicate execution profile %q", profile.Name)
		}
		builder.profiles[profile.Name] = copyProfile(profile)
	}
	for _, policy := range egressPolicies {
		if err := validateEgressPolicy(policy); err != nil {
			return nil, fmt.Errorf("egress policy %q: %w", policy.Name, err)
		}
		if _, exists := builder.egressPolicies[policy.Name]; exists {
			return nil, fmt.Errorf("duplicate egress policy %q", policy.Name)
		}
		builder.egressPolicies[policy.Name] = copyEgressPolicy(policy)
	}
	return builder, nil
}

// NewDefaultBuilder builds isolated workloads using the standard dedicated-node profile.
func NewDefaultBuilder() *Builder {
	builder, err := NewBuilder([]ExecutionProfile{StandardProfile()}, nil)
	if err != nil {
		panic(fmt.Sprintf("invalid built-in execution profile: %v", err))
	}
	return builder
}

// BuildAll produces and validates all execution resources for an AgentRun.
func (b *Builder) BuildAll(run *executionv1alpha1.AgentRun) (*ExecutionResources, error) {
	context, err := b.resolve(run)
	if err != nil {
		return nil, err
	}
	result := &ExecutionResources{
		ServiceAccount: b.buildServiceAccount(context),
		Configuration:  b.buildConfigMap(context),
		Workspace:      b.buildPVC(context),
		NetworkPolicy:  b.buildNetworkPolicy(context),
		Job:            b.buildJob(context),
	}
	if err := ValidateSecurity(result); err != nil {
		return nil, fmt.Errorf("validate built resources: %w", err)
	}
	return result, nil
}

// BuildServiceAccount produces the tokenless workload identity object.
func (b *Builder) BuildServiceAccount(run *executionv1alpha1.AgentRun) (*corev1.ServiceAccount, error) {
	context, err := b.resolve(run)
	if err != nil {
		return nil, err
	}
	return b.buildServiceAccount(context), nil
}

// BuildConfigMap produces the owned non-sensitive runner configuration.
func (b *Builder) BuildConfigMap(run *executionv1alpha1.AgentRun) (*corev1.ConfigMap, error) {
	context, err := b.resolve(run)
	if err != nil {
		return nil, err
	}
	return b.buildConfigMap(context), nil
}

// BuildPVC produces deterministic workspace storage.
func (b *Builder) BuildPVC(run *executionv1alpha1.AgentRun) (*corev1.PersistentVolumeClaim, error) {
	context, err := b.resolve(run)
	if err != nil {
		return nil, err
	}
	return b.buildPVC(context), nil
}

// BuildNetworkPolicy produces deny-by-default ingress and approved egress.
func (b *Builder) BuildNetworkPolicy(run *executionv1alpha1.AgentRun) (*networkingv1.NetworkPolicy, error) {
	context, err := b.resolve(run)
	if err != nil {
		return nil, err
	}
	return b.buildNetworkPolicy(context), nil
}

// BuildJob produces the secure, bounded execution Job.
func (b *Builder) BuildJob(run *executionv1alpha1.AgentRun) (*batchv1.Job, error) {
	context, err := b.resolve(run)
	if err != nil {
		return nil, err
	}
	job := b.buildJob(context)
	resources := &ExecutionResources{ServiceAccount: b.buildServiceAccount(context), NetworkPolicy: b.buildNetworkPolicy(context), Job: job}
	if err := ValidateSecurity(resources); err != nil {
		return nil, err
	}
	return job, nil
}

func (b *Builder) resolve(run *executionv1alpha1.AgentRun) (*buildContext, error) {
	if b == nil {
		return nil, fmt.Errorf("resource builder is required")
	}
	if run == nil || run.Namespace == "" || run.Name == "" || run.UID == "" {
		return nil, fmt.Errorf("AgentRun must be persisted with namespace, name, and UID")
	}
	if len(k8svalidation.IsDNS1123Subdomain(run.Namespace)) != 0 || len(k8svalidation.IsDNS1123Subdomain(run.Name)) != 0 {
		return nil, fmt.Errorf("AgentRun metadata is not a valid Kubernetes identity")
	}
	if err := validateRunForBuild(run); err != nil {
		return nil, err
	}
	names, err := naming.ForAgentRun(run)
	if err != nil {
		return nil, err
	}
	profile, err := profileForRun(b.profiles, run)
	if err != nil {
		return nil, err
	}
	egress, err := egressForRun(b.egressPolicies, run)
	if err != nil {
		return nil, err
	}
	return &buildContext{
		run: run, names: names, profile: profile, egress: egress,
		labels: labelsForRun(run), owner: ownerReference(run),
	}, nil
}

func validateRunForBuild(run *executionv1alpha1.AgentRun) error {
	digestSeparator := strings.LastIndex(run.Spec.RunnerImage, "@sha256:")
	if digestSeparator < 1 || digestSeparator+8+64 != len(run.Spec.RunnerImage) {
		return fmt.Errorf("runner image must use an immutable sha256 digest")
	}
	for _, character := range run.Spec.RunnerImage[digestSeparator+8:] {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return fmt.Errorf("runner image digest must be lowercase hexadecimal")
		}
	}
	if run.Spec.TimeoutSeconds < 1 || run.Spec.TimeoutSeconds > 86400 || run.Spec.Workspace.SizeGiB < 1 {
		return fmt.Errorf("execution timeout and workspace size must be positive and bounded")
	}
	if run.Spec.Resources.Requests.CPUMillis < 1 || run.Spec.Resources.Requests.MemoryMiB < 1 ||
		run.Spec.Resources.Limits.CPUMillis < run.Spec.Resources.Requests.CPUMillis ||
		run.Spec.Resources.Limits.MemoryMiB < run.Spec.Resources.Requests.MemoryMiB {
		return fmt.Errorf("resource requests and limits are invalid")
	}
	for _, reference := range append(append(slices.Clone(run.Spec.ConfigurationRefs), run.Spec.SecretRefs...), run.Spec.ArtifactDestinationRef) {
		if len(k8svalidation.IsDNS1123Subdomain(reference.Name)) != 0 {
			return fmt.Errorf("resource reference %q is not a DNS subdomain", reference.Name)
		}
	}
	return nil
}

func (b *Builder) buildServiceAccount(context *buildContext) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ServiceAccount"},
		ObjectMeta: metav1.ObjectMeta{
			Name: context.names.ServiceAccount, Namespace: context.run.Namespace,
			Labels: mapsClone(context.labels), Annotations: annotationsForRun(context.run),
			OwnerReferences: []metav1.OwnerReference{context.owner},
		},
		AutomountServiceAccountToken: ptr.To(false),
	}
}

func (b *Builder) buildConfigMap(context *buildContext) *corev1.ConfigMap {
	configurationRefs := referenceNames(context.run.Spec.ConfigurationRefs)
	secretRefs := referenceNames(context.run.Spec.SecretRefs)
	configJSON, err := json.Marshal(map[string]any{
		"artifactDestinationRef": context.run.Spec.ArtifactDestinationRef.Name,
		"attempt":                naming.CurrentAttempt(context.run),
		"attemptId":              context.run.Spec.AttemptID,
		"configurationRefs":      configurationRefs,
		"deployOnSuccess":        context.run.Spec.DeployOnSuccess,
		"projectId":              context.run.Spec.ProjectID,
		"runId":                  context.run.Spec.RunID,
		"runtime":                context.run.Spec.Runtime,
		"secretRefs":             secretRefs,
		"taskRef":                context.run.Spec.TaskRef,
		"tenantId":               context.run.Spec.TenantID,
	})
	if err != nil {
		panic(fmt.Sprintf("marshal bounded runner configuration: %v", err))
	}
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{
			Name: context.names.Configuration, Namespace: context.run.Namespace,
			Labels: mapsClone(context.labels), Annotations: annotationsForRun(context.run),
			OwnerReferences: []metav1.OwnerReference{context.owner},
		},
		Immutable: ptr.To(true),
		Data:      map[string]string{"config.json": string(configJSON)},
	}
}

func (b *Builder) buildPVC(context *buildContext) *corev1.PersistentVolumeClaim {
	metadata := metav1.ObjectMeta{
		Name: context.names.Workspace, Namespace: context.run.Namespace,
		Labels: mapsClone(context.labels), Annotations: annotationsForRun(context.run),
	}
	if context.run.Spec.Workspace.RetentionPolicy == executionv1alpha1.WorkspaceRetentionDelete {
		metadata.OwnerReferences = []metav1.OwnerReference{context.owner}
	} else {
		metadata.Annotations["execution.agentforge.dev/retention-policy"] = "Retain"
	}
	claim := &corev1.PersistentVolumeClaim{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "PersistentVolumeClaim"},
		ObjectMeta: metadata,
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{
				corev1.ResourceStorage: *resource.NewQuantity(int64(context.run.Spec.Workspace.SizeGiB)*1024*1024*1024, resource.BinarySI),
			}},
		},
	}
	if context.run.Spec.Workspace.StorageClassName != "" {
		claim.Spec.StorageClassName = ptr.To(context.run.Spec.Workspace.StorageClassName)
	}
	return claim
}

func (b *Builder) buildNetworkPolicy(context *buildContext) *networkingv1.NetworkPolicy {
	egress := []networkingv1.NetworkPolicyEgressRule(nil)
	if context.egress != nil {
		egress = copyEgressPolicy(*context.egress).Rules
	}
	return &networkingv1.NetworkPolicy{
		TypeMeta: metav1.TypeMeta{APIVersion: networkingv1.SchemeGroupVersion.String(), Kind: "NetworkPolicy"},
		ObjectMeta: metav1.ObjectMeta{
			Name: context.names.NetworkPolicy, Namespace: context.run.Namespace,
			Labels: mapsClone(context.labels), Annotations: annotationsForRun(context.run),
			OwnerReferences: []metav1.OwnerReference{context.owner},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: workloadSelectorLabels(context.run)},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress},
			Ingress:     []networkingv1.NetworkPolicyIngressRule{},
			Egress:      egress,
		},
	}
}

func (b *Builder) buildJob(context *buildContext) *batchv1.Job {
	podLabels := mapsClone(context.labels)
	volumes, mounts := volumesAndMounts(context)
	topologyConstraints := make([]corev1.TopologySpreadConstraint, 0, len(context.profile.TopologyKeys))
	for _, topologyKey := range context.profile.TopologyKeys {
		topologyConstraints = append(topologyConstraints, corev1.TopologySpreadConstraint{
			MaxSkew: 1, TopologyKey: topologyKey, WhenUnsatisfiable: corev1.ScheduleAnyway,
			LabelSelector: selectorForRun(context.run),
		})
	}
	podSpec := corev1.PodSpec{
		ServiceAccountName:            context.names.ServiceAccount,
		AutomountServiceAccountToken:  ptr.To(false),
		RestartPolicy:                 corev1.RestartPolicyNever,
		TerminationGracePeriodSeconds: ptr.To(context.profile.TerminationGracePeriodSeconds),
		EnableServiceLinks:            ptr.To(false),
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		NodeSelector:                  mapsClone(context.profile.NodeSelector),
		Tolerations:                   slices.Clone(context.profile.Tolerations),
		TopologySpreadConstraints:     topologyConstraints,
		PriorityClassName:             context.profile.PriorityClassName,
		Affinity: &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{
				MatchExpressions: []corev1.NodeSelectorRequirement{
					{Key: "node-role.kubernetes.io/control-plane", Operator: corev1.NodeSelectorOpDoesNotExist},
					{Key: "node-role.kubernetes.io/master", Operator: corev1.NodeSelectorOpDoesNotExist},
				},
			}}},
		}},
		SecurityContext: &corev1.PodSecurityContext{
			RunAsNonRoot: ptr.To(true), RunAsUser: ptr.To(nonRootUID), RunAsGroup: ptr.To(nonRootUID), FSGroup: ptr.To(nonRootUID),
			FSGroupChangePolicy: ptr.To(corev1.FSGroupChangeOnRootMismatch),
			SeccompProfile:      &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
		},
		Containers: []corev1.Container{{
			Name: runnerContainerName, Image: context.run.Spec.RunnerImage, ImagePullPolicy: corev1.PullIfNotPresent,
			Env: []corev1.EnvVar{
				{Name: "AGENTFORGE_RUN_CONFIG", Value: runtimeConfigPath + "/config.json"},
				{Name: "AGENTFORGE_WORKSPACE", Value: workspaceMountPath},
			},
			Resources: corev1.ResourceRequirements{
				Requests: resourceList(context.run.Spec.Resources.Requests),
				Limits:   resourceList(context.run.Spec.Resources.Limits),
			},
			SecurityContext: &corev1.SecurityContext{
				RunAsNonRoot: ptr.To(true), RunAsUser: ptr.To(nonRootUID), RunAsGroup: ptr.To(nonRootUID),
				Privileged: ptr.To(false), AllowPrivilegeEscalation: ptr.To(false), ReadOnlyRootFilesystem: ptr.To(true),
				Capabilities:   &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
				SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
			},
			VolumeMounts:             mounts,
			TerminationMessagePolicy: corev1.TerminationMessageReadFile,
		}},
		Volumes: volumes,
	}
	if context.profile.RuntimeClassName != "" {
		podSpec.RuntimeClassName = ptr.To(context.profile.RuntimeClassName)
	}
	return &batchv1.Job{
		TypeMeta: metav1.TypeMeta{APIVersion: batchv1.SchemeGroupVersion.String(), Kind: "Job"},
		ObjectMeta: metav1.ObjectMeta{
			Name: context.names.Job, Namespace: context.run.Namespace,
			Labels: mapsClone(context.labels), Annotations: annotationsForRun(context.run),
			OwnerReferences: []metav1.OwnerReference{context.owner},
		},
		Spec: batchv1.JobSpec{
			Parallelism: ptr.To[int32](1), Completions: ptr.To[int32](1), BackoffLimit: ptr.To[int32](0),
			ActiveDeadlineSeconds: ptr.To[int64](int64(context.run.Spec.TimeoutSeconds)),
			Template:              corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: podLabels, Annotations: annotationsForRun(context.run)}, Spec: podSpec},
		},
	}
}

func labelsForRun(run *executionv1alpha1.AgentRun) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":              "agentforge-runner",
		"app.kubernetes.io/managed-by":        "agentforge-operator",
		"execution.agentforge.dev/tenant-id":  string(run.Spec.TenantID),
		"execution.agentforge.dev/project-id": string(run.Spec.ProjectID),
		"execution.agentforge.dev/run-id":     string(run.Spec.RunID),
		"execution.agentforge.dev/attempt":    strconv.FormatInt(int64(naming.CurrentAttempt(run)), 10),
		"execution.agentforge.dev/runtime":    safeLabelValue(run.Spec.Runtime),
		"execution.agentforge.dev/workload":   "agent-run",
	}
}

func workloadSelectorLabels(run *executionv1alpha1.AgentRun) map[string]string {
	return map[string]string{
		"execution.agentforge.dev/run-id":  string(run.Spec.RunID),
		"execution.agentforge.dev/attempt": strconv.FormatInt(int64(naming.CurrentAttempt(run)), 10),
	}
}

func annotationsForRun(run *executionv1alpha1.AgentRun) map[string]string {
	return map[string]string{
		"execution.agentforge.dev/attempt-id":        string(run.Spec.AttemptID),
		"execution.agentforge.dev/execution-profile": run.Spec.ExecutionProfile,
		"execution.agentforge.dev/network-profile":   string(run.Spec.Network.Profile),
	}
}

func ownerReference(run *executionv1alpha1.AgentRun) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion: executionv1alpha1.GroupVersion.String(), Kind: "AgentRun", Name: run.Name, UID: types.UID(run.UID),
		Controller: ptr.To(true), BlockOwnerDeletion: ptr.To(true),
	}
}

func resourceList(values executionv1alpha1.ResourceValues) corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourceCPU:    *resource.NewMilliQuantity(int64(values.CPUMillis), resource.DecimalSI),
		corev1.ResourceMemory: *resource.NewQuantity(int64(values.MemoryMiB)*1024*1024, resource.BinarySI),
	}
}

func volumesAndMounts(context *buildContext) ([]corev1.Volume, []corev1.VolumeMount) {
	readOnlyMode := int32(0444)
	secretMode := int32(0440)
	volumes := []corev1.Volume{
		{Name: workspaceVolumeName, VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: context.names.Workspace}}},
		{Name: "runtime-config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: context.names.Configuration}, DefaultMode: &readOnlyMode}}},
		{Name: "artifact-destination", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: context.run.Spec.ArtifactDestinationRef.Name}, DefaultMode: &readOnlyMode}}},
		{Name: "tmp", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: resource.NewQuantity(64*1024*1024, resource.BinarySI)}}},
		{Name: "home", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: resource.NewQuantity(256*1024*1024, resource.BinarySI)}}},
	}
	mounts := []corev1.VolumeMount{
		{Name: workspaceVolumeName, MountPath: workspaceMountPath},
		{Name: "runtime-config", MountPath: runtimeConfigPath, ReadOnly: true},
		{Name: "artifact-destination", MountPath: "/etc/agentforge/artifacts", ReadOnly: true},
		{Name: "tmp", MountPath: "/tmp"},
		{Name: "home", MountPath: "/home/agent"},
	}
	for index, reference := range sortedReferences(context.run.Spec.ConfigurationRefs) {
		volumeName := fmt.Sprintf("external-config-%02d", index)
		volumes = append(volumes, corev1.Volume{Name: volumeName, VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: reference.Name}, DefaultMode: &readOnlyMode,
		}}})
		mounts = append(mounts, corev1.VolumeMount{Name: volumeName, MountPath: configMountRoot + "/" + reference.Name, ReadOnly: true})
	}
	for index, reference := range sortedReferences(context.run.Spec.SecretRefs) {
		volumeName := fmt.Sprintf("external-secret-%02d", index)
		volumes = append(volumes, corev1.Volume{Name: volumeName, VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{
			SecretName: reference.Name, DefaultMode: &secretMode,
		}}})
		mounts = append(mounts, corev1.VolumeMount{Name: volumeName, MountPath: secretMountRoot + "/" + reference.Name, ReadOnly: true})
	}
	return volumes, mounts
}

func sortedReferences(references []executionv1alpha1.LocalObjectReference) []executionv1alpha1.LocalObjectReference {
	result := slices.Clone(references)
	slices.SortFunc(result, func(left, right executionv1alpha1.LocalObjectReference) int {
		return stringCompare(left.Name, right.Name)
	})
	return result
}

func referenceNames(references []executionv1alpha1.LocalObjectReference) []string {
	sorted := sortedReferences(references)
	result := make([]string, len(sorted))
	for index := range sorted {
		result[index] = sorted[index].Name
	}
	return result
}

func stringCompare(left, right string) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func safeLabelValue(value string) string {
	if len(k8svalidation.IsValidLabelValue(value)) == 0 {
		return value
	}
	digest := sha256.Sum256([]byte(value))
	return "sha256-" + hex.EncodeToString(digest[:8])
}

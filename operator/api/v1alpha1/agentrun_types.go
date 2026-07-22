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

package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// UUIDv7 is a lowercase RFC 9562 UUID version 7 identifier.
// +kubebuilder:validation:Pattern=`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`
type UUIDv7 string

// LocalObjectReference identifies an object in the AgentRun namespace.
type LocalObjectReference struct {
	// name is a DNS subdomain name of the referenced object.
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	Name string `json:"name"`
}

// DesiredState declares whether execution should continue or be cancelled.
// +kubebuilder:validation:Enum=Running;Cancelled
type DesiredState string

const (
	// DesiredStateRunning requests normal execution reconciliation.
	DesiredStateRunning DesiredState = "Running"
	// DesiredStateCancelled requests cancellation reconciliation.
	DesiredStateCancelled DesiredState = "Cancelled"
)

// NetworkProfile identifies an approved egress posture.
// +kubebuilder:validation:Enum=Isolated;RestrictedEgress
type NetworkProfile string

const (
	// NetworkProfileIsolated denies all workload ingress and egress.
	NetworkProfileIsolated NetworkProfile = "Isolated"
	// NetworkProfileRestrictedEgress permits only destinations in an approved reference.
	NetworkProfileRestrictedEgress NetworkProfile = "RestrictedEgress"
)

// WorkspaceRetentionPolicy controls PVC retention after execution cleanup.
// +kubebuilder:validation:Enum=Delete;Retain
type WorkspaceRetentionPolicy string

const (
	// WorkspaceRetentionDelete removes the owned workspace with the AgentRun.
	WorkspaceRetentionDelete WorkspaceRetentionPolicy = "Delete"
	// WorkspaceRetentionRetain preserves the workspace under explicit cleanup policy.
	WorkspaceRetentionRetain WorkspaceRetentionPolicy = "Retain"
)

// FailureCategory is the stable, non-sensitive platform failure taxonomy.
// +kubebuilder:validation:Enum=VALIDATION;AUTHENTICATION;AUTHORIZATION;QUOTA;CONFLICT;TRANSIENT_DEPENDENCY;PERMANENT_DEPENDENCY;EXECUTION;POLICY;INTERNAL
type FailureCategory string

// AgentRunPhase is the observed Kubernetes execution phase.
// +kubebuilder:validation:Enum=Pending;Provisioning;Running;Succeeded;Failed;Cancelling;Cancelled
type AgentRunPhase string

const (
	// AgentRunPhasePending has not created an execution Job.
	AgentRunPhasePending AgentRunPhase = "Pending"
	// AgentRunPhaseProvisioning is ensuring prerequisites or waiting for a Pod.
	AgentRunPhaseProvisioning AgentRunPhase = "Provisioning"
	// AgentRunPhaseRunning has an active execution container.
	AgentRunPhaseRunning AgentRunPhase = "Running"
	// AgentRunPhaseSucceeded has complete mandatory result evidence.
	AgentRunPhaseSucceeded AgentRunPhase = "Succeeded"
	// AgentRunPhaseFailed is terminal for the current desired attempt.
	AgentRunPhaseFailed AgentRunPhase = "Failed"
	// AgentRunPhaseCancelling is terminating active work.
	AgentRunPhaseCancelling AgentRunPhase = "Cancelling"
	// AgentRunPhaseCancelled completed cancellation.
	AgentRunPhaseCancelled AgentRunPhase = "Cancelled"
)

// RetryPolicy defines bounded Operator retry behavior.
// +kubebuilder:validation:XValidation:rule="self.maxBackoffSeconds >= self.initialBackoffSeconds",message="maxBackoffSeconds must be greater than or equal to initialBackoffSeconds"
type RetryPolicy struct {
	// maxAttempts is the total attempt ceiling, including the current attempt.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	MaxAttempts int32 `json:"maxAttempts"`

	// initialBackoffSeconds is the base delay before a retry.
	// +kubebuilder:default=5
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=3600
	InitialBackoffSeconds int32 `json:"initialBackoffSeconds,omitempty"`

	// maxBackoffSeconds caps exponential retry delay.
	// +kubebuilder:default=300
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=21600
	MaxBackoffSeconds int32 `json:"maxBackoffSeconds,omitempty"`

	// retryableFailureCategories is an allowlist further restricted by platform policy.
	// +kubebuilder:validation:MaxItems=2
	// +kubebuilder:validation:items:Enum=TRANSIENT_DEPENDENCY;INTERNAL
	// +listType=set
	RetryableFailureCategories []FailureCategory `json:"retryableFailureCategories,omitempty"`
}

// ResourceValues contains bounded CPU and memory quantities.
type ResourceValues struct {
	// cpuMillis is CPU in Kubernetes millicores.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=128000
	CPUMillis int32 `json:"cpuMillis"`

	// memoryMiB is memory in mebibytes.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=524288
	MemoryMiB int32 `json:"memoryMiB"`
}

// ResourceRequirements declares requests and hard limits.
// +kubebuilder:validation:XValidation:rule="self.limits.cpuMillis >= self.requests.cpuMillis",message="CPU limit must be greater than or equal to the request"
// +kubebuilder:validation:XValidation:rule="self.limits.memoryMiB >= self.requests.memoryMiB",message="memory limit must be greater than or equal to the request"
type ResourceRequirements struct {
	Requests ResourceValues `json:"requests"`
	Limits   ResourceValues `json:"limits"`
}

// WorkspaceSpec defines deterministic workspace storage.
type WorkspaceSpec struct {
	// sizeGiB is the requested workspace capacity in gibibytes.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=2048
	SizeGiB int32 `json:"sizeGiB"`

	// storageClassName selects an approved cluster StorageClass. Empty uses the cluster default.
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^$|^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	StorageClassName string `json:"storageClassName,omitempty"`

	// retentionPolicy controls whether cleanup may delete the workspace.
	// +kubebuilder:default=Delete
	RetentionPolicy WorkspaceRetentionPolicy `json:"retentionPolicy,omitempty"`
}

// NetworkSpec defines the workload network policy profile.
// +kubebuilder:validation:XValidation:rule="self.profile == 'RestrictedEgress' ? has(self.allowedDestinationsRef) : !has(self.allowedDestinationsRef)",message="allowedDestinationsRef is required only for RestrictedEgress"
type NetworkSpec struct {
	// profile selects an approved deny-by-default network posture.
	// +kubebuilder:default=Isolated
	Profile NetworkProfile `json:"profile,omitempty"`

	// allowedDestinationsRef names an approved destination policy in this namespace.
	// +optional
	AllowedDestinationsRef *LocalObjectReference `json:"allowedDestinationsRef,omitempty"`
}

// AgentRunSpec defines immutable execution intent plus a mutable cancellation request.
// +kubebuilder:validation:XValidation:rule="self.attempt <= self.retryPolicy.maxAttempts",message="attempt must not exceed retryPolicy.maxAttempts"
// +kubebuilder:validation:XValidation:rule="self.tenantId == oldSelf.tenantId && self.projectId == oldSelf.projectId && self.runId == oldSelf.runId && self.attemptId == oldSelf.attemptId && self.attempt == oldSelf.attempt",message="execution identity is immutable"
// +kubebuilder:validation:XValidation:rule="self.runnerImage == oldSelf.runnerImage && self.runtime == oldSelf.runtime && self.executionProfile == oldSelf.executionProfile && self.taskRef == oldSelf.taskRef && self.timeoutSeconds == oldSelf.timeoutSeconds",message="execution configuration is immutable"
// +kubebuilder:validation:XValidation:rule="self.retryPolicy == oldSelf.retryPolicy && self.resources == oldSelf.resources && self.workspace == oldSelf.workspace && self.network == oldSelf.network",message="execution policy is immutable"
// +kubebuilder:validation:XValidation:rule="self.artifactDestinationRef == oldSelf.artifactDestinationRef && self.configurationRefs == oldSelf.configurationRefs && self.secretRefs == oldSelf.secretRefs && self.deployOnSuccess == oldSelf.deployOnSuccess",message="execution references are immutable"
type AgentRunSpec struct {
	TenantID  UUIDv7 `json:"tenantId"`
	ProjectID UUIDv7 `json:"projectId"`
	RunID     UUIDv7 `json:"runId"`
	AttemptID UUIDv7 `json:"attemptId"`

	// attempt is the one-based execution attempt number.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	Attempt int32 `json:"attempt"`

	// runnerImage is an immutable OCI image reference by sha256 digest.
	// +kubebuilder:validation:MaxLength=512
	// +kubebuilder:validation:Pattern=`^(?:[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?(?::[0-9]{1,5})?/)?(?:[a-z0-9]+(?:[._-][a-z0-9]+)*/)*[a-z0-9]+(?:[._-][a-z0-9]+)*@sha256:[a-f0-9]{64}$`
	RunnerImage string `json:"runnerImage"`

	// runtime is the normalized language/tool runtime identifier.
	// +kubebuilder:validation:MaxLength=120
	// +kubebuilder:validation:Pattern=`^[a-z0-9][a-z0-9._-]{0,119}$`
	Runtime string `json:"runtime"`

	// executionProfile selects an approved cluster-side workload profile.
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9][a-z0-9._-]{0,62}$`
	ExecutionProfile string `json:"executionProfile"`

	// taskRef is a secure reference; raw prompts and source are prohibited.
	// +kubebuilder:validation:MaxLength=2048
	// +kubebuilder:validation:Pattern=`^[a-z][a-z0-9+.-]{1,31}://[^[:space:]]+$`
	TaskRef string `json:"taskRef"`

	// timeoutSeconds is the maximum active execution duration.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=86400
	TimeoutSeconds int32 `json:"timeoutSeconds"`

	RetryPolicy RetryPolicy          `json:"retryPolicy"`
	Resources   ResourceRequirements `json:"resources"`
	Workspace   WorkspaceSpec        `json:"workspace"`
	Network     NetworkSpec          `json:"network"`

	// artifactDestinationRef names approved artifact destination configuration.
	ArtifactDestinationRef LocalObjectReference `json:"artifactDestinationRef"`

	// configurationRefs names non-sensitive ConfigMaps projected by the Operator.
	// +kubebuilder:validation:MaxItems=32
	// +listType=map
	// +listMapKey=name
	ConfigurationRefs []LocalObjectReference `json:"configurationRefs,omitempty"`

	// secretRefs names Secrets projected without copying values into this CR.
	// +kubebuilder:validation:MaxItems=32
	// +listType=map
	// +listMapKey=name
	SecretRefs []LocalObjectReference `json:"secretRefs,omitempty"`

	// deployOnSuccess requests the later governed deployment workflow after successful execution.
	// +kubebuilder:default=false
	DeployOnSuccess bool `json:"deployOnSuccess,omitempty"`

	// desiredState is the only ordinary mutable desired-state field.
	// +kubebuilder:default=Running
	// +kubebuilder:validation:XValidation:rule="oldSelf == 'Cancelled' ? self == 'Cancelled' : true",message="desiredState cannot return to Running after cancellation"
	DesiredState DesiredState `json:"desiredState,omitempty"`
}

// AttemptStatus retains bounded metadata for one observed attempt.
type AttemptStatus struct {
	// attempt is the one-based attempt number.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	Attempt int32 `json:"attempt"`

	// jobName and podName are deterministic Kubernetes resource names.
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	JobName string `json:"jobName,omitempty"`
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	PodName string `json:"podName,omitempty"`

	Phase           AgentRunPhase   `json:"phase,omitempty"`
	StartTime       *metav1.Time    `json:"startTime,omitempty"`
	CompletionTime  *metav1.Time    `json:"completionTime,omitempty"`
	FailureCategory FailureCategory `json:"failureCategory,omitempty"`
	// +kubebuilder:validation:MaxLength=1024
	FailureReason string `json:"failureReason,omitempty"`
	// +kubebuilder:validation:MaxLength=2048
	// +kubebuilder:validation:Pattern=`^$|^[a-z][a-z0-9+.-]{1,31}://[^[:space:]]+$`
	ArtifactManifestRef string `json:"artifactManifestRef,omitempty"`
}

// AgentRunStatus defines observed Kubernetes state.
type AgentRunStatus struct {
	Phase AgentRunPhase `json:"phase,omitempty"`

	// observedGeneration is the spec generation represented by status.
	// +kubebuilder:validation:Minimum=0
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// attempt is the attempt currently represented by status.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=10
	Attempt int32 `json:"attempt,omitempty"`

	// namespace, jobName, and podName identify observed resources.
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	Namespace string `json:"namespace,omitempty"`
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	JobName string `json:"jobName,omitempty"`
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	PodName string `json:"podName,omitempty"`

	StartTime      *metav1.Time `json:"startTime,omitempty"`
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
	HeartbeatTime  *metav1.Time `json:"heartbeatTime,omitempty"`

	FailureCategory FailureCategory `json:"failureCategory,omitempty"`
	// +kubebuilder:validation:MaxLength=1024
	FailureReason string `json:"failureReason,omitempty"`

	// artifactManifestRef is a secure reference to the mandatory result manifest.
	// +kubebuilder:validation:MaxLength=2048
	// +kubebuilder:validation:Pattern=`^$|^[a-z][a-z0-9+.-]{1,31}://[^[:space:]]+$`
	ArtifactManifestRef string `json:"artifactManifestRef,omitempty"`

	// attempts retains bounded prior and current attempt metadata.
	// +kubebuilder:validation:MaxItems=10
	// +listType=map
	// +listMapKey=attempt
	Attempts []AttemptStatus `json:"attempts,omitempty"`

	// conditions use Kubernetes condition conventions.
	// +kubebuilder:validation:MaxItems=16
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=arun
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Attempt",type=integer,JSONPath=`.status.attempt`
// +kubebuilder:printcolumn:name="Job",type=string,JSONPath=`.status.jobName`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AgentRun is the Schema for the AgentRun API.
type AgentRun struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitzero"`
	Spec              AgentRunSpec   `json:"spec"`
	Status            AgentRunStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AgentRunList contains a list of AgentRun resources.
type AgentRunList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AgentRun `json:"items"`
}

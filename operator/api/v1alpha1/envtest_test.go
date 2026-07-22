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

package v1alpha1_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apixv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	executionv1alpha1 "github.com/sid995/agentforge/operator/api/v1alpha1"
)

const testNamespace = "agent-run-schema"

func TestAgentRunCRDContract(t *testing.T) {
	testEnvironment := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	configuration, err := testEnvironment.Start()
	if err != nil {
		t.Fatalf("start envtest: %v", err)
	}
	t.Cleanup(func() {
		if err := testEnvironment.Stop(); err != nil {
			t.Errorf("stop envtest: %v", err)
		}
	})

	scheme := runtime.NewScheme()
	for name, addToScheme := range map[string]func(*runtime.Scheme) error{
		"core":     corev1.AddToScheme,
		"CRD":      apixv1.AddToScheme,
		"AgentRun": executionv1alpha1.AddToScheme,
	} {
		if err := addToScheme(scheme); err != nil {
			t.Fatalf("register %s scheme: %v", name, err)
		}
	}
	kubernetesClient, err := client.New(configuration, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("create envtest client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := kubernetesClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}); err != nil {
		t.Fatalf("create namespace: %v", err)
	}

	t.Run("structural contract", func(t *testing.T) {
		assertCRDContract(t, ctx, kubernetesClient)
	})
	t.Run("sample manifest is accepted", func(t *testing.T) {
		assertSampleManifest(t, ctx, kubernetesClient)
	})
	t.Run("safe defaults and status subresource", func(t *testing.T) {
		assertDefaultsAndStatusSubresource(t, ctx, kubernetesClient)
	})
	t.Run("schema rejects invalid desired state", func(t *testing.T) {
		assertInvalidDesiredState(t, ctx, kubernetesClient)
	})
	t.Run("execution intent is immutable and cancellation is one way", func(t *testing.T) {
		assertTransitionRules(t, ctx, kubernetesClient)
	})
}

func assertSampleManifest(t *testing.T, ctx context.Context, kubernetesClient client.Client) {
	t.Helper()
	samplePath := filepath.Join("..", "..", "config", "samples", "execution_v1alpha1_agentrun.yaml")
	sampleFile, err := os.Open(samplePath)
	if err != nil {
		t.Fatalf("open sample manifest: %v", err)
	}
	t.Cleanup(func() {
		if err := sampleFile.Close(); err != nil {
			t.Errorf("close sample manifest: %v", err)
		}
	})
	run := &executionv1alpha1.AgentRun{}
	if err := utilyaml.NewYAMLOrJSONDecoder(sampleFile, 4096).Decode(run); err != nil {
		t.Fatalf("decode sample manifest: %v", err)
	}
	if err := kubernetesClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: run.Namespace}}); err != nil {
		t.Fatalf("create sample namespace: %v", err)
	}
	if err := kubernetesClient.Create(ctx, run); err != nil {
		t.Fatalf("create sample AgentRun: %v", err)
	}
}

func assertCRDContract(t *testing.T, ctx context.Context, kubernetesClient client.Client) {
	t.Helper()
	crd := &apixv1.CustomResourceDefinition{}
	if err := kubernetesClient.Get(ctx, types.NamespacedName{Name: "agentruns.execution.agentforge.dev"}, crd); err != nil {
		t.Fatalf("get installed CRD: %v", err)
	}
	if crd.Spec.Scope != apixv1.NamespaceScoped {
		t.Fatalf("expected namespaced CRD, got %q", crd.Spec.Scope)
	}
	if len(crd.Spec.Names.ShortNames) != 1 || crd.Spec.Names.ShortNames[0] != "arun" {
		t.Fatalf("unexpected short names: %v", crd.Spec.Names.ShortNames)
	}
	if len(crd.Spec.Versions) != 1 {
		t.Fatalf("expected exactly one CRD version, got %d", len(crd.Spec.Versions))
	}
	version := crd.Spec.Versions[0]
	if version.Name != "v1alpha1" || !version.Served || !version.Storage {
		t.Fatalf("unexpected CRD version: %#v", version)
	}
	if version.Subresources == nil || version.Subresources.Status == nil {
		t.Fatal("status subresource is not enabled")
	}
	if len(version.AdditionalPrinterColumns) != 4 {
		t.Fatalf("expected four printer columns, got %d", len(version.AdditionalPrinterColumns))
	}
	wantColumns := []string{"Phase", "Attempt", "Job", "Age"}
	for index, want := range wantColumns {
		if version.AdditionalPrinterColumns[index].Name != want {
			t.Fatalf("printer column %d: want %q, got %q", index, want, version.AdditionalPrinterColumns[index].Name)
		}
	}
	rootSchema := version.Schema.OpenAPIV3Schema
	if rootSchema == nil {
		t.Fatal("OpenAPI schema is missing")
	}
	if slices.Contains(rootSchema.Required, "status") {
		t.Fatal("controller-owned status must not be required when clients create AgentRuns")
	}
	specSchema, ok := rootSchema.Properties["spec"]
	if !ok {
		t.Fatal("spec schema is missing")
	}
	for _, field := range []string{
		"tenantId", "projectId", "runId", "attemptId", "attempt", "runnerImage", "runtime",
		"executionProfile", "taskRef", "timeoutSeconds", "retryPolicy", "resources", "workspace",
		"network", "artifactDestinationRef",
	} {
		if !slices.Contains(specSchema.Required, field) {
			t.Errorf("spec.%s is not required", field)
		}
	}
}

func assertDefaultsAndStatusSubresource(t *testing.T, ctx context.Context, kubernetesClient client.Client) {
	t.Helper()
	run := validAgentRun("defaults")
	if err := kubernetesClient.Create(ctx, run); err != nil {
		t.Fatalf("create valid AgentRun: %v", err)
	}
	stored := getAgentRun(t, ctx, kubernetesClient, run.Name)
	if stored.UID == "" {
		t.Fatal("stored AgentRun has no UID")
	}
	if stored.Spec.DesiredState != executionv1alpha1.DesiredStateRunning {
		t.Errorf("desiredState default: want Running, got %q", stored.Spec.DesiredState)
	}
	if stored.Spec.Network.Profile != executionv1alpha1.NetworkProfileIsolated {
		t.Errorf("network.profile default: want Isolated, got %q", stored.Spec.Network.Profile)
	}
	if stored.Spec.Workspace.RetentionPolicy != executionv1alpha1.WorkspaceRetentionDelete {
		t.Errorf("workspace.retentionPolicy default: want Delete, got %q", stored.Spec.Workspace.RetentionPolicy)
	}
	if stored.Spec.RetryPolicy.InitialBackoffSeconds != 5 || stored.Spec.RetryPolicy.MaxBackoffSeconds != 300 {
		t.Errorf("retry backoff defaults: want 5/300, got %d/%d", stored.Spec.RetryPolicy.InitialBackoffSeconds, stored.Spec.RetryPolicy.MaxBackoffSeconds)
	}

	stored.Status.Phase = executionv1alpha1.AgentRunPhaseRunning
	if err := kubernetesClient.Update(ctx, stored); err != nil {
		t.Fatalf("ordinary update with status field: %v", err)
	}
	stored = getAgentRun(t, ctx, kubernetesClient, run.Name)
	if stored.Status.Phase != "" {
		t.Fatalf("ordinary update changed status despite status subresource: %q", stored.Status.Phase)
	}
	stored.Status.Phase = executionv1alpha1.AgentRunPhaseRunning
	stored.Status.ObservedGeneration = stored.Generation
	if err := kubernetesClient.Status().Update(ctx, stored); err != nil {
		t.Fatalf("status update: %v", err)
	}
	stored = getAgentRun(t, ctx, kubernetesClient, run.Name)
	if stored.Status.Phase != executionv1alpha1.AgentRunPhaseRunning || stored.Status.ObservedGeneration != stored.Generation {
		t.Fatalf("status subresource did not persist controller state: %#v", stored.Status)
	}
}

func assertInvalidDesiredState(t *testing.T, ctx context.Context, kubernetesClient client.Client) {
	t.Helper()
	tests := []struct {
		name   string
		mutate func(*executionv1alpha1.AgentRun)
	}{
		{name: "invalid tenant UUID", mutate: func(run *executionv1alpha1.AgentRun) { run.Spec.TenantID = "not-a-uuid" }},
		{name: "mutable image tag", mutate: func(run *executionv1alpha1.AgentRun) { run.Spec.RunnerImage = "ghcr.io/agentforge/runner:latest" }},
		{name: "raw task content", mutate: func(run *executionv1alpha1.AgentRun) { run.Spec.TaskRef = "implement this prompt" }},
		{name: "timeout above maximum", mutate: func(run *executionv1alpha1.AgentRun) { run.Spec.TimeoutSeconds = 86401 }},
		{name: "CPU limit below request", mutate: func(run *executionv1alpha1.AgentRun) { run.Spec.Resources.Limits.CPUMillis = 999 }},
		{name: "memory limit below request", mutate: func(run *executionv1alpha1.AgentRun) { run.Spec.Resources.Limits.MemoryMiB = 2047 }},
		{name: "invalid storage class reference", mutate: func(run *executionv1alpha1.AgentRun) { run.Spec.Workspace.StorageClassName = "invalid..class" }},
		{name: "invalid artifact destination reference", mutate: func(run *executionv1alpha1.AgentRun) { run.Spec.ArtifactDestinationRef.Name = "Invalid" }},
		{name: "attempt above retry ceiling", mutate: func(run *executionv1alpha1.AgentRun) { run.Spec.Attempt = 2 }},
		{name: "backoff maximum below initial", mutate: func(run *executionv1alpha1.AgentRun) {
			run.Spec.RetryPolicy.InitialBackoffSeconds = 10
			run.Spec.RetryPolicy.MaxBackoffSeconds = 5
		}},
		{name: "unapproved retry category", mutate: func(run *executionv1alpha1.AgentRun) {
			run.Spec.RetryPolicy.RetryableFailureCategories = []executionv1alpha1.FailureCategory{"POLICY"}
		}},
		{name: "restricted egress without destination policy", mutate: func(run *executionv1alpha1.AgentRun) {
			run.Spec.Network.Profile = executionv1alpha1.NetworkProfileRestrictedEgress
		}},
		{name: "isolated profile with destination policy", mutate: func(run *executionv1alpha1.AgentRun) {
			run.Spec.Network.AllowedDestinationsRef = &executionv1alpha1.LocalObjectReference{Name: "approved-egress"}
		}},
		{name: "duplicate configuration reference", mutate: func(run *executionv1alpha1.AgentRun) {
			run.Spec.ConfigurationRefs = []executionv1alpha1.LocalObjectReference{{Name: "runner-config"}, {Name: "runner-config"}}
		}},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			run := validAgentRun(fmt.Sprintf("invalid-%02d", index))
			test.mutate(run)
			err := kubernetesClient.Create(ctx, run)
			if err == nil {
				t.Fatal("expected API server to reject AgentRun")
			}
			if !apierrors.IsInvalid(err) {
				t.Fatalf("expected Invalid error, got %T: %v", err, err)
			}
		})
	}
}

func assertTransitionRules(t *testing.T, ctx context.Context, kubernetesClient client.Client) {
	t.Helper()
	run := validAgentRun("transitions")
	if err := kubernetesClient.Create(ctx, run); err != nil {
		t.Fatalf("create AgentRun: %v", err)
	}
	stored := getAgentRun(t, ctx, kubernetesClient, run.Name)
	stored.Spec.Runtime = "go-1.26"
	if err := kubernetesClient.Update(ctx, stored); !apierrors.IsInvalid(err) {
		t.Fatalf("expected immutable runtime update to be Invalid, got %v", err)
	}

	stored = getAgentRun(t, ctx, kubernetesClient, run.Name)
	stored.Spec.DesiredState = executionv1alpha1.DesiredStateCancelled
	if err := kubernetesClient.Update(ctx, stored); err != nil {
		t.Fatalf("request cancellation: %v", err)
	}
	stored = getAgentRun(t, ctx, kubernetesClient, run.Name)
	stored.Spec.DesiredState = executionv1alpha1.DesiredStateRunning
	if err := kubernetesClient.Update(ctx, stored); !apierrors.IsInvalid(err) {
		t.Fatalf("expected cancellation reversal to be Invalid, got %v", err)
	}
}

func validAgentRun(name string) *executionv1alpha1.AgentRun {
	return &executionv1alpha1.AgentRun{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
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
				MaxAttempts: 1,
			},
			Resources: executionv1alpha1.ResourceRequirements{
				Requests: executionv1alpha1.ResourceValues{CPUMillis: 1000, MemoryMiB: 2048},
				Limits:   executionv1alpha1.ResourceValues{CPUMillis: 2000, MemoryMiB: 4096},
			},
			Workspace: executionv1alpha1.WorkspaceSpec{SizeGiB: 20},
			Network:   executionv1alpha1.NetworkSpec{},
			ArtifactDestinationRef: executionv1alpha1.LocalObjectReference{
				Name: "artifact-store",
			},
			ConfigurationRefs: []executionv1alpha1.LocalObjectReference{{Name: "runner-config"}},
			SecretRefs:        []executionv1alpha1.LocalObjectReference{{Name: "runner-credentials"}},
		},
	}
}

func getAgentRun(t *testing.T, ctx context.Context, kubernetesClient client.Client, name string) *executionv1alpha1.AgentRun {
	t.Helper()
	stored := &executionv1alpha1.AgentRun{}
	if err := kubernetesClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: name}, stored); err != nil {
		t.Fatalf("get AgentRun %q: %v", name, err)
	}
	return stored
}

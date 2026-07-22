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
	"path/filepath"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	executionv1alpha1 "github.com/sid995/agentforge/operator/api/v1alpha1"
)

func TestAgentRunCRDServedByEnvtest(t *testing.T) {
	t.Parallel()

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
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("register core scheme: %v", err)
	}
	if err := executionv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("register AgentRun scheme: %v", err)
	}
	kubernetesClient, err := client.New(configuration, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("create envtest client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "operator-bootstrap"}}
	if err := kubernetesClient.Create(ctx, namespace); err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	run := &executionv1alpha1.AgentRun{ObjectMeta: metav1.ObjectMeta{Name: "bootstrap", Namespace: namespace.Name}}
	if err := kubernetesClient.Create(ctx, run); err != nil {
		t.Fatalf("create AgentRun: %v", err)
	}
	stored := &executionv1alpha1.AgentRun{}
	if err := kubernetesClient.Get(ctx, client.ObjectKeyFromObject(run), stored); err != nil {
		t.Fatalf("get AgentRun: %v", err)
	}
	if stored.Name != run.Name || stored.Namespace != namespace.Name || stored.UID == "" {
		t.Fatalf("unexpected stored AgentRun identity: namespace=%q name=%q uid=%q", stored.Namespace, stored.Name, stored.UID)
	}
}

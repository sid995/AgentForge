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

package main

import (
	"testing"

	executionv1alpha1 "github.com/sid995/agentforge/operator/api/v1alpha1"
)

func TestManagerSchemeRegistersAgentRun(t *testing.T) {
	gvks, unversioned, err := scheme.ObjectKinds(&executionv1alpha1.AgentRun{})
	if err != nil {
		t.Fatalf("resolve AgentRun kind: %v", err)
	}
	if unversioned || len(gvks) != 1 || gvks[0].Group != "execution.agentforge.dev" || gvks[0].Version != "v1alpha1" || gvks[0].Kind != "AgentRun" {
		t.Fatalf("unexpected AgentRun kinds: %#v unversioned=%v", gvks, unversioned)
	}
}

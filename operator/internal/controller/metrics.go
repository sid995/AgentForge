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

package controller

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	agentRunReconciliations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "agentforge_operator_agentrun_reconciliations_total",
			Help: "AgentRun reconciliation outcomes.",
		},
		[]string{"outcome"},
	)
	agentRunReconcileDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "agentforge_operator_agentrun_reconcile_duration_seconds",
			Help:    "AgentRun reconciliation duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
	)
	agentRunStatusWrites = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "agentforge_operator_agentrun_status_writes_total",
			Help: "AgentRun status write outcomes.",
		},
		[]string{"outcome"},
	)
)

func init() {
	ctrlmetrics.Registry.MustRegister(agentRunReconciliations, agentRunReconcileDuration, agentRunStatusWrites)
}

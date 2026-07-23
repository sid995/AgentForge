// Package kubernetes selects explicitly registered cluster clients.
package kubernetes

import (
	"context"
	"fmt"
	"sync"

	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	handoffapp "github.com/sid995/agentforge/services/platform-api/internal/application/handoff"
)

// ClientSelector builds one cached client per configured cluster context.
type ClientSelector struct {
	kubeconfig string
	contexts   map[string]string
	mu         sync.Mutex
	clients    map[string]client.Client
}

func NewClientSelector(kubeconfig string, contexts map[string]string) (*ClientSelector, error) {
	if kubeconfig == "" || len(contexts) == 0 {
		return nil, fmt.Errorf("kubeconfig and cluster contexts are required")
	}
	raw, err := clientcmd.LoadFromFile(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	seenContexts := make(map[string]struct{}, len(contexts))
	for clusterID, contextName := range contexts {
		configured, exists := raw.Contexts[contextName]
		if !exists || configured == nil || configured.Cluster == "" || raw.Clusters[configured.Cluster] == nil {
			return nil, fmt.Errorf("cluster %q maps to an unknown kubeconfig context", clusterID)
		}
		if _, duplicate := seenContexts[contextName]; duplicate {
			return nil, fmt.Errorf("kubeconfig context %q is mapped to multiple clusters", contextName)
		}
		seenContexts[contextName] = struct{}{}
	}
	return &ClientSelector{kubeconfig: kubeconfig, contexts: contexts, clients: make(map[string]client.Client)}, nil
}

func (selector *ClientSelector) Select(_ context.Context, clusterID string) (client.Client, error) {
	selector.mu.Lock()
	defer selector.mu.Unlock()
	if existing := selector.clients[clusterID]; existing != nil {
		return existing, nil
	}
	contextName, ok := selector.contexts[clusterID]
	if !ok {
		return nil, handoffapp.ErrUnauthorizedCluster
	}
	rules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: selector.kubeconfig}
	overrides := &clientcmd.ConfigOverrides{CurrentContext: contextName}
	restConfiguration, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load cluster client configuration: %w", err)
	}
	scheme, err := handoffapp.Scheme()
	if err != nil {
		return nil, err
	}
	kubernetes, err := client.New(restConfiguration, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("create cluster client: %w", err)
	}
	selector.clients[clusterID] = kubernetes
	return kubernetes, nil
}

var _ handoffapp.ClientSelector = (*ClientSelector)(nil)

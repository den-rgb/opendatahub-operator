package e2e_test

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlcfg "sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/clusterhealth"
)

// ClusterHealthStatus is the circuit breaker's view of cluster health.
type ClusterHealthStatus struct {
	Healthy bool
	Issues  []string
}

func (s ClusterHealthStatus) String() string {
	if s.Healthy {
		return "cluster healthy"
	}
	return fmt.Sprintf("cluster unhealthy: [%s]", strings.Join(s.Issues, "; "))
}

// ClusterHealthChecker wraps pkg/clusterhealth.Run for use by the circuit breaker.
type ClusterHealthChecker struct {
	mu        sync.Mutex
	k8sClient client.Client
}

func NewClusterHealthChecker() *ClusterHealthChecker {
	return &ClusterHealthChecker{}
}

func (c *ClusterHealthChecker) ensureClient() (client.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.k8sClient != nil {
		return c.k8sClient, nil
	}

	if globalDebugClient != nil {
		c.k8sClient = globalDebugClient
		return c.k8sClient, nil
	}

	cfg, err := ctrlcfg.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	c.k8sClient, err = client.New(cfg, client.Options{Scheme: Scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return c.k8sClient, nil
}

// Check runs the nodes and operator health sections via pkg/clusterhealth and
// translates the report into the ClusterHealthStatus the circuit breaker expects.
func (c *ClusterHealthChecker) Check() ClusterHealthStatus {
	status := ClusterHealthStatus{Healthy: true}

	cl, err := c.ensureClient()
	if err != nil {
		status.Healthy = false
		status.Issues = append(status.Issues, fmt.Sprintf("cannot create Kubernetes client: %v", err))
		return status
	}

	// Ensure the global debug client is available for getControllerDeploymentName(),
	// which may run before any test context has been created (e.g. pre-flight check).
	SetGlobalDebugClient(cl)

	cfg := clusterhealth.Config{
		Client: cl,
		Operator: clusterhealth.OperatorConfig{
			Namespace: testOpts.operatorNamespace,
			Name:      getControllerDeploymentName(),
		},
		OnlySections: []string{
			clusterhealth.SectionNodes,
			clusterhealth.SectionOperator,
		},
	}

	report, err := clusterhealth.Run(context.TODO(), cfg)
	if err != nil {
		status.Healthy = false
		status.Issues = append(status.Issues, fmt.Sprintf("health check failed: %v", err))
		return status
	}

	status.Healthy = report.Healthy()

	for _, s := range []struct{ name, err string }{
		{"nodes", report.Nodes.Error},
		{"operator", report.Operator.Error},
	} {
		if s.err != "" {
			status.Issues = append(status.Issues, s.err)
		}
	}

	return status
}

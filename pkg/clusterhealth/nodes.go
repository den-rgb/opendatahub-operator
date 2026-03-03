package clusterhealth

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func runNodesSection(ctx context.Context, c client.Client, _ NamespaceConfig) SectionResult[NodesSection] {
	var out SectionResult[NodesSection]

	nodes := &corev1.NodeList{}
	if err := c.List(ctx, nodes); err != nil {
		out.Error = fmt.Sprintf("cannot list nodes: %v", err)
		return out
	}

	if len(nodes.Items) == 0 {
		out.Error = "no nodes found in cluster"
		return out
	}

	problemConditions := map[corev1.NodeConditionType]corev1.ConditionStatus{
		corev1.NodeMemoryPressure:     corev1.ConditionTrue,
		corev1.NodeDiskPressure:       corev1.ConditionTrue,
		corev1.NodePIDPressure:        corev1.ConditionTrue,
		corev1.NodeNetworkUnavailable: corev1.ConditionTrue,
	}

	var issues []string

	for i := range nodes.Items {
		node := &nodes.Items[i]
		info := NodeInfo{Name: node.Name}

		for _, cond := range node.Status.Conditions {
			info.Conditions = append(info.Conditions, ConditionSummary{
				Type:    string(cond.Type),
				Status:  string(cond.Status),
				Message: cond.Message,
			})

			if cond.Type == corev1.NodeReady && cond.Status != corev1.ConditionTrue {
				reason := fmt.Sprintf("node %s not ready: %s", node.Name, cond.Message)
				info.UnhealthyReason = reason
				issues = append(issues, reason)
			} else if badStatus, tracked := problemConditions[cond.Type]; tracked && cond.Status == badStatus {
				reason := fmt.Sprintf("node %s has %s: %s", node.Name, cond.Type, cond.Message)
				info.UnhealthyReason = reason
				issues = append(issues, reason)
			}
		}

		alloc := node.Status.Allocatable
		capacity := node.Status.Capacity
		info.Allocatable = fmt.Sprintf("%dm CPU, %s memory", alloc.Cpu().MilliValue(), alloc.Memory().String())
		info.Capacity = fmt.Sprintf("%dm CPU, %s memory", capacity.Cpu().MilliValue(), capacity.Memory().String())

		out.Data.Nodes = append(out.Data.Nodes, info)
	}

	if len(issues) > 0 {
		out.Error = strings.Join(issues, "; ")
	}

	return out
}

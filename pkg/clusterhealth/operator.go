package clusterhealth

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// badWaitingReasons are container waiting states that indicate infrastructure problems.
var badWaitingReasons = map[string]bool{
	"CrashLoopBackOff":           true,
	"ImagePullBackOff":           true,
	"ErrImagePull":               true,
	"InvalidImageName":           true,
	"CreateContainerConfigError": true,
	"CreateContainerError":       true,
}

func runOperatorSection(ctx context.Context, c client.Client, op OperatorConfig) SectionResult[OperatorSection] {
	var out SectionResult[OperatorSection]

	deploy := &appsv1.Deployment{}
	err := c.Get(ctx, types.NamespacedName{
		Name:      op.Name,
		Namespace: op.Namespace,
	}, deploy)
	if err != nil {
		out.Error = fmt.Sprintf("cannot get operator deployment %s/%s: %v", op.Namespace, op.Name, err)
		return out
	}

	desiredReplicas := int32(1)
	if deploy.Spec.Replicas != nil {
		desiredReplicas = *deploy.Spec.Replicas
	}

	info := &DeploymentInfo{
		Namespace: op.Namespace,
		Name:      op.Name,
		Ready:     deploy.Status.ReadyReplicas,
		Replicas:  desiredReplicas,
	}
	for _, cond := range deploy.Status.Conditions {
		info.Conditions = append(info.Conditions, ConditionSummary{
			Type:    string(cond.Type),
			Status:  string(cond.Status),
			Message: cond.Message,
		})
	}
	out.Data.Deployment = info

	var issues []string

	if desiredReplicas == 0 {
		issues = append(issues, fmt.Sprintf("operator deployment %s: scaled to 0 replicas", op.Name))
	} else if deploy.Status.ReadyReplicas != desiredReplicas {
		issues = append(issues, fmt.Sprintf("operator deployment %s: %d/%d replicas ready",
			op.Name, deploy.Status.ReadyReplicas, desiredReplicas))
	}

	operatorPods := collectOperatorPods(ctx, c, op.Namespace)
	for i := range operatorPods {
		pod := &operatorPods[i]
		podInfo := PodInfo{
			Namespace: pod.Namespace,
			Name:      pod.Name,
			Phase:     string(pod.Status.Phase),
		}

		if pod.Status.Phase == corev1.PodPending {
			issues = append(issues, fmt.Sprintf("operator pod %s stuck in Pending", pod.Name))
		}

		appendContainerHealth(&podInfo, pod.Status.InitContainerStatuses, pod.Name, &issues)
		appendContainerHealth(&podInfo, pod.Status.ContainerStatuses, pod.Name, &issues)

		out.Data.Pods = append(out.Data.Pods, podInfo)
	}

	if len(issues) > 0 {
		out.Error = strings.Join(issues, "; ")
	}

	return out
}

// collectOperatorPods finds operator pods using known label selectors for ODH and RHOAI.
func collectOperatorPods(ctx context.Context, c client.Client, namespace string) []corev1.Pod {
	labelSelectors := []map[string]string{
		{"control-plane": "controller-manager"},
		{"name": "rhods-operator"},
	}

	seen := make(map[string]bool)
	var result []corev1.Pod

	for _, labels := range labelSelectors {
		pods := &corev1.PodList{}
		if err := c.List(ctx, pods,
			client.InNamespace(namespace),
			client.MatchingLabels(labels)); err != nil {
			continue
		}
		for i := range pods.Items {
			if !seen[pods.Items[i].Name] {
				seen[pods.Items[i].Name] = true
				result = append(result, pods.Items[i])
			}
		}
	}

	return result
}

func appendContainerHealth(podInfo *PodInfo, statuses []corev1.ContainerStatus, podName string, issues *[]string) {
	for _, cs := range statuses {
		ci := ContainerInfo{
			Name:         cs.Name,
			Ready:        cs.Ready,
			RestartCount: cs.RestartCount,
		}
		if cs.State.Waiting != nil {
			ci.Waiting = cs.State.Waiting.Reason
			if badWaitingReasons[cs.State.Waiting.Reason] {
				*issues = append(*issues, fmt.Sprintf("pod %s container %s: %s",
					podName, cs.Name, cs.State.Waiting.Reason))
			}
		}
		if cs.State.Terminated != nil {
			ci.Terminated = fmt.Sprintf("%s (exit %d)", cs.State.Terminated.Reason, cs.State.Terminated.ExitCode)
		}
		podInfo.Containers = append(podInfo.Containers, ci)
	}
}

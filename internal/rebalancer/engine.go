package rebalancer

import (
	"context"
	"fmt"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	korev1alpha1 "github.com/cxfcxf/pod-rebalancer/api/v1alpha1"
)

const (
	// RebalanceEnabledLabel is the label that must be present on pods to be considered for rebalancing
	RebalanceEnabledLabel = "kore.boring.io/rebalance"
)

// Engine handles the core rebalancing logic
type Engine struct {
	Client client.Client
}

// NewEngine creates a new rebalancer engine
func NewEngine(c client.Client) *Engine {
	return &Engine{Client: c}
}

// NodePodCount represents a node and its pod count for balancing decisions
type NodePodCount struct {
	NodeName  string
	Node      *corev1.Node
	PodCount  int
	TargetPods int // Target pods for this node (-1 means use average)
	Pods      []corev1.Pod
}

// RebalanceResult contains the result of a rebalance operation
type RebalanceResult struct {
	PodsEvicted int32
	TotalPods   int32
	Error       error
	Message     string
}

// ExecuteRebalance performs the rebalancing operation based on the RebalanceRequest spec
func (e *Engine) ExecuteRebalance(ctx context.Context, req *korev1alpha1.RebalanceRequest) RebalanceResult {
	logger := log.FromContext(ctx)

	// Get all ready nodes
	nodes, err := e.getReadyNodes(ctx)
	if err != nil {
		return RebalanceResult{Error: fmt.Errorf("failed to get nodes: %w", err)}
	}

	if len(nodes) < 2 {
		return RebalanceResult{Message: "Not enough nodes for rebalancing (need at least 2)"}
	}

	// Get pods that are candidates for rebalancing
	pods, err := e.getCandidatePods(ctx, req)
	if err != nil {
		return RebalanceResult{Error: fmt.Errorf("failed to get candidate pods: %w", err)}
	}

	if len(pods) == 0 {
		return RebalanceResult{Message: "No pods found matching criteria"}
	}

	// Calculate node imbalance and identify pods to evict
	podsToEvict := e.calculatePodsToEvict(nodes, pods, req.Spec.NodeTargets)

	if len(podsToEvict) == 0 {
		return RebalanceResult{
			TotalPods: int32(len(pods)),
			Message:   "Cluster is already balanced",
		}
	}

	logger.Info("Starting rebalance operation",
		"totalCandidatePods", len(pods),
		"podsToEvict", len(podsToEvict),
		"dryRun", req.Spec.DryRun,
	)

	// Perform rolling eviction
	batchSize := int(req.Spec.BatchSize)
	if batchSize <= 0 {
		batchSize = 5
	}
	batchInterval := time.Duration(req.Spec.BatchIntervalSeconds) * time.Second
	if batchInterval <= 0 {
		batchInterval = 30 * time.Second
	}

	var evicted int32
	for i := 0; i < len(podsToEvict); i += batchSize {
		end := i + batchSize
		if end > len(podsToEvict) {
			end = len(podsToEvict)
		}
		batch := podsToEvict[i:end]

		for _, pod := range batch {
			if req.Spec.DryRun {
				logger.Info("DryRun: would evict pod", "pod", pod.Name, "namespace", pod.Namespace, "node", pod.Spec.NodeName)
				evicted++
				continue
			}

			if err := e.evictPod(ctx, &pod); err != nil {
				logger.Error(err, "Failed to evict pod", "pod", pod.Name, "namespace", pod.Namespace)
				continue
			}
			evicted++
			logger.Info("Evicted pod", "pod", pod.Name, "namespace", pod.Namespace, "node", pod.Spec.NodeName)
		}

		// Wait between batches (except for the last batch)
		if end < len(podsToEvict) {
			logger.Info("Waiting between batches", "interval", batchInterval)
			select {
			case <-ctx.Done():
				return RebalanceResult{
					PodsEvicted: evicted,
					TotalPods:   int32(len(pods)),
					Error:       ctx.Err(),
					Message:     "Rebalance interrupted",
				}
			case <-time.After(batchInterval):
			}
		}
	}

	return RebalanceResult{
		PodsEvicted: evicted,
		TotalPods:   int32(len(pods)),
		Message:     fmt.Sprintf("Successfully evicted %d pods", evicted),
	}
}

// getReadyNodes returns all nodes that are Ready
func (e *Engine) getReadyNodes(ctx context.Context) ([]corev1.Node, error) {
	var nodeList corev1.NodeList
	if err := e.Client.List(ctx, &nodeList); err != nil {
		return nil, err
	}

	var readyNodes []corev1.Node
	for _, node := range nodeList.Items {
		if isNodeReady(&node) && !isNodeUnschedulable(&node) {
			readyNodes = append(readyNodes, node)
		}
	}
	return readyNodes, nil
}

// getCandidatePods returns pods that are candidates for rebalancing
func (e *Engine) getCandidatePods(ctx context.Context, req *korev1alpha1.RebalanceRequest) ([]corev1.Pod, error) {
	var allPods []corev1.Pod

	// Determine namespaces to search
	namespaces := req.Spec.Namespaces
	if len(namespaces) == 0 {
		// List all namespaces
		var nsList corev1.NamespaceList
		if err := e.Client.List(ctx, &nsList); err != nil {
			return nil, err
		}
		for _, ns := range nsList.Items {
			namespaces = append(namespaces, ns.Name)
		}
	}

	// Build label selector
	var selector labels.Selector
	if req.Spec.Selector != nil {
		var err error
		selector, err = metav1.LabelSelectorAsSelector(req.Spec.Selector)
		if err != nil {
			return nil, fmt.Errorf("invalid selector: %w", err)
		}
	}

	for _, ns := range namespaces {
		// Skip system namespaces
		if ns == "kube-system" || ns == "kube-public" || ns == "kube-node-lease" {
			continue
		}

		var podList corev1.PodList
		listOpts := []client.ListOption{client.InNamespace(ns)}
		if err := e.Client.List(ctx, &podList, listOpts...); err != nil {
			return nil, err
		}

		for _, pod := range podList.Items {
			// Must have the rebalance enabled label
			if pod.Labels[RebalanceEnabledLabel] != "true" {
				continue
			}

			// Must match additional selector if provided
			if selector != nil && !selector.Matches(labels.Set(pod.Labels)) {
				continue
			}

			// Must be running and scheduled
			if pod.Status.Phase != corev1.PodRunning {
				continue
			}
			if pod.Spec.NodeName == "" {
				continue
			}

			// Skip pods owned by DaemonSets
			if isOwnedByDaemonSet(&pod) {
				continue
			}

			// Skip pods with local storage (unless they explicitly opt-in)
			if hasLocalStorage(&pod) && pod.Labels["kore.boring.io/allow-local-storage-eviction"] != "true" {
				continue
			}

			allPods = append(allPods, pod)
		}
	}

	return allPods, nil
}

// calculatePodsToEvict determines which pods should be evicted to achieve balance
func (e *Engine) calculatePodsToEvict(nodes []corev1.Node, pods []corev1.Pod, nodeTargets []korev1alpha1.NodeTarget) []corev1.Pod {
	// Build node -> pods mapping with target info
	nodeMap := make(map[string]*corev1.Node)
	nodePodMap := make(map[string][]corev1.Pod)
	for i := range nodes {
		node := &nodes[i]
		nodeMap[node.Name] = node
		nodePodMap[node.Name] = []corev1.Pod{}
	}
	for _, pod := range pods {
		if _, ok := nodePodMap[pod.Spec.NodeName]; ok {
			nodePodMap[pod.Spec.NodeName] = append(nodePodMap[pod.Spec.NodeName], pod)
		}
	}

	// Convert to slice with target information
	var nodeCounts []NodePodCount
	for nodeName, nodePods := range nodePodMap {
		node := nodeMap[nodeName]
		target := e.getTargetForNode(node, nodeTargets)
		nodeCounts = append(nodeCounts, NodePodCount{
			NodeName:   nodeName,
			Node:       node,
			PodCount:   len(nodePods),
			TargetPods: target,
			Pods:       nodePods,
		})
	}

	// If no node targets specified, calculate average-based targets
	if len(nodeTargets) == 0 {
		totalPods := len(pods)
		avgPodsPerNode := float64(totalPods) / float64(len(nodes))
		for i := range nodeCounts {
			nodeCounts[i].TargetPods = int(avgPodsPerNode)
		}
	}

	// Sort nodes by excess pods (descending) - nodes with most excess first
	sort.Slice(nodeCounts, func(i, j int) bool {
		excessI := nodeCounts[i].PodCount - nodeCounts[i].TargetPods
		excessJ := nodeCounts[j].PodCount - nodeCounts[j].TargetPods
		return excessI > excessJ
	})

	// Identify pods to evict from overloaded nodes
	var podsToEvict []corev1.Pod
	for _, nc := range nodeCounts {
		// Calculate excess pods (pods above target)
		// Allow 1 pod tolerance to avoid thrashing
		excessPods := nc.PodCount - nc.TargetPods - 1
		if excessPods <= 0 {
			continue
		}

		// Select pods to evict (prefer newer pods)
		podsOnNode := nc.Pods
		sort.Slice(podsOnNode, func(i, j int) bool {
			return podsOnNode[i].CreationTimestamp.After(podsOnNode[j].CreationTimestamp.Time)
		})

		for i := 0; i < excessPods && i < len(podsOnNode); i++ {
			podsToEvict = append(podsToEvict, podsOnNode[i])
		}
	}

	return podsToEvict
}

// getTargetForNode returns the target pod count for a node based on nodeTargets.
// Returns -1 if no matching target is found (will use average later).
func (e *Engine) getTargetForNode(node *corev1.Node, nodeTargets []korev1alpha1.NodeTarget) int {
	if len(nodeTargets) == 0 {
		return -1
	}

	for _, target := range nodeTargets {
		if matchesNodeSelector(node, target.NodeSelector) {
			return int(target.TargetPodsPerNode)
		}
	}

	// No matching target found - this node won't have pods scheduled to it
	// Return 0 to evict all pods from unmatched nodes
	return 0
}

// matchesNodeSelector checks if a node matches the given selector
func matchesNodeSelector(node *corev1.Node, selector map[string]string) bool {
	if len(selector) == 0 {
		// Empty selector matches all nodes
		return true
	}

	for key, value := range selector {
		if nodeValue, ok := node.Labels[key]; !ok || nodeValue != value {
			return false
		}
	}
	return true
}

// evictPod evicts a pod using the eviction API
func (e *Engine) evictPod(ctx context.Context, pod *corev1.Pod) error {
	eviction := &policyv1.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
		DeleteOptions: &metav1.DeleteOptions{},
	}

	return e.Client.SubResource("eviction").Create(ctx, pod, eviction)
}

// isNodeReady checks if a node is in Ready condition
func isNodeReady(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

// isNodeUnschedulable checks if a node is cordoned
func isNodeUnschedulable(node *corev1.Node) bool {
	return node.Spec.Unschedulable
}

// isOwnedByDaemonSet checks if a pod is owned by a DaemonSet
func isOwnedByDaemonSet(pod *corev1.Pod) bool {
	for _, ref := range pod.OwnerReferences {
		if ref.Kind == "DaemonSet" {
			return true
		}
	}
	return false
}

// hasLocalStorage checks if a pod uses local storage
func hasLocalStorage(pod *corev1.Pod) bool {
	for _, vol := range pod.Spec.Volumes {
		if vol.EmptyDir != nil || vol.HostPath != nil {
			return true
		}
	}
	return false
}

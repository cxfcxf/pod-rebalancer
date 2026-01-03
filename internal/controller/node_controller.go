package controller

import (
	"context"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	korev1alpha1 "github.com/cxfcxf/pod-rebalancer/api/v1alpha1"
)

const (
	// DefaultCooldownPeriod is the minimum time between auto-triggered rebalances
	DefaultCooldownPeriod = 5 * time.Minute

	// AutoRebalanceNamespace is the namespace where auto-triggered RebalanceRequests are created
	AutoRebalanceNamespace = "kube-system"
)

// NodeReconciler watches for node changes and triggers rebalancing
type NodeReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// CooldownPeriod is the minimum time between auto-triggered rebalances
	CooldownPeriod time.Duration

	// lastRebalanceTime tracks when the last auto-rebalance was triggered
	lastRebalanceTime time.Time
	mu                sync.Mutex

	// knownNodes tracks nodes we've seen to detect additions/removals
	knownNodes map[string]bool
	nodesMu    sync.RWMutex
}

// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=kore.boring.io,resources=rebalancerequests,verbs=create

// Reconcile handles node events
func (r *NodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the node
	var node corev1.Node
	if err := r.Get(ctx, req.NamespacedName, &node); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		// Node was deleted
		r.nodesMu.Lock()
		wasKnown := r.knownNodes[req.Name]
		delete(r.knownNodes, req.Name)
		r.nodesMu.Unlock()

		if wasKnown {
			logger.Info("Node removed from cluster", "node", req.Name)
			r.triggerRebalanceIfNeeded(ctx, "node-removed", req.Name)
		}
		return ctrl.Result{}, nil
	}

	// Check if this is a new node
	r.nodesMu.Lock()
	if r.knownNodes == nil {
		r.knownNodes = make(map[string]bool)
	}
	wasKnown := r.knownNodes[node.Name]
	isReady := isNodeReady(&node)
	r.knownNodes[node.Name] = isReady
	r.nodesMu.Unlock()

	// Trigger rebalance if a new node became ready
	if !wasKnown && isReady {
		logger.Info("New node joined cluster and is ready", "node", node.Name)
		r.triggerRebalanceIfNeeded(ctx, "node-added", node.Name)
	} else if wasKnown && isReady {
		// Node transitioned to ready state
		logger.V(1).Info("Node is ready", "node", node.Name)
	}

	return ctrl.Result{}, nil
}

// triggerRebalanceIfNeeded creates a RebalanceRequest if cooldown has passed
func (r *NodeReconciler) triggerRebalanceIfNeeded(ctx context.Context, reason, nodeName string) {
	logger := log.FromContext(ctx)

	r.mu.Lock()
	defer r.mu.Unlock()

	cooldown := r.CooldownPeriod
	if cooldown == 0 {
		cooldown = DefaultCooldownPeriod
	}

	if time.Since(r.lastRebalanceTime) < cooldown {
		logger.Info("Skipping auto-rebalance due to cooldown",
			"reason", reason,
			"node", nodeName,
			"cooldownRemaining", cooldown-time.Since(r.lastRebalanceTime),
		)
		return
	}

	// Create a RebalanceRequest
	rebalanceReq := &korev1alpha1.RebalanceRequest{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "auto-rebalance-",
			Namespace:    AutoRebalanceNamespace,
			Labels: map[string]string{
				"kore.boring.io/auto-triggered": "true",
				"kore.boring.io/trigger-reason": reason,
				"kore.boring.io/trigger-node":   nodeName,
			},
		},
		Spec: korev1alpha1.RebalanceRequestSpec{
			BatchSize:            5,
			BatchIntervalSeconds: 30,
			DryRun:               false,
		},
	}

	if err := r.Create(ctx, rebalanceReq); err != nil {
		logger.Error(err, "Failed to create auto RebalanceRequest",
			"reason", reason,
			"node", nodeName,
		)
		return
	}

	r.lastRebalanceTime = time.Now()
	logger.Info("Created auto RebalanceRequest",
		"name", rebalanceReq.Name,
		"reason", reason,
		"node", nodeName,
	)
}

// SetupWithManager sets up the controller with the Manager
func (r *NodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize the known nodes map
	r.knownNodes = make(map[string]bool)

	// Pre-populate known nodes
	ctx := context.Background()
	var nodeList corev1.NodeList
	if err := mgr.GetClient().List(ctx, &nodeList); err == nil {
		r.nodesMu.Lock()
		for _, node := range nodeList.Items {
			r.knownNodes[node.Name] = isNodeReady(&node)
		}
		r.nodesMu.Unlock()
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Node{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return true
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				// Only process if Ready condition changed
				oldNode := e.ObjectOld.(*corev1.Node)
				newNode := e.ObjectNew.(*corev1.Node)
				return isNodeReady(oldNode) != isNodeReady(newNode)
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return true
			},
		}).
		Complete(r)
}

// isNodeReady checks if a node has Ready=True condition
func isNodeReady(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

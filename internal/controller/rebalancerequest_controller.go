package controller

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	korev1alpha1 "github.com/cxfcxf/pod-rebalancer/api/v1alpha1"
	"github.com/cxfcxf/pod-rebalancer/internal/rebalancer"
)

// RebalanceRequestReconciler reconciles a RebalanceRequest object
type RebalanceRequestReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Engine *rebalancer.Engine
}

// +kubebuilder:rbac:groups=kore.boring.io,resources=rebalancerequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kore.boring.io,resources=rebalancerequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kore.boring.io,resources=rebalancerequests/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods/eviction,verbs=create
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

// Reconcile handles RebalanceRequest reconciliation
func (r *RebalanceRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the RebalanceRequest
	var rebalanceReq korev1alpha1.RebalanceRequest
	if err := r.Get(ctx, req.NamespacedName, &rebalanceReq); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Skip if already completed or failed
	if rebalanceReq.Status.Phase == korev1alpha1.RebalancePhaseCompleted ||
		rebalanceReq.Status.Phase == korev1alpha1.RebalancePhaseFailed {
		return ctrl.Result{}, nil
	}

	// Initialize status if pending
	if rebalanceReq.Status.Phase == "" || rebalanceReq.Status.Phase == korev1alpha1.RebalancePhasePending {
		rebalanceReq.Status.Phase = korev1alpha1.RebalancePhaseRunning
		now := metav1.Now()
		rebalanceReq.Status.StartTime = &now
		rebalanceReq.Status.Message = "Starting rebalance operation"

		if err := r.Status().Update(ctx, &rebalanceReq); err != nil {
			logger.Error(err, "Failed to update status to Running")
			return ctrl.Result{RequeueAfter: 5 * time.Second}, err
		}
		// Requeue to process
		return ctrl.Result{Requeue: true}, nil
	}

	// Execute the rebalance
	logger.Info("Executing rebalance", "name", rebalanceReq.Name, "namespace", rebalanceReq.Namespace)

	result := r.Engine.ExecuteRebalance(ctx, &rebalanceReq)

	// Update status based on result
	rebalanceReq.Status.PodsEvicted = result.PodsEvicted
	rebalanceReq.Status.TotalPods = result.TotalPods
	now := metav1.Now()
	rebalanceReq.Status.CompletionTime = &now

	if result.Error != nil {
		rebalanceReq.Status.Phase = korev1alpha1.RebalancePhaseFailed
		rebalanceReq.Status.Message = result.Error.Error()
		logger.Error(result.Error, "Rebalance failed")
	} else {
		rebalanceReq.Status.Phase = korev1alpha1.RebalancePhaseCompleted
		rebalanceReq.Status.Message = result.Message
		logger.Info("Rebalance completed", "evicted", result.PodsEvicted, "total", result.TotalPods)
	}

	if err := r.Status().Update(ctx, &rebalanceReq); err != nil {
		logger.Error(err, "Failed to update final status")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RebalanceRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korev1alpha1.RebalanceRequest{}).
		Complete(r)
}

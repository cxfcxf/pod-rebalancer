package controller

import (
	"context"
	"fmt"
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

	// Get interval (default 60 seconds)
	interval := time.Duration(rebalanceReq.Spec.IntervalSeconds) * time.Second
	if interval < 30*time.Second {
		interval = 60 * time.Second
	}

	// Initialize status if pending
	if rebalanceReq.Status.Phase == "" || rebalanceReq.Status.Phase == korev1alpha1.RebalancePhasePending {
		now := metav1.Now()
		rebalanceReq.Status.Phase = korev1alpha1.RebalancePhaseActive
		rebalanceReq.Status.StartTime = &now
		rebalanceReq.Status.Message = "Rebalancer active"

		if err := r.Status().Update(ctx, &rebalanceReq); err != nil {
			logger.Error(err, "Failed to update status")
			return ctrl.Result{RequeueAfter: 5 * time.Second}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Check if it's time to run
	if rebalanceReq.Status.NextRunTime != nil {
		if time.Now().Before(rebalanceReq.Status.NextRunTime.Time) {
			waitDuration := time.Until(rebalanceReq.Status.NextRunTime.Time)
			return ctrl.Result{RequeueAfter: waitDuration}, nil
		}
	}

	// Execute the rebalance
	logger.Info("Running rebalance check",
		"name", rebalanceReq.Name,
		"namespace", rebalanceReq.Namespace,
		"run", rebalanceReq.Status.RunCount+1,
	)

	result := r.Engine.ExecuteRebalance(ctx, &rebalanceReq)
	now := metav1.Now()

	// Update status
	rebalanceReq.Status.LastEvictedCount = result.PodsEvicted
	rebalanceReq.Status.TotalPodsEvicted += result.PodsEvicted
	rebalanceReq.Status.RunCount++
	rebalanceReq.Status.LastRunTime = &now

	// Schedule next run
	nextRun := metav1.NewTime(now.Add(interval))
	rebalanceReq.Status.NextRunTime = &nextRun

	if result.Error != nil {
		rebalanceReq.Status.Message = fmt.Sprintf("Run %d error: %s", rebalanceReq.Status.RunCount, result.Error.Error())
		logger.Error(result.Error, "Rebalance check failed, will retry")
	} else {
		rebalanceReq.Status.Message = fmt.Sprintf("Run %d: %s", rebalanceReq.Status.RunCount, result.Message)
		if result.PodsEvicted > 0 {
			logger.Info("Rebalance check completed",
				"evicted", result.PodsEvicted,
				"totalEvicted", rebalanceReq.Status.TotalPodsEvicted,
			)
		}
	}

	if err := r.Status().Update(ctx, &rebalanceReq); err != nil {
		logger.Error(err, "Failed to update status")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, err
	}

	return ctrl.Result{RequeueAfter: interval}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RebalanceRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korev1alpha1.RebalanceRequest{}).
		Complete(r)
}

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

	isIntervalBased := rebalanceReq.Spec.IntervalSeconds > 0

	// For one-shot requests, skip if already completed or failed
	if !isIntervalBased {
		if rebalanceReq.Status.Phase == korev1alpha1.RebalancePhaseCompleted ||
			rebalanceReq.Status.Phase == korev1alpha1.RebalancePhaseFailed {
			return ctrl.Result{}, nil
		}
	}

	// Initialize status if pending or first run
	if rebalanceReq.Status.Phase == "" || rebalanceReq.Status.Phase == korev1alpha1.RebalancePhasePending {
		now := metav1.Now()
		rebalanceReq.Status.StartTime = &now

		if isIntervalBased {
			rebalanceReq.Status.Phase = korev1alpha1.RebalancePhaseActive
			rebalanceReq.Status.Message = "Starting interval-based rebalancing"
		} else {
			rebalanceReq.Status.Phase = korev1alpha1.RebalancePhaseRunning
			rebalanceReq.Status.Message = "Starting rebalance operation"
		}

		if err := r.Status().Update(ctx, &rebalanceReq); err != nil {
			logger.Error(err, "Failed to update status")
			return ctrl.Result{RequeueAfter: 5 * time.Second}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// For interval-based: check if it's time to run
	if isIntervalBased && rebalanceReq.Status.Phase == korev1alpha1.RebalancePhaseActive {
		if rebalanceReq.Status.NextRunTime != nil {
			if time.Now().Before(rebalanceReq.Status.NextRunTime.Time) {
				// Not time yet, requeue for the next run
				waitDuration := time.Until(rebalanceReq.Status.NextRunTime.Time)
				logger.V(1).Info("Waiting for next scheduled run", "nextRun", rebalanceReq.Status.NextRunTime.Time)
				return ctrl.Result{RequeueAfter: waitDuration}, nil
			}
		}
	}

	// Execute the rebalance
	logger.Info("Executing rebalance",
		"name", rebalanceReq.Name,
		"namespace", rebalanceReq.Namespace,
		"runCount", rebalanceReq.Status.RunCount+1,
		"intervalBased", isIntervalBased,
	)

	result := r.Engine.ExecuteRebalance(ctx, &rebalanceReq)
	now := metav1.Now()

	// Update status based on result
	rebalanceReq.Status.PodsEvicted = result.PodsEvicted
	rebalanceReq.Status.TotalPods = result.TotalPods
	rebalanceReq.Status.TotalPodsEvicted += result.PodsEvicted
	rebalanceReq.Status.RunCount++
	rebalanceReq.Status.LastRunTime = &now

	if result.Error != nil {
		if isIntervalBased {
			// For interval-based, log error but continue
			rebalanceReq.Status.Message = fmt.Sprintf("Run %d failed: %s", rebalanceReq.Status.RunCount, result.Error.Error())
			logger.Error(result.Error, "Rebalance run failed, will retry on next interval")
		} else {
			rebalanceReq.Status.Phase = korev1alpha1.RebalancePhaseFailed
			rebalanceReq.Status.CompletionTime = &now
			rebalanceReq.Status.Message = result.Error.Error()
			logger.Error(result.Error, "Rebalance failed")
		}
	} else {
		if isIntervalBased {
			rebalanceReq.Status.Message = fmt.Sprintf("Run %d: %s", rebalanceReq.Status.RunCount, result.Message)
			logger.Info("Rebalance run completed",
				"run", rebalanceReq.Status.RunCount,
				"evicted", result.PodsEvicted,
				"totalEvicted", rebalanceReq.Status.TotalPodsEvicted,
			)
		} else {
			rebalanceReq.Status.Phase = korev1alpha1.RebalancePhaseCompleted
			rebalanceReq.Status.CompletionTime = &now
			rebalanceReq.Status.Message = result.Message
			logger.Info("Rebalance completed", "evicted", result.PodsEvicted, "total", result.TotalPods)
		}
	}

	// Schedule next run for interval-based
	var requeueAfter time.Duration
	if isIntervalBased {
		interval := time.Duration(rebalanceReq.Spec.IntervalSeconds) * time.Second
		nextRun := metav1.NewTime(now.Add(interval))
		rebalanceReq.Status.NextRunTime = &nextRun
		requeueAfter = interval
	}

	if err := r.Status().Update(ctx, &rebalanceReq); err != nil {
		logger.Error(err, "Failed to update status")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, err
	}

	if isIntervalBased {
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RebalanceRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korev1alpha1.RebalanceRequest{}).
		Complete(r)
}

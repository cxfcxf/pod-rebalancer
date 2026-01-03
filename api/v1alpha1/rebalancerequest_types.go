package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeTarget defines the target number of pods for nodes matching a selector.
// This allows different hardware types to have different pod densities.
type NodeTarget struct {
	// NodeSelector selects nodes by labels (e.g., hardware=x, hardware=z)
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// TargetPodsPerNode is the desired number of pods on each node matching this selector.
	// +kubebuilder:validation:Minimum=0
	TargetPodsPerNode int32 `json:"targetPodsPerNode"`
}

// RebalanceRequestSpec defines the desired state of RebalanceRequest
type RebalanceRequestSpec struct {
	// Selector specifies which pods to consider for rebalancing.
	// Only pods matching this selector AND having the label kore.boring.io/rebalance=true will be considered.
	// +optional
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// Namespaces to target. Empty means all namespaces.
	// +optional
	Namespaces []string `json:"namespaces,omitempty"`

	// NodeTargets defines per-hardware-type pod targets.
	// If specified, the rebalancer will try to achieve these specific pod counts per node type.
	// If not specified, pods are distributed evenly across all nodes.
	// +optional
	NodeTargets []NodeTarget `json:"nodeTargets,omitempty"`

	// IntervalSeconds sets how often the rebalancer runs to maintain balance.
	// If set to 0 or not specified, this is a one-shot rebalance request.
	// If set to a positive value, the rebalancer will continuously run at this interval.
	// +kubebuilder:validation:Minimum=0
	// +optional
	IntervalSeconds int32 `json:"intervalSeconds,omitempty"`

	// BatchSize is the number of pods to evict per batch.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=5
	// +optional
	BatchSize int32 `json:"batchSize,omitempty"`

	// BatchIntervalSeconds is the time to wait between batches.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=30
	// +optional
	BatchIntervalSeconds int32 `json:"batchIntervalSeconds,omitempty"`

	// DryRun if true, will only log what would be evicted without actually evicting.
	// +kubebuilder:default=false
	// +optional
	DryRun bool `json:"dryRun,omitempty"`
}

// RebalancePhase represents the current phase of a rebalance operation
// +kubebuilder:validation:Enum=Pending;Running;Completed;Failed;Active
type RebalancePhase string

const (
	RebalancePhasePending   RebalancePhase = "Pending"
	RebalancePhaseRunning   RebalancePhase = "Running"
	RebalancePhaseCompleted RebalancePhase = "Completed"
	RebalancePhaseFailed    RebalancePhase = "Failed"
	RebalancePhaseActive    RebalancePhase = "Active" // For interval-based continuous rebalancing
)

// RebalanceRequestStatus defines the observed state of RebalanceRequest
type RebalanceRequestStatus struct {
	// Phase represents the current phase of the rebalance operation.
	// +kubebuilder:default=Pending
	Phase RebalancePhase `json:"phase,omitempty"`

	// PodsEvicted is the number of pods that have been evicted in the current/last run.
	PodsEvicted int32 `json:"podsEvicted,omitempty"`

	// TotalPodsEvicted is the cumulative number of pods evicted across all runs (for interval-based).
	TotalPodsEvicted int32 `json:"totalPodsEvicted,omitempty"`

	// TotalPods is the total number of pods that were candidates for eviction.
	TotalPods int32 `json:"totalPods,omitempty"`

	// RunCount tracks how many times the rebalancer has run (for interval-based).
	RunCount int32 `json:"runCount,omitempty"`

	// StartTime is when the rebalance operation started.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// LastRunTime is when the last rebalance run completed (for interval-based).
	// +optional
	LastRunTime *metav1.Time `json:"lastRunTime,omitempty"`

	// NextRunTime is when the next rebalance run is scheduled (for interval-based).
	// +optional
	NextRunTime *metav1.Time `json:"nextRunTime,omitempty"`

	// CompletionTime is when the rebalance operation completed (only set for one-shot).
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// Message provides additional information about the current status.
	// +optional
	Message string `json:"message,omitempty"`

	// Conditions represent the latest available observations of the rebalance request's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Interval",type=integer,JSONPath=`.spec.intervalSeconds`
// +kubebuilder:printcolumn:name="Runs",type=integer,JSONPath=`.status.runCount`
// +kubebuilder:printcolumn:name="Evicted",type=integer,JSONPath=`.status.totalPodsEvicted`
// +kubebuilder:printcolumn:name="LastRun",type=date,JSONPath=`.status.lastRunTime`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// RebalanceRequest is the Schema for the rebalancerequests API
type RebalanceRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RebalanceRequestSpec   `json:"spec,omitempty"`
	Status RebalanceRequestStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RebalanceRequestList contains a list of RebalanceRequest
type RebalanceRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RebalanceRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RebalanceRequest{}, &RebalanceRequestList{})
}

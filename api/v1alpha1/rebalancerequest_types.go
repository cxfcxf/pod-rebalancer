package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeTarget defines the maximum number of pods for nodes matching a selector.
// Pods are only evicted when a node exceeds its maximum, allowing the scheduler
// to place them on nodes with capacity.
type NodeTarget struct {
	// NodeSelector selects nodes by labels (e.g., hardware=x, hardware=z)
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// MaxPodsPerNode is the maximum number of pods allowed on each node matching this selector.
	// Pods are evicted only when this limit is exceeded.
	// +kubebuilder:validation:Minimum=1
	MaxPodsPerNode int32 `json:"maxPodsPerNode"`
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

	// NodeTargets defines per-hardware-type maximum pod counts.
	// Pods are evicted from nodes exceeding their maximum to allow redistribution.
	// If not specified, pods are balanced evenly across all nodes.
	// +optional
	NodeTargets []NodeTarget `json:"nodeTargets,omitempty"`

	// IntervalSeconds sets how often the rebalancer checks and maintains balance.
	// +kubebuilder:validation:Minimum=30
	// +kubebuilder:default=60
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
// +kubebuilder:validation:Enum=Pending;Active;Failed
type RebalancePhase string

const (
	RebalancePhasePending RebalancePhase = "Pending"
	RebalancePhaseActive  RebalancePhase = "Active"
	RebalancePhaseFailed  RebalancePhase = "Failed"
)

// RebalanceRequestStatus defines the observed state of RebalanceRequest
type RebalanceRequestStatus struct {
	// Phase represents the current phase of the rebalance operation.
	// +kubebuilder:default=Pending
	Phase RebalancePhase `json:"phase,omitempty"`

	// LastEvictedCount is the number of pods evicted in the last run.
	LastEvictedCount int32 `json:"lastEvictedCount,omitempty"`

	// TotalPodsEvicted is the cumulative number of pods evicted across all runs.
	TotalPodsEvicted int32 `json:"totalPodsEvicted,omitempty"`

	// RunCount tracks how many times the rebalancer has run.
	RunCount int32 `json:"runCount,omitempty"`

	// StartTime is when the rebalancer started.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// LastRunTime is when the last rebalance check completed.
	// +optional
	LastRunTime *metav1.Time `json:"lastRunTime,omitempty"`

	// NextRunTime is when the next rebalance check is scheduled.
	// +optional
	NextRunTime *metav1.Time `json:"nextRunTime,omitempty"`

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

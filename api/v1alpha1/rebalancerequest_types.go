package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RebalanceRequestSpec defines the desired state of RebalanceRequest
type RebalanceRequestSpec struct {
	// Selector specifies which pods to consider for rebalancing.
	// Only pods matching this selector AND having the label kore.boring.io/rebalance=true will be considered.
	// +optional
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// Namespaces to target. Empty means all namespaces.
	// +optional
	Namespaces []string `json:"namespaces,omitempty"`

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
// +kubebuilder:validation:Enum=Pending;Running;Completed;Failed
type RebalancePhase string

const (
	RebalancePhasePending   RebalancePhase = "Pending"
	RebalancePhaseRunning   RebalancePhase = "Running"
	RebalancePhaseCompleted RebalancePhase = "Completed"
	RebalancePhaseFailed    RebalancePhase = "Failed"
)

// RebalanceRequestStatus defines the observed state of RebalanceRequest
type RebalanceRequestStatus struct {
	// Phase represents the current phase of the rebalance operation.
	// +kubebuilder:default=Pending
	Phase RebalancePhase `json:"phase,omitempty"`

	// PodsEvicted is the number of pods that have been evicted.
	PodsEvicted int32 `json:"podsEvicted,omitempty"`

	// TotalPods is the total number of pods that were candidates for eviction.
	TotalPods int32 `json:"totalPods,omitempty"`

	// StartTime is when the rebalance operation started.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is when the rebalance operation completed.
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
// +kubebuilder:printcolumn:name="Evicted",type=integer,JSONPath=`.status.podsEvicted`
// +kubebuilder:printcolumn:name="Total",type=integer,JSONPath=`.status.totalPods`
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

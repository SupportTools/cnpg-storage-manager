/*
Copyright 2025 SupportTools.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EventType defines the type of storage event
// +kubebuilder:validation:Enum=expansion;wal-cleanup;alert;circuit-breaker
type EventType string

const (
	// EventTypeExpansion represents a PVC expansion event
	EventTypeExpansion EventType = "expansion"
	// EventTypeWALCleanup represents a WAL cleanup event
	EventTypeWALCleanup EventType = "wal-cleanup"
	// EventTypeAlert represents an alert event
	EventTypeAlert EventType = "alert"
	// EventTypeCircuitBreaker represents a circuit breaker state change
	EventTypeCircuitBreaker EventType = "circuit-breaker"
)

// TriggerType defines what triggered the storage event
// +kubebuilder:validation:Enum=threshold-breach;manual;scheduled;automatic
type TriggerType string

const (
	// TriggerTypeThresholdBreach indicates event was triggered by threshold breach
	TriggerTypeThresholdBreach TriggerType = "threshold-breach"
	// TriggerTypeManual indicates event was triggered manually
	TriggerTypeManual TriggerType = "manual"
	// TriggerTypeScheduled indicates event was triggered by schedule
	TriggerTypeScheduled TriggerType = "scheduled"
	// TriggerTypeAutomatic indicates event was triggered automatically by policy
	TriggerTypeAutomatic TriggerType = "automatic"
)

// PolicyReference identifies a specific StoragePolicy
type PolicyReference struct {
	// Name of the StoragePolicy
	Name string `json:"name"`

	// Namespace of the StoragePolicy
	Namespace string `json:"namespace"`
}

// EventPhase defines the phase of the storage event
// +kubebuilder:validation:Enum=Pending;InProgress;Completed;Failed
type EventPhase string

const (
	// EventPhasePending indicates the event is pending
	EventPhasePending EventPhase = "Pending"
	// EventPhaseInProgress indicates the event is in progress
	EventPhaseInProgress EventPhase = "InProgress"
	// EventPhaseCompleted indicates the event completed successfully
	EventPhaseCompleted EventPhase = "Completed"
	// EventPhaseFailed indicates the event failed
	EventPhaseFailed EventPhase = "Failed"
)

// AffectedPVC represents a PVC affected by an expansion event
type AffectedPVC struct {
	// Name of the PVC
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Node where the PVC is mounted
	// +optional
	Node string `json:"node,omitempty"`
}

// ExpansionDetails contains details for expansion events
type ExpansionDetails struct {
	// OriginalSize is the size before expansion
	// +kubebuilder:validation:Required
	OriginalSize resource.Quantity `json:"originalSize"`

	// RequestedSize is the requested new size
	// +kubebuilder:validation:Required
	RequestedSize resource.Quantity `json:"requestedSize"`

	// AffectedPVCs is the list of PVCs being expanded
	// +optional
	AffectedPVCs []AffectedPVC `json:"affectedPVCs,omitempty"`
}

// WALCleanupDetails contains details for WAL cleanup events
type WALCleanupDetails struct {
	// PodName is the name of the pod where WAL cleanup was performed
	// +optional
	PodName string `json:"podName,omitempty"`

	// FilesRemoved is the number of WAL files removed
	// +optional
	FilesRemoved int32 `json:"filesRemoved,omitempty"`

	// SpaceFreedBytes is the amount of space freed in bytes
	// +optional
	SpaceFreedBytes int64 `json:"spaceFreedBytes,omitempty"`

	// OldestRetained is the name of the oldest retained WAL segment
	// +optional
	OldestRetained string `json:"oldestRetained,omitempty"`
}

// PVCPhase represents the phase of a single PVC operation
// +kubebuilder:validation:Enum=Pending;InProgress;Completed;Failed
type PVCPhase string

const (
	// PVCPhasePending indicates PVC operation is pending
	PVCPhasePending PVCPhase = "Pending"
	// PVCPhaseInProgress indicates PVC operation is in progress
	PVCPhaseInProgress PVCPhase = "InProgress"
	// PVCPhaseCompleted indicates PVC operation completed
	PVCPhaseCompleted PVCPhase = "Completed"
	// PVCPhaseFailed indicates PVC operation failed
	PVCPhaseFailed PVCPhase = "Failed"
)

// PVCStatus represents the status of a single PVC operation
type PVCStatus struct {
	// Name of the PVC
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Phase of the PVC operation
	// +kubebuilder:validation:Required
	Phase PVCPhase `json:"phase"`

	// OriginalSize is the size before operation
	// +optional
	OriginalSize *resource.Quantity `json:"originalSize,omitempty"`

	// NewSize is the size after operation
	// +optional
	NewSize *resource.Quantity `json:"newSize,omitempty"`

	// FilesystemResized indicates if the filesystem was resized
	// +optional
	FilesystemResized bool `json:"filesystemResized,omitempty"`

	// Error message if the operation failed
	// +optional
	Error string `json:"error,omitempty"`
}

// StorageEventSpec defines the desired state of StorageEvent
type StorageEventSpec struct {
	// ClusterRef references the CNPG cluster this event relates to
	// +kubebuilder:validation:Required
	ClusterRef ClusterReference `json:"clusterRef"`

	// PolicyRef references the StoragePolicy that triggered this event
	// +optional
	PolicyRef PolicyReference `json:"policyRef,omitempty"`

	// EventType is the type of storage event
	// +kubebuilder:validation:Required
	EventType EventType `json:"eventType"`

	// Trigger is what triggered this event
	// +kubebuilder:validation:Required
	Trigger TriggerType `json:"trigger"`

	// Reason explains why this event was triggered
	// +optional
	Reason string `json:"reason,omitempty"`

	// Expansion contains details for expansion events
	// +optional
	Expansion *ExpansionDetails `json:"expansion,omitempty"`

	// WALCleanup contains details for WAL cleanup events
	// +optional
	WALCleanup *WALCleanupDetails `json:"walCleanup,omitempty"`

	// DryRun indicates this is a dry-run event
	// +kubebuilder:default=false
	// +optional
	DryRun bool `json:"dryRun,omitempty"`
}

// StorageEventStatus defines the observed state of StorageEvent
type StorageEventStatus struct {
	// Phase is the current phase of the event
	// +kubebuilder:default=Pending
	// +optional
	Phase EventPhase `json:"phase,omitempty"`

	// StartTime is when the event started
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is when the event completed
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// PVCStatuses contains per-PVC status for expansion events
	// +optional
	PVCStatuses []PVCStatus `json:"pvcStatuses,omitempty"`

	// Conditions represent the current state of the event
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// RetryCount is the number of retry attempts
	// +optional
	RetryCount int32 `json:"retryCount,omitempty"`

	// NextRetryTime is when the next retry will be attempted
	// +optional
	NextRetryTime *metav1.Time `json:"nextRetryTime,omitempty"`

	// Message provides additional details about the current status
	// +optional
	Message string `json:"message,omitempty"`
}

// StorageEvent condition types
const (
	// StorageEventConditionComplete indicates the event completed
	StorageEventConditionComplete = "Complete"
	// StorageEventConditionProgressing indicates the event is progressing
	StorageEventConditionProgressing = "Progressing"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=se
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".spec.clusterRef.name"
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.eventType"
// +kubebuilder:printcolumn:name="Trigger",type="string",JSONPath=".spec.trigger"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// StorageEvent is the Schema for the storageevents API.
// It records storage-related events for audit trail.
type StorageEvent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StorageEventSpec   `json:"spec,omitempty"`
	Status StorageEventStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// StorageEventList contains a list of StorageEvent
type StorageEventList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StorageEvent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StorageEvent{}, &StorageEventList{})
}

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

// ClusterReference identifies a specific CNPG cluster
type ClusterReference struct {
	// Name of the CNPG cluster
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the CNPG cluster
	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`
}

// ThresholdsConfig defines storage usage thresholds as percentages
type ThresholdsConfig struct {
	// Warning threshold percentage for generating warning alerts
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=70
	// +optional
	Warning int32 `json:"warning,omitempty"`

	// Critical threshold percentage for generating critical alerts
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=80
	// +optional
	Critical int32 `json:"critical,omitempty"`

	// Expansion threshold percentage for triggering automatic PVC expansion
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=85
	// +optional
	Expansion int32 `json:"expansion,omitempty"`

	// Emergency threshold percentage for triggering WAL cleanup
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=90
	// +optional
	Emergency int32 `json:"emergency,omitempty"`
}

// ExpansionConfig defines PVC expansion settings
type ExpansionConfig struct {
	// Enabled determines if automatic PVC expansion is enabled
	// +kubebuilder:default=true
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Percentage to expand PVC by when threshold is breached
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=500
	// +kubebuilder:default=50
	// +optional
	Percentage int32 `json:"percentage,omitempty"`

	// MinIncrementGi is the minimum expansion size in Gi
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=5
	// +optional
	MinIncrementGi int32 `json:"minIncrementGi,omitempty"`

	// MaxSize is the maximum PVC size limit
	// +optional
	MaxSize *resource.Quantity `json:"maxSize,omitempty"`

	// CooldownMinutes is the minimum time between expansions
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=30
	// +optional
	CooldownMinutes int32 `json:"cooldownMinutes,omitempty"`
}

// WALCleanupConfig defines WAL file cleanup settings
type WALCleanupConfig struct {
	// Enabled determines if WAL cleanup is enabled
	// +kubebuilder:default=true
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// RetainCount is the minimum number of WAL files to retain
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=10
	// +optional
	RetainCount int32 `json:"retainCount,omitempty"`

	// RequireArchived ensures only archived WAL files are cleaned
	// +kubebuilder:default=true
	// +optional
	RequireArchived bool `json:"requireArchived,omitempty"`

	// CooldownMinutes is the minimum time between WAL cleanups
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=15
	// +optional
	CooldownMinutes int32 `json:"cooldownMinutes,omitempty"`
}

// CircuitBreakerScope defines the scope of circuit breaker tracking
// +kubebuilder:validation:Enum=per-cluster;global
type CircuitBreakerScope string

const (
	// CircuitBreakerScopePerCluster tracks failures per cluster
	CircuitBreakerScopePerCluster CircuitBreakerScope = "per-cluster"
	// CircuitBreakerScopeGlobal tracks failures globally
	CircuitBreakerScopeGlobal CircuitBreakerScope = "global"
)

// CircuitBreakerConfig defines circuit breaker settings
type CircuitBreakerConfig struct {
	// MaxFailures is the number of failures before circuit opens
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=3
	// +optional
	MaxFailures int32 `json:"maxFailures,omitempty"`

	// ResetMinutes is the time before circuit resets
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=60
	// +optional
	ResetMinutes int32 `json:"resetMinutes,omitempty"`

	// Scope defines whether circuit breaker is per-cluster or global
	// +kubebuilder:default="per-cluster"
	// +optional
	Scope CircuitBreakerScope `json:"scope,omitempty"`
}

// AlertChannelType defines the type of alert channel
// +kubebuilder:validation:Enum=alertmanager;slack;pagerduty
type AlertChannelType string

const (
	// AlertChannelTypeAlertmanager sends alerts to Prometheus Alertmanager
	AlertChannelTypeAlertmanager AlertChannelType = "alertmanager"
	// AlertChannelTypeSlack sends alerts to Slack
	AlertChannelTypeSlack AlertChannelType = "slack"
	// AlertChannelTypePagerDuty sends alerts to PagerDuty
	AlertChannelTypePagerDuty AlertChannelType = "pagerduty"
)

// AlertChannel defines a single alert channel configuration
type AlertChannel struct {
	// Type of alert channel
	// +kubebuilder:validation:Required
	Type AlertChannelType `json:"type"`

	// Endpoint for alertmanager type
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// WebhookSecret is the name of the secret containing webhook URL for slack
	// +optional
	WebhookSecret string `json:"webhookSecret,omitempty"`

	// RoutingKeySecret is the name of the secret containing routing key for pagerduty
	// +optional
	RoutingKeySecret string `json:"routingKeySecret,omitempty"`

	// Channel for slack notifications
	// +optional
	Channel string `json:"channel,omitempty"`
}

// AlertingConfig defines alerting settings
type AlertingConfig struct {
	// Channels is the list of alert channels
	// +optional
	Channels []AlertChannel `json:"channels,omitempty"`

	// SuppressDuringRemediation suppresses alerts while remediation is active
	// +kubebuilder:default=true
	// +optional
	SuppressDuringRemediation bool `json:"suppressDuringRemediation,omitempty"`

	// EscalationMinutes is the time before re-alerting on unresolved issues
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=15
	// +optional
	EscalationMinutes int32 `json:"escalationMinutes,omitempty"`
}

// StoragePolicySpec defines the desired state of StoragePolicy
type StoragePolicySpec struct {
	// Selector is a label selector for matching CNPG clusters
	// +optional
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// ExcludeClusters is a list of clusters to exclude even if they match the selector
	// +optional
	ExcludeClusters []ClusterReference `json:"excludeClusters,omitempty"`

	// Thresholds defines storage usage thresholds
	// +optional
	Thresholds ThresholdsConfig `json:"thresholds,omitempty"`

	// Expansion defines PVC expansion settings
	// +optional
	Expansion ExpansionConfig `json:"expansion,omitempty"`

	// WALCleanup defines WAL file cleanup settings
	// +optional
	WALCleanup WALCleanupConfig `json:"walCleanup,omitempty"`

	// CircuitBreaker defines circuit breaker settings
	// +optional
	CircuitBreaker CircuitBreakerConfig `json:"circuitBreaker,omitempty"`

	// Alerting defines alerting settings
	// +optional
	Alerting AlertingConfig `json:"alerting,omitempty"`

	// DryRun enables dry-run mode where no actions are taken
	// +kubebuilder:default=false
	// +optional
	DryRun bool `json:"dryRun,omitempty"`
}

// ManagedCluster represents a cluster managed by this policy
type ManagedCluster struct {
	// Name of the CNPG cluster
	Name string `json:"name"`

	// Namespace of the CNPG cluster
	Namespace string `json:"namespace"`

	// LastChecked is when the cluster was last evaluated
	LastChecked metav1.Time `json:"lastChecked"`

	// UsagePercent is the current storage usage percentage
	UsagePercent int32 `json:"usagePercent"`

	// Status is the current status of the cluster
	Status string `json:"status"`
}

// StoragePolicyStatus defines the observed state of StoragePolicy
type StoragePolicyStatus struct {
	// Conditions represent the current state of the StoragePolicy
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ManagedClusters is the list of clusters managed by this policy
	// +optional
	ManagedClusters []ManagedCluster `json:"managedClusters,omitempty"`

	// LastEvaluated is the timestamp of the last policy evaluation
	// +optional
	LastEvaluated *metav1.Time `json:"lastEvaluated,omitempty"`

	// ObservedGeneration is the generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// StoragePolicy condition types
const (
	// StoragePolicyConditionActive indicates the policy is actively monitoring clusters
	StoragePolicyConditionActive = "Active"
	// StoragePolicyConditionConflicting indicates the policy conflicts with another policy
	StoragePolicyConditionConflicting = "Conflicting"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=sp
// +kubebuilder:printcolumn:name="Managed",type="integer",JSONPath=".status.managedClusters"
// +kubebuilder:printcolumn:name="Warning",type="integer",JSONPath=".spec.thresholds.warning"
// +kubebuilder:printcolumn:name="Critical",type="integer",JSONPath=".spec.thresholds.critical"
// +kubebuilder:printcolumn:name="Expansion",type="integer",JSONPath=".spec.thresholds.expansion"
// +kubebuilder:printcolumn:name="DryRun",type="boolean",JSONPath=".spec.dryRun"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// StoragePolicy is the Schema for the storagepolicies API
type StoragePolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StoragePolicySpec   `json:"spec,omitempty"`
	Status StoragePolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// StoragePolicyList contains a list of StoragePolicy
type StoragePolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StoragePolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StoragePolicy{}, &StoragePolicyList{})
}

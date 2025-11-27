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

package annotations

import (
	"fmt"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// AnnotationPrefix is the prefix for all CNPG Storage Manager annotations
	AnnotationPrefix = "storage.cnpg.supporttools.io"

	// Management annotations
	AnnotationManaged         = AnnotationPrefix + "/managed"
	AnnotationPaused          = AnnotationPrefix + "/paused"
	AnnotationPauseReason     = AnnotationPrefix + "/pause-reason"
	AnnotationPauseUntil      = AnnotationPrefix + "/pause-until"
	AnnotationPolicyName      = AnnotationPrefix + "/policy-name"
	AnnotationPolicyNamespace = AnnotationPrefix + "/policy-namespace"

	// Status annotations
	AnnotationLastCheck           = AnnotationPrefix + "/last-check"
	AnnotationCurrentUsagePercent = AnnotationPrefix + "/current-usage-percent"
	AnnotationTargetSize          = AnnotationPrefix + "/target-size"

	// Expansion annotations
	AnnotationExpansionRequested = AnnotationPrefix + "/expansion-requested"
	AnnotationExpansionReason    = AnnotationPrefix + "/expansion-reason"
	AnnotationExpansionCompleted = AnnotationPrefix + "/expansion-completed"
	AnnotationLastExpansion      = AnnotationPrefix + "/last-expansion"

	// WAL cleanup annotations
	AnnotationWALCleanupLast      = AnnotationPrefix + "/wal-cleanup-last"
	AnnotationWALCleanupCompleted = AnnotationPrefix + "/wal-cleanup-completed"

	// Circuit breaker annotations
	AnnotationCircuitBreakerOpen  = AnnotationPrefix + "/circuit-breaker-open"
	AnnotationCircuitBreakerReset = AnnotationPrefix + "/reset-circuit-breaker"
	AnnotationFailureCount        = AnnotationPrefix + "/failure-count"
	AnnotationLastFailure         = AnnotationPrefix + "/last-failure"
)

// ClusterAnnotations provides helpers for reading/writing cluster annotations
type ClusterAnnotations struct {
	annotations map[string]string
}

// NewClusterAnnotations creates a new ClusterAnnotations from an object's annotations
func NewClusterAnnotations(obj metav1.Object) *ClusterAnnotations {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	return &ClusterAnnotations{annotations: annotations}
}

// GetAnnotations returns the underlying annotations map
func (ca *ClusterAnnotations) GetAnnotations() map[string]string {
	return ca.annotations
}

// IsManaged returns true if the cluster is managed by CNPG Storage Manager
func (ca *ClusterAnnotations) IsManaged() bool {
	return ca.annotations[AnnotationManaged] == "true"
}

// SetManaged marks the cluster as managed
func (ca *ClusterAnnotations) SetManaged(managed bool) {
	ca.annotations[AnnotationManaged] = strconv.FormatBool(managed)
}

// IsPaused returns true if the cluster is paused
func (ca *ClusterAnnotations) IsPaused() bool {
	if ca.annotations[AnnotationPaused] != "true" {
		return false
	}

	// Check if pause has expired
	if pauseUntil, ok := ca.annotations[AnnotationPauseUntil]; ok {
		if t, err := time.Parse(time.RFC3339, pauseUntil); err == nil {
			if time.Now().After(t) {
				return false // Pause expired
			}
		}
	}

	return true
}

// SetPaused sets the paused state
func (ca *ClusterAnnotations) SetPaused(paused bool, reason string, until *time.Time) {
	ca.annotations[AnnotationPaused] = strconv.FormatBool(paused)
	if paused && reason != "" {
		ca.annotations[AnnotationPauseReason] = reason
	} else {
		delete(ca.annotations, AnnotationPauseReason)
	}
	if paused && until != nil {
		ca.annotations[AnnotationPauseUntil] = until.Format(time.RFC3339)
	} else {
		delete(ca.annotations, AnnotationPauseUntil)
	}
}

// GetPauseReason returns the pause reason
func (ca *ClusterAnnotations) GetPauseReason() string {
	return ca.annotations[AnnotationPauseReason]
}

// GetPolicyReference returns the policy name and namespace managing this cluster
func (ca *ClusterAnnotations) GetPolicyReference() (name, namespace string) {
	return ca.annotations[AnnotationPolicyName], ca.annotations[AnnotationPolicyNamespace]
}

// SetPolicyReference sets the policy reference
func (ca *ClusterAnnotations) SetPolicyReference(name, namespace string) {
	ca.annotations[AnnotationPolicyName] = name
	ca.annotations[AnnotationPolicyNamespace] = namespace
}

// GetLastCheck returns the last check timestamp
func (ca *ClusterAnnotations) GetLastCheck() *time.Time {
	if ts, ok := ca.annotations[AnnotationLastCheck]; ok {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			return &t
		}
	}
	return nil
}

// SetLastCheck sets the last check timestamp
func (ca *ClusterAnnotations) SetLastCheck(t time.Time) {
	ca.annotations[AnnotationLastCheck] = t.Format(time.RFC3339)
}

// GetCurrentUsagePercent returns the current usage percentage
func (ca *ClusterAnnotations) GetCurrentUsagePercent() (int32, bool) {
	if v, ok := ca.annotations[AnnotationCurrentUsagePercent]; ok {
		if percent, err := strconv.ParseInt(v, 10, 32); err == nil {
			return int32(percent), true
		}
	}
	return 0, false
}

// SetCurrentUsagePercent sets the current usage percentage
func (ca *ClusterAnnotations) SetCurrentUsagePercent(percent int32) {
	ca.annotations[AnnotationCurrentUsagePercent] = strconv.FormatInt(int64(percent), 10)
}

// GetTargetSize returns the target PVC size
func (ca *ClusterAnnotations) GetTargetSize() *resource.Quantity {
	if v, ok := ca.annotations[AnnotationTargetSize]; ok {
		if q, err := resource.ParseQuantity(v); err == nil {
			return &q
		}
	}
	return nil
}

// SetTargetSize sets the target PVC size
func (ca *ClusterAnnotations) SetTargetSize(size resource.Quantity) {
	ca.annotations[AnnotationTargetSize] = size.String()
}

// GetExpansionRequested returns the expansion request timestamp
func (ca *ClusterAnnotations) GetExpansionRequested() *time.Time {
	if ts, ok := ca.annotations[AnnotationExpansionRequested]; ok {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			return &t
		}
	}
	return nil
}

// SetExpansionRequested marks an expansion as requested
func (ca *ClusterAnnotations) SetExpansionRequested(reason string) {
	ca.annotations[AnnotationExpansionRequested] = time.Now().Format(time.RFC3339)
	ca.annotations[AnnotationExpansionReason] = reason
}

// ClearExpansionRequested clears the expansion request
func (ca *ClusterAnnotations) ClearExpansionRequested() {
	delete(ca.annotations, AnnotationExpansionRequested)
	delete(ca.annotations, AnnotationExpansionReason)
}

// GetExpansionReason returns the expansion reason
func (ca *ClusterAnnotations) GetExpansionReason() string {
	return ca.annotations[AnnotationExpansionReason]
}

// GetLastExpansion returns the last expansion timestamp
func (ca *ClusterAnnotations) GetLastExpansion() *time.Time {
	if ts, ok := ca.annotations[AnnotationLastExpansion]; ok {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			return &t
		}
	}
	return nil
}

// SetLastExpansion sets the last expansion timestamp
func (ca *ClusterAnnotations) SetLastExpansion(t time.Time) {
	ca.annotations[AnnotationLastExpansion] = t.Format(time.RFC3339)
}

// GetLastWALCleanup returns the last WAL cleanup timestamp
func (ca *ClusterAnnotations) GetLastWALCleanup() *time.Time {
	if ts, ok := ca.annotations[AnnotationWALCleanupLast]; ok {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			return &t
		}
	}
	return nil
}

// SetLastWALCleanup sets the last WAL cleanup timestamp
func (ca *ClusterAnnotations) SetLastWALCleanup(t time.Time) {
	ca.annotations[AnnotationWALCleanupLast] = t.Format(time.RFC3339)
}

// IsCircuitBreakerOpen returns true if the circuit breaker is open
func (ca *ClusterAnnotations) IsCircuitBreakerOpen() bool {
	return ca.annotations[AnnotationCircuitBreakerOpen] == "true"
}

// SetCircuitBreakerOpen sets the circuit breaker state
func (ca *ClusterAnnotations) SetCircuitBreakerOpen(open bool) {
	ca.annotations[AnnotationCircuitBreakerOpen] = strconv.FormatBool(open)
}

// ShouldResetCircuitBreaker returns true if a manual reset was requested
func (ca *ClusterAnnotations) ShouldResetCircuitBreaker() bool {
	return ca.annotations[AnnotationCircuitBreakerReset] == "true"
}

// ClearCircuitBreakerReset clears the manual reset annotation
func (ca *ClusterAnnotations) ClearCircuitBreakerReset() {
	delete(ca.annotations, AnnotationCircuitBreakerReset)
}

// GetFailureCount returns the current failure count
func (ca *ClusterAnnotations) GetFailureCount() int32 {
	if v, ok := ca.annotations[AnnotationFailureCount]; ok {
		if count, err := strconv.ParseInt(v, 10, 32); err == nil {
			return int32(count)
		}
	}
	return 0
}

// SetFailureCount sets the failure count
func (ca *ClusterAnnotations) SetFailureCount(count int32) {
	ca.annotations[AnnotationFailureCount] = strconv.FormatInt(int64(count), 10)
}

// IncrementFailureCount increments the failure count and returns the new value
func (ca *ClusterAnnotations) IncrementFailureCount() int32 {
	count := ca.GetFailureCount() + 1
	ca.SetFailureCount(count)
	ca.annotations[AnnotationLastFailure] = time.Now().Format(time.RFC3339)
	return count
}

// ResetFailureCount resets the failure count to zero
func (ca *ClusterAnnotations) ResetFailureCount() {
	ca.SetFailureCount(0)
	delete(ca.annotations, AnnotationLastFailure)
}

// GetLastFailure returns the last failure timestamp
func (ca *ClusterAnnotations) GetLastFailure() *time.Time {
	if ts, ok := ca.annotations[AnnotationLastFailure]; ok {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			return &t
		}
	}
	return nil
}

// CanExpand checks if expansion is allowed based on cooldown
func (ca *ClusterAnnotations) CanExpand(cooldownMinutes int32) (bool, string) {
	if ca.IsPaused() {
		return false, fmt.Sprintf("cluster is paused: %s", ca.GetPauseReason())
	}

	if ca.IsCircuitBreakerOpen() {
		return false, "circuit breaker is open"
	}

	lastExpansion := ca.GetLastExpansion()
	if lastExpansion != nil {
		cooldown := time.Duration(cooldownMinutes) * time.Minute
		nextAllowed := lastExpansion.Add(cooldown)
		if time.Now().Before(nextAllowed) {
			remaining := time.Until(nextAllowed).Round(time.Second)
			return false, fmt.Sprintf("cooldown active, %s remaining", remaining)
		}
	}

	return true, ""
}

// CanWALCleanup checks if WAL cleanup is allowed based on cooldown
func (ca *ClusterAnnotations) CanWALCleanup(cooldownMinutes int32) (bool, string) {
	if ca.IsPaused() {
		return false, fmt.Sprintf("cluster is paused: %s", ca.GetPauseReason())
	}

	if ca.IsCircuitBreakerOpen() {
		return false, "circuit breaker is open"
	}

	lastCleanup := ca.GetLastWALCleanup()
	if lastCleanup != nil {
		cooldown := time.Duration(cooldownMinutes) * time.Minute
		nextAllowed := lastCleanup.Add(cooldown)
		if time.Now().Before(nextAllowed) {
			remaining := time.Until(nextAllowed).Round(time.Second)
			return false, fmt.Sprintf("cooldown active, %s remaining", remaining)
		}
	}

	return true, ""
}

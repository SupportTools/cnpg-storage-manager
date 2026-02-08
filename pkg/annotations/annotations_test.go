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
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewClusterAnnotations(t *testing.T) {
	// Test with nil annotations
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-pod",
			Annotations: nil,
		},
	}
	ca := NewClusterAnnotations(pod)
	if ca.annotations == nil {
		t.Error("expected non-nil annotations map")
	}

	// Test with existing annotations
	pod.Annotations = map[string]string{
		"existing": "value",
	}
	ca = NewClusterAnnotations(pod)
	if ca.annotations["existing"] != "value" {
		t.Error("expected existing annotation to be preserved")
	}
}

func TestIsManaged(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    bool
	}{
		{
			name:        "managed true",
			annotations: map[string]string{AnnotationManaged: "true"},
			expected:    true,
		},
		{
			name:        "managed false",
			annotations: map[string]string{AnnotationManaged: "false"},
			expected:    false,
		},
		{
			name:        "managed not set",
			annotations: map[string]string{},
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ca := &ClusterAnnotations{annotations: tt.annotations}
			if got := ca.IsManaged(); got != tt.expected {
				t.Errorf("IsManaged() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSetManaged(t *testing.T) {
	ca := &ClusterAnnotations{annotations: map[string]string{}}

	ca.SetManaged(true)
	//nolint:goconst // "true" is a test assertion value
	if ca.annotations[AnnotationManaged] != "true" {
		t.Error("expected managed to be true")
	}

	ca.SetManaged(false)
	if ca.annotations[AnnotationManaged] != "false" {
		t.Error("expected managed to be false")
	}
}

func TestIsPaused(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    bool
	}{
		{
			name:        "paused true",
			annotations: map[string]string{AnnotationPaused: "true"},
			expected:    true,
		},
		{
			name:        "paused false",
			annotations: map[string]string{AnnotationPaused: "false"},
			expected:    false,
		},
		{
			name:        "paused not set",
			annotations: map[string]string{},
			expected:    false,
		},
		{
			name: "paused with expired time",
			annotations: map[string]string{
				AnnotationPaused:     "true",
				AnnotationPauseUntil: time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
			},
			expected: false, // Pause has expired
		},
		{
			name: "paused with future time",
			annotations: map[string]string{
				AnnotationPaused:     "true",
				AnnotationPauseUntil: time.Now().Add(1 * time.Hour).Format(time.RFC3339),
			},
			expected: true, // Pause still active
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ca := &ClusterAnnotations{annotations: tt.annotations}
			if got := ca.IsPaused(); got != tt.expected {
				t.Errorf("IsPaused() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSetPaused(t *testing.T) {
	ca := &ClusterAnnotations{annotations: map[string]string{}}

	// Test pause with reason and time
	until := time.Now().Add(1 * time.Hour)
	ca.SetPaused(true, "maintenance", &until)

	if ca.annotations[AnnotationPaused] != "true" {
		t.Error("expected paused to be true")
	}
	if ca.annotations[AnnotationPauseReason] != "maintenance" {
		t.Error("expected pause reason to be 'maintenance'")
	}
	if ca.annotations[AnnotationPauseUntil] == "" {
		t.Error("expected pause until to be set")
	}

	// Test unpause
	ca.SetPaused(false, "", nil)
	if ca.annotations[AnnotationPaused] != "false" {
		t.Error("expected paused to be false")
	}
	if _, ok := ca.annotations[AnnotationPauseReason]; ok {
		t.Error("expected pause reason to be deleted")
	}
	if _, ok := ca.annotations[AnnotationPauseUntil]; ok {
		t.Error("expected pause until to be deleted")
	}
}

func TestPolicyReference(t *testing.T) {
	ca := &ClusterAnnotations{annotations: map[string]string{}}

	// Set policy reference
	ca.SetPolicyReference("test-policy", "default")

	name, namespace := ca.GetPolicyReference()
	if name != "test-policy" {
		t.Errorf("expected policy name 'test-policy', got '%s'", name)
	}
	if namespace != "default" {
		t.Errorf("expected policy namespace 'default', got '%s'", namespace)
	}
}

func TestLastCheck(t *testing.T) {
	ca := &ClusterAnnotations{annotations: map[string]string{}}

	// Not set
	if ca.GetLastCheck() != nil {
		t.Error("expected nil when not set")
	}

	// Set and get
	now := time.Now()
	ca.SetLastCheck(now)

	got := ca.GetLastCheck()
	if got == nil {
		t.Error("expected non-nil time")
		return
	}
	// Compare with some tolerance
	if got.Sub(now) > time.Second {
		t.Error("times don't match")
	}
}

func TestCurrentUsagePercent(t *testing.T) {
	ca := &ClusterAnnotations{annotations: map[string]string{}}

	// Not set
	if _, ok := ca.GetCurrentUsagePercent(); ok {
		t.Error("expected ok to be false when not set")
	}

	// Set and get
	ca.SetCurrentUsagePercent(75)

	got, ok := ca.GetCurrentUsagePercent()
	if !ok {
		t.Error("expected ok to be true")
	}
	if got != 75 {
		t.Errorf("expected 75, got %d", got)
	}
}

func TestTargetSize(t *testing.T) {
	ca := &ClusterAnnotations{annotations: map[string]string{}}

	// Not set
	if ca.GetTargetSize() != nil {
		t.Error("expected nil when not set")
	}

	// Set and get
	size := resource.MustParse("100Gi")
	ca.SetTargetSize(size)

	got := ca.GetTargetSize()
	if got == nil {
		t.Error("expected non-nil quantity")
	}
	if got.String() != size.String() {
		t.Errorf("expected %s, got %s", size.String(), got.String())
	}
}

func TestExpansionRequested(t *testing.T) {
	ca := &ClusterAnnotations{annotations: map[string]string{}}

	// Not set
	if ca.GetExpansionRequested() != nil {
		t.Error("expected nil when not set")
	}

	// Set
	ca.SetExpansionRequested("threshold breach")

	if ca.GetExpansionRequested() == nil {
		t.Error("expected non-nil time")
	}
	if ca.GetExpansionReason() != "threshold breach" {
		t.Error("expected reason to be 'threshold breach'")
	}

	// Clear
	ca.ClearExpansionRequested()
	if ca.GetExpansionRequested() != nil {
		t.Error("expected nil after clear")
	}
	if ca.GetExpansionReason() != "" {
		t.Error("expected empty reason after clear")
	}
}

func TestCircuitBreaker(t *testing.T) {
	ca := &ClusterAnnotations{annotations: map[string]string{}}

	// Default state
	if ca.IsCircuitBreakerOpen() {
		t.Error("expected circuit breaker to be closed by default")
	}

	// Open
	ca.SetCircuitBreakerOpen(true)
	if !ca.IsCircuitBreakerOpen() {
		t.Error("expected circuit breaker to be open")
	}

	// Close
	ca.SetCircuitBreakerOpen(false)
	if ca.IsCircuitBreakerOpen() {
		t.Error("expected circuit breaker to be closed")
	}
}

func TestResetCircuitBreaker(t *testing.T) {
	ca := &ClusterAnnotations{annotations: map[string]string{}}

	// Not set
	if ca.ShouldResetCircuitBreaker() {
		t.Error("expected false when not set")
	}

	// Set reset flag
	ca.annotations[AnnotationCircuitBreakerReset] = "true"
	if !ca.ShouldResetCircuitBreaker() {
		t.Error("expected true when set")
	}

	// Clear
	ca.ClearCircuitBreakerReset()
	if ca.ShouldResetCircuitBreaker() {
		t.Error("expected false after clear")
	}
}

func TestFailureCount(t *testing.T) {
	ca := &ClusterAnnotations{annotations: map[string]string{}}

	// Default
	if ca.GetFailureCount() != 0 {
		t.Error("expected 0 by default")
	}

	// Set
	ca.SetFailureCount(5)
	if ca.GetFailureCount() != 5 {
		t.Error("expected 5")
	}

	// Increment
	count := ca.IncrementFailureCount()
	if count != 6 {
		t.Errorf("expected 6, got %d", count)
	}
	if ca.GetLastFailure() == nil {
		t.Error("expected last failure time to be set")
	}

	// Reset
	ca.ResetFailureCount()
	if ca.GetFailureCount() != 0 {
		t.Error("expected 0 after reset")
	}
	if ca.GetLastFailure() != nil {
		t.Error("expected nil last failure after reset")
	}
}

func TestCanExpand(t *testing.T) {
	tests := []struct {
		name            string
		annotations     map[string]string
		cooldownMinutes int32
		expectAllowed   bool
	}{
		{
			name:            "can expand - no previous expansion",
			annotations:     map[string]string{},
			cooldownMinutes: 30,
			expectAllowed:   true,
		},
		{
			name: "cannot expand - paused",
			annotations: map[string]string{
				AnnotationPaused:      "true",
				AnnotationPauseReason: "maintenance",
			},
			cooldownMinutes: 30,
			expectAllowed:   false,
		},
		{
			name: "cannot expand - circuit breaker open",
			annotations: map[string]string{
				AnnotationCircuitBreakerOpen: "true",
			},
			cooldownMinutes: 30,
			expectAllowed:   false,
		},
		{
			name: "cannot expand - cooldown active",
			annotations: map[string]string{
				AnnotationLastExpansion: time.Now().Add(-10 * time.Minute).Format(time.RFC3339),
			},
			cooldownMinutes: 30,
			expectAllowed:   false,
		},
		{
			name: "can expand - cooldown expired",
			annotations: map[string]string{
				AnnotationLastExpansion: time.Now().Add(-60 * time.Minute).Format(time.RFC3339),
			},
			cooldownMinutes: 30,
			expectAllowed:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ca := &ClusterAnnotations{annotations: tt.annotations}
			allowed, _ := ca.CanExpand(tt.cooldownMinutes)
			if allowed != tt.expectAllowed {
				t.Errorf("CanExpand() = %v, want %v", allowed, tt.expectAllowed)
			}
		})
	}
}

func TestCanWALCleanup(t *testing.T) {
	tests := []struct {
		name            string
		annotations     map[string]string
		cooldownMinutes int32
		expectAllowed   bool
	}{
		{
			name:            "can cleanup - no previous cleanup",
			annotations:     map[string]string{},
			cooldownMinutes: 15,
			expectAllowed:   true,
		},
		{
			name: "cannot cleanup - paused",
			annotations: map[string]string{
				AnnotationPaused: "true",
			},
			cooldownMinutes: 15,
			expectAllowed:   false,
		},
		{
			name: "cannot cleanup - circuit breaker open",
			annotations: map[string]string{
				AnnotationCircuitBreakerOpen: "true",
			},
			cooldownMinutes: 15,
			expectAllowed:   false,
		},
		{
			name: "cannot cleanup - cooldown active",
			annotations: map[string]string{
				AnnotationWALCleanupLast: time.Now().Add(-5 * time.Minute).Format(time.RFC3339),
			},
			cooldownMinutes: 15,
			expectAllowed:   false,
		},
		{
			name: "can cleanup - cooldown expired",
			annotations: map[string]string{
				AnnotationWALCleanupLast: time.Now().Add(-30 * time.Minute).Format(time.RFC3339),
			},
			cooldownMinutes: 15,
			expectAllowed:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ca := &ClusterAnnotations{annotations: tt.annotations}
			allowed, _ := ca.CanWALCleanup(tt.cooldownMinutes)
			if allowed != tt.expectAllowed {
				t.Errorf("CanWALCleanup() = %v, want %v", allowed, tt.expectAllowed)
			}
		})
	}
}

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

package policy

import (
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cnpgv1alpha1 "github.com/supporttools/cnpg-storage-manager/api/v1alpha1"
)

func TestEvaluateThresholds(t *testing.T) {
	evaluator := NewEvaluator()

	tests := []struct {
		name          string
		usagePercent  float64
		thresholds    cnpgv1alpha1.ThresholdsConfig
		expectedLevel ThresholdLevel
		shouldAlert   bool
		shouldExpand  bool
		shouldCleanup bool
	}{
		{
			name:         "normal usage below warning",
			usagePercent: 50.0,
			thresholds: cnpgv1alpha1.ThresholdsConfig{
				Warning:   70,
				Critical:  80,
				Expansion: 85,
				Emergency: 90,
			},
			expectedLevel: ThresholdLevelNormal,
			shouldAlert:   false,
			shouldExpand:  false,
			shouldCleanup: false,
		},
		{
			name:         "warning threshold breached",
			usagePercent: 75.0,
			thresholds: cnpgv1alpha1.ThresholdsConfig{
				Warning:   70,
				Critical:  80,
				Expansion: 85,
				Emergency: 90,
			},
			expectedLevel: ThresholdLevelWarning,
			shouldAlert:   true,
			shouldExpand:  false,
			shouldCleanup: false,
		},
		{
			name:         "critical threshold breached",
			usagePercent: 82.0,
			thresholds: cnpgv1alpha1.ThresholdsConfig{
				Warning:   70,
				Critical:  80,
				Expansion: 85,
				Emergency: 90,
			},
			expectedLevel: ThresholdLevelCritical,
			shouldAlert:   true,
			shouldExpand:  false,
			shouldCleanup: false,
		},
		{
			name:         "expansion threshold breached",
			usagePercent: 87.0,
			thresholds: cnpgv1alpha1.ThresholdsConfig{
				Warning:   70,
				Critical:  80,
				Expansion: 85,
				Emergency: 90,
			},
			expectedLevel: ThresholdLevelExpansion,
			shouldAlert:   true,
			shouldExpand:  true,
			shouldCleanup: false,
		},
		{
			name:         "emergency threshold breached",
			usagePercent: 92.0,
			thresholds: cnpgv1alpha1.ThresholdsConfig{
				Warning:   70,
				Critical:  80,
				Expansion: 85,
				Emergency: 90,
			},
			expectedLevel: ThresholdLevelEmergency,
			shouldAlert:   true,
			shouldExpand:  true,
			shouldCleanup: true,
		},
		{
			name:          "uses defaults when thresholds are zero",
			usagePercent:  72.0,
			thresholds:    cnpgv1alpha1.ThresholdsConfig{}, // All zeros, should use defaults
			expectedLevel: ThresholdLevelWarning,           // Default warning is 70
			shouldAlert:   true,
			shouldExpand:  false,
			shouldCleanup: false,
		},
		{
			name:         "exactly at warning threshold",
			usagePercent: 70.0,
			thresholds: cnpgv1alpha1.ThresholdsConfig{
				Warning:   70,
				Critical:  80,
				Expansion: 85,
				Emergency: 90,
			},
			expectedLevel: ThresholdLevelWarning,
			shouldAlert:   true,
			shouldExpand:  false,
			shouldCleanup: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evaluator.EvaluateThresholds(tt.usagePercent, tt.thresholds)

			if result.Level != tt.expectedLevel {
				t.Errorf("expected level %s, got %s", tt.expectedLevel, result.Level)
			}
			if result.ShouldAlert != tt.shouldAlert {
				t.Errorf("expected shouldAlert %v, got %v", tt.shouldAlert, result.ShouldAlert)
			}
			if result.ShouldExpand != tt.shouldExpand {
				t.Errorf("expected shouldExpand %v, got %v", tt.shouldExpand, result.ShouldExpand)
			}
			if result.ShouldCleanupWAL != tt.shouldCleanup {
				t.Errorf("expected shouldCleanupWAL %v, got %v", tt.shouldCleanup, result.ShouldCleanupWAL)
			}
			if result.CurrentUsagePercent != tt.usagePercent {
				t.Errorf("expected usagePercent %f, got %f", tt.usagePercent, result.CurrentUsagePercent)
			}
		})
	}
}

func TestCalculateExpansionSize(t *testing.T) {
	evaluator := NewEvaluator()

	tests := []struct {
		name             string
		currentSizeBytes int64
		policy           *cnpgv1alpha1.StoragePolicy
		expectedSize     int64
		expectError      bool
	}{
		{
			name:             "standard 50% expansion",
			currentSizeBytes: 10 * 1024 * 1024 * 1024, // 10Gi
			policy: &cnpgv1alpha1.StoragePolicy{
				Spec: cnpgv1alpha1.StoragePolicySpec{
					Expansion: cnpgv1alpha1.ExpansionConfig{
						Percentage:     50,
						MinIncrementGi: 5,
					},
				},
			},
			expectedSize: 15 * 1024 * 1024 * 1024, // 15Gi
			expectError:  false,
		},
		{
			name:             "minimum increment applied",
			currentSizeBytes: 2 * 1024 * 1024 * 1024, // 2Gi
			policy: &cnpgv1alpha1.StoragePolicy{
				Spec: cnpgv1alpha1.StoragePolicySpec{
					Expansion: cnpgv1alpha1.ExpansionConfig{
						Percentage:     50, // Would be 1Gi
						MinIncrementGi: 5,  // But min is 5Gi
					},
				},
			},
			expectedSize: 7 * 1024 * 1024 * 1024, // 2Gi + 5Gi = 7Gi
			expectError:  false,
		},
		{
			name:             "max size limit applied",
			currentSizeBytes: 90 * 1024 * 1024 * 1024, // 90Gi
			policy: &cnpgv1alpha1.StoragePolicy{
				Spec: cnpgv1alpha1.StoragePolicySpec{
					Expansion: cnpgv1alpha1.ExpansionConfig{
						Percentage:     50, // Would expand to 135Gi
						MinIncrementGi: 5,
						MaxSize:        quantityPtr(resource.MustParse("100Gi")), // But max is 100Gi
					},
				},
			},
			expectedSize: 100 * 1024 * 1024 * 1024, // 100Gi (capped)
			expectError:  false,
		},
		{
			name:             "already at max size",
			currentSizeBytes: 100 * 1024 * 1024 * 1024, // 100Gi
			policy: &cnpgv1alpha1.StoragePolicy{
				Spec: cnpgv1alpha1.StoragePolicySpec{
					Expansion: cnpgv1alpha1.ExpansionConfig{
						Percentage:     50,
						MinIncrementGi: 5,
						MaxSize:        quantityPtr(resource.MustParse("100Gi")),
					},
				},
			},
			expectedSize: 0,
			expectError:  true,
		},
		{
			name:             "uses defaults when values are zero",
			currentSizeBytes: 100 * 1024 * 1024 * 1024, // 100Gi
			policy: &cnpgv1alpha1.StoragePolicy{
				Spec: cnpgv1alpha1.StoragePolicySpec{
					Expansion: cnpgv1alpha1.ExpansionConfig{}, // All zeros
				},
			},
			expectedSize: 150 * 1024 * 1024 * 1024, // 100Gi + 50Gi (default 50%)
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.CalculateExpansionSize(tt.currentSizeBytes, tt.policy)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.expectError && result != tt.expectedSize {
				t.Errorf("expected size %d, got %d", tt.expectedSize, result)
			}
		})
	}
}

func TestCheckCooldown(t *testing.T) {
	evaluator := NewEvaluator()

	tests := []struct {
		name            string
		lastAction      *time.Time
		cooldownMinutes int32
		expectAllowed   bool
	}{
		{
			name:            "no previous action",
			lastAction:      nil,
			cooldownMinutes: 30,
			expectAllowed:   true,
		},
		{
			name:            "cooldown expired",
			lastAction:      timePtr(time.Now().Add(-60 * time.Minute)),
			cooldownMinutes: 30,
			expectAllowed:   true,
		},
		{
			name:            "cooldown active",
			lastAction:      timePtr(time.Now().Add(-10 * time.Minute)),
			cooldownMinutes: 30,
			expectAllowed:   false,
		},
		{
			name:            "exactly at cooldown boundary",
			lastAction:      timePtr(time.Now().Add(-30 * time.Minute)),
			cooldownMinutes: 30,
			expectAllowed:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, _ := evaluator.CheckCooldown(tt.lastAction, tt.cooldownMinutes)

			if allowed != tt.expectAllowed {
				t.Errorf("expected allowed %v, got %v", tt.expectAllowed, allowed)
			}
		})
	}
}

func TestShouldSuppressAlert(t *testing.T) {
	evaluator := NewEvaluator()

	tests := []struct {
		name              string
		policy            *cnpgv1alpha1.StoragePolicy
		activeRemediation bool
		expectSuppressed  bool
	}{
		{
			name: "suppress during remediation enabled and remediation active",
			policy: &cnpgv1alpha1.StoragePolicy{
				Spec: cnpgv1alpha1.StoragePolicySpec{
					Alerting: cnpgv1alpha1.AlertingConfig{
						SuppressDuringRemediation: true,
					},
				},
			},
			activeRemediation: true,
			expectSuppressed:  true,
		},
		{
			name: "suppress during remediation enabled but no remediation",
			policy: &cnpgv1alpha1.StoragePolicy{
				Spec: cnpgv1alpha1.StoragePolicySpec{
					Alerting: cnpgv1alpha1.AlertingConfig{
						SuppressDuringRemediation: true,
					},
				},
			},
			activeRemediation: false,
			expectSuppressed:  false,
		},
		{
			name: "suppress during remediation disabled",
			policy: &cnpgv1alpha1.StoragePolicy{
				Spec: cnpgv1alpha1.StoragePolicySpec{
					Alerting: cnpgv1alpha1.AlertingConfig{
						SuppressDuringRemediation: false,
					},
				},
			},
			activeRemediation: true,
			expectSuppressed:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suppressed, _ := evaluator.ShouldSuppressAlert(tt.policy, tt.activeRemediation)

			if suppressed != tt.expectSuppressed {
				t.Errorf("expected suppressed %v, got %v", tt.expectSuppressed, suppressed)
			}
		})
	}
}

func TestGetRecommendedActions(t *testing.T) {
	evaluator := NewEvaluator()

	tests := []struct {
		name          string
		result        ThresholdResult
		policy        *cnpgv1alpha1.StoragePolicy
		expectActions []ActionType
	}{
		{
			name: "normal level - no actions",
			result: ThresholdResult{
				Level: ThresholdLevelNormal,
			},
			policy: &cnpgv1alpha1.StoragePolicy{
				Spec: cnpgv1alpha1.StoragePolicySpec{
					Expansion:  cnpgv1alpha1.ExpansionConfig{Enabled: true},
					WALCleanup: cnpgv1alpha1.WALCleanupConfig{Enabled: true},
				},
			},
			expectActions: []ActionType{},
		},
		{
			name: "warning level - alert only",
			result: ThresholdResult{
				Level:   ThresholdLevelWarning,
				Message: "Warning message",
			},
			policy: &cnpgv1alpha1.StoragePolicy{
				Spec: cnpgv1alpha1.StoragePolicySpec{
					Expansion:  cnpgv1alpha1.ExpansionConfig{Enabled: true},
					WALCleanup: cnpgv1alpha1.WALCleanupConfig{Enabled: true},
				},
			},
			expectActions: []ActionType{ActionTypeAlert},
		},
		{
			name: "expansion level - expand and alert",
			result: ThresholdResult{
				Level:   ThresholdLevelExpansion,
				Message: "Expansion message",
			},
			policy: &cnpgv1alpha1.StoragePolicy{
				Spec: cnpgv1alpha1.StoragePolicySpec{
					Expansion:  cnpgv1alpha1.ExpansionConfig{Enabled: true, Percentage: 50},
					WALCleanup: cnpgv1alpha1.WALCleanupConfig{Enabled: true},
				},
			},
			expectActions: []ActionType{ActionTypeExpand, ActionTypeAlert},
		},
		{
			name: "expansion level but expansion disabled",
			result: ThresholdResult{
				Level:   ThresholdLevelExpansion,
				Message: "Expansion message",
			},
			policy: &cnpgv1alpha1.StoragePolicy{
				Spec: cnpgv1alpha1.StoragePolicySpec{
					Expansion:  cnpgv1alpha1.ExpansionConfig{Enabled: false},
					WALCleanup: cnpgv1alpha1.WALCleanupConfig{Enabled: true},
				},
			},
			expectActions: []ActionType{ActionTypeAlert}, // Only alert, no expand
		},
		{
			name: "emergency level - WAL cleanup, expand, and alert",
			result: ThresholdResult{
				Level:   ThresholdLevelEmergency,
				Message: "Emergency message",
			},
			policy: &cnpgv1alpha1.StoragePolicy{
				Spec: cnpgv1alpha1.StoragePolicySpec{
					Expansion:  cnpgv1alpha1.ExpansionConfig{Enabled: true, Percentage: 50},
					WALCleanup: cnpgv1alpha1.WALCleanupConfig{Enabled: true},
				},
			},
			expectActions: []ActionType{ActionTypeWALCleanup, ActionTypeExpand, ActionTypeAlert},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actions := evaluator.GetRecommendedActions(tt.result, tt.policy)

			if len(actions) != len(tt.expectActions) {
				t.Errorf("expected %d actions, got %d", len(tt.expectActions), len(actions))
				return
			}

			for i, expectedAction := range tt.expectActions {
				if actions[i].Action != expectedAction {
					t.Errorf("expected action[%d] to be %s, got %s", i, expectedAction, actions[i].Action)
				}
			}
		})
	}
}

func TestFullEvaluation(t *testing.T) {
	evaluator := NewEvaluator()

	tests := []struct {
		name          string
		ctx           EvaluationContext
		policy        *cnpgv1alpha1.StoragePolicy
		expectBlocked bool
		expectLevel   ThresholdLevel
	}{
		{
			name: "normal evaluation",
			ctx: EvaluationContext{
				ClusterName:       "test-cluster",
				Namespace:         "default",
				CurrentUsageBytes: 50 * 1024 * 1024 * 1024,  // 50Gi
				CapacityBytes:     100 * 1024 * 1024 * 1024, // 100Gi (50% usage)
			},
			policy: &cnpgv1alpha1.StoragePolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "test-policy"},
				Spec: cnpgv1alpha1.StoragePolicySpec{
					Thresholds: cnpgv1alpha1.ThresholdsConfig{
						Warning: 70,
					},
				},
			},
			expectBlocked: false,
			expectLevel:   ThresholdLevelNormal,
		},
		{
			name: "circuit breaker blocks evaluation",
			ctx: EvaluationContext{
				ClusterName:        "test-cluster",
				Namespace:          "default",
				CurrentUsageBytes:  90 * 1024 * 1024 * 1024,
				CapacityBytes:      100 * 1024 * 1024 * 1024,
				CircuitBreakerOpen: true,
			},
			policy: &cnpgv1alpha1.StoragePolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "test-policy"},
				Spec: cnpgv1alpha1.StoragePolicySpec{
					Thresholds: cnpgv1alpha1.ThresholdsConfig{
						Warning: 70,
					},
				},
			},
			expectBlocked: true,
			expectLevel:   ThresholdLevelNormal, // Won't evaluate because blocked
		},
		{
			name: "zero capacity returns error",
			ctx: EvaluationContext{
				ClusterName:       "test-cluster",
				Namespace:         "default",
				CurrentUsageBytes: 50 * 1024 * 1024 * 1024,
				CapacityBytes:     0, // Zero capacity
			},
			policy: &cnpgv1alpha1.StoragePolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "test-policy"},
			},
			expectBlocked: false,
			expectLevel:   ThresholdLevelNormal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.FullEvaluation(tt.ctx, tt.policy)

			if tt.ctx.CapacityBytes == 0 {
				if err == nil {
					t.Error("expected error for zero capacity")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result.Blocked != tt.expectBlocked {
				t.Errorf("expected blocked %v, got %v", tt.expectBlocked, result.Blocked)
			}

			if !tt.expectBlocked && result.ThresholdResult.Level != tt.expectLevel {
				t.Errorf("expected level %s, got %s", tt.expectLevel, result.ThresholdResult.Level)
			}
		})
	}
}

func TestEvaluationResultHasPendingActions(t *testing.T) {
	tests := []struct {
		name     string
		result   *EvaluationResult
		expected bool
	}{
		{
			name: "no actions",
			result: &EvaluationResult{
				Actions: []ActionRecommendation{},
			},
			expected: false,
		},
		{
			name: "has non-blocked action",
			result: &EvaluationResult{
				Actions: []ActionRecommendation{
					{Action: ActionTypeAlert, Parameters: map[string]interface{}{}},
				},
			},
			expected: true,
		},
		{
			name: "all actions blocked",
			result: &EvaluationResult{
				Actions: []ActionRecommendation{
					{Action: ActionTypeExpand, Parameters: map[string]interface{}{"blocked": true}},
				},
			},
			expected: false,
		},
		{
			name: "ActionTypeNone is not pending",
			result: &EvaluationResult{
				Actions: []ActionRecommendation{
					{Action: ActionTypeNone, Parameters: map[string]interface{}{}},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.HasPendingActions(); got != tt.expected {
				t.Errorf("HasPendingActions() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestEvaluationResultGetHighestPriorityAction(t *testing.T) {
	tests := []struct {
		name           string
		result         *EvaluationResult
		expectedAction *ActionType
	}{
		{
			name: "no actions",
			result: &EvaluationResult{
				Actions: []ActionRecommendation{},
			},
			expectedAction: nil,
		},
		{
			name: "single action",
			result: &EvaluationResult{
				Actions: []ActionRecommendation{
					{Action: ActionTypeAlert, Priority: 0, Parameters: map[string]interface{}{}},
				},
			},
			expectedAction: actionTypePtr(ActionTypeAlert),
		},
		{
			name: "highest priority wins",
			result: &EvaluationResult{
				Actions: []ActionRecommendation{
					{Action: ActionTypeAlert, Priority: 2, Parameters: map[string]interface{}{}},
					{Action: ActionTypeWALCleanup, Priority: 1, Parameters: map[string]interface{}{}},
					{Action: ActionTypeExpand, Priority: 0, Parameters: map[string]interface{}{}},
				},
			},
			expectedAction: actionTypePtr(ActionTypeExpand), // Priority 0 is highest
		},
		{
			name: "blocked actions skipped",
			result: &EvaluationResult{
				Actions: []ActionRecommendation{
					{Action: ActionTypeExpand, Priority: 0, Parameters: map[string]interface{}{"blocked": true}},
					{Action: ActionTypeAlert, Priority: 1, Parameters: map[string]interface{}{}},
				},
			},
			expectedAction: actionTypePtr(ActionTypeAlert),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := tt.result.GetHighestPriorityAction()

			if tt.expectedAction == nil {
				if action != nil {
					t.Errorf("expected nil action, got %v", action.Action)
				}
				return
			}

			if action == nil {
				t.Error("expected action but got nil")
				return
			}

			if action.Action != *tt.expectedAction {
				t.Errorf("expected action %s, got %s", *tt.expectedAction, action.Action)
			}
		})
	}
}

// Helper functions
func quantityPtr(q resource.Quantity) *resource.Quantity {
	return &q
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func actionTypePtr(a ActionType) *ActionType {
	return &a
}

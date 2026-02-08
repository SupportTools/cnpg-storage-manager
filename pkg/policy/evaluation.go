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
	"fmt"
	"time"

	cnpgv1alpha1 "github.com/supporttools/cnpg-storage-manager/api/v1alpha1"
)

// ThresholdLevel represents a threshold level
type ThresholdLevel string

const (
	// ThresholdLevelNormal indicates usage is below warning threshold
	ThresholdLevelNormal ThresholdLevel = "normal"
	// ThresholdLevelWarning indicates usage is at warning level
	ThresholdLevelWarning ThresholdLevel = "warning"
	// ThresholdLevelCritical indicates usage is at critical level
	ThresholdLevelCritical ThresholdLevel = "critical"
	// ThresholdLevelExpansion indicates usage requires expansion
	ThresholdLevelExpansion ThresholdLevel = "expansion"
	// ThresholdLevelEmergency indicates usage requires emergency action
	ThresholdLevelEmergency ThresholdLevel = "emergency"
)

// ThresholdResult contains the result of threshold evaluation
type ThresholdResult struct {
	// CurrentUsagePercent is the current storage usage percentage
	CurrentUsagePercent float64
	// Level is the highest breached threshold level
	Level ThresholdLevel
	// ShouldAlert indicates if an alert should be sent
	ShouldAlert bool
	// ShouldExpand indicates if PVC expansion should be triggered
	ShouldExpand bool
	// ShouldCleanupWAL indicates if WAL cleanup should be triggered
	ShouldCleanupWAL bool
	// Message provides a human-readable description
	Message string
}

// ActionRecommendation contains recommended actions based on evaluation
type ActionRecommendation struct {
	// Action is the recommended action type
	Action ActionType
	// Reason explains why this action is recommended
	Reason string
	// Priority indicates the priority of this action
	Priority int
	// Parameters contains action-specific parameters
	Parameters map[string]interface{}
}

// ActionType defines the type of action to take
type ActionType string

const (
	// ActionTypeNone indicates no action is needed
	ActionTypeNone ActionType = "none"
	// ActionTypeAlert indicates an alert should be sent
	ActionTypeAlert ActionType = "alert"
	// ActionTypeExpand indicates PVC expansion
	ActionTypeExpand ActionType = "expand"
	// ActionTypeWALCleanup indicates WAL cleanup
	ActionTypeWALCleanup ActionType = "wal-cleanup"
)

// Evaluator evaluates storage metrics against policy thresholds
type Evaluator struct {
	// HysteresisPercent is the percentage below threshold before clearing alerts
	HysteresisPercent float64
}

// NewEvaluator creates a new threshold evaluator
func NewEvaluator() *Evaluator {
	return &Evaluator{
		HysteresisPercent: 2.0, // 2% hysteresis
	}
}

// EvaluateThresholds evaluates current usage against policy thresholds
func (e *Evaluator) EvaluateThresholds(usagePercent float64, thresholds cnpgv1alpha1.ThresholdsConfig) ThresholdResult {
	result := ThresholdResult{
		CurrentUsagePercent: usagePercent,
		Level:               ThresholdLevelNormal,
	}

	// Get threshold values with defaults
	warningThreshold := getThresholdOrDefault(thresholds.Warning, 70)
	criticalThreshold := getThresholdOrDefault(thresholds.Critical, 80)
	expansionThreshold := getThresholdOrDefault(thresholds.Expansion, 85)
	emergencyThreshold := getThresholdOrDefault(thresholds.Emergency, 90)

	// Evaluate thresholds from highest to lowest
	if usagePercent >= float64(emergencyThreshold) {
		result.Level = ThresholdLevelEmergency
		result.ShouldAlert = true
		result.ShouldExpand = true
		result.ShouldCleanupWAL = true
		result.Message = fmt.Sprintf(
			"Emergency: storage usage %.1f%% exceeds emergency threshold %d%%",
			usagePercent,
			emergencyThreshold,
		)
	} else if usagePercent >= float64(expansionThreshold) {
		result.Level = ThresholdLevelExpansion
		result.ShouldAlert = true
		result.ShouldExpand = true
		result.Message = fmt.Sprintf(
			"Expansion required: storage usage %.1f%% exceeds expansion threshold %d%%",
			usagePercent,
			expansionThreshold,
		)
	} else if usagePercent >= float64(criticalThreshold) {
		result.Level = ThresholdLevelCritical
		result.ShouldAlert = true
		result.Message = fmt.Sprintf(
			"Critical: storage usage %.1f%% exceeds critical threshold %d%%",
			usagePercent,
			criticalThreshold,
		)
	} else if usagePercent >= float64(warningThreshold) {
		result.Level = ThresholdLevelWarning
		result.ShouldAlert = true
		result.Message = fmt.Sprintf(
			"Warning: storage usage %.1f%% exceeds warning threshold %d%%",
			usagePercent,
			warningThreshold,
		)
	} else {
		result.Message = fmt.Sprintf("Normal: storage usage %.1f%% is within acceptable limits", usagePercent)
	}

	return result
}

// GetRecommendedActions returns a list of recommended actions based on evaluation
func (e *Evaluator) GetRecommendedActions(
	result ThresholdResult,
	policy *cnpgv1alpha1.StoragePolicy,
) []ActionRecommendation {
	var actions []ActionRecommendation

	switch result.Level {
	case ThresholdLevelEmergency:
		// WAL cleanup first (faster), then expansion
		if policy.Spec.WALCleanup.Enabled {
			actions = append(actions, ActionRecommendation{
				Action:   ActionTypeWALCleanup,
				Reason:   "Emergency threshold breached - attempting WAL cleanup first",
				Priority: 1,
			})
		}
		if policy.Spec.Expansion.Enabled {
			actions = append(actions, ActionRecommendation{
				Action:   ActionTypeExpand,
				Reason:   "Emergency threshold breached - expansion required",
				Priority: 2,
				Parameters: map[string]interface{}{
					"percentage": policy.Spec.Expansion.Percentage,
				},
			})
		}
		actions = append(actions, ActionRecommendation{
			Action:   ActionTypeAlert,
			Reason:   result.Message,
			Priority: 0,
			Parameters: map[string]interface{}{
				"severity": "critical",
			},
		})

	case ThresholdLevelExpansion:
		if policy.Spec.Expansion.Enabled {
			actions = append(actions, ActionRecommendation{
				Action:   ActionTypeExpand,
				Reason:   "Expansion threshold breached",
				Priority: 1,
				Parameters: map[string]interface{}{
					"percentage": policy.Spec.Expansion.Percentage,
				},
			})
		}
		actions = append(actions, ActionRecommendation{
			Action:   ActionTypeAlert,
			Reason:   result.Message,
			Priority: 0,
			Parameters: map[string]interface{}{
				"severity": "warning",
			},
		})

	case ThresholdLevelCritical:
		actions = append(actions, ActionRecommendation{
			Action:   ActionTypeAlert,
			Reason:   result.Message,
			Priority: 0,
			Parameters: map[string]interface{}{
				"severity": "critical",
			},
		})

	case ThresholdLevelWarning:
		actions = append(actions, ActionRecommendation{
			Action:   ActionTypeAlert,
			Reason:   result.Message,
			Priority: 0,
			Parameters: map[string]interface{}{
				"severity": "warning",
			},
		})

	case ThresholdLevelNormal:
		// No action needed
	}

	return actions
}

// CalculateExpansionSize calculates the new PVC size based on policy
func (e *Evaluator) CalculateExpansionSize(currentSizeBytes int64, policy *cnpgv1alpha1.StoragePolicy) (int64, error) {
	config := policy.Spec.Expansion

	// Calculate percentage increase
	percentage := getThresholdOrDefault(config.Percentage, 50)
	increaseBytes := currentSizeBytes * int64(percentage) / 100

	// Apply minimum increment
	minIncrement := int64(getThresholdOrDefault(config.MinIncrementGi, 5)) * 1024 * 1024 * 1024 // Convert Gi to bytes
	if increaseBytes < minIncrement {
		increaseBytes = minIncrement
	}

	newSize := currentSizeBytes + increaseBytes

	// Check max size limit
	if config.MaxSize != nil {
		maxBytes := config.MaxSize.Value()
		if newSize > maxBytes {
			if currentSizeBytes >= maxBytes {
				return 0, fmt.Errorf("current size %d bytes already at or exceeds max size %d bytes", currentSizeBytes, maxBytes)
			}
			newSize = maxBytes
		}
	}

	return newSize, nil
}

// ShouldSuppressAlert determines if an alert should be suppressed
func (e *Evaluator) ShouldSuppressAlert(policy *cnpgv1alpha1.StoragePolicy, activeRemediation bool) (bool, string) {
	if policy.Spec.Alerting.SuppressDuringRemediation && activeRemediation {
		return true, "remediation in progress"
	}
	return false, ""
}

// CheckCooldown checks if an action is allowed based on cooldown
func (e *Evaluator) CheckCooldown(lastAction *time.Time, cooldownMinutes int32) (bool, time.Duration) {
	if lastAction == nil {
		return true, 0
	}

	cooldown := time.Duration(cooldownMinutes) * time.Minute
	nextAllowed := lastAction.Add(cooldown)
	now := time.Now()

	if now.Before(nextAllowed) {
		return false, nextAllowed.Sub(now)
	}

	return true, 0
}

// getThresholdOrDefault returns the threshold value or a default if zero
func getThresholdOrDefault(value, defaultValue int32) int32 {
	if value == 0 {
		return defaultValue
	}
	return value
}

// EvaluationContext contains context for a complete evaluation
type EvaluationContext struct {
	ClusterName        string
	Namespace          string
	CurrentUsageBytes  int64
	CapacityBytes      int64
	WALSizeBytes       int64
	LastExpansion      *time.Time
	LastWALCleanup     *time.Time
	ActiveRemediation  bool
	CircuitBreakerOpen bool
}

// FullEvaluation performs a complete evaluation with all checks
func (e *Evaluator) FullEvaluation(
	ctx EvaluationContext,
	policy *cnpgv1alpha1.StoragePolicy,
) (*EvaluationResult, error) {
	result := &EvaluationResult{
		ClusterName:   ctx.ClusterName,
		Namespace:     ctx.Namespace,
		EvaluatedAt:   time.Now(),
		Actions:       []ActionRecommendation{},
		Blocked:       false,
		BlockedReason: "",
	}

	// Calculate usage percentage
	if ctx.CapacityBytes == 0 {
		return result, fmt.Errorf("capacity is zero")
	}
	usagePercent := float64(ctx.CurrentUsageBytes) / float64(ctx.CapacityBytes) * 100
	result.UsagePercent = usagePercent

	// Check circuit breaker
	if ctx.CircuitBreakerOpen {
		result.Blocked = true
		result.BlockedReason = "circuit breaker is open"
		return result, nil
	}

	// Evaluate thresholds
	thresholdResult := e.EvaluateThresholds(usagePercent, policy.Spec.Thresholds)
	result.ThresholdResult = thresholdResult

	// Get recommended actions
	actions := e.GetRecommendedActions(thresholdResult, policy)

	// Check cooldowns and filter actions
	for _, action := range actions {
		switch action.Action {
		case ActionTypeExpand:
			if allowed, remaining := e.CheckCooldown(ctx.LastExpansion, policy.Spec.Expansion.CooldownMinutes); !allowed {
				action.Reason = fmt.Sprintf("%s (blocked: cooldown %v remaining)", action.Reason, remaining.Round(time.Second))
				action.Parameters["blocked"] = true
				action.Parameters["cooldown_remaining"] = remaining.Seconds()
			}
		case ActionTypeWALCleanup:
			if allowed, remaining := e.CheckCooldown(ctx.LastWALCleanup, policy.Spec.WALCleanup.CooldownMinutes); !allowed {
				action.Reason = fmt.Sprintf("%s (blocked: cooldown %v remaining)", action.Reason, remaining.Round(time.Second))
				action.Parameters["blocked"] = true
				action.Parameters["cooldown_remaining"] = remaining.Seconds()
			}
		case ActionTypeAlert:
			if suppress, reason := e.ShouldSuppressAlert(policy, ctx.ActiveRemediation); suppress {
				action.Reason = fmt.Sprintf("%s (suppressed: %s)", action.Reason, reason)
				action.Parameters["suppressed"] = true
				action.Parameters["suppress_reason"] = reason
			}
		}
		result.Actions = append(result.Actions, action)
	}

	return result, nil
}

// EvaluationResult contains the complete result of an evaluation
type EvaluationResult struct {
	ClusterName     string
	Namespace       string
	EvaluatedAt     time.Time
	UsagePercent    float64
	ThresholdResult ThresholdResult
	Actions         []ActionRecommendation
	Blocked         bool
	BlockedReason   string
}

// HasPendingActions returns true if there are non-blocked actions
func (r *EvaluationResult) HasPendingActions() bool {
	for _, action := range r.Actions {
		if action.Action != ActionTypeNone {
			if blocked, ok := action.Parameters["blocked"].(bool); !ok || !blocked {
				return true
			}
		}
	}
	return false
}

// GetHighestPriorityAction returns the highest priority non-blocked action
func (r *EvaluationResult) GetHighestPriorityAction() *ActionRecommendation {
	var highest *ActionRecommendation
	for i := range r.Actions {
		action := &r.Actions[i]
		if action.Action == ActionTypeNone {
			continue
		}
		if blocked, ok := action.Parameters["blocked"].(bool); ok && blocked {
			continue
		}
		if highest == nil || action.Priority < highest.Priority {
			highest = action
		}
	}
	return highest
}

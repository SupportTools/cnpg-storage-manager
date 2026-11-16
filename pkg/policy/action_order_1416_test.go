package policy

// cluster-services-1416: the alert recommendation used to carry Priority 0, which outranked
// expand (1/2) and WAL cleanup (1), so the controller only ever alerted and auto-remediation
// never ran. Remediating actions must win; the alert is sent alongside or as the fallback.

import (
	"testing"
	"time"

	cnpgv1alpha1 "github.com/supporttools/cnpg-storage-manager/api/v1alpha1"
)

func remediatingPolicy() *cnpgv1alpha1.StoragePolicy {
	p := &cnpgv1alpha1.StoragePolicy{}
	p.Spec.Thresholds = cnpgv1alpha1.ThresholdsConfig{Warning: 70, Critical: 80, Expansion: 85, Emergency: 90}
	p.Spec.Expansion.Enabled = true
	p.Spec.Expansion.Percentage = 50
	p.Spec.Expansion.CooldownMinutes = 60
	p.Spec.WALCleanup.Enabled = true
	p.Spec.WALCleanup.CooldownMinutes = 30
	return p
}

func evaluate(t *testing.T, usedPercent int64, p *cnpgv1alpha1.StoragePolicy, lastExpand, lastWAL *time.Time) *EvaluationResult {
	t.Helper()
	res, err := NewEvaluator().FullEvaluation(EvaluationContext{
		ClusterName: "c", Namespace: "n",
		CurrentUsageBytes: usedPercent, CapacityBytes: 100,
		LastExpansion: lastExpand, LastWALCleanup: lastWAL,
	}, p)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestHighestPriorityAction_RemediationOutranksAlert(t *testing.T) {
	p := remediatingPolicy()
	if got := evaluate(t, 95, p, nil, nil).GetHighestPriorityAction(); got == nil || got.Action != ActionTypeWALCleanup {
		t.Errorf("emergency: highest = %+v, want wal-cleanup", got)
	}
	if got := evaluate(t, 87, p, nil, nil).GetHighestPriorityAction(); got == nil || got.Action != ActionTypeExpand {
		t.Errorf("expansion level: highest = %+v, want expand", got)
	}
	// Below the expansion threshold there is nothing to remediate: the alert is all there is.
	if got := evaluate(t, 82, p, nil, nil).GetHighestPriorityAction(); got == nil || got.Action != ActionTypeAlert {
		t.Errorf("critical: highest = %+v, want alert", got)
	}
}

func TestPlanActions(t *testing.T) {
	recent := time.Now().Add(-time.Minute) // inside every cooldown
	disabled := remediatingPolicy()
	disabled.Spec.Expansion.Enabled = false
	disabled.Spec.WALCleanup.Enabled = false

	tests := []struct {
		name              string
		used              int64
		policy            *cnpgv1alpha1.StoragePolicy
		lastExpand        *time.Time
		lastWAL           *time.Time
		expansionPossible bool
		wantRemediation   ActionType // "" = none
		wantSkipped       bool
		wantAlertSeverity string // "" = no alert
	}{
		{name: "emergency: WAL cleanup first, alert too", used: 95, policy: remediatingPolicy(),
			expansionPossible: true, wantRemediation: ActionTypeWALCleanup, wantAlertSeverity: "critical"},
		{name: "emergency, WAL cleanup in cooldown: expand", used: 95, policy: remediatingPolicy(), lastWAL: &recent,
			expansionPossible: true, wantRemediation: ActionTypeExpand, wantAlertSeverity: "critical"},
		{name: "emergency, both in cooldown: alert only", used: 95, policy: remediatingPolicy(),
			lastWAL: &recent, lastExpand: &recent, expansionPossible: true, wantAlertSeverity: "critical"},
		{name: "emergency, expansion unsupported: WAL cleanup, expansion skipped", used: 95, policy: remediatingPolicy(),
			expansionPossible: false, wantRemediation: ActionTypeWALCleanup, wantSkipped: true, wantAlertSeverity: "critical"},
		{name: "emergency, WAL in cooldown, expansion unsupported: alert is the fallback", used: 95,
			policy: remediatingPolicy(), lastWAL: &recent, expansionPossible: false, wantSkipped: true,
			wantAlertSeverity: "critical"},
		{name: "expansion level: expand, alert too", used: 87, policy: remediatingPolicy(),
			expansionPossible: true, wantRemediation: ActionTypeExpand, wantAlertSeverity: "critical"},
		{name: "expansion level, expansion unsupported: alert is the fallback", used: 87, policy: remediatingPolicy(),
			expansionPossible: false, wantSkipped: true, wantAlertSeverity: "critical"},
		{name: "expansion level, expansion in cooldown: not reported as skipped", used: 87, policy: remediatingPolicy(),
			lastExpand: &recent, expansionPossible: false, wantAlertSeverity: "critical"},
		{name: "remediation disabled: alert only", used: 95, policy: disabled,
			expansionPossible: true, wantAlertSeverity: "critical"},
		{name: "critical: alert only", used: 82, policy: remediatingPolicy(),
			expansionPossible: true, wantAlertSeverity: "critical"},
		{name: "normal: nothing", used: 10, policy: remediatingPolicy(), expansionPossible: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := evaluate(t, tt.used, tt.policy, tt.lastExpand, tt.lastWAL).PlanActions(tt.expansionPossible)
			var got ActionType
			if plan.Remediation != nil {
				got = plan.Remediation.Action
			}
			if got != tt.wantRemediation {
				t.Errorf("remediation = %q, want %q", got, tt.wantRemediation)
			}
			if plan.ExpansionSkipped != tt.wantSkipped {
				t.Errorf("ExpansionSkipped = %v, want %v", plan.ExpansionSkipped, tt.wantSkipped)
			}
			switch {
			case tt.wantAlertSeverity == "" && plan.Alert != nil:
				t.Errorf("unexpected alert %+v", plan.Alert)
			case tt.wantAlertSeverity != "" && plan.Alert == nil:
				t.Errorf("no alert, want severity %s", tt.wantAlertSeverity)
			case plan.Alert != nil && plan.Alert.Parameters[paramSeverity] != tt.wantAlertSeverity:
				t.Errorf("alert severity = %v, want %s", plan.Alert.Parameters[paramSeverity], tt.wantAlertSeverity)
			}
		})
	}
}

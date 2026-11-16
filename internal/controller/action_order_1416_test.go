package controller

// cluster-services-1416: the controller acted only on the single highest-priority action, and the
// alert outranked expand and WAL cleanup, so it only ever alerted. applyActions must take the
// remediation (or log what it WOULD do in dry-run) and still send the alert.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	cnpgv1alpha1 "github.com/supporttools/cnpg-storage-manager/api/v1alpha1"
	"github.com/supporttools/cnpg-storage-manager/pkg/alerting"
	"github.com/supporttools/cnpg-storage-manager/pkg/cnpg"
	"github.com/supporttools/cnpg-storage-manager/pkg/policy"
	"github.com/supporttools/cnpg-storage-manager/pkg/remediation"
)

func TestApplyActions_Order(t *testing.T) {
	unsupported := &remediation.ExpansionSupport{Supported: false, Reason: `storage class "local-path-cnpg" does not support volume expansion`}
	supported := &remediation.ExpansionSupport{Supported: true}

	tests := []struct {
		name       string
		usedPct    int64
		dryRun     bool
		suppress   bool
		expansion  *remediation.ExpansionSupport
		wantStatus string
		wantAlerts int32
	}{
		{name: "dry-run emergency: would clean WAL, alerts", usedPct: 95, dryRun: true, expansion: supported,
			wantStatus: "DryRun-WouldCleanupWAL", wantAlerts: 1},
		{name: "dry-run expansion level: would expand, alerts", usedPct: 87, dryRun: true, expansion: supported,
			wantStatus: "DryRun-WouldExpand", wantAlerts: 1},
		{name: "expansion level, unknown support: would expand", usedPct: 87, dryRun: true, expansion: nil,
			wantStatus: "DryRun-WouldExpand", wantAlerts: 1},
		{name: "expansion level, storage cannot expand: skipped, alert is the fallback", usedPct: 87, dryRun: true,
			expansion: unsupported, wantStatus: "ExpansionUnsupported", wantAlerts: 1},
		{name: "same, not dry-run: still never expands", usedPct: 87, expansion: unsupported,
			wantStatus: "ExpansionUnsupported", wantAlerts: 1},
		{name: "critical: alert only", usedPct: 82, dryRun: true, expansion: supported,
			wantStatus: "Alert-critical", wantAlerts: 1},
		{name: "normal: nothing", usedPct: 10, dryRun: true, expansion: supported,
			wantStatus: "Healthy", wantAlerts: 0},
		// handleWALCleanup with no engine wired is a successful no-op, which counts as a remediation
		// that ran: suppressDuringRemediation then holds the alert back, and only then.
		{name: "remediation ran, suppressDuringRemediation: alert held back", usedPct: 95, suppress: true,
			expansion: supported, wantStatus: "WALCleanup", wantAlerts: 0},
		{name: "remediation ran, no suppression: alert sent alongside", usedPct: 95,
			expansion: supported, wantStatus: "WALCleanup", wantAlerts: 1},
		{name: "dry-run with suppressDuringRemediation: nothing ran, so the alert is sent", usedPct: 95, dryRun: true,
			suppress: true, expansion: supported, wantStatus: "DryRun-WouldCleanupWAL", wantAlerts: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sent atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				sent.Add(1)
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			p := &cnpgv1alpha1.StoragePolicy{}
			p.Name, p.Namespace = "pol", "ns"
			p.Spec.DryRun = tt.dryRun
			p.Spec.Thresholds = cnpgv1alpha1.ThresholdsConfig{Warning: 70, Critical: 80, Expansion: 85, Emergency: 90}
			p.Spec.Expansion.Enabled = true
			p.Spec.WALCleanup.Enabled = true
			p.Spec.Alerting.SuppressDuringRemediation = tt.suppress
			p.Spec.Alerting.Channels = []cnpgv1alpha1.AlertChannel{
				{Type: cnpgv1alpha1.AlertChannelTypeAlertmanager, Endpoint: srv.URL},
			}

			res, err := policy.NewEvaluator().FullEvaluation(policy.EvaluationContext{
				ClusterName: "c", Namespace: "n", CurrentUsageBytes: tt.usedPct, CapacityBytes: 100,
			}, p)
			if err != nil {
				t.Fatal(err)
			}
			r := &StoragePolicyReconciler{alertManagers: map[string]*alerting.AlertManager{}}
			ca := &clusterAnnotationsWrapper{annotations: map[string]string{}}

			got := r.applyActions(context.Background(), p, cnpg.ClusterInfo{Name: "c", Namespace: "n"}, res, tt.expansion, ca)
			if got != tt.wantStatus {
				t.Errorf("status = %q, want %q", got, tt.wantStatus)
			}
			if n := sent.Load(); n != tt.wantAlerts {
				t.Errorf("alerts sent = %d, want %d", n, tt.wantAlerts)
			}
		})
	}
}

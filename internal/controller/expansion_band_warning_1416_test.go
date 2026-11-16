package controller

// cluster-services-1416 operator decision: the expansion band (expansion..emergency, 85-90% by
// default) is remediated by the manager itself, so its alert must be a warning, never a page.
// Critical and emergency stay paging-grade.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	cnpgv1alpha1 "github.com/supporttools/cnpg-storage-manager/api/v1alpha1"
	"github.com/supporttools/cnpg-storage-manager/pkg/alerting"
	"github.com/supporttools/cnpg-storage-manager/pkg/cnpg"
	"github.com/supporttools/cnpg-storage-manager/pkg/policy"
)

func TestHandleAlert_SeverityByLevel(t *testing.T) {
	tests := []struct {
		level policy.ThresholdLevel
		want  string // "" = no alert sent
	}{
		{policy.ThresholdLevelNormal, ""},
		{policy.ThresholdLevelWarning, "warning"},
		{policy.ThresholdLevelCritical, "critical"},
		{policy.ThresholdLevelExpansion, "warning"},
		{policy.ThresholdLevelEmergency, "emergency"},
	}
	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			var body []byte
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				body, _ = io.ReadAll(req.Body)
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			p := &cnpgv1alpha1.StoragePolicy{}
			p.Name, p.Namespace = "pol", "ns"
			p.Spec.Alerting.Channels = []cnpgv1alpha1.AlertChannel{
				{Type: cnpgv1alpha1.AlertChannelTypeAlertmanager, Endpoint: srv.URL},
			}
			r := &StoragePolicyReconciler{alertManagers: map[string]*alerting.AlertManager{}}
			res := &policy.EvaluationResult{ThresholdResult: policy.ThresholdResult{
				Level: tt.level, CurrentUsagePercent: 87, Message: "usage 87%"}}
			if err := r.handleAlert(context.Background(), p, cnpg.ClusterInfo{Name: "c", Namespace: "n"}, res, nil); err != nil {
				t.Fatal(err)
			}
			if tt.want == "" {
				if body != nil {
					t.Fatalf("no alert expected, got %s", body)
				}
				return
			}
			var alerts []struct {
				Labels map[string]string `json:"labels"`
			}
			if err := json.Unmarshal(body, &alerts); err != nil || len(alerts) != 1 {
				t.Fatalf("payload %s: %v", body, err)
			}
			if got := alerts[0].Labels["severity"]; got != tt.want {
				t.Errorf("severity label = %q, want %q", got, tt.want)
			}
		})
	}
}

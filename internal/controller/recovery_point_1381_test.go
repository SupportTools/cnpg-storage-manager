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

package controller

// Regression tests for cluster-services-1381: the recovery-point check compared the
// FIRST (oldest) recoverability point, i.e. the retention window, and so fired for
// every healthy cluster. It now uses the latest recoverable point.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr/funcr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	cnpgv1alpha1 "github.com/supporttools/cnpg-storage-manager/api/v1alpha1"
	"github.com/supporttools/cnpg-storage-manager/pkg/alerting"
	"github.com/supporttools/cnpg-storage-manager/pkg/cnpg"
	"github.com/supporttools/cnpg-storage-manager/pkg/policy"
)

type fakeProber struct {
	t     *time.Time
	err   error
	calls int
	pod   string
}

func (f *fakeProber) LastArchivedWALTime(_ context.Context, _, podName string) (*time.Time, error) {
	f.calls++
	f.pod = podName
	return f.t, f.err
}

func ago(now time.Time, d time.Duration) *time.Time {
	t := now.Add(-d)
	return &t
}

func TestCheckLatestRecoveryPoint(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name        string
		backup, wal *time.Time
		max         int32
		wantPoint   *time.Time
		wantTooOld  bool
	}{
		{"nothing known", nil, nil, 168, nil, false},
		{"recent WAL beats old backup", ago(now, 200*time.Hour), ago(now, time.Minute), 168, ago(now, time.Minute), false},
		{"backup newer than WAL", ago(now, 2*time.Hour), ago(now, 5*time.Hour), 24, ago(now, 2*time.Hour), false},
		{"backup only, fresh", ago(now, 10*time.Hour), nil, 168, ago(now, 10*time.Hour), false},
		{"both stale", ago(now, 200*time.Hour), ago(now, 190*time.Hour), 168, ago(now, 190*time.Hour), true},
		{"stale but check disabled", ago(now, 200*time.Hour), nil, 0, ago(now, 200*time.Hour), false},
	}
	for _, tc := range cases {
		point, _, tooOld := checkLatestRecoveryPoint(now, tc.backup, tc.wal, tc.max)
		if (point == nil) != (tc.wantPoint == nil) || (point != nil && !point.Equal(*tc.wantPoint)) {
			t.Errorf("%s: point = %v, want %v", tc.name, point, tc.wantPoint)
		}
		if tooOld != tc.wantTooOld {
			t.Errorf("%s: tooOld = %v, want %v", tc.name, tooOld, tc.wantTooOld)
		}
	}
}

func backupPolicy(maxRecoveryHours int32) *cnpgv1alpha1.StoragePolicy {
	p := &cnpgv1alpha1.StoragePolicy{}
	p.Name = "test"
	p.Spec.BackupMonitoring = cnpgv1alpha1.BackupMonitoringConfig{
		Enabled:                    true,
		MaxBackupAgeHours:          24,
		MaxRecoveryPointAgeHours:   maxRecoveryHours,
		RequireContinuousArchiving: true,
	}
	return p // no alert channels: evaluation only, nothing is sent
}

// The live 1370 shape: retention ~90 days, so the first recoverability point is
// 2174h old, while the last backup is hours old and WAL archived seconds ago. Healthy.
func TestEvaluateBackupStatus_OldFirstRecoverabilityPointIsHealthy(t *testing.T) {
	now := time.Now()
	prober := &fakeProber{t: ago(now, 30*time.Second)}
	r := &StoragePolicyReconciler{walArchiveProber: prober}
	cluster := cnpg.ClusterInfo{Name: "calcom-db", Namespace: "calendar"}
	cluster.Status.BackupConfigured = true
	cluster.Status.ContinuousArchivingWorking = true
	cluster.Status.CurrentPrimary = "calcom-db-2"
	cluster.Status.FirstRecoverabilityPoint = ago(now, 2174*time.Hour)
	cluster.Status.LastSuccessfulBackup = ago(now, 9*time.Hour)

	st := r.evaluateBackupStatus(context.Background(), backupPolicy(168), cluster)
	if st.BackupHealthStatus != "Healthy" {
		t.Fatalf("status = %q, want Healthy (first recoverability point age is retention, not staleness)", st.BackupHealthStatus)
	}
	if prober.pod != "calcom-db-2" {
		t.Errorf("probed pod %q, want the current primary", prober.pod)
	}
	if st.LastArchivedWALTime == nil || st.LastRecoverablePoint == nil ||
		!st.LastRecoverablePoint.Time.Equal(st.LastArchivedWALTime.Time) {
		t.Errorf("latest recoverable point should be the archived WAL: %+v", st)
	}
	if st.FirstRecoverabilityPoint == nil {
		t.Error("first recoverability point should still be reported")
	}
}

// Archiving stopped 8 days ago and the last backup is older still: unhealthy.
func TestEvaluateBackupStatus_StaleLatestPointAlerts(t *testing.T) {
	now := time.Now()
	r := &StoragePolicyReconciler{walArchiveProber: &fakeProber{t: ago(now, 192*time.Hour)}}
	cluster := cnpg.ClusterInfo{Name: "x", Namespace: "y"}
	cluster.Status.BackupConfigured = true
	cluster.Status.ContinuousArchivingWorking = true
	cluster.Status.CurrentPrimary = "x-1"
	cluster.Status.FirstRecoverabilityPoint = ago(now, 2000*time.Hour)
	cluster.Status.LastSuccessfulBackup = ago(now, 200*time.Hour)

	p := backupPolicy(168)
	p.Spec.BackupMonitoring.MaxBackupAgeHours = 0 // isolate the recovery-point check
	st := r.evaluateBackupStatus(context.Background(), p, cluster)
	if st.BackupHealthStatus != "RecoveryPointTooOld" {
		t.Fatalf("status = %q, want RecoveryPointTooOld", st.BackupHealthStatus)
	}
}

// The CNPG status field wins and the primary is not exec'd; a probe error falls
// back to the last backup without failing the evaluation.
func TestLastArchivedWALTime_Sources(t *testing.T) {
	now := time.Now()
	prober := &fakeProber{t: ago(now, time.Hour)}
	r := &StoragePolicyReconciler{walArchiveProber: prober}

	c := cnpg.ClusterInfo{Name: "a", Namespace: "b"}
	c.Status.CurrentPrimary = "a-1"
	c.Status.LastArchivedWALTime = ago(now, time.Minute)
	if got := r.lastArchivedWALTime(context.Background(), c); got == nil || !got.Equal(*c.Status.LastArchivedWALTime) {
		t.Errorf("status field not used: %v", got)
	}
	if prober.calls != 0 {
		t.Error("prober must not run when the status field is set")
	}

	c.Status.LastArchivedWALTime = nil
	prober.t, prober.err = nil, errors.New("exec failed")
	if got := r.lastArchivedWALTime(context.Background(), c); got != nil {
		t.Errorf("probe error should yield nil, got %v", got)
	}

	c.Status.CurrentPrimary = ""
	prober.calls = 0
	_ = r.lastArchivedWALTime(context.Background(), c)
	if prober.calls != 0 {
		t.Error("no primary: prober must not run")
	}
}

// handleAlert must not log "Alert sent successfully" for an alert that was only
// suppressed (it misled the cluster-services-1370 triage).
func TestHandleAlert_SuppressedIsNotLoggedAsSent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var sentLogs int
	logger := funcr.New(func(_, args string) {
		if strings.Contains(args, `"Alert sent successfully"`) {
			sentLogs++
		}
	}, funcr.Options{Verbosity: 1})
	ctx := logf.IntoContext(context.Background(), logger)

	p := &cnpgv1alpha1.StoragePolicy{}
	p.Name, p.Namespace = "pol", "ns"
	p.Spec.Alerting.Channels = []cnpgv1alpha1.AlertChannel{
		{Type: cnpgv1alpha1.AlertChannelTypeAlertmanager, Endpoint: srv.URL},
	}
	r := &StoragePolicyReconciler{alertManagers: map[string]*alerting.AlertManager{}}
	cluster := cnpg.ClusterInfo{Name: "c", Namespace: "n"}
	res := &policy.EvaluationResult{ThresholdResult: policy.ThresholdResult{
		Level: policy.ThresholdLevelCritical, CurrentUsagePercent: 85}}

	for i := 0; i < 3; i++ {
		if err := r.handleAlert(ctx, p, cluster, res); err != nil {
			t.Fatalf("handleAlert #%d: %v", i, err)
		}
	}
	if sentLogs != 1 {
		t.Errorf("logged %d sends for 1 real send + 2 suppressed", sentLogs)
	}
}

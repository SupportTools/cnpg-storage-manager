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

package alerting

// Regression tests for cluster-services-1381: drifting Details as labels, missing
// endsAt, "sent" reported for suppressed alerts, and no failed-send metric.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	cnpgv1alpha1 "github.com/supporttools/cnpg-storage-manager/api/v1alpha1"
	"github.com/supporttools/cnpg-storage-manager/pkg/metrics"
)

func newTestManager(t *testing.T, endpoint string) *AlertManager {
	t.Helper()
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	return NewAlertManager(c, []cnpgv1alpha1.AlertChannel{
		{Type: cnpgv1alpha1.AlertChannelTypeAlertmanager, Endpoint: endpoint},
	})
}

func captureAlertmanager(t *testing.T, status int) (*httptest.Server, *[]map[string]interface{}, *int) {
	t.Helper()
	var payload []map[string]interface{}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		payload = nil
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode: %v", err)
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, &payload, &calls
}

// Drifting values (usage_percent, issue_N) must be annotations so the alert keeps one
// fingerprint; identity keys (threshold, alert_type, policy) stay labels.
func TestAlertmanagerPayload_DriftingDetailsAreAnnotations(t *testing.T) {
	srv, payload, _ := captureAlertmanager(t, http.StatusOK)
	m := newTestManager(t, srv.URL)

	alert := &Alert{
		ClusterName: "c1", ClusterNamespace: "ns1", Severity: AlertSeverityWarning,
		Message: "backup", Timestamp: time.Now(),
		Details: map[string]string{
			DetailAlertType: AlertTypeBackup,
			DetailPolicy:    "default-postgres",
			DetailThreshold: "warning",
			"usage_percent": "73.7",
			"issue_1":       "last backup is 25 hours old (max: 24)",
			"issue_count":   "1",
		},
	}
	if err := m.sendToAlertmanager(context.Background(), alert, m.channels[0]); err != nil {
		t.Fatal(err)
	}
	labels := (*payload)[0]["labels"].(map[string]interface{})
	annotations := (*payload)[0]["annotations"].(map[string]interface{})

	identity := []string{DetailAlertType, DetailPolicy, DetailThreshold, "alertname", "cluster", "namespace", "severity"}
	for _, k := range identity {
		if _, ok := labels[k]; !ok {
			t.Errorf("label %q missing", k)
		}
	}
	for _, k := range []string{"usage_percent", "issue_1", "issue_count"} {
		if _, ok := labels[k]; ok {
			t.Errorf("drifting key %q must not be a label", k)
		}
		if _, ok := annotations[k]; !ok {
			t.Errorf("drifting key %q missing from annotations", k)
		}
	}
}

// Two sends whose drifting details differ must carry identical label sets.
func TestAlertmanagerPayload_LabelsStableAcrossDrift(t *testing.T) {
	a1 := &Alert{ClusterName: "c", ClusterNamespace: "n", Severity: AlertSeverityCritical,
		Details: map[string]string{"usage_percent": "80.1", DetailThreshold: string(AlertSeverityCritical),
			DetailPolicy: "p", DetailAlertType: AlertTypeStorage}}
	a2 := &Alert{ClusterName: "c", ClusterNamespace: "n", Severity: AlertSeverityCritical,
		Details: map[string]string{"usage_percent": "81.9", DetailThreshold: string(AlertSeverityCritical),
			DetailPolicy: "p", DetailAlertType: AlertTypeStorage}}
	l1, _ := alertmanagerLabelsAndAnnotations(a1)
	l2, _ := alertmanagerLabelsAndAnnotations(a2)
	if len(l1) != len(l2) {
		t.Fatalf("label sets differ: %v vs %v", l1, l2)
	}
	for k, v := range l1 {
		if l2[k] != v {
			t.Errorf("label %q differs: %q vs %q", k, v, l2[k])
		}
	}
}

// endsAt must be set and outlast the suppression window, or Alertmanager resolves the
// alert (resolve_timeout 5m) before the next send.
func TestAlertmanagerPayload_EndsAtOutlastsResendInterval(t *testing.T) {
	srv, payload, _ := captureAlertmanager(t, http.StatusOK)
	m := newTestManager(t, srv.URL)
	fixed := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return fixed }

	alert := &Alert{ClusterName: "c", ClusterNamespace: "n", Severity: AlertSeverityWarning, Message: "x"}
	if err := m.sendToAlertmanager(context.Background(), alert, m.channels[0]); err != nil {
		t.Fatal(err)
	}
	endsRaw, ok := (*payload)[0]["endsAt"].(string)
	if !ok {
		t.Fatal("endsAt missing")
	}
	endsAt, err := time.Parse(time.RFC3339, endsRaw)
	if err != nil {
		t.Fatal(err)
	}
	if got := endsAt.Sub(fixed); got != AlertmanagerEndsAtTTL {
		t.Errorf("endsAt = now+%v, want now+%v", got, AlertmanagerEndsAtTTL)
	}
	// Must survive at least two missed resends.
	if AlertmanagerEndsAtTTL < 2*SuppressionWindow+time.Minute {
		t.Errorf("AlertmanagerEndsAtTTL %v too close to SuppressionWindow %v", AlertmanagerEndsAtTTL, SuppressionWindow)
	}
	if _, ok := (*payload)[0]["startsAt"].(string); !ok {
		t.Error("startsAt missing")
	}
}

// A suppressed alert returns ErrAlertSuppressed (not nil) and is not delivered.
func TestSendAlert_SuppressedReturnsSentinel(t *testing.T) {
	srv, _, calls := captureAlertmanager(t, http.StatusOK)
	m := newTestManager(t, srv.URL)
	alert := &Alert{ClusterName: "sup", ClusterNamespace: "n", Severity: AlertSeverityWarning, Message: "x"}

	if err := m.SendAlert(context.Background(), alert); err != nil {
		t.Fatalf("first send: %v", err)
	}
	err := m.SendAlert(context.Background(), alert)
	if !errors.Is(err, ErrAlertSuppressed) {
		t.Fatalf("second send: got %v, want ErrAlertSuppressed", err)
	}
	if *calls != 1 {
		t.Errorf("alertmanager called %d times, want 1", *calls)
	}

	// After the window it is sent again.
	m.now = func() time.Time { return time.Now().Add(SuppressionWindow + time.Second) }
	if err := m.SendAlert(context.Background(), alert); err != nil {
		t.Fatalf("send after window: %v", err)
	}
	if *calls != 2 {
		t.Errorf("alertmanager called %d times, want 2", *calls)
	}
}

// A failed send increments alerts_failed_total, returns an error, and is NOT
// suppressed, so the next reconcile retries.
func TestSendAlert_FailureCountedAndNotSuppressed(t *testing.T) {
	srv, _, calls := captureAlertmanager(t, http.StatusInternalServerError)
	m := newTestManager(t, srv.URL)
	alert := &Alert{ClusterName: "fail-1381", ClusterNamespace: "n", Severity: AlertSeverityCritical, Message: "x"}

	failed := metrics.AlertsFailedTotal.WithLabelValues("fail-1381", "n", string(AlertSeverityCritical), "alertmanager")
	before := testutil.ToFloat64(failed)

	err := m.SendAlert(context.Background(), alert)
	if err == nil || errors.Is(err, ErrAlertSuppressed) {
		t.Fatalf("got %v, want a delivery error", err)
	}
	if got := testutil.ToFloat64(failed) - before; got != 1 {
		t.Errorf("alerts_failed_total increased by %v, want 1", got)
	}
	err = m.SendAlert(context.Background(), alert)
	if errors.Is(err, ErrAlertSuppressed) {
		t.Fatal("failed alert was suppressed; it must be retried")
	}
	if *calls != 2 {
		t.Errorf("alertmanager called %d times, want 2 (retry)", *calls)
	}
}

// A storage alert and a backup alert of the same severity on the same cluster are
// different alerts and must not suppress each other.
func TestSendAlert_DifferentAlertTypesNotSuppressedTogether(t *testing.T) {
	srv, _, calls := captureAlertmanager(t, http.StatusOK)
	m := newTestManager(t, srv.URL)
	storage := &Alert{ClusterName: "c", ClusterNamespace: "n", Severity: AlertSeverityWarning,
		Details: map[string]string{DetailAlertType: AlertTypeStorage}}
	backup := &Alert{ClusterName: "c", ClusterNamespace: "n", Severity: AlertSeverityWarning,
		Details: map[string]string{DetailAlertType: AlertTypeBackup}}
	if err := m.SendAlert(context.Background(), storage); err != nil {
		t.Fatal(err)
	}
	if err := m.SendAlert(context.Background(), backup); err != nil {
		t.Fatalf("backup alert: %v", err)
	}
	if *calls != 2 {
		t.Errorf("calls = %d, want 2", *calls)
	}
}

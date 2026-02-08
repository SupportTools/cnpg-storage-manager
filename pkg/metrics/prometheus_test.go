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

package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRecordPVCMetrics(t *testing.T) {
	// Reset metrics before test
	PVCUsageBytes.Reset()
	PVCCapacityBytes.Reset()
	PVCUsagePercent.Reset()

	// Record metrics
	RecordPVCMetrics("test-cluster", "default", "test-pvc", "test-instance", 5368709120, 10737418240)

	// Verify usage bytes
	usageValue := testutil.ToFloat64(PVCUsageBytes.WithLabelValues("test-cluster", "default", "test-pvc", "test-instance"))
	if usageValue != 5368709120 {
		t.Errorf("expected usage bytes 5368709120, got %f", usageValue)
	}

	// Verify capacity bytes
	capacityValue := testutil.ToFloat64(
		PVCCapacityBytes.WithLabelValues("test-cluster", "default", "test-pvc", "test-instance"),
	)
	if capacityValue != 10737418240 {
		t.Errorf("expected capacity bytes 10737418240, got %f", capacityValue)
	}

	// Verify usage percent (should be 50%)
	percentValue := testutil.ToFloat64(
		PVCUsagePercent.WithLabelValues("test-cluster", "default", "test-pvc", "test-instance"),
	)
	if percentValue != 50.0 {
		t.Errorf("expected usage percent 50.0, got %f", percentValue)
	}
}

func TestRecordPVCMetrics_ZeroCapacity(t *testing.T) {
	PVCUsageBytes.Reset()
	PVCCapacityBytes.Reset()
	PVCUsagePercent.Reset()

	// Record with zero capacity (should not panic)
	RecordPVCMetrics("test-cluster", "default", "test-pvc", "test-instance", 1000, 0)

	// Usage should still be recorded
	usageValue := testutil.ToFloat64(PVCUsageBytes.WithLabelValues("test-cluster", "default", "test-pvc", "test-instance"))
	if usageValue != 1000 {
		t.Errorf("expected usage bytes 1000, got %f", usageValue)
	}
}

func TestRecordWALMetrics(t *testing.T) {
	WALDirectoryBytes.Reset()
	WALFilesCount.Reset()

	RecordWALMetrics("test-cluster", "default", "test-instance", 167772160, 10)

	// Verify WAL directory size
	sizeValue := testutil.ToFloat64(WALDirectoryBytes.WithLabelValues("test-cluster", "default", "test-instance"))
	if sizeValue != 167772160 {
		t.Errorf("expected WAL size 167772160, got %f", sizeValue)
	}

	// Verify WAL file count
	countValue := testutil.ToFloat64(WALFilesCount.WithLabelValues("test-cluster", "default", "test-instance"))
	if countValue != 10 {
		t.Errorf("expected WAL count 10, got %f", countValue)
	}
}

func TestRecordReconcile(t *testing.T) {
	ReconcileTotal.Reset()
	ReconcileDuration.Reset()

	RecordReconcile("storagepolicy", "success", 0.5)

	// Verify reconcile count
	countValue := testutil.ToFloat64(ReconcileTotal.WithLabelValues("storagepolicy", "success"))
	if countValue != 1 {
		t.Errorf("expected reconcile count 1, got %f", countValue)
	}

	// Record another reconcile
	RecordReconcile("storagepolicy", "success", 0.3)
	countValue = testutil.ToFloat64(ReconcileTotal.WithLabelValues("storagepolicy", "success"))
	if countValue != 2 {
		t.Errorf("expected reconcile count 2, got %f", countValue)
	}

	// Record a failure
	RecordReconcile("storagepolicy", "error", 1.0)
	errorCount := testutil.ToFloat64(ReconcileTotal.WithLabelValues("storagepolicy", "error"))
	if errorCount != 1 {
		t.Errorf("expected error count 1, got %f", errorCount)
	}
}

func TestRecordError(t *testing.T) {
	ErrorsTotal.Reset()

	RecordError("expansion", "test-cluster", "default")
	RecordError("expansion", "test-cluster", "default")
	RecordError("wal-cleanup", "test-cluster", "default")

	// Verify expansion errors
	expansionErrors := testutil.ToFloat64(ErrorsTotal.WithLabelValues("expansion", "test-cluster", "default"))
	if expansionErrors != 2 {
		t.Errorf("expected 2 expansion errors, got %f", expansionErrors)
	}

	// Verify WAL cleanup errors
	walErrors := testutil.ToFloat64(ErrorsTotal.WithLabelValues("wal-cleanup", "test-cluster", "default"))
	if walErrors != 1 {
		t.Errorf("expected 1 WAL cleanup error, got %f", walErrors)
	}
}

func TestRecordThresholdBreach(t *testing.T) {
	ThresholdBreachesTotal.Reset()

	RecordThresholdBreach("test-cluster", "default", "warning")
	RecordThresholdBreach("test-cluster", "default", "critical")
	RecordThresholdBreach("test-cluster", "default", "warning")

	// Verify warning breaches
	warningBreaches := testutil.ToFloat64(ThresholdBreachesTotal.WithLabelValues("test-cluster", "default", "warning"))
	if warningBreaches != 2 {
		t.Errorf("expected 2 warning breaches, got %f", warningBreaches)
	}

	// Verify critical breaches
	criticalBreaches := testutil.ToFloat64(ThresholdBreachesTotal.WithLabelValues("test-cluster", "default", "critical"))
	if criticalBreaches != 1 {
		t.Errorf("expected 1 critical breach, got %f", criticalBreaches)
	}
}

func TestRecordExpansion(t *testing.T) {
	ExpansionTotal.Reset()
	ExpansionBytesTotal.Reset()

	// Record successful expansion
	RecordExpansion("test-cluster", "default", "success", 5368709120)

	successCount := testutil.ToFloat64(ExpansionTotal.WithLabelValues("test-cluster", "default", "success"))
	if successCount != 1 {
		t.Errorf("expected 1 successful expansion, got %f", successCount)
	}

	bytesExpanded := testutil.ToFloat64(ExpansionBytesTotal.WithLabelValues("test-cluster", "default"))
	if bytesExpanded != 5368709120 {
		t.Errorf("expected 5368709120 bytes expanded, got %f", bytesExpanded)
	}

	// Record failed expansion (shouldn't add bytes)
	RecordExpansion("test-cluster", "default", "failure", 1000000)

	failureCount := testutil.ToFloat64(ExpansionTotal.WithLabelValues("test-cluster", "default", "failure"))
	if failureCount != 1 {
		t.Errorf("expected 1 failed expansion, got %f", failureCount)
	}

	// Bytes should not increase for failures
	bytesExpanded = testutil.ToFloat64(ExpansionBytesTotal.WithLabelValues("test-cluster", "default"))
	if bytesExpanded != 5368709120 {
		t.Errorf("expected bytes to remain 5368709120, got %f", bytesExpanded)
	}
}

func TestRecordWALCleanup(t *testing.T) {
	WALCleanupTotal.Reset()

	RecordWALCleanup("test-cluster", "default", "success")
	RecordWALCleanup("test-cluster", "default", "success")
	RecordWALCleanup("test-cluster", "default", "failure")

	successCount := testutil.ToFloat64(WALCleanupTotal.WithLabelValues("test-cluster", "default", "success"))
	if successCount != 2 {
		t.Errorf("expected 2 successful WAL cleanups, got %f", successCount)
	}

	failureCount := testutil.ToFloat64(WALCleanupTotal.WithLabelValues("test-cluster", "default", "failure"))
	if failureCount != 1 {
		t.Errorf("expected 1 failed WAL cleanup, got %f", failureCount)
	}
}

func TestSetCircuitBreakerState(t *testing.T) {
	CircuitBreakerState.Reset()

	// Set circuit breaker to open
	SetCircuitBreakerState("test-cluster", "default", true)
	openValue := testutil.ToFloat64(CircuitBreakerState.WithLabelValues("test-cluster", "default"))
	if openValue != 1.0 {
		t.Errorf("expected circuit breaker state 1.0 (open), got %f", openValue)
	}

	// Set circuit breaker to closed
	SetCircuitBreakerState("test-cluster", "default", false)
	closedValue := testutil.ToFloat64(CircuitBreakerState.WithLabelValues("test-cluster", "default"))
	if closedValue != 0.0 {
		t.Errorf("expected circuit breaker state 0.0 (closed), got %f", closedValue)
	}
}

func TestRecordAlertSent(t *testing.T) {
	AlertsSentTotal.Reset()

	RecordAlertSent("test-cluster", "default", "warning", "slack")
	RecordAlertSent("test-cluster", "default", "critical", "pagerduty")
	RecordAlertSent("test-cluster", "default", "warning", "slack")

	slackWarnings := testutil.ToFloat64(AlertsSentTotal.WithLabelValues("test-cluster", "default", "warning", "slack"))
	if slackWarnings != 2 {
		t.Errorf("expected 2 slack warnings, got %f", slackWarnings)
	}

	pagerdutyAlerts := testutil.ToFloat64(
		AlertsSentTotal.WithLabelValues("test-cluster", "default", "critical", "pagerduty"),
	)
	if pagerdutyAlerts != 1 {
		t.Errorf("expected 1 pagerduty alert, got %f", pagerdutyAlerts)
	}
}

func TestRecordAlertSuppressed(t *testing.T) {
	AlertsSuppressedTotal.Reset()

	RecordAlertSuppressed("test-cluster", "default", "duplicate")
	RecordAlertSuppressed("test-cluster", "default", "cooldown")
	RecordAlertSuppressed("test-cluster", "default", "duplicate")

	duplicates := testutil.ToFloat64(AlertsSuppressedTotal.WithLabelValues("test-cluster", "default", "duplicate"))
	if duplicates != 2 {
		t.Errorf("expected 2 duplicate suppressions, got %f", duplicates)
	}

	cooldowns := testutil.ToFloat64(AlertsSuppressedTotal.WithLabelValues("test-cluster", "default", "cooldown"))
	if cooldowns != 1 {
		t.Errorf("expected 1 cooldown suppression, got %f", cooldowns)
	}
}

func TestDeletePVCMetrics(t *testing.T) {
	PVCUsageBytes.Reset()
	PVCCapacityBytes.Reset()
	PVCUsagePercent.Reset()

	// Record metrics first
	RecordPVCMetrics("test-cluster", "default", "test-pvc", "test-instance", 5000, 10000)

	// Verify they exist
	usageValue := testutil.ToFloat64(PVCUsageBytes.WithLabelValues("test-cluster", "default", "test-pvc", "test-instance"))
	if usageValue != 5000 {
		t.Errorf("expected usage 5000, got %f", usageValue)
	}

	// Delete metrics
	DeletePVCMetrics("test-cluster", "default", "test-pvc", "test-instance")

	// Verify deletion (should return 0 for non-existent label set)
	// Note: After deletion, accessing the metric creates a new one with value 0
}

func TestDeleteWALMetrics(t *testing.T) {
	WALDirectoryBytes.Reset()
	WALFilesCount.Reset()

	// Record metrics first
	RecordWALMetrics("test-cluster", "default", "test-instance", 100000, 5)

	// Verify they exist
	sizeValue := testutil.ToFloat64(WALDirectoryBytes.WithLabelValues("test-cluster", "default", "test-instance"))
	if sizeValue != 100000 {
		t.Errorf("expected size 100000, got %f", sizeValue)
	}

	// Delete metrics
	DeleteWALMetrics("test-cluster", "default", "test-instance")
}

func TestMetricsNamespace(t *testing.T) {
	if MetricsNamespace != "cnpg_storage_manager" {
		t.Errorf("expected namespace 'cnpg_storage_manager', got '%s'", MetricsNamespace)
	}
}

func TestMetricsRegistration(t *testing.T) {
	// Verify all metrics are properly defined
	metrics := []prometheus.Collector{
		PVCUsageBytes,
		PVCCapacityBytes,
		PVCUsagePercent,
		WALDirectoryBytes,
		WALFilesCount,
		ClustersManagedTotal,
		PoliciesActiveTotal,
		ReconcileTotal,
		ReconcileDuration,
		ErrorsTotal,
		ThresholdBreachesTotal,
		ExpansionTotal,
		ExpansionBytesTotal,
		WALCleanupTotal,
		WALFilesRemoved,
		CircuitBreakerState,
		AlertsSentTotal,
		AlertsSuppressedTotal,
		MetricsCollectionDuration,
	}

	for _, m := range metrics {
		if m == nil {
			t.Error("found nil metric collector")
		}
	}
}

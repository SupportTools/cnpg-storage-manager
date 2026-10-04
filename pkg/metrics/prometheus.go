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
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// String literals shared across this file (goconst).
const (
	labelCluster   = "cluster"
	labelResult    = "result"
	labelType      = "type"
	labelInstance  = "instance"
	labelNamespace = "namespace"
	labelPVC       = "pvc"
)

const (
	// MetricsNamespace is the namespace for all CNPG Storage Manager metrics
	MetricsNamespace = "cnpg_storage_manager"
)

var (
	// PVCUsageBytes tracks the current usage of PVCs in bytes
	PVCUsageBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: MetricsNamespace,
			Name:      "pvc_usage_bytes",
			Help:      "Current PVC usage in bytes",
		},
		[]string{labelCluster, labelNamespace, labelPVC, labelInstance},
	)

	// PVCCapacityBytes tracks the total capacity of PVCs in bytes
	PVCCapacityBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: MetricsNamespace,
			Name:      "pvc_capacity_bytes",
			Help:      "Total PVC capacity in bytes",
		},
		[]string{labelCluster, labelNamespace, labelPVC, labelInstance},
	)

	// PVCUsagePercent tracks the usage percentage of PVCs
	PVCUsagePercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: MetricsNamespace,
			Name:      "pvc_usage_percent",
			Help:      "PVC usage as a percentage of capacity",
		},
		[]string{labelCluster, labelNamespace, labelPVC, labelInstance},
	)

	// WALDirectoryBytes tracks the WAL directory size in bytes
	WALDirectoryBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: MetricsNamespace,
			Name:      "wal_directory_bytes",
			Help:      "WAL directory size in bytes",
		},
		[]string{labelCluster, labelNamespace, labelInstance},
	)

	// WALFilesCount tracks the number of WAL files
	WALFilesCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: MetricsNamespace,
			Name:      "wal_files_count",
			Help:      "Number of WAL files",
		},
		[]string{labelCluster, labelNamespace, labelInstance},
	)

	// ClustersManagedTotal tracks the number of clusters managed by policies
	ClustersManagedTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: MetricsNamespace,
			Name:      "clusters_managed_total",
			Help:      "Total number of clusters managed by storage policies",
		},
		[]string{labelNamespace},
	)

	// PoliciesActiveTotal tracks the number of active storage policies
	PoliciesActiveTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: MetricsNamespace,
			Name:      "policies_active_total",
			Help:      "Total number of active storage policies",
		},
		[]string{labelNamespace},
	)

	// ReconcileTotal tracks the total number of reconciliations
	ReconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricsNamespace,
			Name:      "reconcile_total",
			Help:      "Total number of reconciliations",
		},
		[]string{"controller", labelResult},
	)

	// ReconcileDuration tracks the duration of reconciliations
	ReconcileDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: MetricsNamespace,
			Name:      "reconcile_duration_seconds",
			Help:      "Duration of reconciliation in seconds",
			Buckets:   prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms to ~16s
		},
		[]string{"controller"},
	)

	// ErrorsTotal tracks the total number of errors
	ErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricsNamespace,
			Name:      "errors_total",
			Help:      "Total number of errors",
		},
		[]string{labelType, labelCluster, labelNamespace},
	)

	// ThresholdBreachesTotal tracks threshold breaches
	ThresholdBreachesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricsNamespace,
			Name:      "threshold_breaches_total",
			Help:      "Total number of threshold breaches",
		},
		[]string{labelCluster, labelNamespace, "level"},
	)

	// ExpansionTotal tracks expansion operations
	ExpansionTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricsNamespace,
			Name:      "expansion_total",
			Help:      "Total number of expansion operations",
		},
		[]string{labelCluster, labelNamespace, labelResult},
	)

	// ExpansionBytesTotal tracks bytes expanded
	ExpansionBytesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricsNamespace,
			Name:      "expansion_bytes_total",
			Help:      "Total bytes expanded",
		},
		[]string{labelCluster, labelNamespace},
	)

	// WALCleanupTotal tracks WAL cleanup operations
	WALCleanupTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricsNamespace,
			Name:      "wal_cleanup_total",
			Help:      "Total number of WAL cleanup operations",
		},
		[]string{labelCluster, labelNamespace, labelResult},
	)

	// WALFilesRemoved tracks the number of WAL files removed
	WALFilesRemoved = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricsNamespace,
			Name:      "wal_files_removed_total",
			Help:      "Total number of WAL files removed",
		},
		[]string{labelCluster, labelNamespace},
	)

	// CircuitBreakerState tracks circuit breaker state (0=closed, 1=open)
	CircuitBreakerState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: MetricsNamespace,
			Name:      "circuit_breaker_open",
			Help:      "Circuit breaker state (0=closed, 1=open)",
		},
		[]string{labelCluster, labelNamespace},
	)

	// AlertsSentTotal tracks alerts sent
	AlertsSentTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricsNamespace,
			Name:      "alerts_sent_total",
			Help:      "Total number of alerts sent",
		},
		[]string{labelCluster, labelNamespace, "severity", "channel"},
	)

	// AlertsSuppressedTotal tracks suppressed alerts
	AlertsSuppressedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricsNamespace,
			Name:      "alerts_suppressed_total",
			Help:      "Total number of suppressed alerts",
		},
		[]string{labelCluster, labelNamespace, "reason"},
	)

	// MetricsCollectionDuration tracks metrics collection duration
	MetricsCollectionDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: MetricsNamespace,
			Name:      "metrics_collection_duration_seconds",
			Help:      "Duration of metrics collection in seconds",
		},
		[]string{labelType},
	)

	// BackupLastSuccessTimestamp tracks the last successful backup timestamp
	BackupLastSuccessTimestamp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: MetricsNamespace,
			Name:      "backup_last_success_timestamp",
			Help:      "Unix timestamp of the last successful backup",
		},
		[]string{labelCluster, labelNamespace},
	)

	// BackupLastSuccessAgeHours tracks hours since last successful backup
	BackupLastSuccessAgeHours = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: MetricsNamespace,
			Name:      "backup_last_success_age_hours",
			Help:      "Hours since the last successful backup",
		},
		[]string{labelCluster, labelNamespace},
	)

	// BackupFirstRecoverabilityTimestamp tracks the first recoverability point
	BackupFirstRecoverabilityTimestamp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: MetricsNamespace,
			Name:      "backup_first_recoverability_timestamp",
			Help:      "Unix timestamp of the first recoverability point",
		},
		[]string{labelCluster, labelNamespace},
	)

	// BackupFirstRecoverabilityAgeHours tracks hours since first recoverability point
	BackupFirstRecoverabilityAgeHours = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: MetricsNamespace,
			Name:      "backup_first_recoverability_age_hours",
			Help:      "Hours since the first recoverability point",
		},
		[]string{labelCluster, labelNamespace},
	)

	// BackupContinuousArchivingWorking tracks if WAL archiving is working
	BackupContinuousArchivingWorking = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: MetricsNamespace,
			Name:      "backup_continuous_archiving_working",
			Help:      "Whether continuous WAL archiving is working (1=yes, 0=no)",
		},
		[]string{labelCluster, labelNamespace},
	)

	// BackupConfigured tracks if backups are configured for a cluster
	BackupConfigured = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: MetricsNamespace,
			Name:      "backup_configured",
			Help:      "Whether backups are configured for the cluster (1=yes, 0=no)",
		},
		[]string{labelCluster, labelNamespace},
	)

	// BackupHealthy tracks overall backup health status
	BackupHealthy = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: MetricsNamespace,
			Name:      "backup_healthy",
			Help:      "Whether backups are healthy (1=yes, 0=no)",
		},
		[]string{labelCluster, labelNamespace},
	)

	// BackupAlertsTotal tracks backup-related alerts
	BackupAlertsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricsNamespace,
			Name:      "backup_alerts_total",
			Help:      "Total number of backup-related alerts",
		},
		[]string{labelCluster, labelNamespace, labelType},
	)
)

func init() {
	// Register all metrics with the controller-runtime metrics registry
	metrics.Registry.MustRegister(
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
		// Backup metrics
		BackupLastSuccessTimestamp,
		BackupLastSuccessAgeHours,
		BackupFirstRecoverabilityTimestamp,
		BackupFirstRecoverabilityAgeHours,
		BackupContinuousArchivingWorking,
		BackupConfigured,
		BackupHealthy,
		BackupAlertsTotal,
	)
}

// RecordPVCMetrics records PVC usage metrics
func RecordPVCMetrics(cluster, namespace, pvc, instance string, usageBytes, capacityBytes int64) {
	PVCUsageBytes.WithLabelValues(cluster, namespace, pvc, instance).Set(float64(usageBytes))
	PVCCapacityBytes.WithLabelValues(cluster, namespace, pvc, instance).Set(float64(capacityBytes))
	if capacityBytes > 0 {
		percent := float64(usageBytes) / float64(capacityBytes) * 100
		PVCUsagePercent.WithLabelValues(cluster, namespace, pvc, instance).Set(percent)
	}
}

// RecordWALMetrics records WAL directory metrics
func RecordWALMetrics(cluster, namespace, instance string, sizeBytes int64, fileCount int) {
	WALDirectoryBytes.WithLabelValues(cluster, namespace, instance).Set(float64(sizeBytes))
	WALFilesCount.WithLabelValues(cluster, namespace, instance).Set(float64(fileCount))
}

// RecordReconcile records a reconciliation
func RecordReconcile(controller, result string, duration float64) {
	ReconcileTotal.WithLabelValues(controller, result).Inc()
	ReconcileDuration.WithLabelValues(controller).Observe(duration)
}

// RecordError records an error
func RecordError(errorType, cluster, namespace string) {
	ErrorsTotal.WithLabelValues(errorType, cluster, namespace).Inc()
}

// RecordThresholdBreach records a threshold breach
func RecordThresholdBreach(cluster, namespace, level string) {
	ThresholdBreachesTotal.WithLabelValues(cluster, namespace, level).Inc()
}

// RecordExpansion records an expansion operation
func RecordExpansion(cluster, namespace, result string, bytes int64) {
	ExpansionTotal.WithLabelValues(cluster, namespace, result).Inc()
	if result == "success" && bytes > 0 {
		ExpansionBytesTotal.WithLabelValues(cluster, namespace).Add(float64(bytes))
	}
}

// RecordWALCleanup records a WAL cleanup operation
func RecordWALCleanup(cluster, namespace, result string) {
	WALCleanupTotal.WithLabelValues(cluster, namespace, result).Inc()
}

// SetCircuitBreakerState sets the circuit breaker state
func SetCircuitBreakerState(cluster, namespace string, open bool) {
	value := 0.0
	if open {
		value = 1.0
	}
	CircuitBreakerState.WithLabelValues(cluster, namespace).Set(value)
}

// RecordAlertSent records an alert being sent
func RecordAlertSent(cluster, namespace, severity, channel string) {
	AlertsSentTotal.WithLabelValues(cluster, namespace, severity, channel).Inc()
}

// RecordAlertSuppressed records a suppressed alert
func RecordAlertSuppressed(cluster, namespace, reason string) {
	AlertsSuppressedTotal.WithLabelValues(cluster, namespace, reason).Inc()
}

// DeletePVCMetrics deletes PVC metrics for a specific PVC
func DeletePVCMetrics(cluster, namespace, pvc, instance string) {
	PVCUsageBytes.DeleteLabelValues(cluster, namespace, pvc, instance)
	PVCCapacityBytes.DeleteLabelValues(cluster, namespace, pvc, instance)
	PVCUsagePercent.DeleteLabelValues(cluster, namespace, pvc, instance)
}

// DeleteWALMetrics deletes WAL metrics for a specific instance
func DeleteWALMetrics(cluster, namespace, instance string) {
	WALDirectoryBytes.DeleteLabelValues(cluster, namespace, instance)
	WALFilesCount.DeleteLabelValues(cluster, namespace, instance)
}

// RecordBackupMetrics records backup-related metrics for a cluster
func RecordBackupMetrics(
	cluster, namespace string,
	lastBackupTimestamp, firstRecoverabilityTimestamp *float64,
	archivingWorking, configured, healthy bool,
) {
	// Set boolean metrics
	archivingValue := 0.0
	if archivingWorking {
		archivingValue = 1.0
	}
	BackupContinuousArchivingWorking.WithLabelValues(cluster, namespace).Set(archivingValue)

	configuredValue := 0.0
	if configured {
		configuredValue = 1.0
	}
	BackupConfigured.WithLabelValues(cluster, namespace).Set(configuredValue)

	healthyValue := 0.0
	if healthy {
		healthyValue = 1.0
	}
	BackupHealthy.WithLabelValues(cluster, namespace).Set(healthyValue)

	// Set timestamp metrics if available
	if lastBackupTimestamp != nil {
		BackupLastSuccessTimestamp.WithLabelValues(cluster, namespace).Set(*lastBackupTimestamp)
	}
	if firstRecoverabilityTimestamp != nil {
		BackupFirstRecoverabilityTimestamp.WithLabelValues(cluster, namespace).Set(*firstRecoverabilityTimestamp)
	}
}

// RecordBackupAge records the age of the last backup in hours
func RecordBackupAge(cluster, namespace string, ageHours float64) {
	BackupLastSuccessAgeHours.WithLabelValues(cluster, namespace).Set(ageHours)
}

// RecordFirstRecoverabilityAge records the age of the first recoverability point in hours
func RecordFirstRecoverabilityAge(cluster, namespace string, ageHours float64) {
	BackupFirstRecoverabilityAgeHours.WithLabelValues(cluster, namespace).Set(ageHours)
}

// RecordBackupAlert records a backup-related alert
func RecordBackupAlert(cluster, namespace, alertType string) {
	BackupAlertsTotal.WithLabelValues(cluster, namespace, alertType).Inc()
}

// DeleteBackupMetrics deletes backup metrics for a specific cluster
func DeleteBackupMetrics(cluster, namespace string) {
	BackupLastSuccessTimestamp.DeleteLabelValues(cluster, namespace)
	BackupLastSuccessAgeHours.DeleteLabelValues(cluster, namespace)
	BackupFirstRecoverabilityTimestamp.DeleteLabelValues(cluster, namespace)
	BackupFirstRecoverabilityAgeHours.DeleteLabelValues(cluster, namespace)
	BackupContinuousArchivingWorking.DeleteLabelValues(cluster, namespace)
	BackupConfigured.DeleteLabelValues(cluster, namespace)
	BackupHealthy.DeleteLabelValues(cluster, namespace)
}

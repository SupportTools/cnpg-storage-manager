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

import (
	"context"
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	cnpgv1alpha1 "github.com/supporttools/cnpg-storage-manager/api/v1alpha1"
	"github.com/supporttools/cnpg-storage-manager/pkg/alerting"
	"github.com/supporttools/cnpg-storage-manager/pkg/annotations"
	"github.com/supporttools/cnpg-storage-manager/pkg/cnpg"
	"github.com/supporttools/cnpg-storage-manager/pkg/metrics"
	"github.com/supporttools/cnpg-storage-manager/pkg/policy"
	"github.com/supporttools/cnpg-storage-manager/pkg/remediation"
)

const (
	// FinalizerName is the finalizer name for StoragePolicy
	FinalizerName = "storagepolicy.cnpg.supporttools.io/finalizer"

	// DefaultRequeueInterval is the default requeue interval
	DefaultRequeueInterval = 30 * time.Second
)

// StoragePolicyReconciler reconciles a StoragePolicy object
type StoragePolicyReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	RestConfig *rest.Config

	// GlobalDryRun enables dry-run mode for all operations regardless of policy settings.
	// When true, no actual changes are made to PVCs or WAL files.
	GlobalDryRun bool

	// Internal components
	discovery        *cnpg.Discovery
	metricsCollector *metrics.Collector
	evaluator        *policy.Evaluator
	expansionEngine  *remediation.ExpansionEngine
	walCleanupEngine *remediation.WALCleanupEngine
	alertManagers    map[string]*alerting.AlertManager // per-policy alert managers
}

// RBAC for StoragePolicy management
// +kubebuilder:rbac:groups=cnpg.supporttools.io,resources=storagepolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cnpg.supporttools.io,resources=storagepolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cnpg.supporttools.io,resources=storagepolicies/finalizers,verbs=update

// RBAC for StorageEvent management (audit trail)
// +kubebuilder:rbac:groups=cnpg.supporttools.io,resources=storageevents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cnpg.supporttools.io,resources=storageevents/status,verbs=get;update;patch

// RBAC for CNPG Cluster access (read and annotate)
// +kubebuilder:rbac:groups=postgresql.cnpg.io,resources=clusters,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups=postgresql.cnpg.io,resources=clusters/status,verbs=get

// RBAC for ObjectStore access (barman-cloud plugin backup status)
// +kubebuilder:rbac:groups=barmancloud.cnpg.io,resources=objectstores,verbs=get;list;watch
// +kubebuilder:rbac:groups=barmancloud.cnpg.io,resources=objectstores/status,verbs=get

// RBAC for PVC management (expansion)
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;patch;update

// RBAC for Pod access (WAL cleanup via exec)
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods/exec,verbs=create

// RBAC for Node access (kubelet metrics via proxy)
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list
// +kubebuilder:rbac:groups="",resources=nodes/proxy,verbs=get

// RBAC for Kubernetes Events (create events for auditing)
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// RBAC for StorageClass validation
// +kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch

// RBAC for Secret access (alert channel credentials)
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get

// RBAC for leader election
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *StoragePolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	startTime := time.Now()

	defer func() {
		duration := time.Since(startTime).Seconds()
		metrics.ReconcileDuration.WithLabelValues("storagepolicy").Observe(duration)
	}()

	// Fetch the StoragePolicy instance
	var policyObj cnpgv1alpha1.StoragePolicy
	if err := r.Get(ctx, req.NamespacedName, &policyObj); err != nil {
		if errors.IsNotFound(err) {
			log.Info("StoragePolicy not found, may have been deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get StoragePolicy")
		metrics.RecordReconcile("storagepolicy", "error", time.Since(startTime).Seconds())
		return ctrl.Result{}, err
	}

	log.Info("Reconciling StoragePolicy", "name", policyObj.Name, "namespace", policyObj.Namespace)

	// Handle deletion
	if !policyObj.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, &policyObj)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(&policyObj, FinalizerName) {
		controllerutil.AddFinalizer(&policyObj, FinalizerName)
		if err := r.Update(ctx, &policyObj); err != nil {
			log.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Initialize internal components if needed
	r.initComponents()

	// Find matching CNPG clusters
	clusters, err := r.findMatchingClusters(ctx, &policyObj)
	if err != nil {
		log.Error(err, "Failed to find matching clusters")
		r.setCondition(&policyObj, "Ready", metav1.ConditionFalse, "ClusterDiscoveryFailed", err.Error())
		if statusErr := r.Status().Update(ctx, &policyObj); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		metrics.RecordReconcile("storagepolicy", "error", time.Since(startTime).Seconds())
		return ctrl.Result{RequeueAfter: DefaultRequeueInterval}, err
	}

	log.Info("Found matching clusters", "count", len(clusters))

	// Update managed clusters count metric
	metrics.ClustersManagedTotal.WithLabelValues(policyObj.Namespace).Set(float64(len(clusters)))

	// Process each cluster
	managedClusters := make([]cnpgv1alpha1.ManagedCluster, 0, len(clusters))
	var reconciledCount, errorCount int

	for _, cluster := range clusters {
		clusterResult, err := r.processCluster(ctx, &policyObj, cluster)
		if err != nil {
			log.Error(err, "Failed to process cluster", "cluster", cluster.Name, "namespace", cluster.Namespace)
			errorCount++
			metrics.RecordError("cluster_processing", cluster.Name, cluster.Namespace)

			managedClusters = append(managedClusters, cnpgv1alpha1.ManagedCluster{
				Name:         cluster.Name,
				Namespace:    cluster.Namespace,
				LastChecked:  metav1.Now(),
				UsagePercent: 0,
				Status:       "Error",
			})
			continue
		}

		reconciledCount++
		managedClusters = append(managedClusters, *clusterResult)
	}

	// Update policy status
	policyObj.Status.ManagedClusters = managedClusters
	policyObj.Status.LastEvaluated = &metav1.Time{Time: time.Now()}
	policyObj.Status.ObservedGeneration = policyObj.Generation

	if errorCount > 0 {
		r.setCondition(&policyObj, "Ready", metav1.ConditionFalse, "PartialSuccess",
			fmt.Sprintf("Processed %d clusters, %d errors", reconciledCount, errorCount))
	} else if len(clusters) == 0 {
		r.setCondition(&policyObj, "Ready", metav1.ConditionTrue, "NoClustersMatched",
			"No CNPG clusters matched the selector")
	} else {
		r.setCondition(&policyObj, "Ready", metav1.ConditionTrue, "ReconcileSucceeded",
			fmt.Sprintf("Successfully processed %d clusters", reconciledCount))
	}

	if err := r.Status().Update(ctx, &policyObj); err != nil {
		log.Error(err, "Failed to update status")
		metrics.RecordReconcile("storagepolicy", "error", time.Since(startTime).Seconds())
		return ctrl.Result{}, err
	}

	metrics.RecordReconcile("storagepolicy", "success", time.Since(startTime).Seconds())

	// Requeue for next evaluation
	return ctrl.Result{RequeueAfter: DefaultRequeueInterval}, nil
}

// isDryRun returns true if dry-run mode is enabled either globally or for the policy
func (r *StoragePolicyReconciler) isDryRun(policyObj *cnpgv1alpha1.StoragePolicy) bool {
	return r.GlobalDryRun || policyObj.Spec.DryRun
}

// initComponents initializes internal components if not already done
func (r *StoragePolicyReconciler) initComponents() {
	if r.discovery == nil {
		r.discovery = cnpg.NewDiscovery(r.Client)
	}
	if r.metricsCollector == nil && r.RestConfig != nil {
		r.metricsCollector = metrics.NewCollector(r.Client, r.RestConfig)
	}
	if r.evaluator == nil {
		r.evaluator = policy.NewEvaluator()
	}
	if r.expansionEngine == nil {
		r.expansionEngine = remediation.NewExpansionEngine(r.Client)
	}
	if r.walCleanupEngine == nil && r.RestConfig != nil {
		// WAL cleanup engine requires rest config for pod exec
		engine, err := remediation.NewWALCleanupEngine(r.Client, r.RestConfig)
		if err == nil {
			r.walCleanupEngine = engine
		}
	}
	if r.alertManagers == nil {
		r.alertManagers = make(map[string]*alerting.AlertManager)
	}
}

// getAlertManager returns the alert manager for a policy, creating one if needed
func (r *StoragePolicyReconciler) getAlertManager(policyObj *cnpgv1alpha1.StoragePolicy) *alerting.AlertManager {
	key := fmt.Sprintf("%s/%s", policyObj.Namespace, policyObj.Name)

	if am, ok := r.alertManagers[key]; ok {
		// Update channels in case they changed
		am.UpdateChannels(policyObj.Spec.Alerting.Channels)
		return am
	}

	// Create new alert manager
	am := alerting.NewAlertManager(r.Client, policyObj.Spec.Alerting.Channels)
	r.alertManagers[key] = am
	return am
}

// handleDeletion handles the deletion of a StoragePolicy
//
//nolint:unparam // ctrl.Result always nil but kept for consistency with Reconcile signature
func (r *StoragePolicyReconciler) handleDeletion(ctx context.Context, policyObj *cnpgv1alpha1.StoragePolicy) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("Handling StoragePolicy deletion")

	if controllerutil.ContainsFinalizer(policyObj, FinalizerName) {
		// Clean up: remove annotations from managed clusters
		if err := r.cleanupManagedClusters(ctx, policyObj); err != nil {
			log.Error(err, "Failed to cleanup managed clusters")
			return ctrl.Result{}, err
		}

		// Remove finalizer
		controllerutil.RemoveFinalizer(policyObj, FinalizerName)
		if err := r.Update(ctx, policyObj); err != nil {
			log.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// cleanupManagedClusters removes annotations from all managed clusters
//
//nolint:unparam // error return kept for future extensibility
func (r *StoragePolicyReconciler) cleanupManagedClusters(ctx context.Context, policyObj *cnpgv1alpha1.StoragePolicy) error {
	log := logf.FromContext(ctx)

	for _, mc := range policyObj.Status.ManagedClusters {
		existingAnnotations, err := r.discovery.GetClusterAnnotations(ctx, mc.Name, mc.Namespace)
		if err != nil {
			log.Error(err, "Failed to get cluster annotations", "cluster", mc.Name)
			continue
		}

		// Remove our annotations
		ca := &annotations.ClusterAnnotations{}
		for k := range existingAnnotations {
			if len(k) > len(annotations.AnnotationPrefix) && k[:len(annotations.AnnotationPrefix)] == annotations.AnnotationPrefix {
				delete(existingAnnotations, k)
			}
		}

		if err := r.discovery.UpdateClusterAnnotations(ctx, mc.Name, mc.Namespace, ca.GetAnnotations()); err != nil {
			log.Error(err, "Failed to update cluster annotations", "cluster", mc.Name)
		}
	}

	return nil
}

// findMatchingClusters finds CNPG clusters matching the policy selector
func (r *StoragePolicyReconciler) findMatchingClusters(ctx context.Context, policyObj *cnpgv1alpha1.StoragePolicy) ([]cnpg.ClusterInfo, error) {
	// Get clusters by selector
	clusters, err := r.discovery.GetClustersBySelector(ctx, "", policyObj.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("failed to get clusters by selector: %w", err)
	}

	// Filter out excluded clusters
	excludedSet := make(map[string]bool)
	for _, ref := range policyObj.Spec.ExcludeClusters {
		key := fmt.Sprintf("%s/%s", ref.Namespace, ref.Name)
		excludedSet[key] = true
	}

	var filtered []cnpg.ClusterInfo
	for _, cluster := range clusters {
		key := fmt.Sprintf("%s/%s", cluster.Namespace, cluster.Name)
		if !excludedSet[key] {
			filtered = append(filtered, cluster)
		}
	}

	return filtered, nil
}

// processCluster processes a single CNPG cluster
func (r *StoragePolicyReconciler) processCluster(ctx context.Context, policyObj *cnpgv1alpha1.StoragePolicy, cluster cnpg.ClusterInfo) (*cnpgv1alpha1.ManagedCluster, error) {
	log := logf.FromContext(ctx)
	log.Info("Processing cluster", "cluster", cluster.Name, "namespace", cluster.Namespace)

	// Get cluster pods for metrics collection
	pods, err := r.discovery.GetClusterPods(ctx, cluster.Name, cluster.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster pods: %w", err)
	}

	// Collect metrics
	var clusterMetrics *metrics.ClusterMetrics
	if r.metricsCollector != nil {
		clusterMetrics, err = r.metricsCollector.CollectClusterMetrics(ctx, cluster.Name, cluster.Namespace, pods)
		if err != nil {
			log.Error(err, "Failed to collect metrics", "cluster", cluster.Name)
			// Continue without metrics - we'll use what we have
		}
	}

	// Get or create cluster annotations
	existingAnnotations, err := r.discovery.GetClusterAnnotations(ctx, cluster.Name, cluster.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster annotations: %w", err)
	}

	// Create annotations wrapper using the existing annotations map directly
	clusterAnnotations := &clusterAnnotationsWrapper{annotations: existingAnnotations}
	if clusterAnnotations.annotations == nil {
		clusterAnnotations.annotations = make(map[string]string)
	}

	// Check if cluster is paused
	if clusterAnnotations.IsPaused() {
		log.Info("Cluster is paused, skipping", "cluster", cluster.Name, "reason", clusterAnnotations.GetPauseReason())
		return &cnpgv1alpha1.ManagedCluster{
			Name:         cluster.Name,
			Namespace:    cluster.Namespace,
			LastChecked:  metav1.Now(),
			UsagePercent: 0,
			Status:       "Paused",
		}, nil
	}

	// Calculate usage
	var usagePercent float64
	if clusterMetrics != nil {
		usagePercent = clusterMetrics.TotalUsagePercent()
	}

	// Build evaluation context
	evalCtx := policy.EvaluationContext{
		ClusterName:        cluster.Name,
		Namespace:          cluster.Namespace,
		CurrentUsageBytes:  0,
		CapacityBytes:      0,
		ActiveRemediation:  false,
		CircuitBreakerOpen: clusterAnnotations.IsCircuitBreakerOpen(),
	}

	if clusterMetrics != nil {
		evalCtx.CurrentUsageBytes = clusterMetrics.TotalUsedBytes
		evalCtx.CapacityBytes = clusterMetrics.TotalCapacityBytes
	}

	// Get last action times from annotations
	evalCtx.LastExpansion = clusterAnnotations.GetLastExpansion()
	evalCtx.LastWALCleanup = clusterAnnotations.GetLastWALCleanup()

	// Perform evaluation
	evalResult, err := r.evaluator.FullEvaluation(evalCtx, policyObj)
	if err != nil {
		log.Error(err, "Evaluation failed", "cluster", cluster.Name)
		return nil, fmt.Errorf("evaluation failed: %w", err)
	}

	// Record threshold breach if applicable
	if evalResult.ThresholdResult.Level != policy.ThresholdLevelNormal {
		metrics.RecordThresholdBreach(cluster.Name, cluster.Namespace, string(evalResult.ThresholdResult.Level))
	}

	// Process recommended actions
	//nolint:goconst // "Healthy" is a descriptive status string, not a constant
	status := "Healthy"
	if evalResult.HasPendingActions() {
		action := evalResult.GetHighestPriorityAction()
		if action != nil {
			switch action.Action {
			case policy.ActionTypeExpand:
				dryRun := r.isDryRun(policyObj)
				if !dryRun {
					if err := r.handleExpansion(ctx, policyObj, cluster, evalResult, clusterAnnotations); err != nil {
						log.Error(err, "Expansion failed", "cluster", cluster.Name)
						status = "ExpansionFailed"
					} else {
						status = "Expanding"
					}
				} else {
					log.Info("DryRun: Would expand PVCs", "cluster", cluster.Name, "globalDryRun", r.GlobalDryRun, "policyDryRun", policyObj.Spec.DryRun)
					status = "DryRun-WouldExpand"
				}

			case policy.ActionTypeWALCleanup:
				dryRun := r.isDryRun(policyObj)
				if !dryRun {
					if err := r.handleWALCleanup(ctx, policyObj, cluster, clusterAnnotations); err != nil {
						log.Error(err, "WAL cleanup failed", "cluster", cluster.Name)
						status = "WALCleanupFailed"
					} else {
						status = "WALCleanup"
					}
				} else {
					log.Info("DryRun: Would cleanup WAL", "cluster", cluster.Name, "globalDryRun", r.GlobalDryRun, "policyDryRun", policyObj.Spec.DryRun)
					status = "DryRun-WouldCleanupWAL"
				}

			case policy.ActionTypeAlert:
				// Send alert if not suppressed during remediation
				if !policyObj.Spec.Alerting.SuppressDuringRemediation || status == "Healthy" {
					if err := r.handleAlert(ctx, policyObj, cluster, evalResult); err != nil {
						log.Error(err, "Failed to send alert", "cluster", cluster.Name)
					}
				}
				status = fmt.Sprintf("Alert-%s", evalResult.ThresholdResult.Level)
			}
		}
	}

	// Update cluster annotations
	clusterAnnotations.SetManaged(true)
	clusterAnnotations.SetPolicyReference(policyObj.Name, policyObj.Namespace)
	clusterAnnotations.SetLastCheck(time.Now())
	clusterAnnotations.SetCurrentUsagePercent(int32(usagePercent))

	// Update circuit breaker state metric
	metrics.SetCircuitBreakerState(cluster.Name, cluster.Namespace, clusterAnnotations.IsCircuitBreakerOpen())

	if err := r.discovery.UpdateClusterAnnotations(ctx, cluster.Name, cluster.Namespace, clusterAnnotations.GetAnnotations()); err != nil {
		log.Error(err, "Failed to update cluster annotations", "cluster", cluster.Name)
	}

	// Collect and evaluate backup status
	var backupStatus *cnpgv1alpha1.ClusterBackupStatus
	if policyObj.Spec.BackupMonitoring.Enabled {
		backupStatus = r.evaluateBackupStatus(ctx, policyObj, cluster)
	}

	return &cnpgv1alpha1.ManagedCluster{
		Name:         cluster.Name,
		Namespace:    cluster.Namespace,
		LastChecked:  metav1.Now(),
		UsagePercent: int32(usagePercent),
		Status:       status,
		BackupStatus: backupStatus,
	}, nil
}

// handleExpansion handles PVC expansion for a cluster using the remediation engine
func (r *StoragePolicyReconciler) handleExpansion(ctx context.Context, policyObj *cnpgv1alpha1.StoragePolicy, cluster cnpg.ClusterInfo, evalResult *policy.EvaluationResult, ca *clusterAnnotationsWrapper) error {
	log := logf.FromContext(ctx)

	// Check if expansion is allowed (cooldown, circuit breaker, etc.)
	if allowed, reason := ca.CanExpand(policyObj.Spec.Expansion.CooldownMinutes); !allowed {
		log.Info("Expansion not allowed", "cluster", cluster.Name, "reason", reason)
		return nil
	}

	// Get cluster PVCs
	pvcs, err := r.discovery.GetClusterPVCs(ctx, cluster.Name, cluster.Namespace)
	if err != nil {
		return fmt.Errorf("failed to get cluster PVCs: %w", err)
	}

	if len(pvcs) == 0 {
		log.Info("No PVCs found for cluster", "cluster", cluster.Name)
		return nil
	}

	// Build expansion request
	req := &remediation.ExpansionRequest{
		ClusterName:      cluster.Name,
		ClusterNamespace: cluster.Namespace,
		PVCs:             pvcs,
		Policy:           policyObj,
		Reason:           fmt.Sprintf("threshold breach: %.1f%%", evalResult.ThresholdResult.CurrentUsagePercent),
		DryRun:           r.isDryRun(policyObj),
	}

	// Execute expansion using the remediation engine
	result, err := r.expansionEngine.ExpandClusterPVCs(ctx, req)
	if err != nil {
		log.Error(err, "Expansion engine error", "cluster", cluster.Name)
		ca.IncrementFailureCount()
		return fmt.Errorf("expansion failed: %w", err)
	}

	// Process results
	if !result.Success {
		log.Info("Expansion completed with failures", "cluster", cluster.Name, "results", len(result.PVCResults))

		// Count failures
		failCount := 0
		for _, pvcResult := range result.PVCResults {
			if !pvcResult.Success && !pvcResult.Skipped {
				failCount++
				log.Error(fmt.Errorf("%s", pvcResult.Error), "PVC expansion failed", "pvc", pvcResult.PVCName)
			}
		}

		ca.IncrementFailureCount()

		// Check if we should open circuit breaker
		if ca.GetFailureCount() >= policyObj.Spec.CircuitBreaker.MaxFailures {
			ca.SetCircuitBreakerOpen(true)
			log.Info("Opening circuit breaker", "cluster", cluster.Name, "failures", ca.GetFailureCount())
		}

		return fmt.Errorf("expansion failed for %d PVCs", failCount)
	}

	// Log success details
	expandedCount := 0
	skippedCount := 0
	for _, pvcResult := range result.PVCResults {
		if pvcResult.Skipped {
			skippedCount++
			log.V(1).Info("PVC skipped", "pvc", pvcResult.PVCName, "reason", pvcResult.SkipReason)
		} else if pvcResult.Success {
			expandedCount++
			log.Info("PVC expanded successfully",
				"pvc", pvcResult.PVCName,
				"originalSize", pvcResult.OriginalSize.String(),
				"newSize", pvcResult.NewSize.String())
		}
	}

	log.Info("Expansion completed",
		"cluster", cluster.Name,
		"expanded", expandedCount,
		"skipped", skippedCount,
		"totalBytesAdded", result.TotalBytesAdded,
		"duration", result.Duration)

	// Update annotations
	ca.SetLastExpansion(time.Now())
	ca.ResetFailureCount()

	// Create StorageEvent for audit trail
	if !r.isDryRun(policyObj) {
		event, err := r.expansionEngine.CreateExpansionEvent(ctx, req, result)
		if err != nil {
			log.Error(err, "Failed to create storage event")
		} else {
			// Update event status
			if err := r.expansionEngine.UpdateExpansionEventStatus(ctx, event, result); err != nil {
				log.Error(err, "Failed to update storage event status")
			}
		}
	}

	return nil
}

// handleWALCleanup handles WAL cleanup for a cluster using the remediation engine
func (r *StoragePolicyReconciler) handleWALCleanup(ctx context.Context, policyObj *cnpgv1alpha1.StoragePolicy, cluster cnpg.ClusterInfo, ca *clusterAnnotationsWrapper) error {
	log := logf.FromContext(ctx)

	// Check if WAL cleanup is allowed
	if allowed, reason := ca.CanWALCleanup(policyObj.Spec.WALCleanup.CooldownMinutes); !allowed {
		log.Info("WAL cleanup not allowed", "cluster", cluster.Name, "reason", reason)
		return nil
	}

	// Check if WAL cleanup engine is available
	if r.walCleanupEngine == nil {
		log.Info("WAL cleanup engine not available, skipping", "cluster", cluster.Name)
		return nil
	}

	// Get primary pod
	primaryPod, err := r.discovery.GetPrimaryPod(ctx, cluster.Name, cluster.Namespace)
	if err != nil {
		return fmt.Errorf("failed to get primary pod: %w", err)
	}

	// Build cleanup request
	req := &remediation.WALCleanupRequest{
		ClusterName:      cluster.Name,
		ClusterNamespace: cluster.Namespace,
		PrimaryPod:       primaryPod,
		Policy:           policyObj,
		Reason:           "emergency threshold breach",
		DryRun:           r.isDryRun(policyObj),
	}

	// Execute WAL cleanup
	result, err := r.walCleanupEngine.CleanupClusterWAL(ctx, req)
	if err != nil {
		log.Error(err, "WAL cleanup failed", "cluster", cluster.Name)
		ca.IncrementFailureCount()

		// Check if we should open circuit breaker
		if ca.GetFailureCount() >= policyObj.Spec.CircuitBreaker.MaxFailures {
			ca.SetCircuitBreakerOpen(true)
			log.Info("Opening circuit breaker after WAL cleanup failure", "cluster", cluster.Name)
		}

		return fmt.Errorf("WAL cleanup failed: %w", err)
	}

	if !result.Success {
		log.Info("WAL cleanup completed with no files removed", "cluster", cluster.Name)
	} else {
		log.Info("WAL cleanup completed successfully",
			"cluster", cluster.Name,
			"filesRemoved", result.FilesRemoved,
			"bytesFreed", result.BytesFreed,
			"duration", result.Duration)
	}

	// Update annotations
	ca.SetLastWALCleanup(time.Now())
	ca.ResetFailureCount()

	// Create StorageEvent for audit trail
	if !r.isDryRun(policyObj) && result.FilesRemoved > 0 {
		event, err := r.walCleanupEngine.CreateWALCleanupEvent(ctx, req, result)
		if err != nil {
			log.Error(err, "Failed to create WAL cleanup event")
		} else {
			if err := r.walCleanupEngine.UpdateWALCleanupEventStatus(ctx, event, result); err != nil {
				log.Error(err, "Failed to update WAL cleanup event status")
			}
		}
	}

	return nil
}

// handleAlert handles sending alerts for a cluster
func (r *StoragePolicyReconciler) handleAlert(ctx context.Context, policyObj *cnpgv1alpha1.StoragePolicy, cluster cnpg.ClusterInfo, evalResult *policy.EvaluationResult) error {
	log := logf.FromContext(ctx)

	// Skip if no alert channels are configured
	if len(policyObj.Spec.Alerting.Channels) == 0 {
		log.V(1).Info("No alert channels configured, skipping alert", "cluster", cluster.Name)
		return nil
	}

	// Get the alert manager for this policy
	am := r.getAlertManager(policyObj)

	// Map threshold level to alert severity
	var severity alerting.AlertSeverity
	switch evalResult.ThresholdResult.Level {
	case policy.ThresholdLevelWarning:
		severity = alerting.AlertSeverityWarning
	case policy.ThresholdLevelCritical:
		severity = alerting.AlertSeverityCritical
	case policy.ThresholdLevelEmergency:
		severity = alerting.AlertSeverityEmergency
	default:
		// Don't alert for normal levels
		return nil
	}

	// Build alert
	alert := &alerting.Alert{
		ClusterName:      cluster.Name,
		ClusterNamespace: cluster.Namespace,
		Severity:         severity,
		Message:          evalResult.ThresholdResult.Message,
		Details: map[string]string{
			"usage_percent": fmt.Sprintf("%.1f", evalResult.ThresholdResult.CurrentUsagePercent),
			"threshold":     string(evalResult.ThresholdResult.Level),
			"policy":        policyObj.Name,
		},
		Timestamp: time.Now(),
	}

	// Send alert
	if err := am.SendAlert(ctx, alert); err != nil {
		log.Error(err, "Failed to send alert", "cluster", cluster.Name, "severity", severity)
		return err
	}

	log.Info("Alert sent successfully", "cluster", cluster.Name, "severity", severity)
	return nil
}

// evaluateBackupStatus evaluates the backup status of a cluster and sends alerts if needed
func (r *StoragePolicyReconciler) evaluateBackupStatus(
	ctx context.Context,
	policyObj *cnpgv1alpha1.StoragePolicy,
	cluster cnpg.ClusterInfo,
) *cnpgv1alpha1.ClusterBackupStatus {
	log := logf.FromContext(ctx)

	status := &cnpgv1alpha1.ClusterBackupStatus{
		BackupConfigured:           cluster.Status.BackupConfigured,
		ContinuousArchivingWorking: cluster.Status.ContinuousArchivingWorking,
		BackupHealthStatus:         "Healthy",
	}

	now := time.Now()
	config := policyObj.Spec.BackupMonitoring
	healthy := true
	var alertReasons []string

	// Check if backup is configured
	if !cluster.Status.BackupConfigured && config.AlertOnNoBackupConfigured {
		healthy = false
		status.BackupHealthStatus = "NoBackupConfigured"
		alertReasons = append(alertReasons, "no backup configured")
		metrics.RecordBackupAlert(cluster.Name, cluster.Namespace, "no_backup_configured")
		log.Info("Cluster has no backup configured",
			"cluster", cluster.Name, "namespace", cluster.Namespace)
	}

	// Get backup timestamps - check ObjectStore first if barman-cloud plugin is configured
	var lastSuccessfulBackup *time.Time
	var firstRecoverabilityPoint *time.Time

	if cluster.Status.BarmanCloudPlugin != nil && cluster.Status.BarmanCloudPlugin.Enabled {
		// Get backup status from ObjectStore CRD
		objectStoreStatus, err := r.discovery.GetBackupStatusForCluster(ctx, cluster)
		if err != nil {
			log.Error(err, "Failed to get ObjectStore backup status, falling back to cluster status",
				"cluster", cluster.Name, "objectStore", cluster.Status.BarmanCloudPlugin.ObjectStoreName)
		} else if objectStoreStatus != nil {
			lastSuccessfulBackup = objectStoreStatus.LastSuccessfulBackupTime
			firstRecoverabilityPoint = objectStoreStatus.FirstRecoverabilityPoint
			log.V(1).Info("Using backup status from ObjectStore",
				"cluster", cluster.Name,
				"objectStore", cluster.Status.BarmanCloudPlugin.ObjectStoreName,
				"lastBackup", lastSuccessfulBackup,
				"firstRecovery", firstRecoverabilityPoint)
		}
	}

	// Fall back to cluster status if ObjectStore didn't provide timestamps
	if lastSuccessfulBackup == nil {
		lastSuccessfulBackup = cluster.Status.LastSuccessfulBackup
	}
	if firstRecoverabilityPoint == nil {
		firstRecoverabilityPoint = cluster.Status.FirstRecoverabilityPoint
	}

	// Check last successful backup
	if lastSuccessfulBackup != nil {
		t := metav1.NewTime(*lastSuccessfulBackup)
		status.LastBackupTime = &t
		backupAge := now.Sub(*lastSuccessfulBackup)
		status.LastBackupAgeHours = int32(backupAge.Hours())

		// Record metrics
		ts := float64(lastSuccessfulBackup.Unix())
		metrics.RecordBackupMetrics(cluster.Name, cluster.Namespace, &ts, nil,
			cluster.Status.ContinuousArchivingWorking, cluster.Status.BackupConfigured, healthy)
		metrics.RecordBackupAge(cluster.Name, cluster.Namespace, backupAge.Hours())

		// Check if backup is too old
		if config.MaxBackupAgeHours > 0 && status.LastBackupAgeHours > config.MaxBackupAgeHours {
			healthy = false
			status.BackupHealthStatus = "BackupTooOld"
			alertReasons = append(alertReasons, fmt.Sprintf(
				"last backup is %d hours old (max: %d)",
				status.LastBackupAgeHours, config.MaxBackupAgeHours))
			metrics.RecordBackupAlert(cluster.Name, cluster.Namespace, "backup_too_old")
			log.Info("Cluster backup is too old",
				"cluster", cluster.Name, "namespace", cluster.Namespace,
				"ageHours", status.LastBackupAgeHours, "maxHours", config.MaxBackupAgeHours)
		}
	} else if cluster.Status.BackupConfigured {
		// Backup is configured but no successful backup recorded
		healthy = false
		status.BackupHealthStatus = "NoSuccessfulBackup"
		alertReasons = append(alertReasons, "no successful backup recorded")
		metrics.RecordBackupAlert(cluster.Name, cluster.Namespace, "no_successful_backup")
		log.Info("Cluster has no successful backup",
			"cluster", cluster.Name, "namespace", cluster.Namespace)
	}

	// Check first recoverability point
	if firstRecoverabilityPoint != nil {
		t := metav1.NewTime(*firstRecoverabilityPoint)
		status.FirstRecoverabilityPoint = &t
		recoverabilityAge := now.Sub(*firstRecoverabilityPoint)

		// Record metrics
		ts := float64(firstRecoverabilityPoint.Unix())
		metrics.RecordBackupMetrics(cluster.Name, cluster.Namespace, nil, &ts,
			cluster.Status.ContinuousArchivingWorking, cluster.Status.BackupConfigured, healthy)
		metrics.RecordFirstRecoverabilityAge(cluster.Name, cluster.Namespace, recoverabilityAge.Hours())

		// Check if first recoverability point is too old
		ageHours := int32(recoverabilityAge.Hours())
		if config.MaxRecoveryPointAgeHours > 0 && ageHours > config.MaxRecoveryPointAgeHours {
			healthy = false
			if status.BackupHealthStatus == "Healthy" {
				status.BackupHealthStatus = "RecoveryPointTooOld"
			}
			alertReasons = append(alertReasons, fmt.Sprintf(
				"first recovery point is %d hours old (max: %d)",
				ageHours, config.MaxRecoveryPointAgeHours))
			metrics.RecordBackupAlert(cluster.Name, cluster.Namespace, "recovery_point_too_old")
			log.Info("Cluster recovery point is too old",
				"cluster", cluster.Name, "namespace", cluster.Namespace,
				"ageHours", ageHours, "maxHours", config.MaxRecoveryPointAgeHours)
		}
	}

	// Check continuous archiving status
	// For barman-cloud plugin, also check if the plugin is configured as WAL archiver
	archivingRequired := config.RequireContinuousArchiving && cluster.Status.BackupConfigured
	archivingWorking := cluster.Status.ContinuousArchivingWorking
	if cluster.Status.BarmanCloudPlugin != nil && cluster.Status.BarmanCloudPlugin.IsWALArchiver {
		// If using barman-cloud as WAL archiver and we have recovery point, archiving is working
		archivingWorking = archivingWorking || (firstRecoverabilityPoint != nil)
	}

	if archivingRequired && !archivingWorking {
		healthy = false
		if status.BackupHealthStatus == "Healthy" {
			status.BackupHealthStatus = "ArchivingNotWorking"
		}
		alertReasons = append(alertReasons, "continuous WAL archiving is not working")
		metrics.RecordBackupAlert(cluster.Name, cluster.Namespace, "archiving_not_working")
		log.Info("Cluster WAL archiving is not working",
			"cluster", cluster.Name, "namespace", cluster.Namespace)
	}

	// Update healthy status
	if healthy {
		status.BackupHealthStatus = "Healthy"
	}

	// Record overall backup health metric
	metrics.RecordBackupMetrics(cluster.Name, cluster.Namespace, nil, nil,
		archivingWorking, cluster.Status.BackupConfigured, healthy)

	// Send alerts for backup issues
	if len(alertReasons) > 0 {
		r.sendBackupAlert(ctx, policyObj, cluster, alertReasons)
	}

	return status
}

// sendBackupAlert sends an alert for backup issues
func (r *StoragePolicyReconciler) sendBackupAlert(ctx context.Context, policyObj *cnpgv1alpha1.StoragePolicy, cluster cnpg.ClusterInfo, reasons []string) {
	log := logf.FromContext(ctx)

	// Skip if no alert channels are configured
	if len(policyObj.Spec.Alerting.Channels) == 0 {
		log.V(1).Info("No alert channels configured, skipping backup alert", "cluster", cluster.Name)
		return
	}

	// Get the alert manager for this policy
	am := r.getAlertManager(policyObj)

	// Build alert message
	message := fmt.Sprintf("Backup issues for cluster %s/%s: %s", cluster.Namespace, cluster.Name, reasons[0])
	if len(reasons) > 1 {
		message = fmt.Sprintf("Multiple backup issues for cluster %s/%s: %v", cluster.Namespace, cluster.Name, reasons)
	}

	// Determine severity based on issues
	severity := alerting.AlertSeverityWarning
	for _, reason := range reasons {
		if reason == "no backup configured" || reason == "no successful backup recorded" || strings.Contains(reason, "archiving is not working") {
			severity = alerting.AlertSeverityCritical
			break
		}
	}

	alert := &alerting.Alert{
		ClusterName:      cluster.Name,
		ClusterNamespace: cluster.Namespace,
		Severity:         severity,
		Message:          message,
		Details: map[string]string{
			"alert_type":  "backup",
			"policy":      policyObj.Name,
			"issue_count": fmt.Sprintf("%d", len(reasons)),
		},
		Timestamp: time.Now(),
	}

	// Add each reason as a detail
	for i, reason := range reasons {
		alert.Details[fmt.Sprintf("issue_%d", i+1)] = reason
	}

	// Send alert
	if err := am.SendAlert(ctx, alert); err != nil {
		log.Error(err, "Failed to send backup alert", "cluster", cluster.Name, "severity", severity)
		return
	}

	log.Info("Backup alert sent successfully", "cluster", cluster.Name, "severity", severity, "issues", len(reasons))
}

// setCondition sets a condition on the StoragePolicy status
//
//nolint:unparam // conditionType parameter kept for potential future use with multiple condition types
func (r *StoragePolicyReconciler) setCondition(policyObj *cnpgv1alpha1.StoragePolicy, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: policyObj.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
	meta.SetStatusCondition(&policyObj.Status.Conditions, condition)
}

// SetupWithManager sets up the controller with the Manager.
func (r *StoragePolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cnpgv1alpha1.StoragePolicy{}).
		Named("storagepolicy").
		Complete(r)
}

// clusterAnnotationsWrapper wraps annotations.ClusterAnnotations functionality
// using the annotations from the cluster
type clusterAnnotationsWrapper struct {
	annotations map[string]string
}

func (c *clusterAnnotationsWrapper) GetAnnotations() map[string]string {
	return c.annotations
}

func (c *clusterAnnotationsWrapper) IsManaged() bool {
	//nolint:goconst // "true" comparison with annotation value
	return c.annotations[annotations.AnnotationManaged] == "true"
}

func (c *clusterAnnotationsWrapper) SetManaged(managed bool) {
	if managed {
		c.annotations[annotations.AnnotationManaged] = "true"
	} else {
		c.annotations[annotations.AnnotationManaged] = "false"
	}
}

func (c *clusterAnnotationsWrapper) IsPaused() bool {
	if c.annotations[annotations.AnnotationPaused] != "true" {
		return false
	}
	// Check if pause has expired
	if pauseUntil, ok := c.annotations[annotations.AnnotationPauseUntil]; ok {
		if t, err := time.Parse(time.RFC3339, pauseUntil); err == nil {
			if time.Now().After(t) {
				return false
			}
		}
	}
	return true
}

func (c *clusterAnnotationsWrapper) GetPauseReason() string {
	return c.annotations[annotations.AnnotationPauseReason]
}

func (c *clusterAnnotationsWrapper) SetPolicyReference(name, namespace string) {
	c.annotations[annotations.AnnotationPolicyName] = name
	c.annotations[annotations.AnnotationPolicyNamespace] = namespace
}

func (c *clusterAnnotationsWrapper) SetLastCheck(t time.Time) {
	c.annotations[annotations.AnnotationLastCheck] = t.Format(time.RFC3339)
}

func (c *clusterAnnotationsWrapper) SetCurrentUsagePercent(percent int32) {
	c.annotations[annotations.AnnotationCurrentUsagePercent] = fmt.Sprintf("%d", percent)
}

func (c *clusterAnnotationsWrapper) GetLastExpansion() *time.Time {
	if ts, ok := c.annotations[annotations.AnnotationLastExpansion]; ok {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			return &t
		}
	}
	return nil
}

func (c *clusterAnnotationsWrapper) SetLastExpansion(t time.Time) {
	c.annotations[annotations.AnnotationLastExpansion] = t.Format(time.RFC3339)
}

func (c *clusterAnnotationsWrapper) GetLastWALCleanup() *time.Time {
	if ts, ok := c.annotations[annotations.AnnotationWALCleanupLast]; ok {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			return &t
		}
	}
	return nil
}

func (c *clusterAnnotationsWrapper) SetLastWALCleanup(t time.Time) {
	c.annotations[annotations.AnnotationWALCleanupLast] = t.Format(time.RFC3339)
}

func (c *clusterAnnotationsWrapper) IsCircuitBreakerOpen() bool {
	return c.annotations[annotations.AnnotationCircuitBreakerOpen] == "true"
}

func (c *clusterAnnotationsWrapper) SetCircuitBreakerOpen(open bool) {
	if open {
		c.annotations[annotations.AnnotationCircuitBreakerOpen] = "true"
	} else {
		c.annotations[annotations.AnnotationCircuitBreakerOpen] = "false"
	}
}

func (c *clusterAnnotationsWrapper) GetFailureCount() int32 {
	if v, ok := c.annotations[annotations.AnnotationFailureCount]; ok {
		var count int32
		if _, err := fmt.Sscanf(v, "%d", &count); err == nil {
			return count
		}
	}
	return 0
}

func (c *clusterAnnotationsWrapper) SetFailureCount(count int32) {
	c.annotations[annotations.AnnotationFailureCount] = fmt.Sprintf("%d", count)
}

func (c *clusterAnnotationsWrapper) IncrementFailureCount() int32 {
	count := c.GetFailureCount() + 1
	c.SetFailureCount(count)
	c.annotations[annotations.AnnotationLastFailure] = time.Now().Format(time.RFC3339)
	return count
}

func (c *clusterAnnotationsWrapper) ResetFailureCount() {
	c.SetFailureCount(0)
	delete(c.annotations, annotations.AnnotationLastFailure)
}

func (c *clusterAnnotationsWrapper) CanExpand(cooldownMinutes int32) (bool, string) {
	if c.IsPaused() {
		return false, fmt.Sprintf("cluster is paused: %s", c.GetPauseReason())
	}
	if c.IsCircuitBreakerOpen() {
		return false, "circuit breaker is open"
	}
	lastExpansion := c.GetLastExpansion()
	if lastExpansion != nil {
		cooldown := time.Duration(cooldownMinutes) * time.Minute
		nextAllowed := lastExpansion.Add(cooldown)
		if time.Now().Before(nextAllowed) {
			remaining := time.Until(nextAllowed).Round(time.Second)
			return false, fmt.Sprintf("cooldown active, %s remaining", remaining)
		}
	}
	return true, ""
}

func (c *clusterAnnotationsWrapper) CanWALCleanup(cooldownMinutes int32) (bool, string) {
	if c.IsPaused() {
		return false, fmt.Sprintf("cluster is paused: %s", c.GetPauseReason())
	}
	if c.IsCircuitBreakerOpen() {
		return false, "circuit breaker is open"
	}
	lastCleanup := c.GetLastWALCleanup()
	if lastCleanup != nil {
		cooldown := time.Duration(cooldownMinutes) * time.Minute
		nextAllowed := lastCleanup.Add(cooldown)
		if time.Now().Before(nextAllowed) {
			remaining := time.Until(nextAllowed).Round(time.Second)
			return false, fmt.Sprintf("cooldown active, %s remaining", remaining)
		}
	}
	return true, ""
}

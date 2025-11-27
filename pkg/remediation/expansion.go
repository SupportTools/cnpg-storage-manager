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

package remediation

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cnpgv1alpha1 "github.com/supporttools/cnpg-storage-manager/api/v1alpha1"
	"github.com/supporttools/cnpg-storage-manager/pkg/metrics"
)

// ExpansionEngine handles PVC expansion operations
type ExpansionEngine struct {
	client    client.Client
	validator *StorageClassValidator
}

// NewExpansionEngine creates a new expansion engine
func NewExpansionEngine(c client.Client) *ExpansionEngine {
	return &ExpansionEngine{
		client:    c,
		validator: NewStorageClassValidator(c),
	}
}

// ExpansionRequest represents a request to expand PVCs
type ExpansionRequest struct {
	ClusterName      string
	ClusterNamespace string
	PVCs             []corev1.PersistentVolumeClaim
	Policy           *cnpgv1alpha1.StoragePolicy
	Reason           string
	DryRun           bool
}

// ExpansionResult contains the result of an expansion operation
type ExpansionResult struct {
	ClusterName      string
	ClusterNamespace string
	Success          bool
	PVCResults       []PVCExpansionResult
	TotalBytesAdded  int64
	Duration         time.Duration
	Error            error
}

// PVCExpansionResult contains the result for a single PVC
type PVCExpansionResult struct {
	PVCName      string
	Namespace    string
	OriginalSize resource.Quantity
	NewSize      resource.Quantity
	BytesAdded   int64
	Success      bool
	Error        string
	Skipped      bool
	SkipReason   string
}

// ExpandClusterPVCs expands all PVCs for a cluster
func (e *ExpansionEngine) ExpandClusterPVCs(ctx context.Context, req *ExpansionRequest) (*ExpansionResult, error) {
	logger := log.FromContext(ctx)
	startTime := time.Now()

	result := &ExpansionResult{
		ClusterName:      req.ClusterName,
		ClusterNamespace: req.ClusterNamespace,
		PVCResults:       make([]PVCExpansionResult, 0, len(req.PVCs)),
	}

	logger.Info("Starting cluster PVC expansion",
		"cluster", req.ClusterName,
		"namespace", req.ClusterNamespace,
		"pvcCount", len(req.PVCs),
		"dryRun", req.DryRun,
	)

	if len(req.PVCs) == 0 {
		result.Success = true
		result.Duration = time.Since(startTime)
		return result, nil
	}

	// Calculate expansion parameters
	expansionConfig := req.Policy.Spec.Expansion
	percentage := getExpansionPercentage(expansionConfig.Percentage)
	minIncrement := getMinIncrementBytes(expansionConfig.MinIncrementGi)
	maxSize := getMaxSizeBytes(expansionConfig.MaxSize)

	// Process each PVC
	var successCount, failCount, skipCount int

	for i := range req.PVCs {
		pvc := &req.PVCs[i]
		pvcResult := e.expandSinglePVC(ctx, pvc, percentage, minIncrement, maxSize, req.DryRun)
		result.PVCResults = append(result.PVCResults, pvcResult)

		if pvcResult.Skipped {
			skipCount++
		} else if pvcResult.Success {
			successCount++
			result.TotalBytesAdded += pvcResult.BytesAdded
		} else {
			failCount++
		}
	}

	result.Duration = time.Since(startTime)
	result.Success = failCount == 0

	// Record metrics
	if result.Success {
		metrics.RecordExpansion(req.ClusterName, req.ClusterNamespace, "success", result.TotalBytesAdded)
	} else {
		metrics.RecordExpansion(req.ClusterName, req.ClusterNamespace, "failure", 0)
	}

	logger.Info("Completed cluster PVC expansion",
		"cluster", req.ClusterName,
		"success", result.Success,
		"expanded", successCount,
		"failed", failCount,
		"skipped", skipCount,
		"totalBytesAdded", result.TotalBytesAdded,
		"duration", result.Duration,
	)

	return result, nil
}

// expandSinglePVC expands a single PVC
func (e *ExpansionEngine) expandSinglePVC(ctx context.Context, pvc *corev1.PersistentVolumeClaim, percentage int32, minIncrement, maxSize int64, dryRun bool) PVCExpansionResult {
	logger := log.FromContext(ctx)

	result := PVCExpansionResult{
		PVCName:   pvc.Name,
		Namespace: pvc.Namespace,
	}

	// Get current size
	currentSize := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	result.OriginalSize = currentSize
	currentBytes := currentSize.Value()

	// Get storage class name
	storageClassName := ""
	if pvc.Spec.StorageClassName != nil {
		storageClassName = *pvc.Spec.StorageClassName
	}

	// Validate storage class supports expansion
	pvcInfo := &PVCInfo{
		Name:             pvc.Name,
		Namespace:        pvc.Namespace,
		StorageClassName: storageClassName,
		Phase:            string(pvc.Status.Phase),
		AccessModes:      accessModesToStrings(pvc.Spec.AccessModes),
		CurrentSize:      currentBytes,
	}

	preflight, err := e.validator.ValidatePVCForExpansion(ctx, pvcInfo)
	if err != nil {
		result.Error = fmt.Sprintf("preflight validation error: %v", err)
		return result
	}

	if !preflight.CanExpand {
		result.Skipped = true
		result.SkipReason = preflight.Summary()
		return result
	}

	// Calculate new size
	increaseBytes := currentBytes * int64(percentage) / 100
	if increaseBytes < minIncrement {
		increaseBytes = minIncrement
	}

	newBytes := currentBytes + increaseBytes

	// Check max size limit
	if maxSize > 0 && newBytes > maxSize {
		if currentBytes >= maxSize {
			result.Skipped = true
			result.SkipReason = fmt.Sprintf("PVC already at max size (%s)", formatBytes(maxSize))
			return result
		}
		newBytes = maxSize
		increaseBytes = newBytes - currentBytes
	}

	newSize := resource.NewQuantity(newBytes, resource.BinarySI)
	result.NewSize = *newSize
	result.BytesAdded = increaseBytes

	logger.Info("Expanding PVC",
		"pvc", pvc.Name,
		"namespace", pvc.Namespace,
		"currentSize", currentSize.String(),
		"newSize", newSize.String(),
		"increase", formatBytes(increaseBytes),
		"dryRun", dryRun,
	)

	if dryRun {
		result.Success = true
		return result
	}

	// Perform the actual expansion
	pvcCopy := pvc.DeepCopy()
	pvcCopy.Spec.Resources.Requests[corev1.ResourceStorage] = *newSize

	if err := e.client.Update(ctx, pvcCopy); err != nil {
		result.Error = fmt.Sprintf("failed to update PVC: %v", err)
		logger.Error(err, "Failed to expand PVC", "pvc", pvc.Name)
		return result
	}

	result.Success = true
	return result
}

// VerifyExpansion verifies that a PVC expansion completed successfully
func (e *ExpansionEngine) VerifyExpansion(ctx context.Context, pvc *corev1.PersistentVolumeClaim, expectedSize resource.Quantity, timeout time.Duration) (*VerificationResult, error) {
	logger := log.FromContext(ctx)

	result := &VerificationResult{
		PVCName:      pvc.Name,
		Namespace:    pvc.Namespace,
		ExpectedSize: expectedSize,
	}

	startTime := time.Now()
	deadline := startTime.Add(timeout)

	for time.Now().Before(deadline) {
		// Fetch current PVC state
		var currentPVC corev1.PersistentVolumeClaim
		if err := e.client.Get(ctx, client.ObjectKeyFromObject(pvc), &currentPVC); err != nil {
			return nil, fmt.Errorf("failed to get PVC: %w", err)
		}

		// Check for FileSystemResizePending condition
		resizePending := false
		for _, cond := range currentPVC.Status.Conditions {
			if cond.Type == corev1.PersistentVolumeClaimFileSystemResizePending {
				resizePending = cond.Status == corev1.ConditionTrue
				result.FileSystemResizePending = resizePending
				break
			}
		}

		// Get actual capacity
		if currentPVC.Status.Capacity != nil {
			actualSize := currentPVC.Status.Capacity[corev1.ResourceStorage]
			result.ActualSize = actualSize

			// Check if expansion is complete
			if actualSize.Cmp(expectedSize) >= 0 && !resizePending {
				result.Complete = true
				result.Duration = time.Since(startTime)
				logger.Info("PVC expansion verified",
					"pvc", pvc.Name,
					"actualSize", actualSize.String(),
					"expectedSize", expectedSize.String(),
					"duration", result.Duration,
				)
				return result, nil
			}
		}

		// Wait before checking again
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}

	result.Complete = false
	result.Duration = time.Since(startTime)
	result.Error = "timeout waiting for expansion to complete"

	logger.Info("PVC expansion verification timed out",
		"pvc", pvc.Name,
		"timeout", timeout,
		"fileSystemResizePending", result.FileSystemResizePending,
	)

	return result, nil
}

// VerificationResult contains the result of expansion verification
type VerificationResult struct {
	PVCName                 string
	Namespace               string
	ExpectedSize            resource.Quantity
	ActualSize              resource.Quantity
	Complete                bool
	FileSystemResizePending bool
	Duration                time.Duration
	Error                   string
}

// GetValidator returns the storage class validator
func (e *ExpansionEngine) GetValidator() *StorageClassValidator {
	return e.validator
}

// Helper functions

func getExpansionPercentage(configValue int32) int32 {
	if configValue <= 0 {
		return 50 // Default 50%
	}
	return configValue
}

func getMinIncrementBytes(configValueGi int32) int64 {
	if configValueGi <= 0 {
		configValueGi = 5 // Default 5Gi
	}
	return int64(configValueGi) * 1024 * 1024 * 1024
}

func getMaxSizeBytes(maxSize *resource.Quantity) int64 {
	if maxSize == nil {
		return 0 // No limit
	}
	return maxSize.Value()
}

func accessModesToStrings(modes []corev1.PersistentVolumeAccessMode) []string {
	result := make([]string, len(modes))
	for i, mode := range modes {
		result[i] = string(mode)
	}
	return result
}

func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.2fTi", float64(bytes)/TB)
	case bytes >= GB:
		return fmt.Sprintf("%.2fGi", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2fMi", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2fKi", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d bytes", bytes)
	}
}

// CreateExpansionEvent creates a StorageEvent for an expansion operation
func (e *ExpansionEngine) CreateExpansionEvent(ctx context.Context, req *ExpansionRequest, result *ExpansionResult) (*cnpgv1alpha1.StorageEvent, error) {
	// Build affected PVCs list
	affectedPVCs := make([]cnpgv1alpha1.AffectedPVC, 0, len(result.PVCResults))
	for _, pvcResult := range result.PVCResults {
		if !pvcResult.Skipped {
			affectedPVCs = append(affectedPVCs, cnpgv1alpha1.AffectedPVC{
				Name: pvcResult.PVCName,
			})
		}
	}

	// Determine original and requested sizes from first non-skipped PVC
	var originalSize, requestedSize resource.Quantity
	for _, pvcResult := range result.PVCResults {
		if !pvcResult.Skipped {
			originalSize = pvcResult.OriginalSize
			requestedSize = pvcResult.NewSize
			break
		}
	}

	event := &cnpgv1alpha1.StorageEvent{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-expansion-", req.ClusterName),
			Namespace:    req.ClusterNamespace,
			Labels: map[string]string{
				"cnpg.supporttools.io/cluster":    req.ClusterName,
				"cnpg.supporttools.io/event-type": string(cnpgv1alpha1.EventTypeExpansion),
			},
		},
		Spec: cnpgv1alpha1.StorageEventSpec{
			ClusterRef: cnpgv1alpha1.ClusterReference{
				Name:      req.ClusterName,
				Namespace: req.ClusterNamespace,
			},
			PolicyRef: cnpgv1alpha1.PolicyReference{
				Name:      req.Policy.Name,
				Namespace: req.Policy.Namespace,
			},
			EventType: cnpgv1alpha1.EventTypeExpansion,
			Trigger:   cnpgv1alpha1.TriggerTypeThresholdBreach,
			Reason:    req.Reason,
			Expansion: &cnpgv1alpha1.ExpansionDetails{
				OriginalSize:  originalSize,
				RequestedSize: requestedSize,
				AffectedPVCs:  affectedPVCs,
			},
			DryRun: req.DryRun,
		},
	}

	if err := e.client.Create(ctx, event); err != nil {
		return nil, fmt.Errorf("failed to create storage event: %w", err)
	}

	return event, nil
}

// UpdateExpansionEventStatus updates the status of an expansion event
func (e *ExpansionEngine) UpdateExpansionEventStatus(ctx context.Context, event *cnpgv1alpha1.StorageEvent, result *ExpansionResult) error {
	// Build PVC statuses
	pvcStatuses := make([]cnpgv1alpha1.PVCStatus, 0, len(result.PVCResults))
	for _, pvcResult := range result.PVCResults {
		if pvcResult.Skipped {
			continue
		}

		status := cnpgv1alpha1.PVCStatus{
			Name:         pvcResult.PVCName,
			OriginalSize: &pvcResult.OriginalSize,
			NewSize:      &pvcResult.NewSize,
		}

		if pvcResult.Success {
			status.Phase = cnpgv1alpha1.PVCPhaseCompleted
		} else {
			status.Phase = cnpgv1alpha1.PVCPhaseFailed
			status.Error = pvcResult.Error
		}

		pvcStatuses = append(pvcStatuses, status)
	}

	// Update event status
	now := metav1.Now()
	if event.Status.StartTime == nil {
		event.Status.StartTime = &now
	}

	if result.Success {
		event.Status.Phase = cnpgv1alpha1.EventPhaseCompleted
	} else {
		event.Status.Phase = cnpgv1alpha1.EventPhaseFailed
	}

	event.Status.CompletionTime = &now
	event.Status.PVCStatuses = pvcStatuses
	event.Status.Message = fmt.Sprintf("Expansion completed: %d PVCs, %s added",
		len(pvcStatuses), formatBytes(result.TotalBytesAdded))

	return e.client.Status().Update(ctx, event)
}

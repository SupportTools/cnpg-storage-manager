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
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cnpgv1alpha1 "github.com/supporttools/cnpg-storage-manager/api/v1alpha1"
	"github.com/supporttools/cnpg-storage-manager/pkg/metrics"
)

// WALCleanupEngine handles WAL file cleanup operations
type WALCleanupEngine struct {
	client     client.Client
	restConfig *rest.Config
	clientset  kubernetes.Interface
}

// NewWALCleanupEngine creates a new WAL cleanup engine
func NewWALCleanupEngine(c client.Client, restConfig *rest.Config) (*WALCleanupEngine, error) {
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	return &WALCleanupEngine{
		client:     c,
		restConfig: restConfig,
		clientset:  clientset,
	}, nil
}

// WALCleanupRequest represents a request to cleanup WAL files
type WALCleanupRequest struct {
	ClusterName      string
	ClusterNamespace string
	PrimaryPod       *corev1.Pod
	Policy           *cnpgv1alpha1.StoragePolicy
	Reason           string
	DryRun           bool
}

// WALCleanupResult contains the result of a WAL cleanup operation
type WALCleanupResult struct {
	ClusterName      string
	ClusterNamespace string
	PodName          string
	Success          bool
	FilesRemoved     int
	BytesFreed       int64
	WALFilesChecked  int
	ArchivedCount    int
	RetainedCount    int
	Duration         time.Duration
	Error            string
}

// WALFileInfo represents information about a WAL file
type WALFileInfo struct {
	Name       string
	Size       int64
	ModTime    time.Time
	IsArchived bool
}

// CleanupClusterWAL performs WAL cleanup for a cluster
func (e *WALCleanupEngine) CleanupClusterWAL(ctx context.Context, req *WALCleanupRequest) (*WALCleanupResult, error) {
	logger := log.FromContext(ctx)
	startTime := time.Now()

	result := &WALCleanupResult{
		ClusterName:      req.ClusterName,
		ClusterNamespace: req.ClusterNamespace,
		PodName:          req.PrimaryPod.Name,
	}

	logger.Info("Starting WAL cleanup",
		"cluster", req.ClusterName,
		"namespace", req.ClusterNamespace,
		"pod", req.PrimaryPod.Name,
		"dryRun", req.DryRun,
	)

	// Get WAL directory path (default CNPG path)
	walDir := "/var/lib/postgresql/data/pgdata/pg_wal"

	// List WAL files
	walFiles, err := e.listWALFiles(ctx, req.PrimaryPod, walDir)
	if err != nil {
		result.Error = fmt.Sprintf("failed to list WAL files: %v", err)
		result.Duration = time.Since(startTime)
		return result, err
	}

	result.WALFilesChecked = len(walFiles)
	logger.Info("Found WAL files", "count", len(walFiles))

	// Get archived WAL status if required
	if req.Policy.Spec.WALCleanup.RequireArchived {
		archivedFiles, err := e.getArchivedWALStatus(ctx, req.PrimaryPod)
		if err != nil {
			logger.Error(err, "Failed to get archived WAL status, proceeding with caution")
		} else {
			// Mark files as archived
			archivedSet := make(map[string]bool)
			for _, af := range archivedFiles {
				archivedSet[af] = true
			}
			for i := range walFiles {
				walFiles[i].IsArchived = archivedSet[walFiles[i].Name]
				if walFiles[i].IsArchived {
					result.ArchivedCount++
				}
			}
		}
	} else {
		// If not requiring archived status, assume all are safe
		for i := range walFiles {
			walFiles[i].IsArchived = true
		}
		result.ArchivedCount = len(walFiles)
	}

	// Sort by name (which is chronological for WAL files)
	sort.Slice(walFiles, func(i, j int) bool {
		return walFiles[i].Name < walFiles[j].Name
	})

	// Determine files to remove
	retainCount := int(req.Policy.Spec.WALCleanup.RetainCount)
	if retainCount <= 0 {
		retainCount = 10 // Default
	}

	var filesToRemove []WALFileInfo
	if len(walFiles) > retainCount {
		// Keep only the latest retainCount files
		cutoffIndex := len(walFiles) - retainCount
		for i := 0; i < cutoffIndex; i++ {
			file := walFiles[i]
			// Only remove archived files
			if file.IsArchived || !req.Policy.Spec.WALCleanup.RequireArchived {
				filesToRemove = append(filesToRemove, file)
			}
		}
	}

	result.RetainedCount = len(walFiles) - len(filesToRemove)

	logger.Info("WAL cleanup analysis",
		"totalFiles", len(walFiles),
		"toRemove", len(filesToRemove),
		"toRetain", result.RetainedCount,
		"archivedCount", result.ArchivedCount,
	)

	if len(filesToRemove) == 0 {
		logger.Info("No WAL files to remove")
		result.Success = true
		result.Duration = time.Since(startTime)
		return result, nil
	}

	if req.DryRun {
		logger.Info("DryRun: Would remove WAL files", "count", len(filesToRemove))
		result.Success = true
		result.FilesRemoved = len(filesToRemove)
		for _, f := range filesToRemove {
			result.BytesFreed += f.Size
		}
		result.Duration = time.Since(startTime)
		return result, nil
	}

	// Remove files
	for _, file := range filesToRemove {
		filePath := filepath.Join(walDir, file.Name)
		if err := e.removeFile(ctx, req.PrimaryPod, filePath); err != nil {
			logger.Error(err, "Failed to remove WAL file", "file", file.Name)
			continue
		}
		result.FilesRemoved++
		result.BytesFreed += file.Size
	}

	result.Success = result.FilesRemoved > 0
	result.Duration = time.Since(startTime)

	// Record metrics
	if result.Success {
		metrics.RecordWALCleanup(req.ClusterName, req.ClusterNamespace, "success")
		metrics.WALFilesRemoved.WithLabelValues(req.ClusterName, req.ClusterNamespace).Add(float64(result.FilesRemoved))
	} else {
		metrics.RecordWALCleanup(req.ClusterName, req.ClusterNamespace, "failure")
	}

	logger.Info("WAL cleanup completed",
		"filesRemoved", result.FilesRemoved,
		"bytesFreed", result.BytesFreed,
		"duration", result.Duration,
	)

	return result, nil
}

// listWALFiles lists WAL files in the specified directory
func (e *WALCleanupEngine) listWALFiles(ctx context.Context, pod *corev1.Pod, walDir string) ([]WALFileInfo, error) {
	// Execute command to list WAL files with their sizes
	cmd := fmt.Sprintf("ls -la %s 2>/dev/null | grep -E '^-' | awk '{print $5, $9}'", walDir)
	output, err := e.execInPod(ctx, pod, "postgres", []string{"sh", "-c", cmd})
	if err != nil {
		return nil, fmt.Errorf("failed to list WAL files: %w", err)
	}

	var files []WALFileInfo
	walFilePattern := regexp.MustCompile(`^[0-9A-F]{24}$`)

	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}

		size, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			continue
		}

		name := parts[1]
		// Only include actual WAL files (24 hex characters)
		if walFilePattern.MatchString(name) {
			files = append(files, WALFileInfo{
				Name: name,
				Size: size,
			})
		}
	}

	return files, nil
}

// getArchivedWALStatus gets the list of archived WAL files
//
//nolint:unparam // error return kept for future extensibility
func (e *WALCleanupEngine) getArchivedWALStatus(ctx context.Context, pod *corev1.Pod) ([]string, error) {
	// Query PostgreSQL for the last archived WAL segment
	cmd := "psql -At -c \"SELECT file_name FROM pg_ls_archive_statusdir() WHERE name LIKE '%.done' ORDER BY name;\""
	output, err := e.execInPod(ctx, pod, "postgres", []string{"sh", "-c", cmd})
	if err != nil {
		// This might fail on some configurations, so return empty list
		return nil, nil
	}

	var archived []string
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		name := strings.TrimSuffix(strings.TrimSpace(line), ".done")
		if name != "" {
			archived = append(archived, name)
		}
	}

	return archived, nil
}

// removeFile removes a file from the pod
func (e *WALCleanupEngine) removeFile(ctx context.Context, pod *corev1.Pod, filePath string) error {
	cmd := fmt.Sprintf("rm -f %s", filePath)
	_, err := e.execInPod(ctx, pod, "postgres", []string{"sh", "-c", cmd})
	return err
}

// execInPod executes a command in a pod container
func (e *WALCleanupEngine) execInPod(
	ctx context.Context,
	pod *corev1.Pod,
	container string,
	command []string,
) (string, error) {
	req := e.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(pod.Namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   command,
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(e.restConfig, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("failed to create executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})

	if err != nil {
		return "", fmt.Errorf("failed to execute command: %w, stderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

// CreateWALCleanupEvent creates a StorageEvent for a WAL cleanup operation
func (e *WALCleanupEngine) CreateWALCleanupEvent(
	ctx context.Context,
	req *WALCleanupRequest,
	result *WALCleanupResult,
) (*cnpgv1alpha1.StorageEvent, error) {
	event := &cnpgv1alpha1.StorageEvent{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-wal-cleanup-", req.ClusterName),
			Namespace:    req.ClusterNamespace,
			Labels: map[string]string{
				"cnpg.supporttools.io/cluster":    req.ClusterName,
				"cnpg.supporttools.io/event-type": string(cnpgv1alpha1.EventTypeWALCleanup),
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
			EventType: cnpgv1alpha1.EventTypeWALCleanup,
			Trigger:   cnpgv1alpha1.TriggerTypeThresholdBreach,
			Reason:    req.Reason,
			WALCleanup: &cnpgv1alpha1.WALCleanupDetails{
				PodName: result.PodName,
			},
			DryRun: req.DryRun,
		},
	}

	if err := e.client.Create(ctx, event); err != nil {
		return nil, fmt.Errorf("failed to create WAL cleanup event: %w", err)
	}

	return event, nil
}

// UpdateWALCleanupEventStatus updates the status of a WAL cleanup event
func (e *WALCleanupEngine) UpdateWALCleanupEventStatus(
	ctx context.Context,
	event *cnpgv1alpha1.StorageEvent,
	result *WALCleanupResult,
) error {
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
	event.Status.Message = fmt.Sprintf("WAL cleanup: %d files removed, %s freed",
		result.FilesRemoved, formatBytes(result.BytesFreed))

	return e.client.Status().Update(ctx, event)
}

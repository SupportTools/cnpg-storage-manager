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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// KubeletStatsSummary represents the kubelet stats/summary response
type KubeletStatsSummary struct {
	Node NodeStats  `json:"node"`
	Pods []PodStats `json:"pods"`
}

// NodeStats contains node-level statistics
type NodeStats struct {
	NodeName string `json:"nodeName"`
}

// PodStats contains pod-level statistics
type PodStats struct {
	PodRef           PodReference  `json:"podRef"`
	VolumeStats      []VolumeStats `json:"volume,omitempty"`
	EphemeralStorage *FsStats      `json:"ephemeral-storage,omitempty"`
}

// PodReference contains identifying information about a pod
type PodReference struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	UID       string `json:"uid"`
}

// VolumeStats contains statistics about a volume
type VolumeStats struct {
	Name           string   `json:"name"`
	PVCRef         *PVCRef  `json:"pvcRef,omitempty"`
	FsStats        *FsStats `json:"fsStats,omitempty"`
	CapacityBytes  *int64   `json:"capacityBytes,omitempty"`
	UsedBytes      *int64   `json:"usedBytes,omitempty"`
	AvailableBytes *int64   `json:"availableBytes,omitempty"`
	InodesUsed     *int64   `json:"inodesUsed,omitempty"`
	Inodes         *int64   `json:"inodes,omitempty"`
	InodesFree     *int64   `json:"inodesFree,omitempty"`
}

// PVCRef contains a reference to a PVC
type PVCRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// FsStats contains filesystem statistics
type FsStats struct {
	Time           *time.Time `json:"time,omitempty"`
	AvailableBytes *int64     `json:"availableBytes,omitempty"`
	CapacityBytes  *int64     `json:"capacityBytes,omitempty"`
	UsedBytes      *int64     `json:"usedBytes,omitempty"`
	InodesFree     *int64     `json:"inodesFree,omitempty"`
	Inodes         *int64     `json:"inodes,omitempty"`
	InodesUsed     *int64     `json:"inodesUsed,omitempty"`
}

// PVCMetrics contains collected metrics for a PVC
type PVCMetrics struct {
	PVCName        string
	PVCNamespace   string
	PodName        string
	PodNamespace   string
	NodeName       string
	UsedBytes      int64
	CapacityBytes  int64
	AvailableBytes int64
	InodesUsed     int64
	Inodes         int64
	InodesFree     int64
	CollectedAt    time.Time
}

// UsagePercent returns the usage percentage
func (m *PVCMetrics) UsagePercent() float64 {
	if m.CapacityBytes == 0 {
		return 0
	}
	return float64(m.UsedBytes) / float64(m.CapacityBytes) * 100
}

// InodesUsedPercent returns the inodes usage percentage
func (m *PVCMetrics) InodesUsedPercent() float64 {
	if m.Inodes == 0 {
		return 0
	}
	return float64(m.InodesUsed) / float64(m.Inodes) * 100
}

// Collector collects storage metrics from kubelet
type Collector struct {
	client        client.Client
	restConfig    *rest.Config
	httpClient    *http.Client
	execCollector *ExecCollector
}

// NewCollector creates a new metrics collector
func NewCollector(c client.Client, restConfig *rest.Config) *Collector {
	// Create HTTP client with TLS config from rest config
	transport := &http.Transport{
		TLSClientConfig: nil, // Will be configured per-request
	}

	// Create exec collector for fallback
	execCollector, err := NewExecCollector(restConfig)
	if err != nil {
		// Log warning but continue without exec collector
		log.Log.Error(err, "Failed to create exec collector, fallback will not be available")
	}

	return &Collector{
		client:        c,
		restConfig:    restConfig,
		execCollector: execCollector,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
}

// CollectPVCMetrics collects metrics for PVCs associated with a cluster
func (c *Collector) CollectPVCMetrics(ctx context.Context, pods []corev1.Pod) ([]PVCMetrics, error) {
	logger := log.FromContext(ctx)
	var allMetrics []PVCMetrics

	// Group pods by node to minimize kubelet requests
	podsByNode := make(map[string][]corev1.Pod)
	for _, pod := range pods {
		if pod.Spec.NodeName != "" && pod.Status.Phase == corev1.PodRunning {
			podsByNode[pod.Spec.NodeName] = append(podsByNode[pod.Spec.NodeName], pod)
		}
	}

	// Collect metrics from each node
	for nodeName, nodePods := range podsByNode {
		stats, err := c.fetchKubeletStats(ctx, nodeName)
		if err != nil {
			logger.Error(err, "Failed to fetch kubelet stats", "node", nodeName)
			RecordError("kubelet_stats_fetch", "", nodeName)
			continue
		}

		// Extract metrics for our pods
		metrics := c.extractPVCMetrics(stats, nodePods, nodeName)
		allMetrics = append(allMetrics, metrics...)
	}

	return allMetrics, nil
}

// fetchKubeletStats fetches stats from kubelet's /stats/summary endpoint
func (c *Collector) fetchKubeletStats(ctx context.Context, nodeName string) (*KubeletStatsSummary, error) {
	logger := log.FromContext(ctx)
	start := time.Now()
	defer func() {
		MetricsCollectionDuration.WithLabelValues("kubelet_stats").Observe(time.Since(start).Seconds())
	}()

	// Use the API server proxy to reach the kubelet
	// This avoids needing direct kubelet access and uses existing RBAC
	url := fmt.Sprintf("%s/api/v1/nodes/%s/proxy/stats/summary", c.restConfig.Host, nodeName)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication
	if c.restConfig.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.restConfig.BearerToken)
	} else if c.restConfig.BearerTokenFile != "" {
		token, err := readTokenFile(c.restConfig.BearerTokenFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read token file: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}

	// Create a client with proper TLS config
	transport, err := rest.TransportFor(c.restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport: %w", err)
	}
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch kubelet stats: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("kubelet stats request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var summary KubeletStatsSummary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		return nil, fmt.Errorf("failed to decode kubelet stats: %w", err)
	}

	logger.V(2).Info("Fetched kubelet stats", "node", nodeName, "podCount", len(summary.Pods))
	return &summary, nil
}

// extractPVCMetrics extracts PVC metrics from kubelet stats for the given pods
func (c *Collector) extractPVCMetrics(stats *KubeletStatsSummary, pods []corev1.Pod, nodeName string) []PVCMetrics {
	var metrics []PVCMetrics
	now := time.Now()

	// Create a map of pod UIDs we're interested in
	podUIDs := make(map[string]corev1.Pod)
	for _, pod := range pods {
		podUIDs[string(pod.UID)] = pod
	}

	// Process each pod's stats
	for _, podStats := range stats.Pods {
		pod, exists := podUIDs[podStats.PodRef.UID]
		if !exists {
			continue
		}

		// Process each volume
		for _, volStats := range podStats.VolumeStats {
			// We only care about PVC-backed volumes
			if volStats.PVCRef == nil {
				continue
			}

			metric := PVCMetrics{
				PVCName:      volStats.PVCRef.Name,
				PVCNamespace: volStats.PVCRef.Namespace,
				PodName:      pod.Name,
				PodNamespace: pod.Namespace,
				NodeName:     nodeName,
				CollectedAt:  now,
			}

			// Extract capacity - prefer top-level fields, fallback to FsStats
			if volStats.CapacityBytes != nil {
				metric.CapacityBytes = *volStats.CapacityBytes
			} else if volStats.FsStats != nil && volStats.FsStats.CapacityBytes != nil {
				metric.CapacityBytes = *volStats.FsStats.CapacityBytes
			}

			// Extract used bytes
			if volStats.UsedBytes != nil {
				metric.UsedBytes = *volStats.UsedBytes
			} else if volStats.FsStats != nil && volStats.FsStats.UsedBytes != nil {
				metric.UsedBytes = *volStats.FsStats.UsedBytes
			}

			// Extract available bytes
			if volStats.AvailableBytes != nil {
				metric.AvailableBytes = *volStats.AvailableBytes
			} else if volStats.FsStats != nil && volStats.FsStats.AvailableBytes != nil {
				metric.AvailableBytes = *volStats.FsStats.AvailableBytes
			}

			// Extract inode stats
			if volStats.Inodes != nil {
				metric.Inodes = *volStats.Inodes
			} else if volStats.FsStats != nil && volStats.FsStats.Inodes != nil {
				metric.Inodes = *volStats.FsStats.Inodes
			}

			if volStats.InodesUsed != nil {
				metric.InodesUsed = *volStats.InodesUsed
			} else if volStats.FsStats != nil && volStats.FsStats.InodesUsed != nil {
				metric.InodesUsed = *volStats.FsStats.InodesUsed
			}

			if volStats.InodesFree != nil {
				metric.InodesFree = *volStats.InodesFree
			} else if volStats.FsStats != nil && volStats.FsStats.InodesFree != nil {
				metric.InodesFree = *volStats.FsStats.InodesFree
			}

			metrics = append(metrics, metric)
		}
	}

	return metrics
}

// CollectClusterMetrics collects all PVC metrics for a CNPG cluster
func (c *Collector) CollectClusterMetrics(
	ctx context.Context,
	clusterName, namespace string,
	pods []corev1.Pod,
) (*ClusterMetrics, error) {
	logger := log.FromContext(ctx)
	start := time.Now()

	pvcMetrics, err := c.CollectPVCMetrics(ctx, pods)
	if err != nil {
		return nil, err
	}

	// Check if we got any PVC metrics from kubelet stats
	// If not, try the exec-based fallback (for storage classes like local-path)
	if len(pvcMetrics) == 0 && c.execCollector != nil && len(pods) > 0 {
		logger.Info("No PVC metrics from kubelet stats, trying exec-based fallback",
			"cluster", clusterName,
			"namespace", namespace,
			"podCount", len(pods),
		)

		execMetrics, execErr := c.execCollector.CollectPVCMetricsViaExec(ctx, pods)
		if execErr != nil {
			logger.Error(execErr, "Exec-based metrics collection also failed",
				"cluster", clusterName,
				"namespace", namespace,
			)
		} else if len(execMetrics) > 0 {
			logger.Info("Successfully collected metrics via exec fallback",
				"cluster", clusterName,
				"namespace", namespace,
				"pvcCount", len(execMetrics),
			)
			pvcMetrics = execMetrics
		}
	}

	clusterMetrics := &ClusterMetrics{
		ClusterName: clusterName,
		Namespace:   namespace,
		PVCMetrics:  pvcMetrics,
		CollectedAt: time.Now(),
	}

	// Calculate aggregates
	for _, pvc := range pvcMetrics {
		clusterMetrics.TotalUsedBytes += pvc.UsedBytes
		clusterMetrics.TotalCapacityBytes += pvc.CapacityBytes

		// Record individual PVC metrics to Prometheus
		RecordPVCMetrics(clusterName, namespace, pvc.PVCName, pvc.PodName, pvc.UsedBytes, pvc.CapacityBytes)
	}

	logger.V(1).Info("Collected cluster metrics",
		"cluster", clusterName,
		"namespace", namespace,
		"pvcCount", len(pvcMetrics),
		"totalUsed", clusterMetrics.TotalUsedBytes,
		"totalCapacity", clusterMetrics.TotalCapacityBytes,
		"duration", time.Since(start),
	)

	return clusterMetrics, nil
}

// ClusterMetrics contains aggregated metrics for a CNPG cluster
type ClusterMetrics struct {
	ClusterName        string
	Namespace          string
	PVCMetrics         []PVCMetrics
	TotalUsedBytes     int64
	TotalCapacityBytes int64
	CollectedAt        time.Time
}

// TotalUsagePercent returns the total usage percentage across all PVCs
func (m *ClusterMetrics) TotalUsagePercent() float64 {
	if m.TotalCapacityBytes == 0 {
		return 0
	}
	return float64(m.TotalUsedBytes) / float64(m.TotalCapacityBytes) * 100
}

// GetPrimaryPVCMetrics returns metrics for the primary instance PVC
func (m *ClusterMetrics) GetPrimaryPVCMetrics(primaryPodName string) *PVCMetrics {
	for i := range m.PVCMetrics {
		if m.PVCMetrics[i].PodName == primaryPodName {
			return &m.PVCMetrics[i]
		}
	}
	return nil
}

// GetHighestUsagePVC returns the PVC with the highest usage percentage
func (m *ClusterMetrics) GetHighestUsagePVC() *PVCMetrics {
	var highest *PVCMetrics
	var highestPercent float64

	for i := range m.PVCMetrics {
		percent := m.PVCMetrics[i].UsagePercent()
		if highest == nil || percent > highestPercent {
			highest = &m.PVCMetrics[i]
			highestPercent = percent
		}
	}

	return highest
}

// readTokenFile reads a bearer token from a file
func readTokenFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

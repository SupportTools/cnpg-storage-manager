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
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// String literals shared across this file (goconst).
const (
	postgresContainerName = "postgres"
)

// ExecCollector collects storage metrics by executing commands inside pods
// This is used as a fallback when kubelet stats don't provide volume metrics
// (e.g., for local-path provisioner volumes)
type ExecCollector struct {
	clientset  *kubernetes.Clientset
	restConfig *rest.Config
}

// NewExecCollector creates a new exec-based metrics collector
func NewExecCollector(restConfig *rest.Config) (*ExecCollector, error) {
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	return &ExecCollector{
		clientset:  clientset,
		restConfig: restConfig,
	}, nil
}

// DfOutput represents parsed output from the df command
type DfOutput struct {
	Filesystem string
	TotalBytes int64
	UsedBytes  int64
	AvailBytes int64
	UsePercent float64
	MountPoint string
}

// CollectPVCMetricsViaExec collects metrics for PVCs by executing df inside the pods
// This is used when kubelet stats don't provide volume metrics
func (e *ExecCollector) CollectPVCMetricsViaExec(ctx context.Context, pods []corev1.Pod) ([]PVCMetrics, error) {
	logger := log.FromContext(ctx)
	var allMetrics []PVCMetrics

	for _, pod := range pods {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		// Find PVC-backed volumes and their mount paths
		volumeMounts := e.getPVCVolumeMounts(pod)
		if len(volumeMounts) == 0 {
			logger.V(2).Info("No PVC volume mounts found in pod", "pod", pod.Name, "namespace", pod.Namespace)
			continue
		}

		// Execute df command inside the pod
		dfOutput, err := e.execDfInPod(ctx, pod)
		if err != nil {
			logger.Error(err, "Failed to exec df in pod", "pod", pod.Name, "namespace", pod.Namespace)
			RecordError("exec_df", pod.Namespace+"/"+pod.Name, pod.Spec.NodeName)
			continue
		}

		// Match df output with PVC mounts
		for pvcName, mountPath := range volumeMounts {
			dfStats := e.findMountPointStats(dfOutput, mountPath)
			if dfStats == nil {
				logger.V(2).Info("No df stats found for mount point", "pod", pod.Name, "pvc", pvcName, "mountPath", mountPath)
				continue
			}

			metric := PVCMetrics{
				PVCName:        pvcName,
				PVCNamespace:   pod.Namespace, // PVC is in the same namespace as the pod
				PodName:        pod.Name,
				PodNamespace:   pod.Namespace,
				NodeName:       pod.Spec.NodeName,
				UsedBytes:      dfStats.UsedBytes,
				CapacityBytes:  dfStats.TotalBytes,
				AvailableBytes: dfStats.AvailBytes,
				CollectedAt:    time.Now(),
			}

			// local-path / hostPath volumes are a directory on a shared node
			// filesystem: df reports that whole filesystem, not the volume
			// (cluster-services-1381: every local-path cluster read the same
			// 73.7% of ~1Ti while the PVCs were 10-16Gi). When df's filesystem is
			// larger than the PVC, measure the volume's own directory with du and
			// compare it to the PVC size instead.
			pvcCapacity := e.pvcCapacityBytes(ctx, pod.Namespace, pvcName)
			if IsSharedFilesystem(dfStats.TotalBytes, pvcCapacity) {
				used, duErr := e.execDuInPod(ctx, pod, mountPath)
				if duErr != nil {
					// Without du the only number is the node filesystem, which
					// is exactly the false reading this branch exists to avoid.
					logger.Error(duErr, "Failed to measure volume directory; skipping PVC",
						"pod", pod.Name, "pvc", pvcName, "mountPath", mountPath)
					RecordError("exec_du", pod.Namespace+"/"+pod.Name, pod.Spec.NodeName)
					continue
				}
				ApplyDirectoryUsage(&metric, used, pvcCapacity, dfStats.AvailBytes)
				logger.V(1).Info("Shared filesystem volume: usage measured with du against PVC size",
					"pod", pod.Name, "pvc", pvcName, "used", used, "pvcCapacity", pvcCapacity,
					"filesystemTotal", dfStats.TotalBytes)
			}

			// Get inode stats if possible
			inodeStats, err := e.execDfInodesInPod(ctx, pod, mountPath)
			if err == nil && inodeStats != nil {
				metric.Inodes = inodeStats.TotalBytes     // Total inodes
				metric.InodesUsed = inodeStats.UsedBytes  // Used inodes
				metric.InodesFree = inodeStats.AvailBytes // Free inodes
			}

			logger.V(1).Info("Collected PVC metrics via exec",
				"pod", pod.Name,
				"pvc", pvcName,
				"used", metric.UsedBytes,
				"capacity", metric.CapacityBytes,
				"percent", metric.UsagePercent(),
			)

			allMetrics = append(allMetrics, metric)
		}
	}

	return allMetrics, nil
}

// sharedFilesystemSlack is how much larger than the PVC the filesystem under its
// mount may be before the volume is treated as a directory on a shared filesystem.
// Real block volumes are formatted at (or a few percent under) the PVC size; a
// filesystem 10% bigger is not this volume's own.
const sharedFilesystemSlack = 1.10

// IsSharedFilesystem reports whether a mount whose filesystem has fsTotalBytes is a
// directory on a larger, shared filesystem (local-path, hostPath) rather than a volume
// with its own filesystem. pvcCapacityBytes <= 0 (unknown) is never shared.
func IsSharedFilesystem(fsTotalBytes, pvcCapacityBytes int64) bool {
	if pvcCapacityBytes <= 0 || fsTotalBytes <= 0 {
		return false
	}
	return float64(fsTotalBytes) > float64(pvcCapacityBytes)*sharedFilesystemSlack
}

// ApplyDirectoryUsage rewrites a df-based metric for a shared-filesystem volume: used
// is the volume directory's own size, capacity the PVC size. Available is what is
// left of the PVC, capped by what is actually free on the shared filesystem. Inode
// figures belong to the shared filesystem and are cleared.
func ApplyDirectoryUsage(metric *PVCMetrics, usedBytes, pvcCapacityBytes, fsAvailBytes int64) {
	metric.UsedBytes = usedBytes
	metric.CapacityBytes = pvcCapacityBytes
	avail := pvcCapacityBytes - usedBytes
	if avail < 0 {
		avail = 0
	}
	if fsAvailBytes >= 0 && fsAvailBytes < avail {
		avail = fsAvailBytes
	}
	metric.AvailableBytes = avail
	metric.Inodes, metric.InodesUsed, metric.InodesFree = 0, 0, 0
}

// pvcCapacityBytes returns the PVC's provisioned size (status.capacity, falling back
// to spec.resources.requests), or 0 if the PVC cannot be read.
func (e *ExecCollector) pvcCapacityBytes(ctx context.Context, namespace, name string) int64 {
	pvc, err := e.clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		log.FromContext(ctx).V(1).Info("Could not read PVC for capacity", "pvc", namespace+"/"+name, "error", err.Error())
		return 0
	}
	return PVCProvisionedBytes(pvc)
}

// PVCProvisionedBytes returns a PVC's provisioned size: status.capacity when bound,
// otherwise spec.resources.requests. 0 when neither is set.
func PVCProvisionedBytes(pvc *corev1.PersistentVolumeClaim) int64 {
	if q, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok {
		return q.Value()
	}
	if q, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
		return q.Value()
	}
	return 0
}

// execDuInPod returns the bytes used by the directory tree at path, staying on one
// filesystem (-x) so other mounts below it are not counted.
func (e *ExecCollector) execDuInPod(ctx context.Context, pod corev1.Pod, path string) (int64, error) {
	stdout, _, err := e.execInPod(ctx, pod, []string{"du", "-sxB1", path})
	if err != nil {
		return 0, err
	}
	return ParseDuOutput(stdout)
}

// ParseDuOutput parses `du -sB1 <path>` output ("<bytes>\t<path>").
func ParseDuOutput(output string) (int64, error) {
	fields := strings.Fields(strings.TrimSpace(output))
	if len(fields) == 0 {
		return 0, fmt.Errorf("empty du output")
	}
	n, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("unparseable du output %q: %w", output, err)
	}
	return n, nil
}

// LastArchivedWALTime asks a CNPG instance's pg_stat_archiver when it last archived a
// WAL segment. It returns nil, nil when the instance has never archived one.
func (e *ExecCollector) LastArchivedWALTime(ctx context.Context, namespace, podName string) (*time.Time, error) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: namespace},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: postgresContainerName}}},
	}
	// Local socket as the container user (postgres): no password needed.
	command := []string{"psql", "-tAXq", "-c",
		"select coalesce(extract(epoch from last_archived_time)::bigint::text, '') from pg_stat_archiver"}
	stdout, _, err := e.execInPod(ctx, pod, command)
	if err != nil {
		return nil, err
	}
	return ParseArchiverEpoch(stdout)
}

// ParseArchiverEpoch parses the epoch-seconds output of the pg_stat_archiver query.
// Empty output means no WAL was ever archived (nil, nil).
func ParseArchiverEpoch(output string) (*time.Time, error) {
	v := strings.TrimSpace(output)
	if v == "" {
		return nil, nil
	}
	secs, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("unparseable pg_stat_archiver output %q: %w", output, err)
	}
	t := time.Unix(secs, 0).UTC()
	return &t, nil
}

// getPVCVolumeMounts returns a map of PVC name to mount path for the pod
func (e *ExecCollector) getPVCVolumeMounts(pod corev1.Pod) map[string]string {
	// First, build a map of volume name to PVC name
	volumeToPVC := make(map[string]string)
	for _, vol := range pod.Spec.Volumes {
		if vol.PersistentVolumeClaim != nil {
			volumeToPVC[vol.Name] = vol.PersistentVolumeClaim.ClaimName
		}
	}

	// Then find the mount paths for each PVC-backed volume
	pvcMounts := make(map[string]string)
	for _, container := range pod.Spec.Containers {
		for _, mount := range container.VolumeMounts {
			if pvcName, ok := volumeToPVC[mount.Name]; ok {
				pvcMounts[pvcName] = mount.MountPath
			}
		}
	}

	return pvcMounts
}

// execDfInPod executes the df command inside a pod and returns parsed output
func (e *ExecCollector) execDfInPod(ctx context.Context, pod corev1.Pod) ([]DfOutput, error) {
	start := time.Now()
	defer func() {
		MetricsCollectionDuration.WithLabelValues("exec_df").Observe(time.Since(start).Seconds())
	}()

	// Use df with -B1 to get bytes, -P for POSIX format (single line per filesystem)
	command := []string{"df", "-B1", "-P"}
	stdout, _, err := e.execInPod(ctx, pod, command)
	if err != nil {
		return nil, err
	}

	return e.parseDfOutput(stdout), nil
}

// execDfInodesInPod executes df -i to get inode stats for a specific mount point
func (e *ExecCollector) execDfInodesInPod(ctx context.Context, pod corev1.Pod, mountPath string) (*DfOutput, error) {
	command := []string{"df", "-i", "-P", mountPath}
	stdout, _, err := e.execInPod(ctx, pod, command)
	if err != nil {
		return nil, err
	}

	outputs := e.parseDfOutput(stdout)
	if len(outputs) > 0 {
		return &outputs[0], nil
	}
	return nil, nil
}

// execInPod executes a command inside a pod and returns stdout/stderr
//
//nolint:unparam // stderr return kept for debugging purposes
func (e *ExecCollector) execInPod(
	ctx context.Context,
	pod corev1.Pod,
	command []string,
) (string, string, error) {
	// Find the first container (typically the postgres container for CNPG)
	containerName := ""
	for _, container := range pod.Spec.Containers {
		// Prefer the postgres container if available
		if container.Name == postgresContainerName {
			containerName = container.Name
			break
		}
		if containerName == "" {
			containerName = container.Name
		}
	}

	req := e.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(pod.Namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: containerName,
			Command:   command,
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(e.restConfig, "POST", req.URL())
	if err != nil {
		return "", "", fmt.Errorf("failed to create executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to execute command: %w (stderr: %s)", err, stderr.String())
	}

	return stdout.String(), stderr.String(), nil
}

// parseDfOutput parses the output of df -B1 -P
func (e *ExecCollector) parseDfOutput(output string) []DfOutput {
	lines := strings.Split(output, "\n")
	results := make([]DfOutput, 0, len(lines))

	for i, line := range lines {
		// Skip header line
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}

		// Parse: Filesystem 1-blocks Used Available Use% Mounted
		totalBytes, _ := strconv.ParseInt(fields[1], 10, 64)
		usedBytes, _ := strconv.ParseInt(fields[2], 10, 64)
		availBytes, _ := strconv.ParseInt(fields[3], 10, 64)

		// Parse percentage (remove % suffix)
		percentStr := strings.TrimSuffix(fields[4], "%")
		percent, _ := strconv.ParseFloat(percentStr, 64)

		results = append(results, DfOutput{
			Filesystem: fields[0],
			TotalBytes: totalBytes,
			UsedBytes:  usedBytes,
			AvailBytes: availBytes,
			UsePercent: percent,
			MountPoint: fields[5],
		})
	}

	return results
}

// findMountPointStats finds the df stats for a specific mount point
func (e *ExecCollector) findMountPointStats(dfOutputs []DfOutput, mountPath string) *DfOutput {
	// Look for exact match first
	for i := range dfOutputs {
		if dfOutputs[i].MountPoint == mountPath {
			return &dfOutputs[i]
		}
	}

	// If no exact match, find the longest prefix match
	// This handles cases where the mount point might be a subdirectory
	var bestMatch *DfOutput
	bestMatchLen := 0

	for i := range dfOutputs {
		mp := dfOutputs[i].MountPoint
		if strings.HasPrefix(mountPath, mp) && len(mp) > bestMatchLen {
			bestMatch = &dfOutputs[i]
			bestMatchLen = len(mp)
		}
	}

	return bestMatch
}

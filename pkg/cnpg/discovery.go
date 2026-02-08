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

package cnpg

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// CNPGGroupVersion is the API group version for CNPG clusters
	CNPGGroupVersion = "postgresql.cnpg.io/v1"
	// CNPGKind is the kind for CNPG clusters
	CNPGKind = "Cluster"
	// BarmanCloudPluginName is the name of the barman-cloud plugin
	BarmanCloudPluginName = "barman-cloud.cloudnative-pg.io"
	// ObjectStoreGroup is the API group for ObjectStore CRD
	ObjectStoreGroup = "barmancloud.cnpg.io"
	// ObjectStoreVersion is the API version for ObjectStore CRD
	ObjectStoreVersion = "v1"
	// ObjectStoreKind is the kind for ObjectStore CRD
	ObjectStoreKind = "ObjectStore"
)

var (
	// CNPGClusterGVK is the GroupVersionKind for CNPG Cluster
	CNPGClusterGVK = schema.GroupVersionKind{
		Group:   "postgresql.cnpg.io",
		Version: "v1",
		Kind:    "Cluster",
	}
	// ObjectStoreGVK is the GroupVersionKind for ObjectStore
	ObjectStoreGVK = schema.GroupVersionKind{
		Group:   ObjectStoreGroup,
		Version: ObjectStoreVersion,
		Kind:    ObjectStoreKind,
	}
)

// ClusterInfo contains information about a CNPG cluster
type ClusterInfo struct {
	Name      string
	Namespace string
	Labels    map[string]string
	Instances int32
	Storage   StorageInfo
	Status    ClusterStatus
}

// StorageInfo contains storage information for a cluster
type StorageInfo struct {
	Size         string
	StorageClass string
	PVCNames     []string
}

// ClusterStatus contains status information for a cluster
type ClusterStatus struct {
	Phase              string
	Ready              bool
	ReadyInstances     int32
	CurrentPrimary     string
	CurrentPrimaryNode string
	// Backup status fields
	FirstRecoverabilityPoint   *time.Time
	LastSuccessfulBackup       *time.Time
	ContinuousArchivingWorking bool
	BackupConfigured           bool
	// Barman-cloud plugin info (when using external ObjectStore)
	BarmanCloudPlugin *BarmanCloudPluginInfo
}

// BarmanCloudPluginInfo contains information about the barman-cloud plugin configuration
type BarmanCloudPluginInfo struct {
	// Enabled indicates if the barman-cloud plugin is configured
	Enabled bool
	// IsWALArchiver indicates if the plugin is configured as WAL archiver
	IsWALArchiver bool
	// ObjectStoreName is the name of the ObjectStore CRD referenced by the plugin
	ObjectStoreName string
	// ObjectStoreNamespace is the namespace of the ObjectStore CRD (defaults to cluster namespace)
	ObjectStoreNamespace string
}

// ObjectStoreBackupStatus contains backup status from an ObjectStore CRD
type ObjectStoreBackupStatus struct {
	// ClusterName is the name of the cluster this status is for
	ClusterName string
	// FirstRecoverabilityPoint is the oldest point in time recovery is possible
	FirstRecoverabilityPoint *time.Time
	// LastSuccessfulBackupTime is the time of the last successful backup
	LastSuccessfulBackupTime *time.Time
}

// Discovery provides methods for discovering CNPG clusters
type Discovery struct {
	client client.Client
}

// NewDiscovery creates a new Discovery
func NewDiscovery(c client.Client) *Discovery {
	return &Discovery{client: c}
}

// ListClusters lists all CNPG clusters in a namespace (or all namespaces if empty)
func (d *Discovery) ListClusters(ctx context.Context, namespace string) ([]ClusterInfo, error) {
	clusterList := &unstructured.UnstructuredList{}
	clusterList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "postgresql.cnpg.io",
		Version: "v1",
		Kind:    "ClusterList",
	})

	opts := []client.ListOption{}
	if namespace != "" {
		opts = append(opts, client.InNamespace(namespace))
	}

	if err := d.client.List(ctx, clusterList, opts...); err != nil {
		return nil, fmt.Errorf("failed to list CNPG clusters: %w", err)
	}

	clusters := make([]ClusterInfo, 0, len(clusterList.Items))
	for _, item := range clusterList.Items {
		info, err := d.extractClusterInfo(&item)
		if err != nil {
			continue // Skip clusters we can't parse
		}
		clusters = append(clusters, info)
	}

	return clusters, nil
}

// GetCluster gets a specific CNPG cluster
func (d *Discovery) GetCluster(ctx context.Context, name, namespace string) (*ClusterInfo, error) {
	cluster := &unstructured.Unstructured{}
	cluster.SetGroupVersionKind(CNPGClusterGVK)

	if err := d.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, cluster); err != nil {
		return nil, fmt.Errorf("failed to get CNPG cluster %s/%s: %w", namespace, name, err)
	}

	info, err := d.extractClusterInfo(cluster)
	if err != nil {
		return nil, err
	}

	return &info, nil
}

// GetClusterBySelector gets clusters matching a label selector
func (d *Discovery) GetClustersBySelector(
	ctx context.Context,
	namespace string,
	selector *metav1.LabelSelector,
) ([]ClusterInfo, error) {
	allClusters, err := d.ListClusters(ctx, namespace)
	if err != nil {
		return nil, err
	}

	if selector == nil {
		return allClusters, nil
	}

	sel, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return nil, fmt.Errorf("invalid label selector: %w", err)
	}

	var matched []ClusterInfo
	for _, cluster := range allClusters {
		if sel.Matches(labels.Set(cluster.Labels)) {
			matched = append(matched, cluster)
		}
	}

	return matched, nil
}

// extractClusterInfo extracts cluster information from an unstructured object
//
//nolint:unparam // error return kept for future extensibility
func (d *Discovery) extractClusterInfo(cluster *unstructured.Unstructured) (ClusterInfo, error) {
	info := ClusterInfo{
		Name:      cluster.GetName(),
		Namespace: cluster.GetNamespace(),
		Labels:    cluster.GetLabels(),
	}

	// Extract spec.instances
	if instances, found, _ := unstructured.NestedInt64(cluster.Object, "spec", "instances"); found {
		info.Instances = int32(instances)
	} else {
		info.Instances = 1 // Default
	}

	// Extract storage info
	if size, found, _ := unstructured.NestedString(cluster.Object, "spec", "storage", "size"); found {
		info.Storage.Size = size
	}
	if storageClass, found, _ := unstructured.NestedString(cluster.Object, "spec", "storage", "storageClass"); found {
		info.Storage.StorageClass = storageClass
	}

	// Extract status
	if phase, found, _ := unstructured.NestedString(cluster.Object, "status", "phase"); found {
		info.Status.Phase = phase
	}
	if readyInstances, found, _ := unstructured.NestedInt64(cluster.Object, "status", "readyInstances"); found {
		info.Status.ReadyInstances = int32(readyInstances)
	}
	if primary, found, _ := unstructured.NestedString(cluster.Object, "status", "currentPrimary"); found {
		info.Status.CurrentPrimary = primary
	}
	if primaryNode, found, _ := unstructured.NestedString(cluster.Object, "status", "currentPrimaryNode"); found {
		info.Status.CurrentPrimaryNode = primaryNode
	}

	info.Status.Ready = info.Status.Phase == "Cluster in healthy state" || info.Status.ReadyInstances >= info.Instances

	// Extract backup status fields
	firstRecoverability, found, _ := unstructured.NestedString(
		cluster.Object, "status", "firstRecoverabilityPoint",
	)
	if found && firstRecoverability != "" {
		if t, err := time.Parse(time.RFC3339, firstRecoverability); err == nil {
			info.Status.FirstRecoverabilityPoint = &t
		}
	}
	lastBackup, found, _ := unstructured.NestedString(
		cluster.Object, "status", "lastSuccessfulBackup",
	)
	if found && lastBackup != "" {
		if t, err := time.Parse(time.RFC3339, lastBackup); err == nil {
			info.Status.LastSuccessfulBackup = &t
		}
	}

	// Check for ContinuousArchiving condition
	if conditions, found, _ := unstructured.NestedSlice(cluster.Object, "status", "conditions"); found {
		for _, cond := range conditions {
			condMap, ok := cond.(map[string]interface{})
			if !ok {
				continue
			}
			if condType, _ := condMap["type"].(string); condType == "ContinuousArchiving" {
				if status, _ := condMap["status"].(string); status == "True" {
					info.Status.ContinuousArchivingWorking = true
				}
			}
		}
	}

	// Check if backup is configured (presence of backup section in spec)
	if _, found, _ := unstructured.NestedMap(cluster.Object, "spec", "backup"); found {
		info.Status.BackupConfigured = true
	}

	// Check for barman-cloud plugin configuration
	info.Status.BarmanCloudPlugin = d.extractBarmanCloudPluginInfo(cluster)
	if info.Status.BarmanCloudPlugin != nil && info.Status.BarmanCloudPlugin.Enabled {
		// If barman-cloud plugin is configured, backup is configured
		info.Status.BackupConfigured = true
	}

	return info, nil
}

// extractBarmanCloudPluginInfo extracts barman-cloud plugin configuration from cluster spec
func (d *Discovery) extractBarmanCloudPluginInfo(
	cluster *unstructured.Unstructured,
) *BarmanCloudPluginInfo {
	plugins, found, _ := unstructured.NestedSlice(cluster.Object, "spec", "plugins")
	if !found {
		return nil
	}

	for _, plugin := range plugins {
		pluginMap, ok := plugin.(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := pluginMap["name"].(string)
		if name != BarmanCloudPluginName {
			continue
		}

		info := &BarmanCloudPluginInfo{
			Enabled:              true,
			ObjectStoreNamespace: cluster.GetNamespace(), // Default to cluster namespace
		}

		// Check if isWALArchiver is set
		if isWALArchiver, ok := pluginMap["isWALArchiver"].(bool); ok {
			info.IsWALArchiver = isWALArchiver
		}

		// Extract parameters to find barmanObjectName
		if params, ok := pluginMap["parameters"].(map[string]interface{}); ok {
			if barmanObjectName, ok := params["barmanObjectName"].(string); ok {
				info.ObjectStoreName = barmanObjectName
			}
		}

		return info
	}

	return nil
}

// GetClusterPVCs gets the PVCs associated with a CNPG cluster
func (d *Discovery) GetClusterPVCs(
	ctx context.Context,
	clusterName, namespace string,
) ([]corev1.PersistentVolumeClaim, error) {
	pvcList := &corev1.PersistentVolumeClaimList{}

	// CNPG labels PVCs with the cluster name
	labelSelector := labels.SelectorFromSet(labels.Set{
		"cnpg.io/cluster": clusterName,
	})

	if err := d.client.List(ctx, pvcList,
		client.InNamespace(namespace),
		client.MatchingLabelsSelector{Selector: labelSelector},
	); err != nil {
		return nil, fmt.Errorf("failed to list PVCs for cluster %s/%s: %w", namespace, clusterName, err)
	}

	return pvcList.Items, nil
}

// GetClusterPods gets the pods associated with a CNPG cluster
func (d *Discovery) GetClusterPods(ctx context.Context, clusterName, namespace string) ([]corev1.Pod, error) {
	podList := &corev1.PodList{}

	// CNPG labels pods with the cluster name
	labelSelector := labels.SelectorFromSet(labels.Set{
		"cnpg.io/cluster": clusterName,
	})

	if err := d.client.List(ctx, podList,
		client.InNamespace(namespace),
		client.MatchingLabelsSelector{Selector: labelSelector},
	); err != nil {
		return nil, fmt.Errorf("failed to list pods for cluster %s/%s: %w", namespace, clusterName, err)
	}

	return podList.Items, nil
}

// GetPrimaryPod gets the primary pod for a CNPG cluster
func (d *Discovery) GetPrimaryPod(ctx context.Context, clusterName, namespace string) (*corev1.Pod, error) {
	pods, err := d.GetClusterPods(ctx, clusterName, namespace)
	if err != nil {
		return nil, err
	}

	for i := range pods {
		if role, ok := pods[i].Labels["cnpg.io/instanceRole"]; ok && role == "primary" {
			return &pods[i], nil
		}
	}

	return nil, fmt.Errorf("no primary pod found for cluster %s/%s", namespace, clusterName)
}

// UpdateClusterAnnotations updates the annotations on a CNPG cluster
func (d *Discovery) UpdateClusterAnnotations(
	ctx context.Context,
	name, namespace string,
	annotations map[string]string,
) error {
	cluster := &unstructured.Unstructured{}
	cluster.SetGroupVersionKind(CNPGClusterGVK)

	if err := d.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, cluster); err != nil {
		return fmt.Errorf("failed to get CNPG cluster %s/%s: %w", namespace, name, err)
	}

	// Merge annotations
	existing := cluster.GetAnnotations()
	if existing == nil {
		existing = make(map[string]string)
	}
	for k, v := range annotations {
		existing[k] = v
	}
	cluster.SetAnnotations(existing)

	if err := d.client.Update(ctx, cluster); err != nil {
		return fmt.Errorf("failed to update CNPG cluster %s/%s annotations: %w", namespace, name, err)
	}

	return nil
}

// GetClusterAnnotations gets the annotations for a CNPG cluster
func (d *Discovery) GetClusterAnnotations(ctx context.Context, name, namespace string) (map[string]string, error) {
	cluster := &unstructured.Unstructured{}
	cluster.SetGroupVersionKind(CNPGClusterGVK)

	if err := d.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, cluster); err != nil {
		return nil, fmt.Errorf("failed to get CNPG cluster %s/%s: %w", namespace, name, err)
	}

	return cluster.GetAnnotations(), nil
}

// GetObjectStoreBackupStatus gets backup status from an ObjectStore CRD for a specific cluster
func (d *Discovery) GetObjectStoreBackupStatus(
	ctx context.Context,
	objectStoreName, objectStoreNamespace, clusterName string,
) (*ObjectStoreBackupStatus, error) {
	objectStore := &unstructured.Unstructured{}
	objectStore.SetGroupVersionKind(ObjectStoreGVK)

	if err := d.client.Get(ctx, client.ObjectKey{
		Name:      objectStoreName,
		Namespace: objectStoreNamespace,
	}, objectStore); err != nil {
		return nil, fmt.Errorf(
			"failed to get ObjectStore %s/%s: %w",
			objectStoreNamespace, objectStoreName, err,
		)
	}

	// Extract serverRecoveryWindow for the specific cluster
	serverRecoveryWindow, found, _ := unstructured.NestedMap(
		objectStore.Object, "status", "serverRecoveryWindow",
	)
	if !found {
		return nil, nil // ObjectStore exists but has no recovery window data yet
	}

	clusterWindow, ok := serverRecoveryWindow[clusterName].(map[string]interface{})
	if !ok {
		return nil, nil // No data for this cluster in the ObjectStore
	}

	status := &ObjectStoreBackupStatus{
		ClusterName: clusterName,
	}

	// Parse firstRecoverabilityPoint
	if frp, ok := clusterWindow["firstRecoverabilityPoint"].(string); ok && frp != "" {
		if t, err := time.Parse(time.RFC3339, frp); err == nil {
			status.FirstRecoverabilityPoint = &t
		}
	}

	// Parse lastSuccessfulBackupTime
	if lsbt, ok := clusterWindow["lastSuccessfulBackupTime"].(string); ok && lsbt != "" {
		if t, err := time.Parse(time.RFC3339, lsbt); err == nil {
			status.LastSuccessfulBackupTime = &t
		}
	}

	return status, nil
}

// GetBackupStatusForCluster gets backup status for a cluster, checking ObjectStore if barman-cloud plugin is used
func (d *Discovery) GetBackupStatusForCluster(
	ctx context.Context,
	cluster ClusterInfo,
) (*ObjectStoreBackupStatus, error) {
	// If barman-cloud plugin is configured, get status from ObjectStore
	if cluster.Status.BarmanCloudPlugin != nil &&
		cluster.Status.BarmanCloudPlugin.Enabled &&
		cluster.Status.BarmanCloudPlugin.ObjectStoreName != "" {

		return d.GetObjectStoreBackupStatus(
			ctx,
			cluster.Status.BarmanCloudPlugin.ObjectStoreName,
			cluster.Status.BarmanCloudPlugin.ObjectStoreNamespace,
			cluster.Name,
		)
	}

	// Otherwise, return status from cluster itself (legacy mode)
	if cluster.Status.LastSuccessfulBackup != nil || cluster.Status.FirstRecoverabilityPoint != nil {
		return &ObjectStoreBackupStatus{
			ClusterName:              cluster.Name,
			FirstRecoverabilityPoint: cluster.Status.FirstRecoverabilityPoint,
			LastSuccessfulBackupTime: cluster.Status.LastSuccessfulBackup,
		}, nil
	}

	return nil, nil
}

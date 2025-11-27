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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNewDiscovery(t *testing.T) {
	scheme := runtime.NewScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	discovery := NewDiscovery(client)
	if discovery == nil {
		t.Fatal("expected discovery to be non-nil")
	}
	if discovery.client == nil {
		t.Error("expected client to be set")
	}
}

func TestClusterInfo_Structure(t *testing.T) {
	info := ClusterInfo{
		Name:      "test-cluster",
		Namespace: "default",
		Labels: map[string]string{
			"environment": "production",
		},
		Instances: 3,
		Storage: StorageInfo{
			Size:         "10Gi",
			StorageClass: "standard",
			PVCNames:     []string{"test-cluster-1", "test-cluster-2", "test-cluster-3"},
		},
		Status: ClusterStatus{
			Phase:              "Cluster in healthy state",
			Ready:              true,
			ReadyInstances:     3,
			CurrentPrimary:     "test-cluster-1",
			CurrentPrimaryNode: "worker-1",
		},
	}

	if info.Name != "test-cluster" {
		t.Errorf("expected name 'test-cluster', got '%s'", info.Name)
	}
	if info.Instances != 3 {
		t.Errorf("expected 3 instances, got %d", info.Instances)
	}
	if !info.Status.Ready {
		t.Error("expected cluster to be ready")
	}
	if info.Storage.Size != "10Gi" {
		t.Errorf("expected storage size '10Gi', got '%s'", info.Storage.Size)
	}
}

func TestStorageInfo_Fields(t *testing.T) {
	storage := StorageInfo{
		Size:         "50Gi",
		StorageClass: "gp3-csi",
		PVCNames:     []string{"pg-1", "pg-2"},
	}

	if storage.Size != "50Gi" {
		t.Errorf("expected size '50Gi', got '%s'", storage.Size)
	}
	if storage.StorageClass != "gp3-csi" {
		t.Errorf("expected storage class 'gp3-csi', got '%s'", storage.StorageClass)
	}
	if len(storage.PVCNames) != 2 {
		t.Errorf("expected 2 PVC names, got %d", len(storage.PVCNames))
	}
}

func TestClusterStatus_Ready(t *testing.T) {
	tests := []struct {
		name     string
		status   ClusterStatus
		expected bool
	}{
		{
			name: "healthy cluster",
			status: ClusterStatus{
				Phase:          "Cluster in healthy state",
				Ready:          true,
				ReadyInstances: 3,
			},
			expected: true,
		},
		{
			name: "unhealthy cluster",
			status: ClusterStatus{
				Phase:          "Creating",
				Ready:          false,
				ReadyInstances: 0,
			},
			expected: false,
		},
		{
			name: "partially ready",
			status: ClusterStatus{
				Phase:          "Cluster in healthy state",
				Ready:          true,
				ReadyInstances: 2,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.status.Ready != tt.expected {
				t.Errorf("expected ready=%v, got %v", tt.expected, tt.status.Ready)
			}
		})
	}
}

func TestDiscovery_GetClusterPVCs(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	// Create test PVCs
	pvc1 := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-1",
			Namespace: "default",
			Labels: map[string]string{
				"cnpg.io/cluster": "test-cluster",
			},
		},
	}
	pvc2 := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-2",
			Namespace: "default",
			Labels: map[string]string{
				"cnpg.io/cluster": "test-cluster",
			},
		},
	}
	otherPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-cluster-1",
			Namespace: "default",
			Labels: map[string]string{
				"cnpg.io/cluster": "other-cluster",
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(pvc1, pvc2, otherPVC).
		Build()

	discovery := NewDiscovery(client)
	ctx := context.Background()

	pvcs, err := discovery.GetClusterPVCs(ctx, "test-cluster", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pvcs) != 2 {
		t.Errorf("expected 2 PVCs, got %d", len(pvcs))
	}
}

func TestDiscovery_GetClusterPods(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	// Create test pods
	primaryPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-1",
			Namespace: "default",
			Labels: map[string]string{
				"cnpg.io/cluster":      "test-cluster",
				"cnpg.io/instanceRole": "primary",
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
	replicaPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-2",
			Namespace: "default",
			Labels: map[string]string{
				"cnpg.io/cluster":      "test-cluster",
				"cnpg.io/instanceRole": "replica",
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(primaryPod, replicaPod).
		Build()

	discovery := NewDiscovery(client)
	ctx := context.Background()

	pods, err := discovery.GetClusterPods(ctx, "test-cluster", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pods) != 2 {
		t.Errorf("expected 2 pods, got %d", len(pods))
	}
}

func TestDiscovery_GetPrimaryPod(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	// Create test pods
	primaryPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-1",
			Namespace: "default",
			Labels: map[string]string{
				"cnpg.io/cluster":      "test-cluster",
				"cnpg.io/instanceRole": "primary",
			},
		},
	}
	replicaPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-2",
			Namespace: "default",
			Labels: map[string]string{
				"cnpg.io/cluster":      "test-cluster",
				"cnpg.io/instanceRole": "replica",
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(primaryPod, replicaPod).
		Build()

	discovery := NewDiscovery(client)
	ctx := context.Background()

	primary, err := discovery.GetPrimaryPod(ctx, "test-cluster", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if primary.Name != "test-cluster-1" {
		t.Errorf("expected primary pod 'test-cluster-1', got '%s'", primary.Name)
	}
}

func TestDiscovery_GetPrimaryPod_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	// Create only replica pods
	replicaPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-2",
			Namespace: "default",
			Labels: map[string]string{
				"cnpg.io/cluster":      "test-cluster",
				"cnpg.io/instanceRole": "replica",
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(replicaPod).
		Build()

	discovery := NewDiscovery(client)
	ctx := context.Background()

	_, err := discovery.GetPrimaryPod(ctx, "test-cluster", "default")
	if err == nil {
		t.Error("expected error for no primary pod found")
	}
}

func TestExtractClusterInfo(t *testing.T) {
	scheme := runtime.NewScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	discovery := NewDiscovery(client)

	// Create unstructured CNPG cluster
	cluster := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "postgresql.cnpg.io/v1",
			"kind":       "Cluster",
			"metadata": map[string]interface{}{
				"name":      "test-cluster",
				"namespace": "default",
				"labels": map[string]interface{}{
					"environment": "production",
				},
			},
			"spec": map[string]interface{}{
				"instances": int64(3),
				"storage": map[string]interface{}{
					"size":         "10Gi",
					"storageClass": "gp3-csi",
				},
			},
			"status": map[string]interface{}{
				"phase":              "Cluster in healthy state",
				"readyInstances":     int64(3),
				"currentPrimary":     "test-cluster-1",
				"currentPrimaryNode": "worker-1",
			},
		},
	}

	info, err := discovery.extractClusterInfo(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.Name != "test-cluster" {
		t.Errorf("expected name 'test-cluster', got '%s'", info.Name)
	}
	if info.Namespace != "default" {
		t.Errorf("expected namespace 'default', got '%s'", info.Namespace)
	}
	if info.Instances != 3 {
		t.Errorf("expected 3 instances, got %d", info.Instances)
	}
	if info.Storage.Size != "10Gi" {
		t.Errorf("expected storage size '10Gi', got '%s'", info.Storage.Size)
	}
	if info.Storage.StorageClass != "gp3-csi" {
		t.Errorf("expected storage class 'gp3-csi', got '%s'", info.Storage.StorageClass)
	}
	if info.Status.Phase != "Cluster in healthy state" {
		t.Errorf("expected phase 'Cluster in healthy state', got '%s'", info.Status.Phase)
	}
	if !info.Status.Ready {
		t.Error("expected cluster to be ready")
	}
}

func TestExtractClusterInfo_Defaults(t *testing.T) {
	scheme := runtime.NewScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	discovery := NewDiscovery(client)

	// Create minimal unstructured CNPG cluster
	cluster := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "postgresql.cnpg.io/v1",
			"kind":       "Cluster",
			"metadata": map[string]interface{}{
				"name":      "minimal-cluster",
				"namespace": "default",
			},
		},
	}

	info, err := discovery.extractClusterInfo(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should default to 1 instance
	if info.Instances != 1 {
		t.Errorf("expected default of 1 instance, got %d", info.Instances)
	}
}

func TestCNPGClusterGVK(t *testing.T) {
	if CNPGClusterGVK.Group != "postgresql.cnpg.io" {
		t.Errorf("expected group 'postgresql.cnpg.io', got '%s'", CNPGClusterGVK.Group)
	}
	if CNPGClusterGVK.Version != "v1" {
		t.Errorf("expected version 'v1', got '%s'", CNPGClusterGVK.Version)
	}
	if CNPGClusterGVK.Kind != "Cluster" {
		t.Errorf("expected kind 'Cluster', got '%s'", CNPGClusterGVK.Kind)
	}
}

func TestCNPGConstants(t *testing.T) {
	if CNPGGroupVersion != "postgresql.cnpg.io/v1" {
		t.Errorf("expected group version 'postgresql.cnpg.io/v1', got '%s'", CNPGGroupVersion)
	}
	if CNPGKind != "Cluster" {
		t.Errorf("expected kind 'Cluster', got '%s'", CNPGKind)
	}
}

func TestClusterInfo_Labels(t *testing.T) {
	info := ClusterInfo{
		Name:      "test",
		Namespace: "default",
		Labels: map[string]string{
			"environment": "production",
			"team":        "database",
		},
	}

	if info.Labels["environment"] != "production" {
		t.Error("expected environment=production label")
	}
	if info.Labels["team"] != "database" {
		t.Error("expected team=database label")
	}
}

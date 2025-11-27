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
	"testing"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	cnpgv1alpha1 "github.com/supporttools/cnpg-storage-manager/api/v1alpha1"
)

func TestExpansionEngine_ExpandClusterPVCs(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = storagev1.AddToScheme(scheme)
	_ = cnpgv1alpha1.AddToScheme(scheme)

	allowExpansion := true
	storageClass := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "expandable-sc",
		},
		Provisioner:          "kubernetes.io/aws-ebs",
		AllowVolumeExpansion: &allowExpansion,
	}

	tests := []struct {
		name             string
		pvcs             []corev1.PersistentVolumeClaim
		policy           *cnpgv1alpha1.StoragePolicy
		dryRun           bool
		expectedSuccess  bool
		expectedExpanded int
		expectedSkipped  int
	}{
		{
			name: "expand single PVC",
			pvcs: []corev1.PersistentVolumeClaim{
				createTestPVC("test-pvc-1", "default", "expandable-sc", "10Gi"),
			},
			policy:           createTestPolicy(50, 5, nil),
			dryRun:           false,
			expectedSuccess:  true,
			expectedExpanded: 1,
			expectedSkipped:  0,
		},
		{
			name: "expand multiple PVCs",
			pvcs: []corev1.PersistentVolumeClaim{
				createTestPVC("test-pvc-1", "default", "expandable-sc", "10Gi"),
				createTestPVC("test-pvc-2", "default", "expandable-sc", "20Gi"),
				createTestPVC("test-pvc-3", "default", "expandable-sc", "15Gi"),
			},
			policy:           createTestPolicy(50, 5, nil),
			dryRun:           false,
			expectedSuccess:  true,
			expectedExpanded: 3,
			expectedSkipped:  0,
		},
		{
			name: "dry run mode",
			pvcs: []corev1.PersistentVolumeClaim{
				createTestPVC("test-pvc-1", "default", "expandable-sc", "10Gi"),
			},
			policy:           createTestPolicy(50, 5, nil),
			dryRun:           true,
			expectedSuccess:  true,
			expectedExpanded: 1,
			expectedSkipped:  0,
		},
		{
			name: "skip PVC at max size",
			pvcs: []corev1.PersistentVolumeClaim{
				createTestPVC("test-pvc-1", "default", "expandable-sc", "100Gi"),
			},
			policy:           createTestPolicy(50, 5, quantityPtr(resource.MustParse("100Gi"))),
			dryRun:           false,
			expectedSuccess:  true,
			expectedExpanded: 0,
			expectedSkipped:  1,
		},
		{
			name:             "no PVCs",
			pvcs:             []corev1.PersistentVolumeClaim{},
			policy:           createTestPolicy(50, 5, nil),
			dryRun:           false,
			expectedSuccess:  true,
			expectedExpanded: 0,
			expectedSkipped:  0,
		},
		{
			name: "skip unbound PVC",
			pvcs: []corev1.PersistentVolumeClaim{
				createUnboundPVC("test-pvc-1", "default", "expandable-sc", "10Gi"),
			},
			policy:           createTestPolicy(50, 5, nil),
			dryRun:           false,
			expectedSuccess:  true,
			expectedExpanded: 0,
			expectedSkipped:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build objects for fake client
			objs := []runtime.Object{storageClass}
			for i := range tt.pvcs {
				objs = append(objs, &tt.pvcs[i])
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

			engine := NewExpansionEngine(client)

			req := &ExpansionRequest{
				ClusterName:      "test-cluster",
				ClusterNamespace: "default",
				PVCs:             tt.pvcs,
				Policy:           tt.policy,
				Reason:           "threshold breach",
				DryRun:           tt.dryRun,
			}

			result, err := engine.ExpandClusterPVCs(context.Background(), req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Success != tt.expectedSuccess {
				t.Errorf("expected success=%v, got %v", tt.expectedSuccess, result.Success)
			}

			expanded := 0
			skipped := 0
			for _, pvcResult := range result.PVCResults {
				if pvcResult.Skipped {
					skipped++
				} else if pvcResult.Success {
					expanded++
				}
			}

			if expanded != tt.expectedExpanded {
				t.Errorf("expected %d expanded PVCs, got %d", tt.expectedExpanded, expanded)
			}
			if skipped != tt.expectedSkipped {
				t.Errorf("expected %d skipped PVCs, got %d", tt.expectedSkipped, skipped)
			}
		})
	}
}

func TestExpansionEngine_ExpansionCalculation(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = storagev1.AddToScheme(scheme)

	allowExpansion := true
	storageClass := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "expandable-sc",
		},
		Provisioner:          "kubernetes.io/aws-ebs",
		AllowVolumeExpansion: &allowExpansion,
	}

	tests := []struct {
		name            string
		currentSize     string
		percentage      int32
		minIncrementGi  int32
		maxSize         *resource.Quantity
		expectedNewSize string
	}{
		{
			name:            "50% expansion of 10Gi",
			currentSize:     "10Gi",
			percentage:      50,
			minIncrementGi:  5,
			maxSize:         nil,
			expectedNewSize: "15Gi", // 10Gi + 5Gi (50% = 5Gi, which equals min)
		},
		{
			name:            "20% expansion uses min increment",
			currentSize:     "10Gi",
			percentage:      20,
			minIncrementGi:  5,
			maxSize:         nil,
			expectedNewSize: "15Gi", // 10Gi + 5Gi (20% = 2Gi, but min is 5Gi)
		},
		{
			name:            "100% expansion of 20Gi",
			currentSize:     "20Gi",
			percentage:      100,
			minIncrementGi:  5,
			maxSize:         nil,
			expectedNewSize: "40Gi", // 20Gi + 20Gi (100%)
		},
		{
			name:            "expansion capped by max size",
			currentSize:     "90Gi",
			percentage:      50,
			minIncrementGi:  5,
			maxSize:         quantityPtr(resource.MustParse("100Gi")),
			expectedNewSize: "100Gi", // 90Gi + 45Gi would exceed max, capped at 100Gi
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pvc := createTestPVC("test-pvc", "default", "expandable-sc", tt.currentSize)
			client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(storageClass, &pvc).Build()

			engine := NewExpansionEngine(client)

			policy := createTestPolicy(tt.percentage, tt.minIncrementGi, tt.maxSize)
			req := &ExpansionRequest{
				ClusterName:      "test-cluster",
				ClusterNamespace: "default",
				PVCs:             []corev1.PersistentVolumeClaim{pvc},
				Policy:           policy,
				Reason:           "test",
				DryRun:           true, // Use dry run to just check calculation
			}

			result, err := engine.ExpandClusterPVCs(context.Background(), req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(result.PVCResults) != 1 {
				t.Fatalf("expected 1 PVC result, got %d", len(result.PVCResults))
			}

			pvcResult := result.PVCResults[0]
			if pvcResult.Skipped {
				// For max size test where we're already at max
				return
			}

			expectedSize := resource.MustParse(tt.expectedNewSize)
			if pvcResult.NewSize.Cmp(expectedSize) != 0 {
				t.Errorf("expected new size %s, got %s", expectedSize.String(), pvcResult.NewSize.String())
			}
		})
	}
}

func TestExpansionEngine_NonExpandableStorageClass(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = storagev1.AddToScheme(scheme)

	denyExpansion := false
	storageClass := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "non-expandable-sc",
		},
		Provisioner:          "kubernetes.io/aws-ebs",
		AllowVolumeExpansion: &denyExpansion,
	}

	pvc := createTestPVCWithStorageClass("test-pvc", "default", "non-expandable-sc", "10Gi")
	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(storageClass, &pvc).Build()

	engine := NewExpansionEngine(client)

	policy := createTestPolicy(50, 5, nil)
	req := &ExpansionRequest{
		ClusterName:      "test-cluster",
		ClusterNamespace: "default",
		PVCs:             []corev1.PersistentVolumeClaim{pvc},
		Policy:           policy,
		Reason:           "test",
		DryRun:           false,
	}

	result, err := engine.ExpandClusterPVCs(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.PVCResults) != 1 {
		t.Fatalf("expected 1 PVC result, got %d", len(result.PVCResults))
	}

	pvcResult := result.PVCResults[0]
	if !pvcResult.Skipped {
		t.Error("expected PVC to be skipped due to non-expandable storage class")
	}
}

func TestExpansionEngine_TotalBytesAdded(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = storagev1.AddToScheme(scheme)

	allowExpansion := true
	storageClass := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "expandable-sc",
		},
		Provisioner:          "kubernetes.io/aws-ebs",
		AllowVolumeExpansion: &allowExpansion,
	}

	// Create PVCs where expansion would be exactly 5Gi each (min increment)
	pvcs := []corev1.PersistentVolumeClaim{
		createTestPVC("test-pvc-1", "default", "expandable-sc", "8Gi"),
		createTestPVC("test-pvc-2", "default", "expandable-sc", "8Gi"),
	}

	objs := []runtime.Object{storageClass}
	for i := range pvcs {
		objs = append(objs, &pvcs[i])
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

	engine := NewExpansionEngine(client)

	// 10% of 8Gi = 0.8Gi, but min increment is 5Gi, so each PVC gets 5Gi added
	policy := createTestPolicy(10, 5, nil)
	req := &ExpansionRequest{
		ClusterName:      "test-cluster",
		ClusterNamespace: "default",
		PVCs:             pvcs,
		Policy:           policy,
		Reason:           "test",
		DryRun:           false,
	}

	result, err := engine.ExpandClusterPVCs(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Each PVC should have 5Gi added (min increment), total 10Gi
	expectedTotal := int64(10 * 1024 * 1024 * 1024)
	if result.TotalBytesAdded != expectedTotal {
		t.Errorf("expected TotalBytesAdded=%d, got %d", expectedTotal, result.TotalBytesAdded)
	}
}

// Helper functions for tests

func createTestPVC(name, namespace, storageClassName, size string) corev1.PersistentVolumeClaim {
	scName := storageClassName
	return corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &scName,
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(size),
				},
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimBound,
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse(size),
			},
		},
	}
}

func createTestPVCWithStorageClass(name, namespace, storageClassName, size string) corev1.PersistentVolumeClaim {
	return createTestPVC(name, namespace, storageClassName, size)
}

func createUnboundPVC(name, namespace, storageClassName, size string) corev1.PersistentVolumeClaim {
	pvc := createTestPVC(name, namespace, storageClassName, size)
	pvc.Status.Phase = corev1.ClaimPending
	return pvc
}

func createTestPolicy(percentage, minIncrementGi int32, maxSize *resource.Quantity) *cnpgv1alpha1.StoragePolicy {
	return &cnpgv1alpha1.StoragePolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: "default",
		},
		Spec: cnpgv1alpha1.StoragePolicySpec{
			Expansion: cnpgv1alpha1.ExpansionConfig{
				Enabled:        true,
				Percentage:     percentage,
				MinIncrementGi: minIncrementGi,
				MaxSize:        maxSize,
			},
		},
	}
}

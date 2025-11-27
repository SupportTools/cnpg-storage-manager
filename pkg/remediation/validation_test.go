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

	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestStorageClassValidator_ValidateStorageClass(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = storagev1.AddToScheme(scheme)

	allowExpansion := true
	denyExpansion := false

	tests := []struct {
		name             string
		storageClassName string
		storageClasses   []storagev1.StorageClass
		expectValid      bool
		expectError      bool
	}{
		{
			name:             "storage class supports expansion",
			storageClassName: "expandable-sc",
			storageClasses: []storagev1.StorageClass{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "expandable-sc",
					},
					Provisioner:          "kubernetes.io/aws-ebs",
					AllowVolumeExpansion: &allowExpansion,
				},
			},
			expectValid: true,
			expectError: false,
		},
		{
			name:             "storage class does not support expansion",
			storageClassName: "no-expand-sc",
			storageClasses: []storagev1.StorageClass{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "no-expand-sc",
					},
					Provisioner:          "kubernetes.io/aws-ebs",
					AllowVolumeExpansion: &denyExpansion,
				},
			},
			expectValid: false,
			expectError: false,
		},
		{
			name:             "storage class with nil allowVolumeExpansion",
			storageClassName: "nil-expand-sc",
			storageClasses: []storagev1.StorageClass{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "nil-expand-sc",
					},
					Provisioner: "kubernetes.io/aws-ebs",
					// AllowVolumeExpansion is nil
				},
			},
			expectValid: false,
			expectError: false,
		},
		{
			name:             "storage class not found",
			storageClassName: "nonexistent-sc",
			storageClasses:   []storagev1.StorageClass{},
			expectValid:      false,
			expectError:      false, // Returns invalid with message, not an error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build fake client with storage classes
			objs := make([]runtime.Object, len(tt.storageClasses))
			for i := range tt.storageClasses {
				objs[i] = &tt.storageClasses[i]
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

			validator := NewStorageClassValidator(client)
			result, err := validator.ValidateStorageClass(context.Background(), tt.storageClassName)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if result.Valid != tt.expectValid {
				t.Errorf("expected valid=%v, got valid=%v", tt.expectValid, result.Valid)
			}
		})
	}
}

func TestStorageClassValidator_Caching(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = storagev1.AddToScheme(scheme)

	allowExpansion := true
	sc := storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sc",
		},
		Provisioner:          "kubernetes.io/aws-ebs",
		AllowVolumeExpansion: &allowExpansion,
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(&sc).Build()
	validator := NewStorageClassValidator(client)

	// First call should populate cache
	_, err := validator.ValidateStorageClass(context.Background(), "test-sc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify cache is populated
	caps, exists := validator.GetCachedCapabilities("test-sc")
	if !exists {
		t.Error("expected cache to be populated")
	}
	if !caps.AllowVolumeExpansion {
		t.Error("expected AllowVolumeExpansion to be true")
	}

	// Invalidate cache
	validator.InvalidateCache("test-sc")
	_, exists = validator.GetCachedCapabilities("test-sc")
	if exists {
		t.Error("expected cache to be invalidated")
	}

	// Invalidate all cache
	_, _ = validator.ValidateStorageClass(context.Background(), "test-sc")
	validator.InvalidateAllCache()
	_, exists = validator.GetCachedCapabilities("test-sc")
	if exists {
		t.Error("expected all cache to be invalidated")
	}
}

func TestStorageClassValidator_CanExpand(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = storagev1.AddToScheme(scheme)

	allowExpansion := true
	sc := storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "expandable-sc",
		},
		Provisioner:          "kubernetes.io/aws-ebs",
		AllowVolumeExpansion: &allowExpansion,
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(&sc).Build()
	validator := NewStorageClassValidator(client)

	canExpand, message, err := validator.CanExpand(context.Background(), "expandable-sc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !canExpand {
		t.Error("expected canExpand to be true")
	}
	if message == "" {
		t.Error("expected non-empty message")
	}
}

func TestStorageClassValidator_ValidatePVCForExpansion(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = storagev1.AddToScheme(scheme)

	allowExpansion := true
	sc := storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "expandable-sc",
		},
		Provisioner:          "kubernetes.io/aws-ebs",
		AllowVolumeExpansion: &allowExpansion,
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(&sc).Build()
	validator := NewStorageClassValidator(client)

	tests := []struct {
		name         string
		pvcInfo      *PVCInfo
		expectExpand bool
	}{
		{
			name: "valid PVC for expansion",
			pvcInfo: &PVCInfo{
				Name:             "test-pvc",
				Namespace:        "default",
				StorageClassName: "expandable-sc",
				Phase:            "Bound",
				AccessModes:      []string{"ReadWriteOnce"},
				CurrentSize:      10 * 1024 * 1024 * 1024,
			},
			expectExpand: true,
		},
		{
			name: "PVC not bound",
			pvcInfo: &PVCInfo{
				Name:             "test-pvc",
				Namespace:        "default",
				StorageClassName: "expandable-sc",
				Phase:            "Pending",
				AccessModes:      []string{"ReadWriteOnce"},
				CurrentSize:      10 * 1024 * 1024 * 1024,
			},
			expectExpand: false,
		},
		{
			name: "PVC without storage class",
			pvcInfo: &PVCInfo{
				Name:             "test-pvc",
				Namespace:        "default",
				StorageClassName: "",
				Phase:            "Bound",
				AccessModes:      []string{"ReadWriteOnce"},
				CurrentSize:      10 * 1024 * 1024 * 1024,
			},
			expectExpand: false,
		},
		{
			name: "PVC with ReadOnlyMany access mode",
			pvcInfo: &PVCInfo{
				Name:             "test-pvc",
				Namespace:        "default",
				StorageClassName: "expandable-sc",
				Phase:            "Bound",
				AccessModes:      []string{"ReadOnlyMany"},
				CurrentSize:      10 * 1024 * 1024 * 1024,
			},
			expectExpand: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validator.ValidatePVCForExpansion(context.Background(), tt.pvcInfo)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.CanExpand != tt.expectExpand {
				t.Errorf("expected CanExpand=%v, got %v", tt.expectExpand, result.CanExpand)
			}
		})
	}
}

func TestPreflightResult_FailedChecks(t *testing.T) {
	result := &PreflightResult{
		PVCName:   "test-pvc",
		Namespace: "default",
		Checks: []PreflightCheck{
			{Name: "check1", Passed: true, Message: "passed"},
			{Name: "check2", Passed: false, Message: "failed"},
			{Name: "check3", Passed: true, Message: "passed"},
			{Name: "check4", Passed: false, Message: "failed"},
		},
	}

	failed := result.FailedChecks()
	if len(failed) != 2 {
		t.Errorf("expected 2 failed checks, got %d", len(failed))
	}
}

func TestPreflightResult_Summary(t *testing.T) {
	tests := []struct {
		name     string
		result   *PreflightResult
		contains string
	}{
		{
			name: "all passed",
			result: &PreflightResult{
				PVCName:   "test-pvc",
				Namespace: "default",
				CanExpand: true,
				Checks: []PreflightCheck{
					{Name: "check1", Passed: true},
					{Name: "check2", Passed: true},
				},
			},
			contains: "passed all 2",
		},
		{
			name: "some failed",
			result: &PreflightResult{
				PVCName:   "test-pvc",
				Namespace: "default",
				CanExpand: false,
				Checks: []PreflightCheck{
					{Name: "check1", Passed: true},
					{Name: "check2", Passed: false},
				},
			},
			contains: "failed 1 of 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := tt.result.Summary()
			if summary == "" {
				t.Error("expected non-empty summary")
			}
			// Check that summary contains expected substring
			if !containsString(summary, tt.contains) {
				t.Errorf("expected summary to contain %q, got %q", tt.contains, summary)
			}
		})
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStringHelper(s, substr))
}

func containsStringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

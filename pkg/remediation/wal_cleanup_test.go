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
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cnpgv1alpha1 "github.com/supporttools/cnpg-storage-manager/api/v1alpha1"
)

func TestWALCleanupRequest_Fields(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-1",
			Namespace: "default",
		},
	}

	policy := &cnpgv1alpha1.StoragePolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: "default",
		},
		Spec: cnpgv1alpha1.StoragePolicySpec{
			WALCleanup: cnpgv1alpha1.WALCleanupConfig{
				Enabled:         true,
				RetainCount:     10,
				RequireArchived: true,
				CooldownMinutes: 15,
			},
		},
	}

	request := &WALCleanupRequest{
		ClusterName:      "test-cluster",
		ClusterNamespace: "default",
		PrimaryPod:       pod,
		Policy:           policy,
		Reason:           "Emergency threshold breach",
		DryRun:           false,
	}

	if request.ClusterName != "test-cluster" {
		t.Errorf("expected cluster name 'test-cluster', got '%s'", request.ClusterName)
	}
	if request.ClusterNamespace != "default" {
		t.Errorf("expected namespace 'default', got '%s'", request.ClusterNamespace)
	}
	if request.PrimaryPod == nil {
		t.Error("expected primary pod to be set")
	}
	if request.Policy == nil {
		t.Error("expected policy to be set")
	}
	if request.DryRun {
		t.Error("expected dry run to be false")
	}
}

func TestWALCleanupResult_Fields(t *testing.T) {
	result := &WALCleanupResult{
		ClusterName:      "test-cluster",
		ClusterNamespace: "default",
		PodName:          "test-cluster-1",
		Success:          true,
		FilesRemoved:     25,
		BytesFreed:       419430400, // 400MB
		WALFilesChecked:  50,
		ArchivedCount:    45,
		RetainedCount:    25,
		Duration:         5 * time.Second,
		Error:            "",
	}

	if !result.Success {
		t.Error("expected success to be true")
	}
	if result.FilesRemoved != 25 {
		t.Errorf("expected 25 files removed, got %d", result.FilesRemoved)
	}
	if result.BytesFreed != 419430400 {
		t.Errorf("expected 419430400 bytes freed, got %d", result.BytesFreed)
	}
	if result.PodName != "test-cluster-1" {
		t.Errorf("expected pod name 'test-cluster-1', got '%s'", result.PodName)
	}
	if result.WALFilesChecked != 50 {
		t.Errorf("expected 50 files checked, got %d", result.WALFilesChecked)
	}
}

func TestWALCleanupResult_Failure(t *testing.T) {
	result := &WALCleanupResult{
		ClusterName:      "test-cluster",
		ClusterNamespace: "default",
		PodName:          "test-cluster-1",
		Success:          false,
		FilesRemoved:     0,
		BytesFreed:       0,
		Error:            "failed to list WAL files",
	}

	if result.Success {
		t.Error("expected success to be false")
	}
	if result.Error == "" {
		t.Error("expected error message to be set")
	}
	if result.FilesRemoved != 0 {
		t.Errorf("expected 0 files removed, got %d", result.FilesRemoved)
	}
}

func TestWALFileInfo_Structure(t *testing.T) {
	files := []WALFileInfo{
		{Name: "000000010000000000000003", Size: 16777216, IsArchived: true},
		{Name: "000000010000000000000001", Size: 16777216, IsArchived: true},
		{Name: "000000010000000000000002", Size: 16777216, IsArchived: false},
	}

	// Verify the files have correct structure
	for _, f := range files {
		if f.Name == "" {
			t.Error("file name should not be empty")
		}
		if f.Size <= 0 {
			t.Error("file size should be positive")
		}
	}

	// Check file ordering by name
	if files[0].Name <= files[1].Name {
		t.Log("WAL file names can be compared for sorting")
	}

	// Check archived status
	archivedCount := 0
	for _, f := range files {
		if f.IsArchived {
			archivedCount++
		}
	}
	if archivedCount != 2 {
		t.Errorf("expected 2 archived files, got %d", archivedCount)
	}
}

func TestWALCleanupConfig_Values(t *testing.T) {
	tests := []struct {
		name   string
		config cnpgv1alpha1.WALCleanupConfig
	}{
		{
			name: "default config",
			config: cnpgv1alpha1.WALCleanupConfig{
				Enabled:         true,
				RetainCount:     10,
				RequireArchived: true,
				CooldownMinutes: 15,
			},
		},
		{
			name: "aggressive cleanup",
			config: cnpgv1alpha1.WALCleanupConfig{
				Enabled:         true,
				RetainCount:     5,
				RequireArchived: false,
				CooldownMinutes: 5,
			},
		},
		{
			name: "conservative cleanup",
			config: cnpgv1alpha1.WALCleanupConfig{
				Enabled:         true,
				RetainCount:     50,
				RequireArchived: true,
				CooldownMinutes: 60,
			},
		},
		{
			name: "disabled",
			config: cnpgv1alpha1.WALCleanupConfig{
				Enabled: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.config.Enabled && tt.config.RetainCount <= 0 {
				t.Error("enabled config should have positive retain count")
			}
		})
	}
}

func TestWALCleanupRequest_DryRun(t *testing.T) {
	request := &WALCleanupRequest{
		ClusterName:      "test-cluster",
		ClusterNamespace: "default",
		DryRun:           true,
	}

	if !request.DryRun {
		t.Error("expected dry run to be true")
	}
}

func TestWALFileSorting(t *testing.T) {
	files := []WALFileInfo{
		{Name: "000000010000000000000005"},
		{Name: "000000010000000000000001"},
		{Name: "000000010000000000000010"},
		{Name: "000000010000000000000003"},
	}

	// Sort by name (alphabetically, which is chronological for WAL files)
	for i := 0; i < len(files)-1; i++ {
		for j := i + 1; j < len(files); j++ {
			if files[i].Name > files[j].Name {
				files[i], files[j] = files[j], files[i]
			}
		}
	}

	// Verify ordering
	expected := []string{
		"000000010000000000000001",
		"000000010000000000000003",
		"000000010000000000000005",
		"000000010000000000000010",
	}

	for i, exp := range expected {
		if files[i].Name != exp {
			t.Errorf("position %d: expected %s, got %s", i, exp, files[i].Name)
		}
	}
}

func TestRetainCountLogic(t *testing.T) {
	tests := []struct {
		name          string
		totalFiles    int
		retainCount   int
		expectedKeep  int
		expectedClean int
	}{
		{
			name:          "more files than retain",
			totalFiles:    50,
			retainCount:   10,
			expectedKeep:  10,
			expectedClean: 40,
		},
		{
			name:          "fewer files than retain",
			totalFiles:    5,
			retainCount:   10,
			expectedKeep:  5,
			expectedClean: 0,
		},
		{
			name:          "exact retain count",
			totalFiles:    10,
			retainCount:   10,
			expectedKeep:  10,
			expectedClean: 0,
		},
		{
			name:          "no files",
			totalFiles:    0,
			retainCount:   10,
			expectedKeep:  0,
			expectedClean: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var toClean int
			if tt.totalFiles > tt.retainCount {
				toClean = tt.totalFiles - tt.retainCount
			}
			toKeep := tt.totalFiles - toClean

			if toKeep != tt.expectedKeep {
				t.Errorf("expected %d files to keep, got %d", tt.expectedKeep, toKeep)
			}
			if toClean != tt.expectedClean {
				t.Errorf("expected %d files to clean, got %d", tt.expectedClean, toClean)
			}
		})
	}
}

func TestWALCleanupDuration(t *testing.T) {
	result := &WALCleanupResult{
		Duration: 2500 * time.Millisecond,
	}

	if result.Duration.Seconds() < 2.4 || result.Duration.Seconds() > 2.6 {
		t.Errorf("expected duration around 2.5 seconds, got %v", result.Duration)
	}
}

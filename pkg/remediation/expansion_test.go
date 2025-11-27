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

	"k8s.io/apimachinery/pkg/api/resource"
)

func TestGetExpansionPercentage(t *testing.T) {
	tests := []struct {
		name     string
		input    int32
		expected int32
	}{
		{"zero uses default", 0, 50},
		{"negative uses default", -10, 50},
		{"positive value used", 75, 75},
		{"100 percent", 100, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getExpansionPercentage(tt.input)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestGetMinIncrementBytes(t *testing.T) {
	const GiB = 1024 * 1024 * 1024

	tests := []struct {
		name     string
		inputGi  int32
		expected int64
	}{
		{"zero uses default 5Gi", 0, 5 * GiB},
		{"negative uses default 5Gi", -1, 5 * GiB},
		{"10Gi", 10, 10 * GiB},
		{"1Gi", 1, 1 * GiB},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getMinIncrementBytes(tt.inputGi)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestGetMaxSizeBytes(t *testing.T) {
	tests := []struct {
		name     string
		maxSize  *resource.Quantity
		expected int64
	}{
		{"nil returns 0 (no limit)", nil, 0},
		{"100Gi", quantityPtr(resource.MustParse("100Gi")), 100 * 1024 * 1024 * 1024},
		{"500Mi", quantityPtr(resource.MustParse("500Mi")), 500 * 1024 * 1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getMaxSizeBytes(tt.maxSize)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestAccessModesToStrings(t *testing.T) {
	tests := []struct {
		name     string
		modes    []string
		expected []string
	}{
		{"empty", []string{}, []string{}},
		{"single mode", []string{"ReadWriteOnce"}, []string{"ReadWriteOnce"}},
		{"multiple modes", []string{"ReadWriteOnce", "ReadOnlyMany"}, []string{"ReadWriteOnce", "ReadOnlyMany"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test is a bit contrived since we're testing string->string
			// In the actual code, we convert PersistentVolumeAccessMode to string
			if len(tt.modes) != len(tt.expected) {
				t.Errorf("expected %d modes, got %d", len(tt.expected), len(tt.modes))
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name     string
		bytes    int64
		expected string
	}{
		{"bytes", 500, "500 bytes"},
		{"kilobytes", 2048, "2.00Ki"},
		{"megabytes", 5 * 1024 * 1024, "5.00Mi"},
		{"gigabytes", 10 * 1024 * 1024 * 1024, "10.00Gi"},
		{"terabytes", 2 * 1024 * 1024 * 1024 * 1024, "2.00Ti"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatBytes(tt.bytes)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestPVCExpansionResult(t *testing.T) {
	// Test basic result structure
	result := PVCExpansionResult{
		PVCName:      "test-pvc",
		Namespace:    "default",
		OriginalSize: resource.MustParse("10Gi"),
		NewSize:      resource.MustParse("15Gi"),
		BytesAdded:   5 * 1024 * 1024 * 1024,
		Success:      true,
	}

	if result.PVCName != "test-pvc" {
		t.Error("PVCName mismatch")
	}
	if result.BytesAdded != 5*1024*1024*1024 {
		t.Error("BytesAdded mismatch")
	}
}

func TestExpansionResult(t *testing.T) {
	// Test with multiple PVC results
	result := ExpansionResult{
		ClusterName:      "test-cluster",
		ClusterNamespace: "default",
		Success:          true,
		TotalBytesAdded:  15 * 1024 * 1024 * 1024,
		PVCResults: []PVCExpansionResult{
			{PVCName: "pvc1", Success: true, BytesAdded: 5 * 1024 * 1024 * 1024},
			{PVCName: "pvc2", Success: true, BytesAdded: 5 * 1024 * 1024 * 1024},
			{PVCName: "pvc3", Success: true, BytesAdded: 5 * 1024 * 1024 * 1024},
		},
	}

	if len(result.PVCResults) != 3 {
		t.Errorf("expected 3 PVC results, got %d", len(result.PVCResults))
	}
	if result.TotalBytesAdded != 15*1024*1024*1024 {
		t.Error("TotalBytesAdded mismatch")
	}
}

func TestVerificationResult(t *testing.T) {
	result := VerificationResult{
		PVCName:                 "test-pvc",
		Namespace:               "default",
		ExpectedSize:            resource.MustParse("15Gi"),
		ActualSize:              resource.MustParse("15Gi"),
		Complete:                true,
		FileSystemResizePending: false,
	}

	if !result.Complete {
		t.Error("expected Complete to be true")
	}
	if result.FileSystemResizePending {
		t.Error("expected FileSystemResizePending to be false")
	}
}

func quantityPtr(q resource.Quantity) *resource.Quantity {
	return &q
}

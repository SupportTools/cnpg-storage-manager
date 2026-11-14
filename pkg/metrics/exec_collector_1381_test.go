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

// Regression tests for cluster-services-1381: local-path volumes must be measured
// as their own directory against the PVC size, not as the node filesystem.

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const gi = int64(1) << 30

func TestIsSharedFilesystem(t *testing.T) {
	cases := []struct {
		name         string
		fsTotal, pvc int64
		want         bool
	}{
		// The live a1-ops-prd case: a 16Gi local-path PVC on a 357Gi node md array.
		{"local-path on node fs", 383664189440, 16 * gi, true},
		{"own block volume formatted slightly under size", 15*gi + gi/2, 16 * gi, false},
		{"own block volume exactly the size", 10 * gi, 10 * gi, false},
		{"just inside slack", 10 * gi * 109 / 100, 10 * gi, false},
		{"just over slack", 10 * gi * 111 / 100, 10 * gi, true},
		{"pvc size unknown", 1000 * gi, 0, false},
		{"df total unknown", 0, 10 * gi, false},
	}
	for _, tc := range cases {
		if got := IsSharedFilesystem(tc.fsTotal, tc.pvc); got != tc.want {
			t.Errorf("%s: IsSharedFilesystem(%d, %d) = %v, want %v", tc.name, tc.fsTotal, tc.pvc, got, tc.want)
		}
	}
}

func TestApplyDirectoryUsage(t *testing.T) {
	// df of the node fs: 1072Gi total, 790Gi used (73.7%), 282Gi free.
	m := PVCMetrics{UsedBytes: 790 * gi, CapacityBytes: 1072 * gi, AvailableBytes: 282 * gi,
		Inodes: 1000, InodesUsed: 500, InodesFree: 500}
	ApplyDirectoryUsage(&m, 1440641024, 10*gi, 282*gi)

	if m.CapacityBytes != 10*gi {
		t.Errorf("capacity = %d, want PVC size %d", m.CapacityBytes, 10*gi)
	}
	if m.UsedBytes != 1440641024 {
		t.Errorf("used = %d, want du result", m.UsedBytes)
	}
	if m.AvailableBytes != 10*gi-1440641024 {
		t.Errorf("available = %d, want %d", m.AvailableBytes, 10*gi-1440641024)
	}
	if p := m.UsagePercent(); p < 13 || p > 14 {
		t.Errorf("usage = %.1f%%, want ~13.4%% (not the node fs 73.7%%)", p)
	}
	if m.Inodes != 0 || m.InodesUsed != 0 || m.InodesFree != 0 {
		t.Error("shared-fs inode figures must be cleared")
	}
}

func TestApplyDirectoryUsage_AvailableCappedByFilesystemAndFloor(t *testing.T) {
	m := PVCMetrics{}
	// Node fs almost full: only 1Gi free though the PVC has 8Gi headroom.
	ApplyDirectoryUsage(&m, 2*gi, 10*gi, gi)
	if m.AvailableBytes != gi {
		t.Errorf("available = %d, want capped at fs free %d", m.AvailableBytes, gi)
	}
	// Directory larger than the PVC (local-path does not enforce size).
	ApplyDirectoryUsage(&m, 12*gi, 10*gi, 100*gi)
	if m.AvailableBytes != 0 {
		t.Errorf("available = %d, want 0", m.AvailableBytes)
	}
	if m.UsagePercent() <= 100 {
		t.Errorf("usage = %.1f, want > 100 so thresholds fire", m.UsagePercent())
	}
}

func TestParseDuOutput(t *testing.T) {
	n, err := ParseDuOutput("1440641024\t/var/lib/postgresql/data\n")
	if err != nil || n != 1440641024 {
		t.Fatalf("got %d, %v", n, err)
	}
	if _, err := ParseDuOutput(""); err == nil {
		t.Error("empty output must error")
	}
	if _, err := ParseDuOutput("du: cannot read directory"); err == nil {
		t.Error("garbage must error")
	}
}

func TestPVCProvisionedBytes(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{}
	if PVCProvisionedBytes(pvc) != 0 {
		t.Error("empty PVC must be 0")
	}
	pvc.Spec.Resources.Requests = corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")}
	if PVCProvisionedBytes(pvc) != 10*gi {
		t.Error("want spec request when unbound")
	}
	pvc.Status.Capacity = corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("16Gi")}
	if PVCProvisionedBytes(pvc) != 16*gi {
		t.Error("want status.capacity when bound")
	}
}

func TestParseArchiverEpoch(t *testing.T) {
	tm, err := ParseArchiverEpoch("1763121600\n")
	if err != nil || tm == nil || !tm.Equal(time.Unix(1763121600, 0)) {
		t.Fatalf("got %v, %v", tm, err)
	}
	tm, err = ParseArchiverEpoch("\n")
	if err != nil || tm != nil {
		t.Fatalf("never archived: got %v, %v; want nil, nil", tm, err)
	}
	if _, err := ParseArchiverEpoch("ERROR: x"); err == nil {
		t.Error("garbage must error")
	}
}

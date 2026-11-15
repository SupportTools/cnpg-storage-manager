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
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	cnpgv1alpha1 "github.com/supporttools/cnpg-storage-manager/api/v1alpha1"
)

func sc(name, provisioner string, allow *bool) *storagev1.StorageClass {
	return &storagev1.StorageClass{
		ObjectMeta:           metav1.ObjectMeta{Name: name},
		Provisioner:          provisioner,
		AllowVolumeExpansion: allow,
	}
}

func boundPVC(name, class string) corev1.PersistentVolumeClaim {
	return corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "db"},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &class,
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")},
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	}
}

func supportEngine(t *testing.T, objs ...runtime.Object) *ExpansionEngine {
	t.Helper()
	scheme := runtime.NewScheme()
	_ = storagev1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	return NewExpansionEngine(fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build())
}

// cluster-services-1399/1413: local-path cannot expand even when its StorageClass claims it can.
func TestClusterExpansionSupport(t *testing.T) {
	yes, no := true, false
	tests := []struct {
		name      string
		classes   []runtime.Object
		pvcs      []corev1.PersistentVolumeClaim
		supported bool
		reason    string
	}{
		{
			name:      "expandable CSI class",
			classes:   []runtime.Object{sc("longhorn", "driver.longhorn.io", &yes)},
			pvcs:      []corev1.PersistentVolumeClaim{boundPVC("db-1", "longhorn"), boundPVC("db-2", "longhorn")},
			supported: true,
		},
		{
			name:      "local-path with allowVolumeExpansion true is still not expandable",
			classes:   []runtime.Object{sc("local-path-cnpg", "rancher.io/local-path", &yes)},
			pvcs:      []corev1.PersistentVolumeClaim{boundPVC("db-1", "local-path-cnpg")},
			supported: false,
			reason:    "no resizer acts on them",
		},
		{
			name:      "allowVolumeExpansion false",
			classes:   []runtime.Object{sc("fixed", "driver.longhorn.io", &no)},
			pvcs:      []corev1.PersistentVolumeClaim{boundPVC("db-1", "fixed")},
			supported: false,
			reason:    "allowVolumeExpansion false or unset",
		},
		{
			name:      "allowVolumeExpansion unset",
			classes:   []runtime.Object{sc("unset", "driver.longhorn.io", nil)},
			pvcs:      []corev1.PersistentVolumeClaim{boundPVC("db-1", "unset")},
			supported: false,
			reason:    "allowVolumeExpansion false or unset",
		},
		{
			name: "one non-expandable class blocks the whole cluster",
			classes: []runtime.Object{
				sc("longhorn", "driver.longhorn.io", &yes),
				sc("local-path-cnpg", "rancher.io/local-path", &no),
			},
			pvcs:      []corev1.PersistentVolumeClaim{boundPVC("db-1", "longhorn"), boundPVC("db-1-wal", "local-path-cnpg")},
			supported: false,
			reason:    "local-path-cnpg",
		},
		{
			name:      "no PVCs",
			supported: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := supportEngine(t, tt.classes...).ClusterExpansionSupport(context.Background(), tt.pvcs)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Supported != tt.supported {
				t.Fatalf("supported=%v, want %v (reason %q)", got.Supported, tt.supported, got.Reason)
			}
			if !strings.Contains(got.Reason, tt.reason) {
				t.Errorf("reason %q does not contain %q", got.Reason, tt.reason)
			}
		})
	}
}

// The auto-expand path must not touch a local-path PVC: it is skipped, never updated.
func TestExpandClusterPVCs_SkipsLocalPath(t *testing.T) {
	yes := true
	pvc := boundPVC("db-1", "local-path-cnpg")
	e := supportEngine(t, sc("local-path-cnpg", "rancher.io/local-path", &yes), &pvc)
	res, err := e.ExpandClusterPVCs(context.Background(), &ExpansionRequest{
		ClusterName: "db", ClusterNamespace: "db",
		PVCs:   []corev1.PersistentVolumeClaim{pvc},
		Policy: &cnpgv1alpha1.StoragePolicy{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.PVCResults) != 1 || !res.PVCResults[0].Skipped {
		t.Fatalf("want the local-path PVC skipped, got %+v", res.PVCResults)
	}
	var live corev1.PersistentVolumeClaim
	if err := e.client.Get(context.Background(), types.NamespacedName{Namespace: "db", Name: "db-1"}, &live); err != nil {
		t.Fatal(err)
	}
	if q := live.Spec.Resources.Requests[corev1.ResourceStorage]; q.String() != "10Gi" {
		t.Errorf("PVC request changed to %s", q.String())
	}
}

// A cached verdict expires, so a class flipped to allowVolumeExpansion=false is seen.
func TestStorageClassValidator_CacheExpires(t *testing.T) {
	yes := true
	v := NewStorageClassValidator(nil)
	v.cache["x"] = &StorageClassCapabilities{Name: "x", AllowVolumeExpansion: yes, Provisioner: "driver.longhorn.io"}
	if _, ok := v.GetCachedCapabilities("x"); !ok {
		t.Fatal("expected cache entry")
	}
	// fetchedAt is zero, i.e. older than the TTL: ValidateStorageClass must refetch, and with
	// a client that has no such class it reports invalid instead of the stale cached "valid".
	scheme := runtime.NewScheme()
	_ = storagev1.AddToScheme(scheme)
	v.client = fake.NewClientBuilder().WithScheme(scheme).Build()
	res, err := v.ValidateStorageClass(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if res.Valid {
		t.Error("stale cache entry was trusted past its TTL")
	}
}

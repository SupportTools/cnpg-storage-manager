package controller

// cluster-services-1413: clusters on storage that cannot expand in place (local-path,
// allowVolumeExpansion false/unset) must be reported as such and never auto-expanded.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	cnpgv1alpha1 "github.com/supporttools/cnpg-storage-manager/api/v1alpha1"
	"github.com/supporttools/cnpg-storage-manager/pkg/alerting"
	"github.com/supporttools/cnpg-storage-manager/pkg/cnpg"
	"github.com/supporttools/cnpg-storage-manager/pkg/metrics"
	"github.com/supporttools/cnpg-storage-manager/pkg/policy"
	"github.com/supporttools/cnpg-storage-manager/pkg/remediation"
)

func localPathReconciler(t *testing.T, allow *bool) *StoragePolicyReconciler {
	t.Helper()
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = storagev1.AddToScheme(scheme)
	class := "local-path-cnpg"
	objs := make([]runtime.Object, 0, 3)
	objs = append(objs, &storagev1.StorageClass{
		ObjectMeta:           metav1.ObjectMeta{Name: class},
		Provisioner:          "rancher.io/local-path",
		AllowVolumeExpansion: allow,
	})
	for _, n := range []string{"rack-control-db-1", "rack-control-db-2"} {
		objs = append(objs, &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: n, Namespace: "rack-control",
				Labels: map[string]string{"cnpg.io/cluster": "rack-control-db"}},
			Spec: corev1.PersistentVolumeClaimSpec{
				StorageClassName: &class,
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("50Gi")}},
			},
		})
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()
	return &StoragePolicyReconciler{
		discovery:       cnpg.NewDiscovery(c),
		expansionEngine: remediation.NewExpansionEngine(c),
		alertManagers:   map[string]*alerting.AlertManager{},
	}
}

func TestExpansionSupport_LocalPath(t *testing.T) {
	yes, no := true, false
	for _, allow := range []*bool{&yes, &no, nil} {
		r := localPathReconciler(t, allow)
		cluster := cnpg.ClusterInfo{Name: "rack-control-db", Namespace: "rack-control"}
		got := r.expansionSupport(context.Background(), cluster)
		if got == nil || got.Supported {
			t.Fatalf("allowVolumeExpansion=%v: want not supported, got %+v", allow, got)
		}
		g := metrics.ExpansionSupported.WithLabelValues("rack-control-db", "rack-control", "local-path-cnpg", "rancher.io/local-path")
		if v := testutil.ToFloat64(g); v != 0 {
			t.Errorf("expansion_supported = %v, want 0", v)
		}
	}
	metrics.ExpansionSupported.Reset()
}

func TestSetExpansionCondition(t *testing.T) {
	yes, no := true, false
	r := &StoragePolicyReconciler{}
	p := &cnpgv1alpha1.StoragePolicy{}
	r.setExpansionCondition(p, []cnpgv1alpha1.ManagedCluster{
		{Name: "rack-control-db", Namespace: "rack-control", ExpansionSupported: &no},
		{Name: "ok", Namespace: "x", ExpansionSupported: &yes},
		{Name: "unknown", Namespace: "x"},
	})
	c := meta.FindStatusCondition(p.Status.Conditions, "ExpansionSupported")
	if c == nil || c.Status != metav1.ConditionFalse || c.Reason != "StorageClassCannotExpand" {
		t.Fatalf("condition = %+v", c)
	}
	if !strings.Contains(c.Message, "rack-control/rack-control-db") || strings.Contains(c.Message, "x/ok") ||
		!strings.Contains(c.Message, "rolling instance replacement") {
		t.Errorf("message = %q", c.Message)
	}

	r.setExpansionCondition(p, []cnpgv1alpha1.ManagedCluster{{Name: "ok", Namespace: "x", ExpansionSupported: &yes}})
	if c := meta.FindStatusCondition(p.Status.Conditions, "ExpansionSupported"); c.Status != metav1.ConditionTrue {
		t.Errorf("want True once no cluster is blocked, got %+v", c)
	}
}

// The Alertmanager alert carries the remediation as annotations; its labels (identity) stay the same.
func TestHandleAlert_ExpansionUnsupportedAnnotations(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ = io.ReadAll(req.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := &cnpgv1alpha1.StoragePolicy{}
	p.Name, p.Namespace = "pol", "ns"
	p.Spec.Alerting.Channels = []cnpgv1alpha1.AlertChannel{
		{Type: cnpgv1alpha1.AlertChannelTypeAlertmanager, Endpoint: srv.URL},
	}
	r := &StoragePolicyReconciler{alertManagers: map[string]*alerting.AlertManager{}}
	res := &policy.EvaluationResult{ThresholdResult: policy.ThresholdResult{
		Level: policy.ThresholdLevelCritical, CurrentUsagePercent: 92, Message: "Critical: 92%"}}
	exp := &remediation.ExpansionSupport{Supported: false, Reason: `storage class "local-path-cnpg" does not support volume expansion`}

	if err := r.handleAlert(context.Background(), p, cnpg.ClusterInfo{Name: "c", Namespace: "n"}, res, exp); err != nil {
		t.Fatal(err)
	}
	var alerts []struct {
		Labels      map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
	}
	if err := json.Unmarshal(body, &alerts); err != nil || len(alerts) != 1 {
		t.Fatalf("payload %s: %v", body, err)
	}
	a := alerts[0]
	if a.Annotations["expansion_supported"] != "false" ||
		!strings.Contains(a.Annotations["remediation"], "rolling instance replacement") ||
		!strings.Contains(a.Annotations["expansion_blocker"], "local-path-cnpg") {
		t.Errorf("annotations = %v", a.Annotations)
	}
	for _, k := range []string{"expansion_supported", "remediation", "expansion_blocker"} {
		if _, ok := a.Labels[k]; ok {
			t.Errorf("%s must be an annotation, not a label", k)
		}
	}
}

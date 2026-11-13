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

package policy

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cnpgv1alpha1 "github.com/supporttools/cnpg-storage-manager/api/v1alpha1"
)

func sp(name string, priority int32, sel *metav1.LabelSelector) cnpgv1alpha1.StoragePolicy {
	return cnpgv1alpha1.StoragePolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "cnpg-storage-manager"},
		Spec:       cnpgv1alpha1.StoragePolicySpec{Priority: priority, Selector: sel},
	}
}

func labelsSel(kv ...string) *metav1.LabelSelector {
	m := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	return &metav1.LabelSelector{MatchLabels: m}
}

// reversed returns a copy of policies in reverse order, to prove the winner
// does not depend on list order (the original flap).
func reversed(in []cnpgv1alpha1.StoragePolicy) []cnpgv1alpha1.StoragePolicy {
	out := make([]cnpgv1alpha1.StoragePolicy, len(in))
	for i := range in {
		out[len(in)-1-i] = in[i]
	}
	return out
}

func winnerName(t *testing.T, policies []cnpgv1alpha1.StoragePolicy, c ClusterIdentity) string {
	t.Helper()
	w, errs := SelectPolicy(policies, c)
	if len(errs) != 0 {
		t.Fatalf("unexpected selector errors: %v", errs)
	}
	if w == nil {
		return ""
	}
	return w.Name
}

func TestSelectPolicy(t *testing.T) {
	// The live a1-ops-prd policy set that produced cluster-services-1343.
	live := []cnpgv1alpha1.StoragePolicy{
		sp("default-postgres", 0, &metav1.LabelSelector{}),
		sp("nonprod-postgres", 0, labelsSel("app.kubernetes.io/component", "database")),
		sp("production-postgres", 0, labelsSel("app.kubernetes.io/instance", "beacon-prod")),
	}

	tests := []struct {
		name     string
		policies []cnpgv1alpha1.StoragePolicy
		cluster  ClusterIdentity
		want     string
	}{
		{
			name:     "empty selector matches an unlabelled cluster",
			policies: live,
			cluster:  ClusterIdentity{Namespace: "teslamate", Name: "teslamate"},
			want:     "default-postgres",
		},
		{
			name:     "more specific selector beats empty selector at equal priority",
			policies: live,
			cluster: ClusterIdentity{Namespace: "taskforge", Name: "taskforge-pg",
				Labels: map[string]string{"app.kubernetes.io/component": "database"}},
			want: "nonprod-postgres",
		},
		{
			name: "higher priority beats a more specific selector",
			policies: []cnpgv1alpha1.StoragePolicy{
				sp("specific", 0, labelsSel("a", "1", "b", "2")),
				sp("broad-but-important", 10, &metav1.LabelSelector{}),
			},
			cluster: ClusterIdentity{Namespace: "x", Name: "db", Labels: map[string]string{"a": "1", "b": "2"}},
			want:    "broad-but-important",
		},
		{
			name: "matchExpressions count toward specificity",
			policies: []cnpgv1alpha1.StoragePolicy{
				sp("one-label", 0, labelsSel("a", "1")),
				sp("label-and-expr", 0, &metav1.LabelSelector{
					MatchLabels: map[string]string{"a": "1"},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{Key: "tier", Operator: metav1.LabelSelectorOpIn, Values: []string{"prod"}},
					},
				}),
			},
			cluster: ClusterIdentity{Namespace: "x", Name: "db", Labels: map[string]string{"a": "1", "tier": "prod"}},
			want:    "label-and-expr",
		},
		{
			name: "full tie goes to the name that sorts first",
			policies: []cnpgv1alpha1.StoragePolicy{
				sp("zeta", 5, labelsSel("a", "1")),
				sp("alpha", 5, labelsSel("b", "2")),
			},
			cluster: ClusterIdentity{Namespace: "x", Name: "db", Labels: map[string]string{"a": "1", "b": "2"}},
			want:    "alpha",
		},
		{
			name: "nil selector is as broad as an empty one",
			policies: []cnpgv1alpha1.StoragePolicy{
				sp("nil-sel", 0, nil),
				sp("narrow", 0, labelsSel("a", "1")),
			},
			cluster: ClusterIdentity{Namespace: "x", Name: "db", Labels: map[string]string{"a": "1"}},
			want:    "narrow",
		},
		{
			name: "excluded cluster falls through to the next policy",
			policies: func() []cnpgv1alpha1.StoragePolicy {
				p := sp("narrow", 0, labelsSel("a", "1"))
				p.Spec.ExcludeClusters = []cnpgv1alpha1.ClusterReference{{Name: "db", Namespace: "x"}}
				return []cnpgv1alpha1.StoragePolicy{p, sp("default", 0, nil)}
			}(),
			cluster: ClusterIdentity{Namespace: "x", Name: "db", Labels: map[string]string{"a": "1"}},
			want:    "default",
		},
		{
			name: "deleting policy gives up its clusters",
			policies: func() []cnpgv1alpha1.StoragePolicy {
				p := sp("narrow", 0, labelsSel("a", "1"))
				now := metav1.Now()
				p.DeletionTimestamp = &now
				return []cnpgv1alpha1.StoragePolicy{p, sp("default", 0, nil)}
			}(),
			cluster: ClusterIdentity{Namespace: "x", Name: "db", Labels: map[string]string{"a": "1"}},
			want:    "default",
		},
		{
			name:     "no match returns no policy",
			policies: []cnpgv1alpha1.StoragePolicy{sp("narrow", 0, labelsSel("a", "1"))},
			cluster:  ClusterIdentity{Namespace: "x", Name: "db"},
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := winnerName(t, tt.policies, tt.cluster); got != tt.want {
				t.Errorf("SelectPolicy = %q, want %q", got, tt.want)
			}
			if got := winnerName(t, reversed(tt.policies), tt.cluster); got != tt.want {
				t.Errorf("SelectPolicy (reversed order) = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSelectPolicyNamespaceTieBreak(t *testing.T) {
	a := sp("same", 0, nil)
	a.Namespace = "ns-b"
	b := sp("same", 0, nil)
	b.Namespace = "ns-a"
	c := ClusterIdentity{Namespace: "x", Name: "db"}
	for _, order := range [][]cnpgv1alpha1.StoragePolicy{{a, b}, {b, a}} {
		w, _ := SelectPolicy(order, c)
		if w == nil || w.Namespace != "ns-a" {
			t.Fatalf("want ns-a/same, got %+v", w)
		}
	}
}

func TestSelectPolicyInvalidSelectorSkipped(t *testing.T) {
	bad := sp("bad", 100, &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "a", Operator: "Bogus"}},
	})
	good := sp("good", 0, nil)
	w, errs := SelectPolicy([]cnpgv1alpha1.StoragePolicy{bad, good}, ClusterIdentity{Namespace: "x", Name: "db"})
	if w == nil || w.Name != "good" {
		t.Fatalf("want good, got %+v", w)
	}
	if len(errs) != 1 {
		t.Fatalf("want 1 selector error, got %d", len(errs))
	}
}

func TestIsSelectedPolicy(t *testing.T) {
	def := sp("default-postgres", 0, &metav1.LabelSelector{})
	nonprod := sp("nonprod-postgres", 0, labelsSel("app.kubernetes.io/component", "database"))
	all := []cnpgv1alpha1.StoragePolicy{def, nonprod}
	db := ClusterIdentity{Namespace: "taskforge", Name: "taskforge-pg",
		Labels: map[string]string{"app.kubernetes.io/component": "database"}}
	plain := ClusterIdentity{Namespace: "teslamate", Name: "teslamate"}

	// Exactly one policy claims each cluster.
	for _, c := range []ClusterIdentity{db, plain} {
		claims := 0
		for i := range all {
			if ok, _ := IsSelectedPolicy(&all[i], all, c); ok {
				claims++
			}
		}
		if claims != 1 {
			t.Errorf("cluster %s/%s claimed by %d policies, want 1", c.Namespace, c.Name, claims)
		}
	}

	if ok, _ := IsSelectedPolicy(&def, all, db); ok {
		t.Error("default-postgres must not claim a component=database cluster")
	}

	// The caller's copy wins over the listed one: raising default's priority
	// in self (fresher than the cache) makes it the winner.
	fresh := def
	fresh.Spec.Priority = 10
	if ok, _ := IsSelectedPolicy(&fresh, all, db); !ok {
		t.Error("fresh copy with higher priority should be selected")
	}

	// A policy absent from the list (stale cache) still competes.
	newPol := sp("aaa-new", 0, labelsSel("app.kubernetes.io/component", "database"))
	if ok, _ := IsSelectedPolicy(&newPol, all, db); !ok {
		t.Error("policy missing from the list should still be considered, and wins the name tie-break")
	}
}

func TestSelectorSpecificity(t *testing.T) {
	if SelectorSpecificity(nil) != 0 || SelectorSpecificity(&metav1.LabelSelector{}) != 0 {
		t.Fatal("nil and empty selectors must score 0")
	}
	if got := SelectorSpecificity(labelsSel("a", "1", "b", "2")); got != 2 {
		t.Fatalf("got %d, want 2", got)
	}
}

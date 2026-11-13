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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	cnpgv1alpha1 "github.com/supporttools/cnpg-storage-manager/api/v1alpha1"
)

// ClusterIdentity is the part of a CNPG cluster that policy selection reads.
type ClusterIdentity struct {
	Namespace string
	Name      string
	Labels    map[string]string
}

// SelectorSpecificity scores how narrow a label selector is: the number of
// matchLabels entries plus matchExpressions. A nil or empty selector matches
// every cluster and scores 0.
func SelectorSpecificity(sel *metav1.LabelSelector) int {
	if sel == nil {
		return 0
	}
	return len(sel.MatchLabels) + len(sel.MatchExpressions)
}

// PolicyMatches reports whether a policy's selector matches the cluster and
// the cluster is not in the policy's excludeClusters list. A policy that is
// being deleted matches nothing, so its clusters fall to the next policy.
func PolicyMatches(p *cnpgv1alpha1.StoragePolicy, c ClusterIdentity) (bool, error) {
	if p == nil || !p.DeletionTimestamp.IsZero() {
		return false, nil
	}
	for _, ref := range p.Spec.ExcludeClusters {
		if ref.Name == c.Name && ref.Namespace == c.Namespace {
			return false, nil
		}
	}
	if p.Spec.Selector == nil {
		return true, nil
	}
	sel, err := metav1.LabelSelectorAsSelector(p.Spec.Selector)
	if err != nil {
		return false, fmt.Errorf("policy %s/%s: invalid label selector: %w", p.Namespace, p.Name, err)
	}
	return sel.Matches(labels.Set(c.Labels)), nil
}

// outranks reports whether policy a beats policy b for a cluster both match.
// Order: higher spec.priority, then the more specific selector, then the
// namespace/name that sorts first. The last step makes the order total, so
// the winner never depends on list order.
func outranks(a, b *cnpgv1alpha1.StoragePolicy) bool {
	if a.Spec.Priority != b.Spec.Priority {
		return a.Spec.Priority > b.Spec.Priority
	}
	sa, sb := SelectorSpecificity(a.Spec.Selector), SelectorSpecificity(b.Spec.Selector)
	if sa != sb {
		return sa > sb
	}
	if a.Namespace != b.Namespace {
		return a.Namespace < b.Namespace
	}
	return a.Name < b.Name
}

// SelectPolicy returns the single policy that manages the cluster, or nil when
// no policy matches it. Policies with an invalid selector are skipped; their
// errors are returned alongside the winner so the caller can log them.
func SelectPolicy(policies []cnpgv1alpha1.StoragePolicy, c ClusterIdentity) (*cnpgv1alpha1.StoragePolicy, []error) {
	var winner *cnpgv1alpha1.StoragePolicy
	var errs []error
	for i := range policies {
		p := &policies[i]
		ok, err := PolicyMatches(p, c)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !ok {
			continue
		}
		if winner == nil || outranks(p, winner) {
			winner = p
		}
	}
	return winner, errs
}

// IsSelectedPolicy reports whether self is the policy SelectPolicy picks for
// the cluster out of all policies. self is matched by namespace/name, so it
// may be a different copy of the object than the one in policies; when self
// is missing from policies (for example a stale cache) it is still considered.
func IsSelectedPolicy(self *cnpgv1alpha1.StoragePolicy, policies []cnpgv1alpha1.StoragePolicy, c ClusterIdentity) (bool, []error) {
	all := make([]cnpgv1alpha1.StoragePolicy, 0, len(policies)+1)
	found := false
	for i := range policies {
		if policies[i].Namespace == self.Namespace && policies[i].Name == self.Name {
			// Use the caller's copy: it is the freshest view of this policy.
			all = append(all, *self)
			found = true
			continue
		}
		all = append(all, policies[i])
	}
	if !found {
		all = append(all, *self)
	}
	winner, errs := SelectPolicy(all, c)
	if winner == nil {
		return false, errs
	}
	return winner.Namespace == self.Namespace && winner.Name == self.Name, errs
}

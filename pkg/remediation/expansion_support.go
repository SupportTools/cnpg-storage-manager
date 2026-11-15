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
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// StorageClassSupport is the expansion verdict for one StorageClass used by a cluster.
type StorageClassSupport struct {
	StorageClass string
	Provisioner  string
	Supported    bool
	Message      string
}

// ExpansionSupport is the expansion verdict for a whole CNPG cluster: expansion is supported
// only when every PVC's StorageClass can expand in place. Auto-expansion grows all instances
// together, so one non-expandable class makes the cluster non-expandable.
type ExpansionSupport struct {
	Supported      bool
	Reason         string
	StorageClasses []StorageClassSupport
}

// ClusterExpansionSupport reports whether the given PVCs can be expanded in place. A PVC with
// no StorageClass counts as non-expandable. With no PVCs it reports supported (nothing to block).
func (e *ExpansionEngine) ClusterExpansionSupport(
	ctx context.Context,
	pvcs []corev1.PersistentVolumeClaim,
) (*ExpansionSupport, error) {
	byClass := map[string]*StorageClassSupport{}
	for i := range pvcs {
		name := ""
		if pvcs[i].Spec.StorageClassName != nil {
			name = *pvcs[i].Spec.StorageClassName
		}
		if _, seen := byClass[name]; seen {
			continue
		}
		if name == "" {
			byClass[name] = &StorageClassSupport{Message: fmt.Sprintf("PVC %s has no storage class", pvcs[i].Name)}
			continue
		}
		res, err := e.validator.ValidateStorageClass(ctx, name)
		if err != nil {
			return nil, err
		}
		byClass[name] = &StorageClassSupport{
			StorageClass: name,
			Provisioner:  res.Provisioner,
			Supported:    res.Valid,
			Message:      res.Message,
		}
	}

	out := &ExpansionSupport{Supported: true}
	var blocked []string
	for _, s := range byClass {
		out.StorageClasses = append(out.StorageClasses, *s)
		if !s.Supported {
			out.Supported = false
			blocked = append(blocked, s.Message)
		}
	}
	sort.Slice(out.StorageClasses, func(i, j int) bool {
		return out.StorageClasses[i].StorageClass < out.StorageClasses[j].StorageClass
	})
	sort.Strings(blocked)
	if !out.Supported {
		out.Reason = strings.Join(blocked, "; ")
	}
	return out, nil
}

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
	"sync"
	"time"

	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// String literals shared across this file (goconst).
const (
	pvcPhaseBound = "Bound"
	accessModeRWO = "ReadWriteOnce"
)

// nonExpandingProvisioners are provisioners whose volumes cannot be resized in place, whatever
// the StorageClass says. rancher.io/local-path volumes are plain host directories: no external
// resizer acts on them, so a PVC whose request was raised sits at ExternalExpanding forever and
// status.capacity (what usage is divided by) never moves (cluster-services-1399).
var nonExpandingProvisioners = map[string]string{
	"rancher.io/local-path":        "local-path volumes are host directories; no resizer acts on them",
	"kubernetes.io/no-provisioner": "statically provisioned local PVs cannot be resized",
}

// RollingReplaceRemediation is the operator-facing instruction for a cluster whose storage
// cannot be expanded in place. It rides on alerts and status so the page says what to do.
const RollingReplaceRemediation = "storage class cannot expand in place: grow the CNPG Cluster by " +
	"rolling instance replacement (raise spec.storage.size, then delete one replica's PVC and pod " +
	"at a time so CNPG re-creates it at the new size, then switch over and replace the old primary; " +
	"see the rolling-replace procedure in the cnpg-storage-manager README)"

// storageClassCacheTTL bounds how long a StorageClass lookup is trusted, so a class flipped to
// allowVolumeExpansion=false is picked up without a restart.
const storageClassCacheTTL = 5 * time.Minute

// ProvisionerExpansionBlocker returns why a provisioner cannot expand volumes, or "" if it is
// not known to be unable to.
func ProvisionerExpansionBlocker(provisioner string) string {
	return nonExpandingProvisioners[provisioner]
}

// StorageClassCapabilities contains cached capabilities of a storage class
type StorageClassCapabilities struct {
	Name                 string
	AllowVolumeExpansion bool
	Provisioner          string
	VolumeBindingMode    storagev1.VolumeBindingMode
	fetchedAt            time.Time
}

// CanExpandInPlace reports whether volumes of this class can be resized in place: the class must
// allow expansion AND the provisioner must not be one known to ignore resize requests.
func (c *StorageClassCapabilities) CanExpandInPlace() bool {
	return c.AllowVolumeExpansion && ProvisionerExpansionBlocker(c.Provisioner) == ""
}

// StorageClassValidator validates storage class capabilities for expansion
type StorageClassValidator struct {
	client client.Client
	cache  map[string]*StorageClassCapabilities
	mu     sync.RWMutex
}

// NewStorageClassValidator creates a new storage class validator
func NewStorageClassValidator(c client.Client) *StorageClassValidator {
	return &StorageClassValidator{
		client: c,
		cache:  make(map[string]*StorageClassCapabilities),
	}
}

// ValidationResult contains the result of storage class validation
type ValidationResult struct {
	Valid                bool
	AllowVolumeExpansion bool
	Provisioner          string
	Message              string
}

// ValidateStorageClass validates if a storage class supports volume expansion
func (v *StorageClassValidator) ValidateStorageClass(
	ctx context.Context,
	storageClassName string,
) (*ValidationResult, error) {
	logger := log.FromContext(ctx)

	// Check cache first
	v.mu.RLock()
	cached, exists := v.cache[storageClassName]
	v.mu.RUnlock()

	if exists && time.Since(cached.fetchedAt) < storageClassCacheTTL {
		return &ValidationResult{
			Valid:                cached.CanExpandInPlace(),
			AllowVolumeExpansion: cached.AllowVolumeExpansion,
			Provisioner:          cached.Provisioner,
			Message:              v.validationMessage(cached),
		}, nil
	}

	// Fetch storage class
	var sc storagev1.StorageClass
	if err := v.client.Get(ctx, types.NamespacedName{Name: storageClassName}, &sc); err != nil {
		logger.Error(err, "Failed to get storage class", "storageClass", storageClassName)
		return &ValidationResult{
			Valid:   false,
			Message: fmt.Sprintf("storage class %q not found: %v", storageClassName, err),
		}, nil
	}

	// Extract capabilities
	caps := &StorageClassCapabilities{
		Name:                 sc.Name,
		AllowVolumeExpansion: sc.AllowVolumeExpansion != nil && *sc.AllowVolumeExpansion,
		Provisioner:          sc.Provisioner,
		fetchedAt:            time.Now(),
	}

	if sc.VolumeBindingMode != nil {
		caps.VolumeBindingMode = *sc.VolumeBindingMode
	}

	// Cache the result
	v.mu.Lock()
	v.cache[storageClassName] = caps
	v.mu.Unlock()

	logger.V(1).Info("Validated storage class",
		"storageClass", storageClassName,
		"allowVolumeExpansion", caps.AllowVolumeExpansion,
		"provisioner", caps.Provisioner,
	)

	return &ValidationResult{
		Valid:                caps.CanExpandInPlace(),
		AllowVolumeExpansion: caps.AllowVolumeExpansion,
		Provisioner:          caps.Provisioner,
		Message:              v.validationMessage(caps),
	}, nil
}

// validationMessage generates a human-readable validation message
func (v *StorageClassValidator) validationMessage(caps *StorageClassCapabilities) string {
	if blocker := ProvisionerExpansionBlocker(caps.Provisioner); blocker != "" {
		return fmt.Sprintf(
			"storage class %q does not support volume expansion (provisioner %s: %s)",
			caps.Name, caps.Provisioner, blocker,
		)
	}
	if caps.AllowVolumeExpansion {
		return fmt.Sprintf("storage class %q supports volume expansion (provisioner: %s)", caps.Name, caps.Provisioner)
	}
	return fmt.Sprintf(
		"storage class %q does not support volume expansion (allowVolumeExpansion false or unset, provisioner: %s)",
		caps.Name,
		caps.Provisioner,
	)
}

// InvalidateCache invalidates the cache for a specific storage class
func (v *StorageClassValidator) InvalidateCache(storageClassName string) {
	v.mu.Lock()
	delete(v.cache, storageClassName)
	v.mu.Unlock()
}

// InvalidateAllCache invalidates the entire cache
func (v *StorageClassValidator) InvalidateAllCache() {
	v.mu.Lock()
	v.cache = make(map[string]*StorageClassCapabilities)
	v.mu.Unlock()
}

// GetCachedCapabilities returns cached capabilities if available
func (v *StorageClassValidator) GetCachedCapabilities(storageClassName string) (*StorageClassCapabilities, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	caps, exists := v.cache[storageClassName]
	return caps, exists
}

// CanExpand is a convenience method that returns true if the storage class supports expansion
func (v *StorageClassValidator) CanExpand(ctx context.Context, storageClassName string) (bool, string, error) {
	result, err := v.ValidateStorageClass(ctx, storageClassName)
	if err != nil {
		return false, "", err
	}
	return result.Valid, result.Message, nil
}

// ValidatePVCForExpansion performs all pre-flight checks for PVC expansion
func (v *StorageClassValidator) ValidatePVCForExpansion(ctx context.Context, pvc *PVCInfo) (*PreflightResult, error) {
	result := &PreflightResult{
		PVCName:   pvc.Name,
		Namespace: pvc.Namespace,
		Checks:    make([]PreflightCheck, 0),
	}

	// Check 1: Storage class exists and supports expansion
	if pvc.StorageClassName == "" {
		result.Checks = append(result.Checks, PreflightCheck{
			Name:    "storage-class-set",
			Passed:  false,
			Message: "PVC does not have a storage class set",
		})
		result.CanExpand = false
		return result, nil
	}

	scResult, err := v.ValidateStorageClass(ctx, pvc.StorageClassName)
	if err != nil {
		return nil, err
	}

	result.Checks = append(result.Checks, PreflightCheck{
		Name:    "storage-class-expansion",
		Passed:  scResult.Valid,
		Message: scResult.Message,
	})

	// Check 2: PVC is bound
	if pvc.Phase != pvcPhaseBound {
		result.Checks = append(result.Checks, PreflightCheck{
			Name:    "pvc-bound",
			Passed:  false,
			Message: fmt.Sprintf("PVC is not bound (phase: %s)", pvc.Phase),
		})
	} else {
		result.Checks = append(result.Checks, PreflightCheck{
			Name:    "pvc-bound",
			Passed:  true,
			Message: "PVC is bound",
		})
	}

	// Check 3: PVC access mode is suitable
	suitableAccessMode := false
	for _, mode := range pvc.AccessModes {
		if mode == accessModeRWO || mode == "ReadWriteMany" {
			suitableAccessMode = true
			break
		}
	}
	result.Checks = append(result.Checks, PreflightCheck{
		Name:    "access-mode",
		Passed:  suitableAccessMode,
		Message: fmt.Sprintf("PVC access modes: %v", pvc.AccessModes),
	})

	// Determine overall result
	result.CanExpand = true
	for _, check := range result.Checks {
		if !check.Passed {
			result.CanExpand = false
			break
		}
	}

	return result, nil
}

// PVCInfo contains information about a PVC for validation
type PVCInfo struct {
	Name             string
	Namespace        string
	StorageClassName string
	Phase            string
	AccessModes      []string
	CurrentSize      int64
}

// PreflightResult contains the result of preflight checks
type PreflightResult struct {
	PVCName   string
	Namespace string
	CanExpand bool
	Checks    []PreflightCheck
}

// PreflightCheck represents a single preflight check
type PreflightCheck struct {
	Name    string
	Passed  bool
	Message string
}

// FailedChecks returns the list of failed checks
func (r *PreflightResult) FailedChecks() []PreflightCheck {
	var failed []PreflightCheck
	for _, check := range r.Checks {
		if !check.Passed {
			failed = append(failed, check)
		}
	}
	return failed
}

// Summary returns a summary of the preflight result
func (r *PreflightResult) Summary() string {
	if r.CanExpand {
		return fmt.Sprintf("PVC %s/%s passed all %d preflight checks", r.Namespace, r.PVCName, len(r.Checks))
	}

	failed := r.FailedChecks()
	return fmt.Sprintf("PVC %s/%s failed %d of %d preflight checks", r.Namespace, r.PVCName, len(failed), len(r.Checks))
}

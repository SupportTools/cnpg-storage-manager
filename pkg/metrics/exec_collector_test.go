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

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestParseDfOutput(t *testing.T) {
	e := &ExecCollector{}

	tests := []struct {
		name     string
		input    string
		expected []DfOutput
	}{
		{
			name: "standard df output",
			input: `Filesystem     1-blocks       Used  Available Use% Mounted on
/dev/sda1      104857600   52428800   52428800  50% /
/dev/sdb1      209715200  104857600  104857600  50% /var/lib/postgresql/data`,
			expected: []DfOutput{
				{
					Filesystem: "/dev/sda1",
					TotalBytes: 104857600,
					UsedBytes:  52428800,
					AvailBytes: 52428800,
					UsePercent: 50,
					MountPoint: "/",
				},
				{
					Filesystem: "/dev/sdb1",
					TotalBytes: 209715200,
					UsedBytes:  104857600,
					AvailBytes: 104857600,
					UsePercent: 50,
					MountPoint: "/var/lib/postgresql/data",
				},
			},
		},
		{
			name: "df output with high usage",
			input: `Filesystem     1-blocks       Used  Available Use% Mounted on
/dev/nvme0n1p1 1073741824  966367641  107374183  90% /pgdata`,
			expected: []DfOutput{
				{
					Filesystem: "/dev/nvme0n1p1",
					TotalBytes: 1073741824,
					UsedBytes:  966367641,
					AvailBytes: 107374183,
					UsePercent: 90,
					MountPoint: "/pgdata",
				},
			},
		},
		{
			name:     "empty output",
			input:    "",
			expected: nil,
		},
		{
			name: "header only",
			input: `Filesystem     1-blocks       Used  Available Use% Mounted on
`,
			expected: nil,
		},
		{
			name: "output with extra whitespace",
			input: `Filesystem     1-blocks       Used  Available Use% Mounted on
/dev/sda1      104857600   52428800   52428800  50% /data
`,
			expected: []DfOutput{
				{
					Filesystem: "/dev/sda1",
					TotalBytes: 104857600,
					UsedBytes:  52428800,
					AvailBytes: 52428800,
					UsePercent: 50,
					MountPoint: "/data",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := e.parseDfOutput(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d entries, got %d", len(tt.expected), len(result))
				return
			}

			for i, exp := range tt.expected {
				if result[i].Filesystem != exp.Filesystem {
					t.Errorf("entry %d: expected filesystem %s, got %s", i, exp.Filesystem, result[i].Filesystem)
				}
				if result[i].TotalBytes != exp.TotalBytes {
					t.Errorf("entry %d: expected total %d, got %d", i, exp.TotalBytes, result[i].TotalBytes)
				}
				if result[i].UsedBytes != exp.UsedBytes {
					t.Errorf("entry %d: expected used %d, got %d", i, exp.UsedBytes, result[i].UsedBytes)
				}
				if result[i].AvailBytes != exp.AvailBytes {
					t.Errorf("entry %d: expected avail %d, got %d", i, exp.AvailBytes, result[i].AvailBytes)
				}
				if result[i].UsePercent != exp.UsePercent {
					t.Errorf("entry %d: expected percent %f, got %f", i, exp.UsePercent, result[i].UsePercent)
				}
				if result[i].MountPoint != exp.MountPoint {
					t.Errorf("entry %d: expected mount %s, got %s", i, exp.MountPoint, result[i].MountPoint)
				}
			}
		})
	}
}

func TestGetPVCVolumeMounts(t *testing.T) {
	e := &ExecCollector{}

	tests := []struct {
		name     string
		pod      corev1.Pod
		expected map[string]string
	}{
		{
			name: "single PVC mount",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "pgdata",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "my-cluster-1",
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name: "postgres",
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "pgdata",
									MountPath: "/var/lib/postgresql/data",
								},
							},
						},
					},
				},
			},
			expected: map[string]string{
				"my-cluster-1": "/var/lib/postgresql/data",
			},
		},
		{
			name: "multiple PVC mounts",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "pgdata",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "data-pvc",
								},
							},
						},
						{
							Name: "waldata",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "wal-pvc",
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name: "postgres",
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "pgdata",
									MountPath: "/var/lib/postgresql/data",
								},
								{
									Name:      "waldata",
									MountPath: "/var/lib/postgresql/wal",
								},
							},
						},
					},
				},
			},
			expected: map[string]string{
				"data-pvc": "/var/lib/postgresql/data",
				"wal-pvc":  "/var/lib/postgresql/wal",
			},
		},
		{
			name: "mixed volume types (PVC and non-PVC)",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "pgdata",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "my-pvc",
								},
							},
						},
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "my-config",
									},
								},
							},
						},
						{
							Name: "secrets",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: "my-secret",
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name: "postgres",
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "pgdata",
									MountPath: "/var/lib/postgresql/data",
								},
								{
									Name:      "config",
									MountPath: "/etc/config",
								},
								{
									Name:      "secrets",
									MountPath: "/etc/secrets",
								},
							},
						},
					},
				},
			},
			expected: map[string]string{
				"my-pvc": "/var/lib/postgresql/data",
			},
		},
		{
			name: "no PVC mounts",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "my-config",
									},
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name: "app",
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/etc/config",
								},
							},
						},
					},
				},
			},
			expected: map[string]string{},
		},
		{
			name: "empty pod",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{},
			},
			expected: map[string]string{},
		},
		{
			name: "PVC volume but not mounted",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "pgdata",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "unused-pvc",
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:         "postgres",
							VolumeMounts: []corev1.VolumeMount{},
						},
					},
				},
			},
			expected: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := e.getPVCVolumeMounts(tt.pod)

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d entries, got %d", len(tt.expected), len(result))
				return
			}

			for pvc, mountPath := range tt.expected {
				if result[pvc] != mountPath {
					t.Errorf("expected PVC %s to mount at %s, got %s", pvc, mountPath, result[pvc])
				}
			}
		})
	}
}

func TestFindMountPointStats(t *testing.T) {
	e := &ExecCollector{}

	dfOutputs := []DfOutput{
		{
			Filesystem: "/dev/sda1",
			TotalBytes: 100000,
			UsedBytes:  50000,
			AvailBytes: 50000,
			UsePercent: 50,
			MountPoint: "/",
		},
		{
			Filesystem: "/dev/sdb1",
			TotalBytes: 200000,
			UsedBytes:  100000,
			AvailBytes: 100000,
			UsePercent: 50,
			MountPoint: "/var/lib/postgresql/data",
		},
		{
			Filesystem: "/dev/sdc1",
			TotalBytes: 300000,
			UsedBytes:  150000,
			AvailBytes: 150000,
			UsePercent: 50,
			MountPoint: "/var/lib/postgresql",
		},
	}

	tests := []struct {
		name       string
		mountPath  string
		expected   *DfOutput
		expectNil  bool
	}{
		{
			name:      "exact match",
			mountPath: "/var/lib/postgresql/data",
			expected:  &dfOutputs[1],
		},
		{
			name:      "exact match root",
			mountPath: "/",
			expected:  &dfOutputs[0],
		},
		{
			name:      "prefix match - should find longest prefix",
			mountPath: "/var/lib/postgresql/data/pgdata",
			expected:  &dfOutputs[1], // Should match /var/lib/postgresql/data, not /var/lib/postgresql
		},
		{
			name:      "falls back to root when no better match",
			mountPath: "/nonexistent/path",
			expected:  &dfOutputs[0], // Falls back to root "/"
		},
		{
			name:      "prefix match to root for unmatched path",
			mountPath: "/var/log/postgresql",
			expected:  &dfOutputs[0], // Falls back to root "/" since no /var/log exists
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := e.findMountPointStats(dfOutputs, tt.mountPath)

			if tt.expectNil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}

			if result == nil {
				t.Errorf("expected non-nil result")
				return
			}

			if result.MountPoint != tt.expected.MountPoint {
				t.Errorf("expected mount point %s, got %s", tt.expected.MountPoint, result.MountPoint)
			}
			if result.TotalBytes != tt.expected.TotalBytes {
				t.Errorf("expected total bytes %d, got %d", tt.expected.TotalBytes, result.TotalBytes)
			}
		})
	}
}

func TestDfOutputParsing_RealWorldExamples(t *testing.T) {
	e := &ExecCollector{}

	// Real-world df output from a PostgreSQL pod
	realOutput := `Filesystem           1-blocks       Used  Available Use% Mounted on
overlay              469306155008  72577277952  396728877056  16% /
tmpfs                    65536          0      65536   0% /dev
/dev/mapper/vg0-root 469306155008  72577277952  396728877056  16% /var/lib/postgresql/data
tmpfs                 16336494592          0  16336494592   0% /dev/shm
tmpfs                 16336494592      12288  16336482304   1% /run/secrets/kubernetes.io/serviceaccount`

	result := e.parseDfOutput(realOutput)

	if len(result) != 5 {
		t.Errorf("expected 5 entries, got %d", len(result))
		return
	}

	// Check the PostgreSQL data volume specifically
	var pgDataEntry *DfOutput
	for i := range result {
		if result[i].MountPoint == "/var/lib/postgresql/data" {
			pgDataEntry = &result[i]
			break
		}
	}

	if pgDataEntry == nil {
		t.Error("expected to find /var/lib/postgresql/data mount")
		return
	}

	if pgDataEntry.TotalBytes != 469306155008 {
		t.Errorf("expected total bytes 469306155008, got %d", pgDataEntry.TotalBytes)
	}
	if pgDataEntry.UsedBytes != 72577277952 {
		t.Errorf("expected used bytes 72577277952, got %d", pgDataEntry.UsedBytes)
	}
	if pgDataEntry.UsePercent != 16 {
		t.Errorf("expected usage percent 16, got %f", pgDataEntry.UsePercent)
	}
}

func TestGetPVCVolumeMounts_CNPGCluster(t *testing.T) {
	e := &ExecCollector{}

	// Simulated CNPG cluster pod
	cnpgPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-postgres-1",
			Namespace: "database",
			Labels: map[string]string{
				"cnpg.io/cluster":      "my-postgres",
				"cnpg.io/instanceRole": "primary",
			},
		},
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{
					Name: "pgdata",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "my-postgres-1",
						},
					},
				},
				{
					Name: "scratch-data",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
				{
					Name: "shm",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{
							Medium: corev1.StorageMediumMemory,
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:  "postgres",
					Image: "ghcr.io/cloudnative-pg/postgresql:16",
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "pgdata",
							MountPath: "/var/lib/postgresql/data",
						},
						{
							Name:      "scratch-data",
							MountPath: "/controller",
						},
						{
							Name:      "shm",
							MountPath: "/dev/shm",
						},
					},
				},
			},
		},
	}

	result := e.getPVCVolumeMounts(cnpgPod)

	// Should only find the PVC mount, not emptyDir mounts
	if len(result) != 1 {
		t.Errorf("expected 1 PVC mount, got %d", len(result))
	}

	mountPath, ok := result["my-postgres-1"]
	if !ok {
		t.Error("expected to find PVC my-postgres-1")
		return
	}

	if mountPath != "/var/lib/postgresql/data" {
		t.Errorf("expected mount path /var/lib/postgresql/data, got %s", mountPath)
	}
}

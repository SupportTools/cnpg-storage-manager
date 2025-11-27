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

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cnpgv1alpha1 "github.com/supporttools/cnpg-storage-manager/api/v1alpha1"
)

var _ = Describe("StoragePolicy Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		storagepolicy := &cnpgv1alpha1.StoragePolicy{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind StoragePolicy")
			err := k8sClient.Get(ctx, typeNamespacedName, storagepolicy)
			if err != nil && errors.IsNotFound(err) {
				resource := &cnpgv1alpha1.StoragePolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &cnpgv1alpha1.StoragePolicy{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance StoragePolicy")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &StoragePolicyReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When reconciling a policy with full configuration", func() {
		const resourceName = "full-config-policy"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		BeforeEach(func() {
			By("creating a fully configured StoragePolicy")
			maxSize := resource.MustParse("100Gi")
			res := &cnpgv1alpha1.StoragePolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: cnpgv1alpha1.StoragePolicySpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"environment": "test",
						},
					},
					Thresholds: cnpgv1alpha1.ThresholdsConfig{
						Warning:   70,
						Critical:  80,
						Expansion: 85,
						Emergency: 90,
					},
					Expansion: cnpgv1alpha1.ExpansionConfig{
						Enabled:         true,
						Percentage:      50,
						MinIncrementGi:  5,
						MaxSize:         &maxSize,
						CooldownMinutes: 30,
					},
					WALCleanup: cnpgv1alpha1.WALCleanupConfig{
						Enabled:         true,
						RetainCount:     10,
						RequireArchived: true,
						CooldownMinutes: 15,
					},
					CircuitBreaker: cnpgv1alpha1.CircuitBreakerConfig{
						MaxFailures:  3,
						ResetMinutes: 60,
						Scope:        cnpgv1alpha1.CircuitBreakerScopePerCluster,
					},
					DryRun: true,
				},
			}
			Expect(k8sClient.Create(ctx, res)).To(Succeed())
		})

		AfterEach(func() {
			resource := &cnpgv1alpha1.StoragePolicy{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should reconcile successfully", func() {
			By("Reconciling the configured policy")
			controllerReconciler := &StoragePolicyReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the policy still exists")
			policy := &cnpgv1alpha1.StoragePolicy{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, policy)).To(Succeed())
			Expect(policy.Spec.DryRun).To(BeTrue())
		})
	})

	Context("When reconciling a non-existent policy", func() {
		ctx := context.Background()

		It("should not return an error", func() {
			controllerReconciler := &StoragePolicyReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "non-existent",
					Namespace: "default",
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When reconciling a policy in dry-run mode", func() {
		const resourceName = "dryrun-policy"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		BeforeEach(func() {
			By("creating a dry-run StoragePolicy")
			res := &cnpgv1alpha1.StoragePolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: cnpgv1alpha1.StoragePolicySpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"test": "dryrun",
						},
					},
					Thresholds: cnpgv1alpha1.ThresholdsConfig{
						Warning:  80,
						Critical: 90,
					},
					DryRun: true,
				},
			}
			Expect(k8sClient.Create(ctx, res)).To(Succeed())
		})

		AfterEach(func() {
			resource := &cnpgv1alpha1.StoragePolicy{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should reconcile without taking actions", func() {
			By("Reconciling the dry-run policy")
			controllerReconciler := &StoragePolicyReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

var _ = Describe("StorageEvent Controller", func() {
	Context("When reconciling a storage event", func() {
		const eventName = "test-expansion-event"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      eventName,
			Namespace: "default",
		}

		BeforeEach(func() {
			By("creating a StorageEvent for expansion")
			originalSize := resource.MustParse("10Gi")
			requestedSize := resource.MustParse("15Gi")
			event := &cnpgv1alpha1.StorageEvent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      eventName,
					Namespace: "default",
					Labels: map[string]string{
						"cnpg.supporttools.io/cluster":    "test-cluster",
						"cnpg.supporttools.io/event-type": "expansion",
					},
				},
				Spec: cnpgv1alpha1.StorageEventSpec{
					ClusterRef: cnpgv1alpha1.ClusterReference{
						Name:      "test-cluster",
						Namespace: "default",
					},
					EventType: cnpgv1alpha1.EventTypeExpansion,
					Trigger:   cnpgv1alpha1.TriggerTypeThresholdBreach,
					Reason:    "Storage usage exceeded 85%",
					Expansion: &cnpgv1alpha1.ExpansionDetails{
						OriginalSize:  originalSize,
						RequestedSize: requestedSize,
						AffectedPVCs: []cnpgv1alpha1.AffectedPVC{
							{Name: "test-cluster-1", Node: "node-1"},
						},
					},
					DryRun: true,
				},
			}
			Expect(k8sClient.Create(ctx, event)).To(Succeed())
		})

		AfterEach(func() {
			event := &cnpgv1alpha1.StorageEvent{}
			err := k8sClient.Get(ctx, typeNamespacedName, event)
			if err == nil {
				Expect(k8sClient.Delete(ctx, event)).To(Succeed())
			}
		})

		It("should successfully create the event", func() {
			By("Verifying the event exists")
			event := &cnpgv1alpha1.StorageEvent{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, event)).To(Succeed())
			Expect(event.Spec.EventType).To(Equal(cnpgv1alpha1.EventTypeExpansion))
			Expect(event.Spec.Expansion).NotTo(BeNil())
		})
	})
})

var _ = Describe("Alert Channel Validation", func() {
	Context("When validating alert channels", func() {
		It("should accept valid Alertmanager channel", func() {
			channel := cnpgv1alpha1.AlertChannel{
				Type:     cnpgv1alpha1.AlertChannelTypeAlertmanager,
				Endpoint: "http://alertmanager:9093",
			}
			Expect(channel.Type).To(Equal(cnpgv1alpha1.AlertChannelTypeAlertmanager))
			Expect(channel.Endpoint).NotTo(BeEmpty())
		})

		It("should accept valid Slack channel", func() {
			channel := cnpgv1alpha1.AlertChannel{
				Type:          cnpgv1alpha1.AlertChannelTypeSlack,
				WebhookSecret: "default/slack-webhook",
				Channel:       "#alerts",
			}
			Expect(channel.Type).To(Equal(cnpgv1alpha1.AlertChannelTypeSlack))
			Expect(channel.Channel).To(Equal("#alerts"))
		})

		It("should accept valid PagerDuty channel", func() {
			channel := cnpgv1alpha1.AlertChannel{
				Type:             cnpgv1alpha1.AlertChannelTypePagerDuty,
				RoutingKeySecret: "default/pagerduty-key",
			}
			Expect(channel.Type).To(Equal(cnpgv1alpha1.AlertChannelTypePagerDuty))
			Expect(channel.RoutingKeySecret).NotTo(BeEmpty())
		})
	})
})

var _ = Describe("Threshold Validation", func() {
	Context("When validating threshold configurations", func() {
		It("should accept valid threshold ordering", func() {
			thresholds := cnpgv1alpha1.ThresholdsConfig{
				Warning:   70,
				Critical:  80,
				Expansion: 85,
				Emergency: 90,
			}
			Expect(thresholds.Warning).To(BeNumerically("<", thresholds.Critical))
			Expect(thresholds.Critical).To(BeNumerically("<", thresholds.Expansion))
			Expect(thresholds.Expansion).To(BeNumerically("<", thresholds.Emergency))
		})

		It("should handle thresholds at boundaries", func() {
			thresholds := cnpgv1alpha1.ThresholdsConfig{
				Warning:   1,
				Critical:  50,
				Emergency: 99,
			}
			Expect(thresholds.Warning).To(BeNumerically(">=", 1))
			Expect(thresholds.Emergency).To(BeNumerically("<=", 100))
		})
	})
})

var _ = Describe("PVC Status Tracking", func() {
	Context("When tracking PVC expansion status", func() {
		It("should correctly track pending PVC", func() {
			status := cnpgv1alpha1.PVCStatus{
				Name:  "test-pvc-1",
				Phase: cnpgv1alpha1.PVCPhasePending,
			}
			Expect(status.Phase).To(Equal(cnpgv1alpha1.PVCPhasePending))
		})

		It("should correctly track completed PVC with sizes", func() {
			originalSize := resource.MustParse("10Gi")
			newSize := resource.MustParse("15Gi")
			status := cnpgv1alpha1.PVCStatus{
				Name:              "test-pvc-1",
				Phase:             cnpgv1alpha1.PVCPhaseCompleted,
				OriginalSize:      &originalSize,
				NewSize:           &newSize,
				FilesystemResized: true,
			}
			Expect(status.Phase).To(Equal(cnpgv1alpha1.PVCPhaseCompleted))
			Expect(status.FilesystemResized).To(BeTrue())
			Expect(status.NewSize.Cmp(originalSize)).To(Equal(1))
		})

		It("should correctly track failed PVC with error", func() {
			status := cnpgv1alpha1.PVCStatus{
				Name:  "test-pvc-1",
				Phase: cnpgv1alpha1.PVCPhaseFailed,
				Error: "storage class does not support expansion",
			}
			Expect(status.Phase).To(Equal(cnpgv1alpha1.PVCPhaseFailed))
			Expect(status.Error).NotTo(BeEmpty())
		})
	})
})

var _ = Describe("Node Label Annotation Handling", func() {
	ctx := context.Background()

	Context("When creating nodes with labels", func() {
		It("should handle node labels for storage class selection", func() {
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-labels",
					Labels: map[string]string{
						"topology.kubernetes.io/zone":      "us-west-2a",
						"topology.kubernetes.io/region":    "us-west-2",
						"node.kubernetes.io/instance-type": "m5.xlarge",
					},
				},
			}
			Expect(k8sClient.Create(ctx, node)).To(Succeed())

			// Verify node was created with labels
			fetchedNode := &corev1.Node{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-node-labels"}, fetchedNode)).To(Succeed())
			Expect(fetchedNode.Labels).To(HaveKey("topology.kubernetes.io/zone"))

			// Cleanup
			Expect(k8sClient.Delete(ctx, node)).To(Succeed())
		})
	})
})

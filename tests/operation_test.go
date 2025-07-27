// Copyright 2025 The Crossplane Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package crossplane

import (
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/conformance/internal"
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	opsv1alpha1 "github.com/crossplane/crossplane/apis/ops/v1alpha1"
	pkgv1 "github.com/crossplane/crossplane/apis/pkg/v1"
)

func TestOperation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	t.Cleanup(cancel)

	kube, err := internal.NewClient()
	if err != nil {
		t.Fatalf("Create client: %v", err)
	}

	// Create a namespace for the operation resources
	internal.CreateNamespace(ctx, t, kube)

	// Create operation-capable function
	fnc := internal.CreateFunction(ctx, t, kube, "xpkg.crossplane.io/crossplane-contrib/function-dummy:v0.4.1")

	// Create an Operation with a pipeline that uses the function
	op := createOperation(ctx, t, kube, fnc)

	// Test that Operation is successful
	t.Run("OperationSucceeds", func(t *testing.T) {
		t.Log("Testing that the Operation status indicates successful completion.")
		if err := wait.PollUntilContextTimeout(ctx, 10*time.Second, 2*time.Minute, true, func(ctx context.Context) (done bool, err error) {
			if err := kube.Get(ctx, types.NamespacedName{Name: op.GetName()}, op); err != nil {
				return false, err
			}

			if op.GetCondition(opsv1alpha1.TypeValidPipeline).Status != corev1.ConditionTrue {
				t.Logf("Operation %q does not yet have a valid pipeline", op.GetName())
				return false, nil
			}

			if op.GetCondition(xpv1.TypeSynced).Status != corev1.ConditionTrue {
				t.Logf("Operation %q is not yet Synced", op.GetName())
				return false, nil
			}

			if op.GetCondition(opsv1alpha1.TypeSucceeded).Status != corev1.ConditionTrue {
				t.Logf("Operation %q has not yet Succeeded", op.GetName())
				return false, nil
			}

			t.Logf("Operation %q has succeeded", op.GetName())
			return true, nil
		}); err != nil {
			t.Errorf("Operation %q never succeeded: %v", op.GetName(), err)
		}
	})

	// Test that the operation has all of its expected applied resource references
	t.Run("AppliedResourceRefs", func(t *testing.T) {
		t.Log("Testing that the Operation has the expected applied resource refs.")

		wantRefs := []internal.ResourceRef{
			{
				Kind:       "ConfigMap",
				APIVersion: "v1",
				Namespace:  internal.SuiteName,
				FieldValues: map[string]string{
					"data.coolData": "I'm cool operation data!",
				},
			},
		}

		if err := wait.PollUntilContextTimeout(ctx, 10*time.Second, 2*time.Minute, true, func(ctx context.Context) (done bool, err error) {
			if err := kube.Get(ctx, types.NamespacedName{Name: op.GetName()}, op); err != nil {
				return false, err
			}

			// convert the applied resource references into the common
			// ResourceRef type that our test helper uses
			actualRefs := make([]internal.ResourceRef, len(op.Status.AppliedResourceRefs))
			for i, ref := range op.Status.AppliedResourceRefs {
				actualRefs[i] = internal.ResourceRef{
					Kind:       ref.Kind,
					APIVersion: ref.APIVersion,
					Namespace:  ptr.Deref(ref.Namespace, ""),
					Name:       ref.Name,
				}
			}

			if err := internal.TestResourceRefs(ctx, t, kube, actualRefs, wantRefs); err != nil {
				t.Logf("Operation %q does not have all expected applied resource refs: %v", op.GetName(), err)
				return false, nil
			}

			t.Logf("Operation %q has all expected applied resource refs", op.GetName())
			return true, nil
		}); err != nil {
			t.Errorf("Operation %q never got all expected applied resource refs: %v", op.GetName(), err)
		}
	})
}

// createOperation creates an Operation with the given function reference
func createOperation(ctx context.Context, t *testing.T, kube client.Client, fnc *pkgv1.Function) *opsv1alpha1.Operation {
	t.Helper()

	// Create a simple operation that uses function-dummy to create a ConfigMap
	op := &opsv1alpha1.Operation{
		ObjectMeta: metav1.ObjectMeta{Name: internal.SuiteName + "-operation"},
		Spec: opsv1alpha1.OperationSpec{
			Mode: opsv1alpha1.OperationModePipeline,
			Pipeline: []opsv1alpha1.PipelineStep{
				{
					Step: "op-step-create-configmap",
					FunctionRef: opsv1alpha1.FunctionReference{
						Name: fnc.GetName(),
					},
					Input: &runtime.RawExtension{
						Raw: []byte(fmt.Sprintf(`{
							"apiVersion": "dummy.fn.crossplane.io/v1beta1",
							"kind": "Response",
							"response": {
								"desired": {
									"resources": {
										"configmap": {
											"resource": {
												"apiVersion": "v1",
												"kind": "ConfigMap",
												"metadata": {
													"namespace": "%s",
													"name": "cool-map"
												},
												"data": {
													"coolData": "I'm cool operation data!"
												}
											}
										}
									}
								},
								"results": [
									{
										"severity": "SEVERITY_NORMAL",
										"message": "Operation success!"
									}
								]
							}
						}`, internal.SuiteName)),
					},
				},
			},
		},
	}

	if err := kube.Create(ctx, op); err != nil {
		t.Fatalf("Create operation %q: %v", op.GetName(), err)
	}
	t.Logf("Created operation %q", op.GetName())

	t.Cleanup(func() {
		t.Logf("Cleaning up operation %q.", op.GetName())
		if err := kube.Get(ctx, types.NamespacedName{Name: op.GetName()}, op); resource.IgnoreNotFound(err) != nil {
			t.Fatalf("Get operation %q: %v", op.GetName(), err)
		}
		if err := kube.Delete(ctx, op); resource.IgnoreNotFound(err) != nil {
			t.Fatalf("Delete operation %q: %v", op.GetName(), err)
		}
		t.Logf("Deleted operation %q", op.GetName())
	})

	return op
}

// TestWatchOperation tests the functionality of WatchOperation objects. A
// WatchOperation is created and set to watch a set of objects. We then create
// one of those objects and verify that an operation pipeline was run and the
// results were successful.
func TestWatchOperation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	t.Cleanup(cancel)

	kube, err := internal.NewClient()
	if err != nil {
		t.Fatalf("Create client: %v", err)
	}

	// Create a namespace for the watch operation resources
	internal.CreateNamespace(ctx, t, kube)

	// Create operation-capable function
	fnc := internal.CreateFunction(ctx, t, kube, "xpkg.crossplane.io/crossplane-contrib/function-dummy:v0.4.1")

	// Create a WatchOperation that watches for ConfigMaps
	watchOp := createWatchOperation(ctx, t, kube, fnc)

	// Wait for WatchOperation to become synced and actively watching
	t.Run("WatchOperationBecomesActive", func(t *testing.T) {
		t.Log("Testing that the WatchOperation becomes synced and watching.")
		if err := wait.PollUntilContextTimeout(ctx, 10*time.Second, 2*time.Minute, true, func(ctx context.Context) (done bool, err error) {
			if err := kube.Get(ctx, types.NamespacedName{Name: watchOp.GetName()}, watchOp); err != nil {
				return false, err
			}

			if watchOp.GetCondition(xpv1.TypeSynced).Status != corev1.ConditionTrue {
				t.Logf("WatchOperation %q is not yet synced", watchOp.GetName())
				return false, nil
			}

			if watchOp.GetCondition(opsv1alpha1.TypeWatching).Status != corev1.ConditionTrue {
				t.Logf("WatchOperation %q is not yet watching", watchOp.GetName())
				return false, nil
			}

			t.Logf("WatchOperation %q is synced and watching", watchOp.GetName())
			return true, nil
		}); err != nil {
			t.Errorf("WatchOperation %q never became synced and watching: %v", watchOp.GetName(), err)
		}
	})

	// Create a ConfigMap that matches the watch criteria and verify a successful Operation runs in response.
	t.Run("WatchDetectsResourceAndTriggersOperation", func(t *testing.T) {
		t.Log("Testing that WatchOperation detects resource changes and triggers Operations.")

		// Create a ConfigMap that should trigger the WatchOperation
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "watched-configmap",
				Namespace: internal.SuiteName,
				Labels: map[string]string{
					"watch-op": "watch-me",
				},
			},
			Data: map[string]string{
				"message": "I should be watched!",
			},
		}

		if err := kube.Create(ctx, cm); err != nil {
			t.Fatalf("Create ConfigMap %q: %v", cm.GetName(), err)
		}
		t.Logf("Created ConfigMap %q", cm.GetName())

		t.Cleanup(func() {
			t.Logf("Cleaning up ConfigMap %q.", cm.GetName())
			if err := kube.Delete(ctx, cm); resource.IgnoreNotFound(err) != nil {
				t.Errorf("Delete ConfigMap %q: %v", cm.GetName(), err)
			}
		})

		// Wait for WatchOperation to detect the resource and run a successful operation
		if err := wait.PollUntilContextTimeout(ctx, 10*time.Second, 2*time.Minute, true, func(ctx context.Context) (done bool, err error) {
			if err := kube.Get(ctx, types.NamespacedName{Name: watchOp.GetName()}, watchOp); err != nil {
				return false, err
			}

			// Check if WatchOperation has detected the resource
			if watchOp.Status.WatchingResources != 1 {
				t.Logf("WatchOperation %q has not yet detected resources to watch", watchOp.GetName())
				return false, nil
			}

			// Verify WatchOperation has lastScheduleTime set
			if watchOp.Status.LastScheduleTime == nil {
				t.Logf("WatchOperation %q does not yet have lastScheduleTime set", watchOp.GetName())
				return false, nil
			}

			// Verify WatchOperation has lastSuccessfulTime set
			if watchOp.Status.LastSuccessfulTime == nil {
				t.Logf("WatchOperation %q does not yet have lastSuccessfulTime set", watchOp.GetName())
				return false, nil
			}

			// Verify the ConfigMap created by the triggered Operation exists and has expected data
			opCreatedCM := &corev1.ConfigMap{}
			if err := kube.Get(ctx, types.NamespacedName{Name: "op-created-cm", Namespace: internal.SuiteName}, opCreatedCM); err != nil {
				t.Logf("ConfigMap 'op-created-cm' created by triggered Operation not yet found: %v", err)
				return false, nil
			}

			expectedData := "I was created by an Operation triggered by a WatchOperation!"
			if opCreatedCM.Data["message"] != expectedData {
				t.Logf("ConfigMap %q does not have expected data. want: %q, got: %q", cm.GetName(), expectedData, opCreatedCM.Data["message"])
				return false, nil
			}

			t.Logf("WatchOperation %q successfully detected resource and triggered a successful Operation", watchOp.GetName())
			return true, nil
		}); err != nil {
			t.Errorf("WatchOperation %q never detected resource and triggered successful Operation: %v", watchOp.GetName(), err)
		}
	})
}

// createWatchOperation creates a WatchOperation that watches for ConfigMaps with specific labels
func createWatchOperation(ctx context.Context, t *testing.T, kube client.Client, fnc *pkgv1.Function) *opsv1alpha1.WatchOperation {
	t.Helper()

	watchOp := &opsv1alpha1.WatchOperation{
		ObjectMeta: metav1.ObjectMeta{Name: internal.SuiteName + "-watch-operation"},
		Spec: opsv1alpha1.WatchOperationSpec{
			Watch: opsv1alpha1.WatchSpec{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Namespace:  internal.SuiteName,
				MatchLabels: map[string]string{
					"watch-op": "watch-me",
				},
			},
			OperationTemplate: opsv1alpha1.OperationTemplate{
				Spec: opsv1alpha1.OperationSpec{
					Mode: opsv1alpha1.OperationModePipeline,
					Pipeline: []opsv1alpha1.PipelineStep{
						{
							Step: "do-the-op",
							FunctionRef: opsv1alpha1.FunctionReference{
								Name: fnc.GetName(),
							},
							Input: &runtime.RawExtension{
								Raw: []byte(fmt.Sprintf(`{
									"apiVersion": "dummy.fn.crossplane.io/v1beta1",
									"kind": "Response",
									"response": {
										"desired": {
											"resources": {
												"op-created-cm": {
													"resource": {
														"apiVersion": "v1",
														"kind": "ConfigMap",
														"metadata": {
															"namespace": "%s",
															"name": "op-created-cm"
														},
														"data": {
															"message": "I was created by an Operation triggered by a WatchOperation!"
														}
													}
												}
											}
										},
										"results": [
											{
												"severity": "SEVERITY_NORMAL",
												"message": "WatchOperation pipeline executed successfully!"
											}
										]
									}
								}`, internal.SuiteName)),
							},
						},
					},
				},
			},
		},
	}

	if err := kube.Create(ctx, watchOp); err != nil {
		t.Fatalf("Create WatchOperation %q: %v", watchOp.GetName(), err)
	}
	t.Logf("Created WatchOperation %q", watchOp.GetName())

	t.Cleanup(func() {
		t.Logf("Cleaning up WatchOperation %q.", watchOp.GetName())
		if err := kube.Get(ctx, types.NamespacedName{Name: watchOp.GetName()}, watchOp); resource.IgnoreNotFound(err) != nil {
			t.Fatalf("Get WatchOperation %q: %v", watchOp.GetName(), err)
		}
		if err := kube.Delete(ctx, watchOp); resource.IgnoreNotFound(err) != nil {
			t.Fatalf("Delete WatchOperation %q: %v", watchOp.GetName(), err)
		}
		t.Logf("Deleted WatchOperation %q", watchOp.GetName())
	})

	return watchOp
}

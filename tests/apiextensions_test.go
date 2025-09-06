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
	"encoding/base64"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	kextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource/unstructured/composite"

	extv1 "github.com/crossplane/crossplane/v2/apis/apiextensions/v1"
	extv2 "github.com/crossplane/crossplane/v2/apis/apiextensions/v2"
	pkgv1 "github.com/crossplane/crossplane/v2/apis/pkg/v1"

	"github.com/crossplane/conformance/internal"
)

// TestCompositeResourceDefinitionNamespace tests the creation of a namespaced XRD and
// verifies that the resulting CRD is conformant.
func TestCompositeResourceDefinitionNamespace(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)

	kube, err := internal.NewClient()
	if err != nil {
		t.Fatalf("Create client: %v", err)
	}

	// create a v2 XRD
	xrd := &extv2.CompositeResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "namespaceconformances.test.crossplane.io"},
		Spec: extv2.CompositeResourceDefinitionSpec{
			Scope: extv2.CompositeResourceScopeNamespaced,
			Group: "test.crossplane.io",
			Names: kextv1.CustomResourceDefinitionNames{
				Kind:     "NamespaceConformance",
				ListKind: "NamespaceConformanceList",
				Plural:   "namespaceconformances",
				Singular: "namespaceconformance",
			},
			Versions: []extv2.CompositeResourceDefinitionVersion{{
				Name:          "v1alpha1",
				Served:        true,
				Referenceable: true,
				Schema: &extv2.CompositeResourceValidation{
					OpenAPIV3Schema: runtime.RawExtension{Raw: []byte("{}")},
				},
			}},
		},
	}

	if err := kube.Create(ctx, xrd); err != nil {
		t.Fatalf("Create XRD %q: %v", xrd.GetName(), err)
	}
	t.Logf("Created XRD %q", xrd.GetName())

	t.Cleanup(func() {
		t.Logf("Cleaning up XRD %q.", xrd.GetName())
		if err := kube.Get(ctx, types.NamespacedName{Name: xrd.GetName()}, xrd); err != nil {
			t.Fatalf("Get XRD %q: %v", xrd.GetName(), err)
		}
		if err := kube.Delete(ctx, xrd); err != nil {
			t.Fatalf("Delete XRD %q: %v", xrd.GetName(), err)
		}
		t.Logf("Deleted XRD %q", xrd.GetName())
	})

	// verify the XRD becomes established
	testXRDIsEstablished(ctx, t, kube, xrd)

	t.Run("CRDIsCreatedForXR", func(t *testing.T) {
		t.Log("Testing that the XRD creates a conformant CRD for its XR.")

		// TODO(negz): Use Crossplane's internal/xcrd package instead? We can't
		// import it because it's internal, so we replicate a bunch of it here.
		// We could fork it, open it up, or move these tests into the core repo.
		want := kextv1.CustomResourceDefinitionSpec{
			Group: xrd.Spec.Group,
			Names: kextv1.CustomResourceDefinitionNames{
				Kind:       xrd.Spec.Names.Kind,
				ListKind:   xrd.Spec.Names.ListKind,
				Plural:     xrd.Spec.Names.Plural,
				Singular:   xrd.Spec.Names.Singular,
				Categories: []string{"composite"},
			},
			Scope: kextv1.NamespaceScoped,
			Versions: []kextv1.CustomResourceDefinitionVersion{{
				Name:         xrd.Spec.Versions[0].Name,
				Served:       true,
				Storage:      true,
				Subresources: &kextv1.CustomResourceSubresources{Status: &kextv1.CustomResourceSubresourceStatus{}},
				Schema: &kextv1.CustomResourceValidation{
					OpenAPIV3Schema: &kextv1.JSONSchemaProps{
						Type:     "object",
						Required: []string{"spec"},
						Properties: map[string]kextv1.JSONSchemaProps{
							"apiVersion": {
								Type: "string",
							},
							"kind": {
								Type: "string",
							},
							"metadata": {
								Type: "object",
								Properties: map[string]kextv1.JSONSchemaProps{
									"name": {
										Type: "string",
										// https://github.com/crossplane/crossplane/commit/0181529f057049fc210ff02345a097bdc9ccc95d
										MaxLength: ptr.To(int64(63)),
									},
								},
							},
							"spec": {
								Type: "object",
								Properties: map[string]kextv1.JSONSchemaProps{
									"crossplane": {
										Type:        "object",
										Description: "Configures how Crossplane will reconcile this composite resource",
										Properties: map[string]kextv1.JSONSchemaProps{
											"compositionRef": {
												Type:     "object",
												Required: []string{"name"},
												Properties: map[string]kextv1.JSONSchemaProps{
													"name": {Type: "string"},
												},
											},
											"compositionSelector": {
												Type:     "object",
												Required: []string{"matchLabels"},
												Properties: map[string]kextv1.JSONSchemaProps{
													"matchLabels": {
														Type: "object",
														AdditionalProperties: &kextv1.JSONSchemaPropsOrBool{
															Allows: true,
															Schema: &kextv1.JSONSchemaProps{Type: "string"},
														},
													},
												},
											},
											"compositionRevisionRef": { // https://github.com/crossplane/crossplane/commit/deb18660640742f236d9057587671c74224afcc4
												Type:     "object",
												Required: []string{"name"},
												Properties: map[string]kextv1.JSONSchemaProps{
													"name": {Type: "string"},
												},
											},
											"compositionRevisionSelector": { // https://github.com/crossplane/crossplane/commit/7802cf85a5dd068038a115c42434e0db2d9dfa1f
												Type:     "object",
												Required: []string{"matchLabels"},
												Properties: map[string]kextv1.JSONSchemaProps{
													"matchLabels": {
														Type: "object",
														AdditionalProperties: &kextv1.JSONSchemaPropsOrBool{
															Allows: true,
															Schema: &kextv1.JSONSchemaProps{Type: "string"},
														},
													},
												},
											},
											"compositionUpdatePolicy": { // https://github.com/crossplane/crossplane/commit/deb18660640742f236d9057587671c74224afcc4
												Type: "string",
												Enum: []kextv1.JSON{
													{Raw: []byte(`"Automatic"`)},
													{Raw: []byte(`"Manual"`)},
												},
												Default: &kextv1.JSON{Raw: []byte(`"Automatic"`)}, // https://github.com/crossplane/crossplane/commit/d346a67573e5cd111a7631bdb0f86ed9c0914204
											},
											"resourceRefs": {
												Type: "array",
												Items: &kextv1.JSONSchemaPropsOrArray{
													Schema: &kextv1.JSONSchemaProps{
														Type: "object",
														Properties: map[string]kextv1.JSONSchemaProps{
															"apiVersion": {Type: "string"},
															"name":       {Type: "string"},
															"kind":       {Type: "string"},
														},
														Required: []string{"apiVersion", "kind"}, // https://github.com/crossplane/crossplane/commit/6750ee120a75662d952689fc24c801384e96baa5
													},
												},
												XListType: ptr.To("atomic"), // https://github.com/crossplane/crossplane/commit/683f0c5763de698e1fec3eee460ba6fee75379a4
											},
										},
									},
								},
							},
							"status": {
								Type: "object",
								Properties: map[string]kextv1.JSONSchemaProps{
									"conditions": {
										Description: "Conditions of the resource.",
										Type:        "array",
										// XListMapKeys and XListType both added in https://github.com/crossplane/crossplane/commit/6ac7567cbb5bf139c22aa90cdec643d1dcf15846
										XListMapKeys: []string{"type"},
										XListType:    ptr.To("map"),
										Items: &kextv1.JSONSchemaPropsOrArray{
											Schema: &kextv1.JSONSchemaProps{
												Type:     "object",
												Required: []string{"lastTransitionTime", "reason", "status", "type"},
												Properties: map[string]kextv1.JSONSchemaProps{
													"lastTransitionTime": {Type: "string", Format: "date-time"},
													"message":            {Type: "string"},
													"observedGeneration": {Type: "integer", Format: "int64"}, // https://github.com/crossplane/crossplane/commit/57dc2400d70a28cff2cade7276dbdec837c48cd5
													"reason":             {Type: "string"},
													"status":             {Type: "string"},
													"type":               {Type: "string"},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				AdditionalPrinterColumns: []kextv1.CustomResourceColumnDefinition{
					{
						// https://github.com/crossplane/crossplane/commit/b0437b7d3901a971fc60f525c8ea63460a4ed00d
						Name:     "SYNCED",
						Type:     "string",
						JSONPath: ".status.conditions[?(@.type=='Synced')].status",
					},
					{
						Name:     "READY",
						Type:     "string",
						JSONPath: ".status.conditions[?(@.type=='Ready')].status",
					},
					{
						Name:     "COMPOSITION",
						Type:     "string",
						JSONPath: ".spec.crossplane.compositionRef.name",
					},
					{
						// https://github.com/crossplane/crossplane/commit/d1587b4484971e448161e62fb4bf9a8fab76bc40
						Name:     "COMPOSITIONREVISION",
						Type:     "string",
						JSONPath: ".spec.crossplane.compositionRevisionRef.name",
						Priority: 1,
					},
					{
						Name:     "AGE",
						Type:     "date",
						JSONPath: ".metadata.creationTimestamp",
					},
				},
			}},
			Conversion: &kextv1.CustomResourceConversion{Strategy: kextv1.NoneConverter},
		}

		// verify that a conformant CRD was created for this XRD
		testXRDCreatesCRD(ctx, t, kube, xrd, want)
	})

	// ensure a claim CRD is not created for the XRD
	testXRDDoesNotCreateClaimCRD(ctx, t, kube, xrd)
}

// TestCompositeResourceDefinitionCluster tests the creation of a cluster scoped
// XRD and verifies that the resulting CRD is conformant.
func TestCompositeResourceDefinitionCluster(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)

	kube, err := internal.NewClient()
	if err != nil {
		t.Fatalf("Create client: %v", err)
	}

	// create a v2 XRD
	xrd := &extv2.CompositeResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "clusterconformances.test.crossplane.io"},
		Spec: extv2.CompositeResourceDefinitionSpec{
			Scope: extv2.CompositeResourceScopeCluster,
			Group: "test.crossplane.io",
			Names: kextv1.CustomResourceDefinitionNames{
				Kind:     "ClusterConformance",
				ListKind: "ClusterConformanceList",
				Plural:   "clusterconformances",
				Singular: "clusterconformance",
			},
			Versions: []extv2.CompositeResourceDefinitionVersion{{
				Name:          "v1alpha1",
				Served:        true,
				Referenceable: true,
				Schema: &extv2.CompositeResourceValidation{
					OpenAPIV3Schema: runtime.RawExtension{Raw: []byte("{}")},
				},
			}},
		},
	}

	if err := kube.Create(ctx, xrd); err != nil {
		t.Fatalf("Create XRD %q: %v", xrd.GetName(), err)
	}
	t.Logf("Created XRD %q", xrd.GetName())

	t.Cleanup(func() {
		t.Logf("Cleaning up XRD %q.", xrd.GetName())
		if err := kube.Get(ctx, types.NamespacedName{Name: xrd.GetName()}, xrd); err != nil {
			t.Fatalf("Get XRD %q: %v", xrd.GetName(), err)
		}
		if err := kube.Delete(ctx, xrd); err != nil {
			t.Fatalf("Delete XRD %q: %v", xrd.GetName(), err)
		}
		t.Logf("Deleted XRD %q", xrd.GetName())
	})

	// verify the XRD becomes established
	testXRDIsEstablished(ctx, t, kube, xrd)

	t.Run("CRDIsCreatedForXR", func(t *testing.T) {
		t.Log("Testing that the XRD creates a conformant CRD for its XR.")

		// TODO(negz): Use Crossplane's internal/xcrd package instead? We can't
		// import it because it's internal, so we replicate a bunch of it here.
		// We could fork it, open it up, or move these tests into the core repo.
		want := kextv1.CustomResourceDefinitionSpec{
			Group: xrd.Spec.Group,
			Names: kextv1.CustomResourceDefinitionNames{
				Kind:       xrd.Spec.Names.Kind,
				ListKind:   xrd.Spec.Names.ListKind,
				Plural:     xrd.Spec.Names.Plural,
				Singular:   xrd.Spec.Names.Singular,
				Categories: []string{"composite"},
			},
			Scope: kextv1.ClusterScoped,
			Versions: []kextv1.CustomResourceDefinitionVersion{{
				Name:         xrd.Spec.Versions[0].Name,
				Served:       true,
				Storage:      true,
				Subresources: &kextv1.CustomResourceSubresources{Status: &kextv1.CustomResourceSubresourceStatus{}},
				Schema: &kextv1.CustomResourceValidation{
					OpenAPIV3Schema: &kextv1.JSONSchemaProps{
						Type:     "object",
						Required: []string{"spec"},
						Properties: map[string]kextv1.JSONSchemaProps{
							"apiVersion": {
								Type: "string",
							},
							"kind": {
								Type: "string",
							},
							"metadata": {
								Type: "object",
								Properties: map[string]kextv1.JSONSchemaProps{
									"name": {
										Type: "string",
										// https://github.com/crossplane/crossplane/commit/0181529f057049fc210ff02345a097bdc9ccc95d
										MaxLength: ptr.To(int64(63)),
									},
								},
							},
							"spec": {
								Type: "object",
								Properties: map[string]kextv1.JSONSchemaProps{
									"crossplane": {
										Type:        "object",
										Description: "Configures how Crossplane will reconcile this composite resource",
										Properties: map[string]kextv1.JSONSchemaProps{
											"compositionRef": {
												Type:     "object",
												Required: []string{"name"},
												Properties: map[string]kextv1.JSONSchemaProps{
													"name": {Type: "string"},
												},
											},
											"compositionSelector": {
												Type:     "object",
												Required: []string{"matchLabels"},
												Properties: map[string]kextv1.JSONSchemaProps{
													"matchLabels": {
														Type: "object",
														AdditionalProperties: &kextv1.JSONSchemaPropsOrBool{
															Allows: true,
															Schema: &kextv1.JSONSchemaProps{Type: "string"},
														},
													},
												},
											},
											"compositionRevisionRef": { // https://github.com/crossplane/crossplane/commit/deb18660640742f236d9057587671c74224afcc4
												Type:     "object",
												Required: []string{"name"},
												Properties: map[string]kextv1.JSONSchemaProps{
													"name": {Type: "string"},
												},
											},
											"compositionRevisionSelector": { // https://github.com/crossplane/crossplane/commit/7802cf85a5dd068038a115c42434e0db2d9dfa1f
												Type:     "object",
												Required: []string{"matchLabels"},
												Properties: map[string]kextv1.JSONSchemaProps{
													"matchLabels": {
														Type: "object",
														AdditionalProperties: &kextv1.JSONSchemaPropsOrBool{
															Allows: true,
															Schema: &kextv1.JSONSchemaProps{Type: "string"},
														},
													},
												},
											},
											"compositionUpdatePolicy": { // https://github.com/crossplane/crossplane/commit/deb18660640742f236d9057587671c74224afcc4
												Type: "string",
												Enum: []kextv1.JSON{
													{Raw: []byte(`"Automatic"`)},
													{Raw: []byte(`"Manual"`)},
												},
												Default: &kextv1.JSON{Raw: []byte(`"Automatic"`)}, // https://github.com/crossplane/crossplane/commit/d346a67573e5cd111a7631bdb0f86ed9c0914204
											},
											"resourceRefs": {
												Type: "array",
												Items: &kextv1.JSONSchemaPropsOrArray{
													Schema: &kextv1.JSONSchemaProps{
														Type: "object",
														Properties: map[string]kextv1.JSONSchemaProps{
															"apiVersion": {Type: "string"},
															"kind":       {Type: "string"},
															"name":       {Type: "string"},
															"namespace":  {Type: "string"}, // cluster scoped XR's can specify the namespace of their resource refs
														},
														Required: []string{"apiVersion", "kind"}, // https://github.com/crossplane/crossplane/commit/6750ee120a75662d952689fc24c801384e96baa5
													},
												},
												XListType: ptr.To("atomic"), // https://github.com/crossplane/crossplane/commit/683f0c5763de698e1fec3eee460ba6fee75379a4
											},
										},
									},
								},
							},
							"status": {
								Type: "object",
								Properties: map[string]kextv1.JSONSchemaProps{
									"conditions": {
										Description: "Conditions of the resource.",
										Type:        "array",
										// XListMapKeys and XListType both added in https://github.com/crossplane/crossplane/commit/6ac7567cbb5bf139c22aa90cdec643d1dcf15846
										XListMapKeys: []string{"type"},
										XListType:    ptr.To("map"),
										Items: &kextv1.JSONSchemaPropsOrArray{
											Schema: &kextv1.JSONSchemaProps{
												Type:     "object",
												Required: []string{"lastTransitionTime", "reason", "status", "type"},
												Properties: map[string]kextv1.JSONSchemaProps{
													"lastTransitionTime": {Type: "string", Format: "date-time"},
													"message":            {Type: "string"},
													"observedGeneration": {Type: "integer", Format: "int64"}, // https://github.com/crossplane/crossplane/commit/57dc2400d70a28cff2cade7276dbdec837c48cd5
													"reason":             {Type: "string"},
													"status":             {Type: "string"},
													"type":               {Type: "string"},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				AdditionalPrinterColumns: []kextv1.CustomResourceColumnDefinition{
					{
						// https://github.com/crossplane/crossplane/commit/b0437b7d3901a971fc60f525c8ea63460a4ed00d
						Name:     "SYNCED",
						Type:     "string",
						JSONPath: ".status.conditions[?(@.type=='Synced')].status",
					},
					{
						Name:     "READY",
						Type:     "string",
						JSONPath: ".status.conditions[?(@.type=='Ready')].status",
					},
					{
						Name:     "COMPOSITION",
						Type:     "string",
						JSONPath: ".spec.crossplane.compositionRef.name",
					},
					{
						// https://github.com/crossplane/crossplane/commit/d1587b4484971e448161e62fb4bf9a8fab76bc40
						Name:     "COMPOSITIONREVISION",
						Type:     "string",
						JSONPath: ".spec.crossplane.compositionRevisionRef.name",
						Priority: 1,
					},
					{
						Name:     "AGE",
						Type:     "date",
						JSONPath: ".metadata.creationTimestamp",
					},
				},
			}},
			Conversion: &kextv1.CustomResourceConversion{Strategy: kextv1.NoneConverter},
		}

		// verify that a conformant CRD was created for this XRD
		testXRDCreatesCRD(ctx, t, kube, xrd, want)
	})

	// ensure a claim CRD is not created for the XRD
	testXRDDoesNotCreateClaimCRD(ctx, t, kube, xrd)
}

// testXRDIsEstablished verifies that the given XRD becomes established.
func testXRDIsEstablished(ctx context.Context, t *testing.T, kube client.Client, xrd *extv2.CompositeResourceDefinition) {
	t.Helper()
	t.Run("BecomesEstablished", func(t *testing.T) {
		t.Log("Testing that the XRD's Established status condition becomes 'True'.")
		if err := wait.PollUntilContextTimeout(ctx, 10*time.Second, 90*time.Second, true, func(ctx context.Context) (done bool, err error) {
			if err := kube.Get(ctx, types.NamespacedName{Name: xrd.GetName()}, xrd); err != nil {
				return false, err
			}

			if xrd.Status.GetCondition(extv1.TypeEstablished).Status != corev1.ConditionTrue {
				t.Logf("XRD %q is not yet Established", xrd.GetName())
				return false, nil
			}

			if xrd.Status.GetCondition(extv1.TypeOffered).Status == corev1.ConditionTrue {
				t.Logf("XRD %q unexpectedly became Offered", xrd.GetName())
				return true, fmt.Errorf("XRD %q unexpectedly became Offered", xrd.GetName())
			}

			t.Logf("XRD %q is Established", xrd.GetName())
			return true, nil
		}); err != nil {
			t.Errorf("XRD %q never became Established: %v", xrd.GetName(), err)
		}
	})
}

// testXRDCreatesCRD verifies that a conformant CRD is created from the given XRD.
func testXRDCreatesCRD(ctx context.Context, t *testing.T, kube client.Client, xrd *extv2.CompositeResourceDefinition, want kextv1.CustomResourceDefinitionSpec) {
	t.Helper()
	if err := wait.PollUntilContextTimeout(ctx, 10*time.Second, 90*time.Second, true, func(ctx context.Context) (done bool, err error) {
		crd := &kextv1.CustomResourceDefinition{}
		if err := kube.Get(ctx, types.NamespacedName{Name: xrd.GetName()}, crd); err != nil {
			if kerrors.IsNotFound(err) {
				t.Logf("CRD %q has not yet been created", xrd.GetName())
				return false, nil
			}
			return false, err
		}

		if !internal.CRDIs(crd.Status.Conditions, kextv1.Established) {
			t.Logf("CRD %q is not yet Established", crd.GetName())
			return false, nil
		}

		if diff := cmp.Diff(want, crd.Spec); diff != "" {
			t.Errorf("CRD %q is not conformant: -want, +got:\n%s", crd.GetName(), diff)
			return true, nil
		}

		t.Logf("XRD %q created a conformant XR CRD", xrd.GetName())
		return true, nil
	}); err != nil {
		t.Errorf("XRD %q never created a conformant XR CRD: %v", xrd.GetName(), err)
	}

	//  verify only 1 CRD was created in this test group, i.e. no CRD was also created for a claim
	crdList := &kextv1.CustomResourceDefinitionList{}
	if err := kube.List(ctx, crdList); err != nil {
		t.Fatalf("Cannot list CRDs: %v", err)
	}

	crdCount := 0
	for _, crd := range crdList.Items {
		if crd.Spec.Group == xrd.Spec.Group {
			crdCount++
		}
	}

	if crdCount != 1 {
		t.Errorf("XRD %q created %d CRDs, but should have only created 1", xrd.GetName(), crdCount)
	}
}

// testXRDDoesNotCreateClaimCRD verifies that no claim CRD is created for an XRD.
func testXRDDoesNotCreateClaimCRD(ctx context.Context, t *testing.T, kube client.Client, xrd *extv2.CompositeResourceDefinition) {
	t.Helper()
	t.Run("NoCRDIsCreatedForXRC", func(t *testing.T) {
		t.Log("Testing that the XRD does not create a CRD for a claim.")

		// Wait a minimum period to ensure a claim CRD does not appear.
		if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 30*time.Second, true, func(ctx context.Context) (done bool, err error) {
			crdList := &kextv1.CustomResourceDefinitionList{}
			if err := kube.List(ctx, crdList); err != nil {
				//nolint:nilerr // ignore any list errors that could be transient - go all the way until the end of the polling period
				return false, nil
			}

			for _, crd := range crdList.Items {
				if crd.Spec.Group != xrd.Spec.Group {
					continue
				}
				for _, c := range crd.Spec.Names.Categories {
					if c == "claim" {
						// We found a claim CRD in the XRDs group, fail the test
						return true, fmt.Errorf("XRD %q unexpectedly created a claim CRD %q", xrd.GetName(), crd.GetName())
					}
				}
			}

			// Keep polling until the timeout to ensure there is no claim CRD the entire time
			return false, nil
		}); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				// A timeout (context.DeadlineExceeded) means we observed no claim CRD for the full window - this is successful
				t.Logf("XRD %q successfully did not create a CRD for a claim.", xrd.GetName())
			} else {
				t.Errorf("Error encountered while testing that XRD %q does not create a CRD for a claim: %v", xrd.GetName(), err)
			}
		}
	})
}

// TestCompositeResourceNamespace tests the creation of a namespaced XRD and
// Composition that uses a function pipeline to compose resources. The function
// pipeline uses function-dummy and this test verifies that some simple logic
// run by the function pipeline was successful.
func TestCompositeResourceNamespace(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	t.Cleanup(cancel)

	kube, err := internal.NewClient()
	if err != nil {
		t.Fatalf("Create client: %v", err)
	}

	// create and wait for function to be installed/healthy that will be used in this test
	fnc := internal.CreateFunction(ctx, t, kube, "xpkg.crossplane.io/crossplane-contrib/function-dummy:v0.4.1")

	// create namespaced XRD and verify it becomes established/offered
	scope := extv2.CompositeResourceScopeNamespaced
	xrdNames := kextv1.CustomResourceDefinitionNames{
		Kind:     "NamespaceConformance",
		ListKind: "NamespaceConformanceList",
		Plural:   "namespaceconformances",
		Singular: "namespaceconformance",
	}
	xrd := createAndTestXRD(ctx, t, kube, scope, xrdNames)

	// create a composition that will create resources using Pipeline mode that
	// exercises functions (specifically function-dummy in this case).
	input := &runtime.RawExtension{
		Raw: []byte(`{
			"apiVersion": "dummy.fn.crossplane.io/v1beta1",
			"kind": "Response",
			"response": {
				"desired": {
					"composite": {
						"resource": {
							"status": {
								"coolerField": "I'M COOLER!"
							}
						}
					},
					"resources": {
						"configmap": {
							"resource": {
								"apiVersion": "v1",
								"kind": "ConfigMap",
								"data": {
									"coolData": "I'm cool composed resource data!"
								}
							},
							"ready": "READY_TRUE"
						},
						"secret": {
							"resource": {
								"apiVersion": "v1",
								"kind": "Secret",
								"data": {
									"secretData": "` + base64.StdEncoding.EncodeToString([]byte("I'm secret composed resource data!")) + `"
								}
							},
							"ready": "READY_TRUE"
						}
					}
				},
				"results": [
					{
						"severity": "SEVERITY_NORMAL",
						"message": "I am doing a compose!"
					}
				]
			}
		}`),
	}
	createComposition(ctx, t, kube, xrdNames.Kind, fnc, input)

	// create a namespace for the XR to live in
	internal.CreateNamespace(ctx, t, kube)

	// test the composite resource, wait for it to become ready
	xr := createAndTestXR(ctx, t, kube, xrd)

	// test the composed resource references
	testXRResourceRefs(ctx, t, kube, xr, []internal.ResourceRef{
		{
			Kind:        "ConfigMap",
			APIVersion:  "v1",
			Namespace:   internal.SuiteName,
			FieldValues: map[string]string{"data.coolData": "I'm cool composed resource data!"},
		},
		{
			Kind:        "Secret",
			APIVersion:  "v1",
			Namespace:   internal.SuiteName,
			FieldValues: map[string]string{"data.secretData": base64.StdEncoding.EncodeToString([]byte("I'm secret composed resource data!"))},
		},
	})
}

// TestCompositeResourceCluster tests the creation of a cluster scoped XRD and
// Composition that uses a function pipeline to compose resources. The function
// pipeline uses function-dummy and this test verifies that some simple logic
// run by the function pipeline was successful.
func TestCompositeResourceCluster(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	t.Cleanup(cancel)

	kube, err := internal.NewClient()
	if err != nil {
		t.Fatalf("Create client: %v", err)
	}

	// create and wait for function to be installed/healthy that will be used in this test
	fnc := internal.CreateFunction(ctx, t, kube, "xpkg.crossplane.io/crossplane-contrib/function-dummy:v0.4.1")

	// create namespaced XRD and verify it becomes established/offered
	scope := extv2.CompositeResourceScopeCluster
	xrdNames := kextv1.CustomResourceDefinitionNames{
		Kind:     "ClusterConformance",
		ListKind: "ClusterConformanceList",
		Plural:   "clusterconformances",
		Singular: "clusterconformance",
	}
	xrd := createAndTestXRD(ctx, t, kube, scope, xrdNames)

	// create a composition that will create resources using Pipeline mode that
	// exercises functions (specifically function-dummy in this case). For this
	// cluster scoped XR, we will compose both a cluster scoped resource (a CRD)
	// and a namespaced scoped resource (a ConfigMap).
	input := &runtime.RawExtension{
		Raw: []byte(fmt.Sprintf(`{
			"apiVersion": "dummy.fn.crossplane.io/v1beta1",
			"kind": "Response",
			"response": {
				"desired": {
					"composite": {
						"resource": {
							"status": {
								"coolerField": "I'M COOLER!"
							}
						}
					},
					"resources": {
						"crd": {
							"resource": {
								"apiVersion": "apiextensions.k8s.io/v1",
								"kind": "CustomResourceDefinition",
								"metadata": {
									"name": "composedcrds.test.crossplane.io"
								},
								"spec": {
									"group": "test.crossplane.io",
									"names": {
										"kind": "ComposedCRD",
										"plural": "composedcrds"
									},
									"scope": "Cluster",
									"versions": [
										{
											"name": "v1alpha3",
											"served": true,
											"storage": true,
											"schema": {
												"openAPIV3Schema": {
													"type": "object"
												}
											}
										}
									]
								}
							},
							"ready": "READY_TRUE"
						},
						"configmap": {
							"resource": {
								"apiVersion": "v1",
								"kind": "ConfigMap",
								"metadata": {
									"namespace": "%s"
								},
								"data": {
									"coolData": "I'm cool composed resource data!"
								}
							},
							"ready": "READY_TRUE"
						}
					}
				},
				"results": [
					{
						"severity": "SEVERITY_NORMAL",
						"message": "I am doing a compose!"
					}
				]
			}
		}`, internal.SuiteName)),
	}
	createComposition(ctx, t, kube, xrdNames.Kind, fnc, input)

	// create a namespace for the namespaced composed resources of the XR to live in
	internal.CreateNamespace(ctx, t, kube)

	// test the composite resource, wait for it to become ready
	xr := createAndTestXR(ctx, t, kube, xrd)

	// test the composed resource references
	testXRResourceRefs(ctx, t, kube, xr, []internal.ResourceRef{
		{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
			FieldValues: map[string]string{
				"spec.versions[0].name": "v1alpha3",
			},
		},
		{
			Kind:       "ConfigMap",
			APIVersion: "v1",
			Namespace:  internal.SuiteName,
			FieldValues: map[string]string{
				"data.coolData": "I'm cool composed resource data!",
			},
		},
	})
}

// createAndTestXRD creates an XRD to use in testing, ensures that it becomes
// established, and ensures its clean up.
func createAndTestXRD(ctx context.Context, t *testing.T, kube client.Client, scope extv2.CompositeResourceScope, names kextv1.CustomResourceDefinitionNames) *extv2.CompositeResourceDefinition {
	t.Helper()
	xrd := &extv2.CompositeResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("%s.test.crossplane.io", names.Plural)},
		Spec: extv2.CompositeResourceDefinitionSpec{
			Scope: scope,
			Group: "test.crossplane.io",
			Names: names,
			Versions: []extv2.CompositeResourceDefinitionVersion{{
				Name:          "v1alpha1",
				Served:        true,
				Referenceable: true,
				Schema: &extv2.CompositeResourceValidation{
					OpenAPIV3Schema: runtime.RawExtension{Raw: []byte(`{
						"type": "object",
						"properties": {
							"spec": {
								"type": "object",
								"properties": {
									"coolField": {
										"type": "string"
									}
								},
								"required": [
									"coolField"
								]
							},
							"status": {
								"type": "object",
								"properties": {
									"coolerField": {
										"type": "string"
									}
								}
							}
						}
					}`)},
				},
			}},
		},
	}
	// XRDs take a while to delete, so we try a few times in case creates are
	// failing due to an old XRD hanging around.
	if err := wait.PollUntilContextTimeout(ctx, 10*time.Second, 60*time.Second, true, func(ctx context.Context) (done bool, err error) {
		if err := kube.Create(ctx, xrd); err != nil {
			t.Logf("Create XRD %q: %v", xrd.GetName(), err)
			return false, nil
		}
		return true, nil
	}); err != nil {
		t.Fatalf("Create XRD %q: %v", xrd.GetName(), err)
	}
	t.Logf("Created XRD %q", xrd.GetName())

	t.Cleanup(func() {
		t.Logf("Cleaning up XRD %q.", xrd.GetName())
		if err := kube.Get(ctx, types.NamespacedName{Name: xrd.GetName()}, xrd); resource.IgnoreNotFound(err) != nil {
			t.Fatalf("Get XRD %q: %v", xrd.GetName(), err)
		}
		if err := kube.Delete(ctx, xrd); resource.IgnoreNotFound(err) != nil {
			t.Fatalf("Delete XRD %q: %v", xrd.GetName(), err)
		}
		t.Logf("Deleted XRD %q", xrd.GetName())
	})

	t.Log("Waiting for the XRD's Established status condition to become 'True'.")
	if err := wait.PollUntilContextTimeout(ctx, 10*time.Second, 60*time.Second, true, func(ctx context.Context) (done bool, err error) {
		if err := kube.Get(ctx, types.NamespacedName{Name: xrd.GetName()}, xrd); err != nil {
			return false, err
		}

		if xrd.Status.GetCondition(extv1.TypeEstablished).Status != corev1.ConditionTrue {
			t.Logf("XRD %q is not yet Established", xrd.GetName())
			return false, nil
		}

		if xrd.Status.GetCondition(extv1.TypeOffered).Status == corev1.ConditionTrue {
			t.Logf("XRD %q unexpectedly became Offered", xrd.GetName())
			return true, fmt.Errorf("XRD %q unexpectedly became Offered", xrd.GetName())
		}

		t.Logf("XRD %q is Established", xrd.GetName())
		return true, nil
	}); err != nil {
		t.Errorf("XRD %q never became Established: %v", xrd.GetName(), err)
	}

	return xrd
}

// createComposition creates a simple function pipeline based composition that
// composes resources using the given input and ensures its clean up.
func createComposition(ctx context.Context, t *testing.T, kube client.Client, xrdKind string, fnc *pkgv1.Function, input *runtime.RawExtension) {
	t.Helper()
	comp := &extv1.Composition{
		ObjectMeta: metav1.ObjectMeta{
			Name: internal.SuiteName,
			Labels: map[string]string{
				"crossplane.io/test": internal.SuiteName,
			},
		},
		Spec: extv1.CompositionSpec{
			CompositeTypeRef: extv1.TypeReference{
				APIVersion: "test.crossplane.io/v1alpha1",
				Kind:       xrdKind,
			},
			Mode: extv1.CompositionModePipeline,
			Pipeline: []extv1.PipelineStep{
				{
					Step: "compose-resources",
					FunctionRef: extv1.FunctionReference{
						Name: fnc.GetName(),
					},
					Input: input,
				},
			},
		},
	}
	if err := kube.Create(ctx, comp); err != nil {
		t.Fatalf("Create composition %q: %v", comp.GetName(), err)
	}
	t.Logf("Created composition %q", comp.GetName())

	t.Cleanup(func() {
		t.Logf("Cleaning up composition %q.", comp.GetName())
		if err := kube.Get(ctx, types.NamespacedName{Name: comp.GetName()}, comp); resource.IgnoreNotFound(err) != nil {
			t.Fatalf("Get composition %q: %v", comp.GetName(), err)
		}
		if err := kube.Delete(ctx, comp); resource.IgnoreNotFound(err) != nil {
			t.Fatalf("Delete composition %q: %v", comp.GetName(), err)
		}
		t.Logf("Deleted composition %q", comp.GetName())
	})
}

// createAndTestXR creates an XR for testing, verify that it becomes ready, and
// verifies the results of the composition pipeline.
func createAndTestXR(ctx context.Context, t *testing.T, kube client.Client, xrd *extv2.CompositeResourceDefinition) *composite.Unstructured {
	t.Helper()

	xr := composite.New(composite.WithGroupVersionKind(schema.GroupVersionKind{
		Group:   xrd.Spec.Group,
		Version: xrd.Spec.Versions[0].Name,
		Kind:    xrd.Spec.Names.Kind,
	}))

	xr.SetName(internal.SuiteName)
	if xrd.Spec.Scope == extv2.CompositeResourceScopeNamespaced {
		xr.SetNamespace(internal.SuiteName)
	}
	xr.SetCompositionSelector(&metav1.LabelSelector{MatchLabels: map[string]string{"crossplane.io/test": internal.SuiteName}})

	paved := fieldpath.Pave(xr.Object)
	_ = paved.SetString("spec.coolField", "I'm cool!")

	if err := kube.Create(ctx, xr); err != nil {
		t.Fatalf("Create XR %q: %v", xr.GetName(), err)
	}
	t.Logf("Created XR %q", xr.GetName())

	t.Cleanup(func() {
		t.Logf("Cleaning up XR %q.", xr.GetName())
		if err := kube.Get(ctx, types.NamespacedName{Namespace: xr.GetNamespace(), Name: xr.GetName()}, xr); resource.IgnoreNotFound(err) != nil {
			t.Fatalf("Get XR %q: %v", xr.GetName(), err)
		}
		if err := kube.Delete(ctx, xr); resource.IgnoreNotFound(err) != nil {
			t.Fatalf("Delete XR %q: %v", xr.GetName(), err)
		}
		t.Logf("Deleted XR %q", xr.GetName())
	})

	t.Run("CompositeBecomesReadyWithValues", func(t *testing.T) {
		t.Log("Testing that the composite resource becomes Ready with expected status values set from the composition pipeline.")

		if err := wait.PollUntilContextTimeout(ctx, 10*time.Second, 2*time.Minute, true, func(ctx context.Context) (done bool, err error) {
			if err := kube.Get(ctx, types.NamespacedName{Name: xr.GetName(), Namespace: xr.GetNamespace()}, xr); err != nil {
				return false, err
			}

			if xr.GetCondition(xpv1.TypeReady).Status != corev1.ConditionTrue {
				t.Logf("XR %q is not yet Ready", xr.GetName())
				return false, nil
			}

			if xr.GetCompositionReference() == nil {
				t.Logf("XR %q did not populate its composition reference", xr.GetName())
				return false, nil
			}

			if xr.GetCompositionRevisionReference() == nil {
				t.Logf("XR %q did not populate its composition revision reference", xr.GetName())
				return false, nil
			}

			// verify all expected field values on the XR, both spec and status
			paved := fieldpath.Pave(xr.Object)
			if coolField, _ := paved.GetString("spec.coolField"); coolField != "I'm cool!" {
				t.Logf("XR %q status.a: want %q, got %q", xr.GetName(), "I'm cool!", coolField)
				return false, nil
			}
			if coolerField, _ := paved.GetString("status.coolerField"); coolerField != "I'M COOLER!" {
				t.Logf("XR %q status.a: want %q, got %q", xr.GetName(), "I'M COOLER!", coolerField)
				return false, nil
			}

			t.Logf("XR %q is Ready with expected values", xr.GetName())
			return true, nil
		}); err != nil {
			t.Errorf("XR %q never became Ready: %v", xr.GetName(), err)
		}
	})

	return xr
}

// testXRResourceRefs verifies that the given XR has the given set of resource references.
func testXRResourceRefs(ctx context.Context, t *testing.T, kube client.Client, xr *composite.Unstructured, wantRefs []internal.ResourceRef) {
	t.Helper()

	t.Run("CompositeResourceRefs", func(t *testing.T) {
		t.Log("Testing that the composite resource has the expected resource refs")

		if err := wait.PollUntilContextTimeout(ctx, 10*time.Second, 2*time.Minute, true, func(ctx context.Context) (done bool, err error) {
			if err := kube.Get(ctx, types.NamespacedName{Name: xr.GetName(), Namespace: xr.GetNamespace()}, xr); err != nil {
				return false, err
			}

			// convert the composed resource references into the common
			// ResourceRef type that our test helper uses
			actualRefs := make([]internal.ResourceRef, len(xr.GetResourceReferences()))
			for i, ref := range xr.GetResourceReferences() {
				namespace := ref.Namespace
				if namespace == "" && xr.GetNamespace() != "" {
					// namespaced XRs will not have a namespace explicitly
					// listed in their references, but we'll need the namespace
					// to look up the reference. Just use the namespace of the
					// XR itself.
					namespace = xr.GetNamespace()
				}
				actualRefs[i] = internal.ResourceRef{
					Kind:       ref.Kind,
					APIVersion: ref.APIVersion,
					Namespace:  namespace,
					Name:       ref.Name,
				}
			}

			if err := internal.TestResourceRefs(ctx, t, kube, actualRefs, wantRefs); err != nil {
				t.Logf("XR %q does not have all expected resource refs: %v", xr.GetName(), err)
				return false, nil
			}

			t.Logf("XR %q has all expected resource refs", xr.GetName())
			return true, nil
		}); err != nil {
			t.Errorf("XR %q resource refs validation failed: %v", xr.GetName(), err)
		}
	})
}

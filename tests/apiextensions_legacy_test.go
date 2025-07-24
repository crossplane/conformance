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

	"github.com/crossplane/conformance/internal"
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/resource/unstructured/claim"
	"github.com/crossplane/crossplane-runtime/pkg/resource/unstructured/composite"
	extv1 "github.com/crossplane/crossplane/apis/apiextensions/v1"
	pkgv1 "github.com/crossplane/crossplane/apis/pkg/v1"
)

// TestCompositeResourceDefinitionLegacy verifies that a legacy XRD can be
// created and that conformant CRDs are created for the XR and claim.
func TestCompositeResourceDefinitionLegacy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)

	kube, err := internal.NewClient()
	if err != nil {
		t.Fatalf("Create client: %v", err)
	}

	// we create a v1 XRD for legacy testing purposes. We expect the shape of
	// the resulting CRDs to be v1 legacy style, e.g., no .spec.crossplane.*
	xrd := &extv1.CompositeResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "clusterconformances.test.crossplane.io"},
		Spec: extv1.CompositeResourceDefinitionSpec{
			Group: "test.crossplane.io",
			Names: kextv1.CustomResourceDefinitionNames{
				Kind:     "ClusterConformance",
				ListKind: "ClusterConformanceList",
				Plural:   "clusterconformances",
				Singular: "clusterconformance",
			},
			ClaimNames: &kextv1.CustomResourceDefinitionNames{
				Kind:     "Conformance",
				ListKind: "ConformanceList",
				Plural:   "conformances",
				Singular: "conformance",
			},
			Versions: []extv1.CompositeResourceDefinitionVersion{{
				Name:          "v1alpha1",
				Served:        true,
				Referenceable: true,
				Schema: &extv1.CompositeResourceValidation{
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

	t.Run("BecomesEstablishedAndOffered", func(t *testing.T) {
		t.Log("Testing that the XRD's Established and Offered status conditions become 'True'.")
		if err := wait.PollUntilContextTimeout(ctx, 10*time.Second, 90*time.Second, true, func(ctx context.Context) (done bool, err error) {
			if err := kube.Get(ctx, types.NamespacedName{Name: xrd.GetName()}, xrd); err != nil {
				return false, err
			}

			if xrd.Status.GetCondition(extv1.TypeEstablished).Status != corev1.ConditionTrue {
				t.Logf("XRD %q is not yet Established", xrd.GetName())
				return false, nil
			}

			if xrd.Status.GetCondition(extv1.TypeOffered).Status != corev1.ConditionTrue {
				t.Logf("XRD %q is not yet Offered", xrd.GetName())
				return false, nil
			}

			t.Logf("XRD %q is Established and Offered", xrd.GetName())
			return true, nil
		}); err != nil {
			t.Errorf("XRD %q never became Established and Offered: %v", xrd.GetName(), err)
		}
	})

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
									"claimRef": {
										Type:     "object",
										Required: []string{"apiVersion", "kind", "namespace", "name"},
										Properties: map[string]kextv1.JSONSchemaProps{
											"apiVersion": {Type: "string"},
											"kind":       {Type: "string"},
											"namespace":  {Type: "string"},
											"name":       {Type: "string"},
										},
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
													"namespace":  {Type: "string"}, // https://github.com/crossplane/crossplane/commit/403816d2699459d115c505a4685847e1d7274cbb
												},
												Required: []string{"apiVersion", "kind"}, // https://github.com/crossplane/crossplane/commit/6750ee120a75662d952689fc24c801384e96baa5
											},
										},
										XListType: ptr.To("atomic"), // https://github.com/crossplane/crossplane/commit/683f0c5763de698e1fec3eee460ba6fee75379a4
									},
									"writeConnectionSecretToRef": {
										Type:     "object",
										Required: []string{"name", "namespace"},
										Properties: map[string]kextv1.JSONSchemaProps{
											"name":      {Type: "string"},
											"namespace": {Type: "string"},
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
									"connectionDetails": {
										Type: "object",
										Properties: map[string]kextv1.JSONSchemaProps{
											"lastPublishedTime": {Type: "string", Format: "date-time"},
										},
									},
									"claimConditionTypes": { // https://github.com/crossplane/crossplane/commit/0b75611324407889a6064a17cb9058a6f40c2c36
										Type:      "array",
										XListType: ptr.To("set"),
										Items: &kextv1.JSONSchemaPropsOrArray{
											Schema: &kextv1.JSONSchemaProps{
												Type: "string",
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
						JSONPath: ".spec.compositionRef.name",
					},
					{
						// https://github.com/crossplane/crossplane/commit/d1587b4484971e448161e62fb4bf9a8fab76bc40
						Name:     "COMPOSITIONREVISION",
						Type:     "string",
						JSONPath: ".spec.compositionRevisionRef.name",
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

			t.Logf("XRD %q created a conformant XR CRD", xrd.GetName())
			return true, nil
		}); err != nil {
			t.Errorf("CRD %q never created a conformant XR CRD: %v", xrd.GetName(), err)
		}
	})

	t.Run("CRDIsCreatedForXRC", func(t *testing.T) {
		t.Log("Testing that the XRD creates a conformant CRD for its XRC.")

		// TODO(negz): Use Crossplane's internal/xcrd package instead? We can't
		// import it because it's internal, so we replicate a bunch of it here.
		// We could fork it, open it up, or move these tests into the core repo.
		want := kextv1.CustomResourceDefinitionSpec{
			Group: xrd.Spec.Group,
			Names: kextv1.CustomResourceDefinitionNames{
				Kind:       xrd.Spec.ClaimNames.Kind,
				ListKind:   xrd.Spec.ClaimNames.ListKind,
				Plural:     xrd.Spec.ClaimNames.Plural,
				Singular:   xrd.Spec.ClaimNames.Singular,
				Categories: []string{"claim"},
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
									},
									"compositeDeletePolicy": { // https://github.com/crossplane/crossplane/commit/ba4c8a43ab800bde0e39300f0bb8bdf6e8bba889
										Type: "string",
										Enum: []kextv1.JSON{
											{Raw: []byte(`"Background"`)},
											{Raw: []byte(`"Foreground"`)},
										},
										Default: &kextv1.JSON{Raw: []byte(`"Background"`)}, // https://github.com/crossplane/crossplane/commit/ca105476823a688f8235aea5383e626fa1a65721
									},
									"resourceRef": {
										Type:     "object",
										Required: []string{"apiVersion", "kind", "name"},
										Properties: map[string]kextv1.JSONSchemaProps{
											"apiVersion": {Type: "string"},
											"kind":       {Type: "string"},
											"name":       {Type: "string"},
										},
									},
									"writeConnectionSecretToRef": {
										Type:     "object",
										Required: []string{"name"},
										Properties: map[string]kextv1.JSONSchemaProps{
											"name": {Type: "string"},
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
									"connectionDetails": {
										Type: "object",
										Properties: map[string]kextv1.JSONSchemaProps{
											"lastPublishedTime": {Type: "string", Format: "date-time"},
										},
									},
									"claimConditionTypes": { // https://github.com/crossplane/crossplane/commit/0b75611324407889a6064a17cb9058a6f40c2c36
										Type:      "array",
										XListType: ptr.To("set"),
										Items: &kextv1.JSONSchemaPropsOrArray{
											Schema: &kextv1.JSONSchemaProps{
												Type: "string",
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
						Name:     "CONNECTION-SECRET",
						Type:     "string",
						JSONPath: ".spec.writeConnectionSecretToRef.name",
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

		if err := wait.PollUntilContextTimeout(ctx, 10*time.Second, 90*time.Second, true, func(ctx context.Context) (done bool, err error) {
			name := xrd.Spec.ClaimNames.Plural + "." + xrd.Spec.Group
			crd := &kextv1.CustomResourceDefinition{}
			if err := kube.Get(ctx, types.NamespacedName{Name: name}, crd); err != nil {
				if kerrors.IsNotFound(err) {
					t.Logf("CRD %q has not yet been created", name)
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

			t.Logf("XRD %q created a conformant XRC CRD", xrd.GetName())
			return true, nil
		}); err != nil {
			t.Errorf("CRD %q never created a conformant XRC CRD: %v", xrd.GetName(), err)
		}
	})
}

// TestCompositeResourcePipelineModLegacy tests the creation of a legacy XRD and
// Composition that uses a function pipeline to compose resources. The function
// pipeline uses function-patch-and-transform and this test verifies that some
// simple logic run by the function pipeline was successful.
func TestCompositeResourceLegacy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	t.Cleanup(cancel)

	kube, err := internal.NewClient()
	if err != nil {
		t.Fatalf("Create client: %v", err)
	}

	// create provider that will be used in this test
	internal.CreateProvider(ctx, t, kube, "xpkg.crossplane.io/crossplane-contrib/provider-nop:v0.4.0")

	// create and wait for function to be installed/healthy that will be used in this test
	fnc := internal.CreateFunction(ctx, t, kube, "xpkg.crossplane.io/crossplane-contrib/function-patch-and-transform:v0.9.0")

	// create XRD and verify it becomes established/offered
	xrd := createAndTestXRDLegacy(ctx, t, kube)

	// create a composition that will create NopResources and patch/transform
	// using Pipeline mode that exercises functions (specifically
	// function-patch-and-transform in this case).
	createCompositionLegacy(ctx, t, kube, "LegacyClusterConformance", fnc)

	// create a namespace for the claim to live in
	internal.CreateNamespace(ctx, t, kube)

	// create the claim and wait for it to become ready
	xrc := createAndTestClaimLegacy(ctx, t, kube, xrd)

	// test the composite resource, wait for it to become ready
	testXRLegacy(ctx, t, kube, xrd, xrc)
}

// createAndTestXRDLegacy creates an XRD to use in testing, ensures that it becomes
// established, and ensures its clean up.
func createAndTestXRDLegacy(ctx context.Context, t *testing.T, kube client.Client) *extv1.CompositeResourceDefinition {
	t.Helper()
	xrd := &extv1.CompositeResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "legacyclusterconformances.test.crossplane.io"},
		Spec: extv1.CompositeResourceDefinitionSpec{
			Group: "test.crossplane.io",
			Names: kextv1.CustomResourceDefinitionNames{
				Kind:     "LegacyClusterConformance",
				ListKind: "LegacyClusterConformanceList",
				Plural:   "legacyclusterconformances",
				Singular: "legacyclusterconformance",
			},
			ClaimNames: &kextv1.CustomResourceDefinitionNames{
				Kind:     "LegacyConformance",
				ListKind: "LegacyConformanceList",
				Plural:   "legacyconformances",
				Singular: "legacyconformance",
			},
			ConnectionSecretKeys: []string{internal.SuiteName},
			Versions: []extv1.CompositeResourceDefinitionVersion{{
				Name:          "v1alpha1",
				Served:        true,
				Referenceable: true,
				Schema: &extv1.CompositeResourceValidation{
					OpenAPIV3Schema: runtime.RawExtension{Raw: []byte(`{
						"type": "object",
						"properties": {
							"spec": {
								"type": "object",
								"properties": {
									"parameters": {
										"type": "object",
										"properties": {
											"a": {"type": "string"},
											"b": {"type": "integer"},
											"c": {"type": "boolean"}
										}
									}
								},
								"required": ["parameters"]
							},
							"status": {
								"type": "object",
								"properties": {
									"a": {"type": "string"},
									"b": {"type": "integer"},
									"c": {"type": "boolean"}
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

	t.Log("Waiting for the XRD's Established and Offered status conditions to become 'True'.")
	if err := wait.PollUntilContextTimeout(ctx, 10*time.Second, 60*time.Second, true, func(ctx context.Context) (done bool, err error) {
		if err := kube.Get(ctx, types.NamespacedName{Name: xrd.GetName()}, xrd); err != nil {
			return false, err
		}

		if xrd.Status.GetCondition(extv1.TypeEstablished).Status != corev1.ConditionTrue {
			t.Logf("XRD %q is not yet Established", xrd.GetName())
			return false, nil
		}

		if xrd.Status.GetCondition(extv1.TypeOffered).Status != corev1.ConditionTrue {
			t.Logf("XRD %q is not yet Offered", xrd.GetName())
			return false, nil
		}

		t.Logf("XRD %q is Established and Offered", xrd.GetName())
		return true, nil
	}); err != nil {
		t.Errorf("XRD %q never became Established and Offered: %v", xrd.GetName(), err)
	}

	return xrd
}

// createComposition creates a simple function pipeline based composition that
// composes resources using function-patch-and-transform and ensures its clean
// up.
func createCompositionLegacy(ctx context.Context, t *testing.T, kube client.Client, xrdKind string, fnc *pkgv1.Function) {
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
					// the raw input below was obtained by taking the runtime
					// yaml of the composition used in the legacy TestCompositeResource
					// test and running crossplane beta convert
					// pipeline-composition on it, then converting that yaml
					// output to json.
					Input: &runtime.RawExtension{
						Raw: []byte(fmt.Sprintf(`{
							"apiVersion": "pt.fn.crossplane.io/v1beta1",
							"environment": null,
							"kind": "Resources",
							"patchSets": [
								{
								"name": "fromcomposite",
								"patches": [
									{
									"fromFieldPath": "spec.parameters.a",
									"policy": {
										"fromFieldPath": "Required"
									},
									"toFieldPath": "metadata.annotations[nop.crossplane.io/a]",
									"transforms": [
										{
										"map": {
											"a": "A"
										},
										"type": "map"
										}
									],
									"type": "FromCompositeFieldPath"
									},
									{
									"fromFieldPath": "spec.parameters.b",
									"policy": {
										"fromFieldPath": "Required"
									},
									"toFieldPath": "metadata.annotations[nop.crossplane.io/b]",
									"transforms": [
										{
										"math": {
											"multiply": 2,
											"type": "Multiply"
										},
										"type": "math"
										},
										{
										"convert": {
											"toType": "string"
										},
										"type": "convert"
										}
									],
									"type": "FromCompositeFieldPath"
									},
									{
									"fromFieldPath": "spec.parameters.c",
									"policy": {
										"fromFieldPath": "Required"
									},
									"toFieldPath": "metadata.annotations[nop.crossplane.io/c]",
									"transforms": [
										{
										"convert": {
											"toType": "string"
										},
										"type": "convert"
										}
									],
									"type": "FromCompositeFieldPath"
									}
								]
								},
								{
								"name": "tocomposite",
								"patches": [
									{
									"fromFieldPath": "metadata.annotations[nop.crossplane.io/a]",
									"toFieldPath": "status.a",
									"type": "ToCompositeFieldPath"
									},
									{
									"fromFieldPath": "metadata.annotations[nop.crossplane.io/b]",
									"toFieldPath": "status.b",
									"transforms": [
										{
										"convert": {
											"toType": "int"
										},
										"type": "convert"
										}
									],
									"type": "ToCompositeFieldPath"
									},
									{
									"fromFieldPath": "metadata.annotations[nop.crossplane.io/c]",
									"toFieldPath": "status.c",
									"transforms": [
										{
										"convert": {
											"toType": "bool"
										},
										"type": "convert"
										}
									],
									"type": "ToCompositeFieldPath"
									}
								]
								}
							],
							"resources": [
								{
								"base": {
									"apiVersion": "nop.crossplane.io/v1alpha1",
									"kind": "NopResource",
									"spec": {
									"forProvider": {
										"conditionAfter": [
										{
											"conditionStatus": "True",
											"conditionType": "Ready",
											"time": "1s"
										}
										]
									},
									"writeConnectionSecretToRef": {
										"namespace": "%s"
									}
									}
								},
								"connectionDetails": [
									{
									"name": "%s",
									"type": "FromValue",
									"value": "%s"
									}
								],
								"name": "nop",
								"patches": [
									{
									"fromFieldPath": "metadata.uid",
									"toFieldPath": "spec.writeConnectionSecretToRef.name",
									"transforms": [
										{
										"string": {
											"fmt": "%%s-nopresource",
											"type": "Format"
										},
										"type": "string"
										}
									],
									"type": "FromCompositeFieldPath"
									},
									{
									"patchSetName": "fromcomposite",
									"type": "PatchSet"
									},
									{
									"patchSetName": "tocomposite",
									"type": "PatchSet"
									}
								],
								"readinessChecks": [
									{
									"matchCondition": {
										"status": "True",
										"type": "Ready"
									},
									"type": "MatchCondition"
									}
								]
								}
							]
						}`, internal.SuiteName, internal.SuiteName, internal.SuiteName)),
					},
				},
			},
			WriteConnectionSecretsToNamespace: ptr.To(internal.SuiteName),
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

// createAndTestClaimLegacy creates a claim for the given XRD type, ensures it becomes
// ready, and verifies the results of the composition pipeline. This function
// ensures the claim is cleaned up.
func createAndTestClaimLegacy(ctx context.Context, t *testing.T, kube client.Client, xrd *extv1.CompositeResourceDefinition) *claim.Unstructured {
	t.Helper()
	xrc := claim.New(claim.WithGroupVersionKind(schema.GroupVersionKind{
		Group:   xrd.Spec.Group,
		Version: xrd.Spec.Versions[0].Name,
		Kind:    xrd.Spec.ClaimNames.Kind,
	}))

	xrc.SetNamespace(internal.SuiteName)
	xrc.SetName(internal.SuiteName)
	xrc.SetCompositionSelector(&metav1.LabelSelector{MatchLabels: map[string]string{"crossplane.io/test": internal.SuiteName}})
	xrc.SetWriteConnectionSecretToReference(&xpv1.LocalSecretReference{Name: internal.SuiteName})

	paved := fieldpath.Pave(xrc.Object)
	_ = paved.SetString("spec.parameters.a", "a")
	_ = paved.SetNumber("spec.parameters.b", 1)
	_ = paved.SetBool("spec.parameters.c", true)

	if err := kube.Create(ctx, xrc); err != nil {
		t.Fatalf("Create claim %q: %v", xrc.GetName(), err)
	}
	t.Logf("Created claim %q", xrc.GetName())

	t.Cleanup(func() {
		t.Logf("Cleaning up claim %q.", xrc.GetName())
		if err := kube.Get(ctx, types.NamespacedName{Namespace: xrc.GetNamespace(), Name: xrc.GetName()}, xrc); resource.IgnoreNotFound(err) != nil {
			t.Fatalf("Get claim %q: %v", xrc.GetName(), err)
		}
		if err := kube.Delete(ctx, xrc); resource.IgnoreNotFound(err) != nil {
			t.Fatalf("Delete claim %q: %v", xrc.GetName(), err)
		}
		t.Logf("Deleted claim %q", xrc.GetName())
	})

	t.Run("ClaimBecomesReady", func(t *testing.T) {
		// Becoming Ready implies the claim selects a composition and creates an
		// XR that successfully composes resources.
		t.Log("Testing that the claim becomes Ready.")
		if err := wait.PollUntilContextTimeout(ctx, 15*time.Second, 2*time.Minute, true, func(ctx context.Context) (done bool, err error) {
			if err := kube.Get(ctx, types.NamespacedName{Namespace: xrc.GetNamespace(), Name: xrc.GetName()}, xrc); err != nil {
				return false, err
			}

			if xrc.GetCondition(xpv1.TypeReady).Status != corev1.ConditionTrue {
				t.Logf("Claim %q is not yet Ready", xrc.GetName())
				return false, nil
			}

			if xrc.GetResourceReference() == nil {
				t.Logf("Claim %q did not populate its resource reference", xrc.GetName())
				return false, nil
			}

			if xrc.GetCompositionReference() == nil {
				t.Logf("Claim %q did not populate its composition reference", xrc.GetName())
				return false, nil
			}

			paved := fieldpath.Pave(xrc.Object)
			if a, _ := paved.GetString("status.a"); a != "A" {
				t.Logf("Claim %q status.a: want %q, got %q", xrc.GetName(), "A", a)
				return false, nil
			}
			if b, _ := paved.GetInteger("status.b"); b != 2 {
				t.Logf("Claim %q status.b: want %d, got %d", xrc.GetName(), 2, b)
				return false, nil
			}
			if c, _ := paved.GetBool("status.c"); c != true {
				t.Logf("Claim %q status.c: want %t, got %t", xrc.GetName(), true, c)
				return false, nil
			}

			t.Logf("Claim %q is Ready", xrc.GetName())
			return true, nil
		}); err != nil {
			t.Errorf("Claim %q never became Ready: %v", xrc.GetName(), err)
		}
	})

	t.Run("ClaimHasConnectionSecret", func(t *testing.T) {
		t.Log("Testing that the claim writes the expected connection secret.")

		s := &corev1.Secret{}
		if err := kube.Get(ctx, types.NamespacedName{Namespace: xrc.GetNamespace(), Name: internal.SuiteName}, s); err != nil {
			t.Errorf("Get secret %q: %v", internal.SuiteName, err)
		}

		want := map[string][]byte{internal.SuiteName: []byte(internal.SuiteName)}
		if diff := cmp.Diff(want, s.Data); diff != "" {
			t.Errorf("Claim %q connection secret %q: -want, +got\n%s", xrc.GetName(), s.GetName(), diff)
		}
	})

	return xrc
}

// testXRLegacy tests that a legacy XR for the given claim exists and becomes ready and that
// it has an expected connection secret.
func testXRLegacy(ctx context.Context, t *testing.T, kube client.Client, xrd *extv1.CompositeResourceDefinition, xrc *claim.Unstructured) {
	t.Helper()
	xrSecretRef := &xpv1.SecretReference{}
	t.Run("CompositeBecomesReady", func(t *testing.T) {
		t.Log("Testing that the composite resource becomes Ready.")

		xr := composite.New(
			composite.WithGroupVersionKind(schema.GroupVersionKind{
				Group:   xrd.Spec.Group,
				Version: xrd.Spec.Versions[0].Name,
				Kind:    xrd.Spec.Names.Kind,
			}),
			composite.WithSchema(composite.SchemaLegacy),
		)

		if err := wait.PollUntilContextTimeout(ctx, 15*time.Second, 2*time.Minute, true, func(ctx context.Context) (done bool, err error) {
			if err := kube.Get(ctx, types.NamespacedName{Name: xrc.GetResourceReference().Name}, xr); err != nil {
				return false, err
			}

			if xr.GetCondition(xpv1.TypeReady).Status != corev1.ConditionTrue {
				t.Logf("Composite resource %q is not yet Ready", xr.GetName())
				return false, nil
			}

			xrSecretRef = xr.GetWriteConnectionSecretToReference()
			if xrSecretRef == nil {
				t.Logf("Composite resource %q does not specify a connection secret", xr.GetName())
				return false, nil
			}

			t.Logf("Composite resource %q is Ready", xr.GetName())
			return true, nil
		}); err != nil {
			t.Errorf("Composite resource %q never became Ready: %v", xr.GetName(), err)
		}
	})
	t.Run("CompositeHasConnectionSecret", func(t *testing.T) {
		t.Log("Testing that the composite resource writes the expected connection secret.")

		s := &corev1.Secret{}
		if err := kube.Get(ctx, types.NamespacedName{Namespace: xrSecretRef.Namespace, Name: xrSecretRef.Name}, s); err != nil {
			t.Errorf("Get secret %q: %v", internal.SuiteName, err)
		}

		want := map[string][]byte{internal.SuiteName: []byte(internal.SuiteName)}
		if diff := cmp.Diff(want, s.Data); diff != "" {
			t.Errorf("Composite resource %q connection secret %q: -want, +got\n%s", xrc.GetResourceReference().Name, s.GetName(), diff)
		}
	})
}

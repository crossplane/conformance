// Copyright 2021 The Crossplane Authors
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
	"k8s.io/utils/pointer"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/resource/unstructured/claim"
	"github.com/crossplane/crossplane-runtime/pkg/resource/unstructured/composite"
	extv1 "github.com/crossplane/crossplane/apis/apiextensions/v1"
	pkgv1 "github.com/crossplane/crossplane/apis/pkg/v1"

	"github.com/crossplane/conformance/internal"
)

func TestCompositeResourceDefinition(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)

	kube, err := internal.NewClient()
	if err != nil {
		t.Fatalf("Create client: %v", err)
	}

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
		if err := wait.PollImmediate(10*time.Second, 90*time.Second, func() (done bool, err error) {
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
				Schema: &kextv1.CustomResourceValidation{OpenAPIV3Schema: &kextv1.JSONSchemaProps{
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
											},
											Required: []string{"apiVersion", "kind", "name"},
										},
									},
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
									Items: &kextv1.JSONSchemaPropsOrArray{
										Schema: &kextv1.JSONSchemaProps{
											Type:     "object",
											Required: []string{"lastTransitionTime", "reason", "status", "type"},
											Properties: map[string]kextv1.JSONSchemaProps{
												"lastTransitionTime": {Type: "string", Format: "date-time"},
												"message":            {Type: "string"},
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
							},
						},
					},
				},
				},
				AdditionalPrinterColumns: []kextv1.CustomResourceColumnDefinition{
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
						Name:     "AGE",
						Type:     "date",
						JSONPath: ".metadata.creationTimestamp",
					},
				},
			}},
			Conversion: &kextv1.CustomResourceConversion{Strategy: kextv1.NoneConverter},
		}

		if err := wait.PollImmediate(10*time.Second, 90*time.Second, func() (done bool, err error) {
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

			if diff := cmp.Diff(crd.Spec, want); diff != "" {
				t.Errorf("CRD %q is not conformant: -want, +got:\n%s", crd.GetName(), diff)
				return true, nil
			}

			t.Logf("XRD %q created a conformant XR CRD", xrd.GetName())
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
				Schema: &kextv1.CustomResourceValidation{OpenAPIV3Schema: &kextv1.JSONSchemaProps{
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
									Items: &kextv1.JSONSchemaPropsOrArray{
										Schema: &kextv1.JSONSchemaProps{
											Type:     "object",
											Required: []string{"lastTransitionTime", "reason", "status", "type"},
											Properties: map[string]kextv1.JSONSchemaProps{
												"lastTransitionTime": {Type: "string", Format: "date-time"},
												"message":            {Type: "string"},
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
							},
						},
					},
				},
				},
				AdditionalPrinterColumns: []kextv1.CustomResourceColumnDefinition{
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

		if err := wait.PollImmediate(10*time.Second, 90*time.Second, func() (done bool, err error) {
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

			if diff := cmp.Diff(crd.Spec, want); diff != "" {
				t.Errorf("CRD %q is not conformant: -want, +got:\n%s", crd.GetName(), diff)
				return true, nil
			}

			t.Logf("XRD %q created a conformant XR CRD", xrd.GetName())
			return true, nil
		}); err != nil {
			t.Errorf("CRD %q never created a conformant XR CRD: %v", xrd.GetName(), err)
		}
	})
}

func TestCompositeResource(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)

	kube, err := internal.NewClient()
	if err != nil {
		t.Fatalf("Create client: %v", err)
	}

	// TODO(negz): Use the other provider-nop from contrib once it's ready.
	// https://github.com/crossplane-contrib/provider-nop
	prv := &pkgv1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: internal.SuiteName},
		Spec: pkgv1.ProviderSpec{
			PackageSpec: pkgv1.PackageSpec{
				Package:                     "negz/provider-nop:v0.1.0",
				IgnoreCrossplaneConstraints: pointer.BoolPtr(true),
			},
		},
	}

	if err := kube.Create(ctx, prv); err != nil {
		t.Fatalf("Create provider %q: %v", prv.GetName(), err)
	}
	t.Logf("Created provider %q", prv.GetName())

	t.Cleanup(func() {
		t.Logf("Cleaning up provider %q.", prv.GetName())
		if err := kube.Get(ctx, types.NamespacedName{Name: prv.GetName()}, prv); resource.IgnoreNotFound(err) != nil {
			t.Fatalf("Get provider %q: %v", prv.GetName(), err)
		}
		if err := kube.Delete(ctx, prv); resource.IgnoreNotFound(err) != nil {
			t.Fatalf("Delete provider %q: %v", prv.GetName(), err)
		}
		t.Logf("Deleted provider %q", prv.GetName())
	})

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
	if err := wait.PollImmediate(10*time.Second, 90*time.Second, func() (done bool, err error) {
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
	if err := wait.PollImmediate(10*time.Second, 90*time.Second, func() (done bool, err error) {
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
				Kind:       "ClusterConformance",
			},
			PatchSets: []extv1.PatchSet{
				{
					// We patch our three spec fields (a string, int, and bool)
					// as (string) annotations on the NopResource, exercising a
					// few of our transforms.
					Name: "fromcomposite",
					Patches: []extv1.Patch{
						{
							Type:          extv1.PatchTypeFromCompositeFieldPath,
							FromFieldPath: pointer.StringPtr("spec.parameters.a"),
							ToFieldPath:   pointer.StringPtr("metadata.annotations[nop.crossplane.io/a]"),
							Transforms: []extv1.Transform{{
								Type: extv1.TransformTypeMap,
								Map:  &extv1.MapTransform{Pairs: map[string]string{"a": "A"}},
							}},
							Policy: &extv1.PatchPolicy{
								FromFieldPath: func() *extv1.FromFieldPathPolicy { p := extv1.FromFieldPathPolicyRequired; return &p }(),
							},
						},
						{
							Type:          extv1.PatchTypeFromCompositeFieldPath,
							FromFieldPath: pointer.StringPtr("spec.parameters.b"),
							ToFieldPath:   pointer.StringPtr("metadata.annotations[nop.crossplane.io/b]"),
							Transforms: []extv1.Transform{
								{
									Type: extv1.TransformTypeMath,
									Math: &extv1.MathTransform{
										Multiply: pointer.Int64Ptr(2),
									},
								},
								{
									Type:    extv1.TransformTypeConvert,
									Convert: &extv1.ConvertTransform{ToType: "string"},
								},
							},
							Policy: &extv1.PatchPolicy{
								FromFieldPath: func() *extv1.FromFieldPathPolicy { p := extv1.FromFieldPathPolicyRequired; return &p }(),
							},
						},
						{
							Type:          extv1.PatchTypeFromCompositeFieldPath,
							FromFieldPath: pointer.StringPtr("spec.parameters.c"),
							ToFieldPath:   pointer.StringPtr("metadata.annotations[nop.crossplane.io/c]"),
							Transforms: []extv1.Transform{{
								Type:    extv1.TransformTypeConvert,
								Convert: &extv1.ConvertTransform{ToType: "string"},
							}},
							Policy: &extv1.PatchPolicy{
								FromFieldPath: func() *extv1.FromFieldPathPolicy { p := extv1.FromFieldPathPolicyRequired; return &p }(),
							},
						},
					},
				},
				{
					// We patch the values we patched from the composite's spec
					// to its status, to ensure patching works in both
					// directions.
					Name: "tocomposite",
					Patches: []extv1.Patch{
						{
							Type:          extv1.PatchTypeToCompositeFieldPath,
							FromFieldPath: pointer.StringPtr("metadata.annotations[nop.crossplane.io/a]"),
							ToFieldPath:   pointer.StringPtr("status.a"),
						},
						{
							Type:          extv1.PatchTypeToCompositeFieldPath,
							FromFieldPath: pointer.StringPtr("metadata.annotations[nop.crossplane.io/b]"),
							ToFieldPath:   pointer.StringPtr("status.b"),
							Transforms: []extv1.Transform{
								{
									Type:    extv1.TransformTypeConvert,
									Convert: &extv1.ConvertTransform{ToType: "int"},
								},
							},
						},
						{
							Type:          extv1.PatchTypeToCompositeFieldPath,
							FromFieldPath: pointer.StringPtr("metadata.annotations[nop.crossplane.io/c]"),
							ToFieldPath:   pointer.StringPtr("status.c"),
							Transforms: []extv1.Transform{{
								Type:    extv1.TransformTypeConvert,
								Convert: &extv1.ConvertTransform{ToType: "bool"},
							}},
						},
					},
				},
			},
			Resources: []extv1.ComposedTemplate{{
				// TODO(negz): Test anonymous resources too?
				Name: pointer.StringPtr("nop"),
				Base: runtime.RawExtension{Raw: []byte(fmt.Sprintf(`{
					"apiVersion": "nop.crossplane.io/v1alpha1",
					"kind": "NopResource",
					"spec": {
						"writeConnectionSecretToRef": {
							"namespace": "%s"
						}
					}
				}`, internal.SuiteName))},
				Patches: []extv1.Patch{
					{
						Type:          extv1.PatchTypeFromCompositeFieldPath,
						FromFieldPath: pointer.StringPtr("metadata.uid"),
						ToFieldPath:   pointer.StringPtr("spec.writeConnectionSecretToRef.name"),
						// TODO(negz): Test all transform types.
						Transforms: []extv1.Transform{
							{
								Type: extv1.TransformTypeString,
								String: &extv1.StringTransform{
									Format: "%s-nopresource",
								},
							},
						},
					},
					{
						Type:         extv1.PatchTypePatchSet,
						PatchSetName: pointer.StringPtr("fromcomposite"),
					},
					{
						Type:         extv1.PatchTypePatchSet,
						PatchSetName: pointer.StringPtr("tocomposite"),
					},
				},
				ConnectionDetails: []extv1.ConnectionDetail{{
					Type:  func() *extv1.ConnectionDetailType { v := extv1.ConnectionDetailTypeFromValue; return &v }(),
					Name:  pointer.StringPtr(internal.SuiteName),
					Value: pointer.StringPtr(internal.SuiteName),
				}},
				// TODO(negz): Test the various kinds of readiness check?
			}},
			WriteConnectionSecretsToNamespace: pointer.StringPtr(internal.SuiteName),
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

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: internal.SuiteName}}
	if err := kube.Create(ctx, ns); err != nil {
		t.Fatalf("Create namespace %q: %v", ns.GetName(), err)
	}
	t.Logf("Created namespace %q", ns.GetName())

	t.Cleanup(func() {
		t.Logf("Cleaning up namespace %q.", ns.GetName())
		if err := kube.Get(ctx, types.NamespacedName{Name: ns.GetName()}, ns); resource.IgnoreNotFound(err) != nil {
			t.Fatalf("Get namespace %q: %v", ns.GetName(), err)
		}
		if err := kube.Delete(ctx, ns); resource.IgnoreNotFound(err) != nil {
			t.Fatalf("Delete namespace %q: %v", ns.GetName(), err)
		}
		t.Logf("Deleted namespace %q", ns.GetName())
	})

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
		if err := wait.PollImmediate(15*time.Second, 3*time.Minute, func() (done bool, err error) {
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

			// TODO(negz): Ensure the claim's compositionRef is set once
			// https://github.com/crossplane/crossplane/issues/2263 is fixed.

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

	xrSecretRef := &xpv1.SecretReference{}
	t.Run("CompositeBecomesReady", func(t *testing.T) {
		t.Log("Testing that the composite resource becomes Ready.")

		xr := composite.New(composite.WithGroupVersionKind(schema.GroupVersionKind{
			Group:   xrd.Spec.Group,
			Version: xrd.Spec.Versions[0].Name,
			Kind:    xrd.Spec.Names.Kind,
		}))

		if err := wait.PollImmediate(15*time.Second, 3*time.Minute, func() (done bool, err error) {
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

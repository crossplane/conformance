package crossplane

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	kextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/google/go-cmp/cmp"

	extv1 "github.com/crossplane/crossplane/apis/apiextensions/v1"

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
		t.Parallel()
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
		t.Parallel()
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

			if !CRDIs(crd.Status.Conditions, kextv1.Established) {
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
		t.Parallel()
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

			if !CRDIs(crd.Status.Conditions, kextv1.Established) {
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

// CRDIs returns true if the supplied conditions array contains a condition of
// the supplied type that is true.
func CRDIs(cs []kextv1.CustomResourceDefinitionCondition, t kextv1.CustomResourceDefinitionConditionType) bool {
	for _, c := range cs {
		if c.Type != t {
			continue
		}
		if c.Status == kextv1.ConditionTrue {
			return true
		}
	}
	return false
}

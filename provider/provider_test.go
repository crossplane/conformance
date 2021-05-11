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

package provider

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	corev1 "k8s.io/api/core/v1"
	kextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	pkgv1 "github.com/crossplane/crossplane/apis/pkg/v1"

	"github.com/crossplane/conformance/internal"
)

func TestPackage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	t.Cleanup(cancel)

	kube, err := internal.NewClient()
	if err != nil {
		t.Fatalf("Create client: %v", err)
	}

	l := &pkgv1.ProviderList{}
	if err := kube.List(ctx, l); err != nil {
		t.Fatalf("List providers: %v", err)
	}

	rl := &pkgv1.ProviderRevisionList{}
	if err := kube.List(ctx, rl); err != nil {
		t.Fatalf("List provider revisions: %v", err)
	}

	if len(l.Items) != 1 {
		t.Fatalf("Provider conformance test requires exactly 1 Provider, found %d", len(l.Items))
	}

	if len(rl.Items) != 1 {
		t.Fatalf("Provider conformance test requires exactly 1 ProviderRevision, found %d", len(rl.Items))
	}

	prv := &l.Items[0]
	rev := &rl.Items[0]

	t.Run("IsInstalledAndHealthy", func(t *testing.T) {
		t.Log("Testing that the provider's Healthy and Installed status conditions are 'True'.")
		if err := wait.PollImmediate(10*time.Second, 90*time.Second, func() (done bool, err error) {
			if err := kube.Get(ctx, types.NamespacedName{Name: prv.GetName()}, prv); err != nil {
				return false, err
			}

			if prv.GetCondition(pkgv1.TypeHealthy).Status != corev1.ConditionTrue {
				t.Logf("Provider %q is not yet Healthy", prv.GetName())
				return false, nil
			}

			if prv.GetCondition(pkgv1.TypeInstalled).Status != corev1.ConditionTrue {
				t.Logf("Provider %q is not yet Installed", prv.GetName())
				return false, nil
			}

			t.Logf("Provider %q is Healthy and Installed", prv.GetName())
			return true, nil
		}); err != nil {
			t.Errorf("Provider %q never became Healthy and Installed: %v", prv.GetName(), err)
		}
	})

	t.Run("DeploysConformantCRDs", func(t *testing.T) {
		t.Logf("Testing that ProviderRevision %q delivers only conformant CRDs.", rev.GetName())

		hasPC := false
		hasPCU := false
		hasMR := false

		want := kextv1.SchemeGroupVersion.WithKind("CustomResourceDefinition")
		for i, ref := range rev.Status.ObjectRefs {
			gv, _ := schema.ParseGroupVersion(ref.APIVersion)
			got := gv.WithKind(ref.Kind)
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("ProviderRevision %q status.objectRefs[%d]: -want, +got:\n%s", rev.GetName(), i, diff)
				continue
			}

			crd := &kextv1.CustomResourceDefinition{}
			if err := kube.Get(ctx, types.NamespacedName{Name: ref.Name}, crd); err != nil {
				t.Errorf("Get CRD %q: %v", ref.Name, err)
				continue
			}

			switch crd.Spec.Names.Kind {
			case "Provider":
				// Deprecated, but still exists in some providers.
				continue
			case "ProviderConfig":
				t.Run(crd.Spec.Names.Kind, SubtestForProviderConfig(crd))
				hasPC = true
			case "ProviderConfigUsage":
				t.Run(crd.Spec.Names.Kind, SubtestForProviderConfigUsage(crd))
				hasPCU = true
			default:
				t.Run(crd.Spec.Names.Kind, SubtestForManagedResource(crd))
				hasMR = true
			}
		}

		if !hasMR {
			t.Errorf("ProviderRevision %q must define at least one conformant managed resource", rev.GetName())
		}

		if !hasPC {
			t.Errorf("ProviderRevision %q must define exactly one conformant provider config", rev.GetName())
		}

		if !hasPCU {
			t.Errorf("ProviderRevision %q must define exactly one conformant provider config usage", rev.GetName())
		}
	})
}

func SubtestForProviderConfig(crd *kextv1.CustomResourceDefinition) func(t *testing.T) {
	return func(t *testing.T) {
		if crd.Spec.Scope != kextv1.ClusterScoped {
			t.Error("provider configs must be cluster scoped")
		}

		cats := internal.AsSet(crd.Spec.Names.Categories)
		if !cats["crossplane"] {
			t.Error("provider configs must be in the 'crossplane' CRD category")
		}

		// We're not opinionated about ProviderConfig specs, so we mostly test that
		// their status object appears to embed our ProviderConfigStatus type.
		// https://github.com/crossplane/crossplane-runtime/blob/v0.13.0/apis/common/v1/resource.go#L223
		want := &kextv1.JSONSchemaProps{
			Type:     "object",
			Required: []string{"spec"},
			Properties: map[string]kextv1.JSONSchemaProps{
				"apiVersion": {Type: "string"},
				"kind":       {Type: "string"},
				"metadata":   {Type: "object"},
				"spec": {
					Type: "object",
				},
				"status": {
					Type: "object",
					Properties: map[string]kextv1.JSONSchemaProps{
						"conditions": {
							Type: "array",
							Items: &kextv1.JSONSchemaPropsOrArray{Schema: &kextv1.JSONSchemaProps{
								Type:     "object",
								Required: []string{"lastTransitionTime", "reason", "status", "type"},
								Properties: map[string]kextv1.JSONSchemaProps{
									"lastTransitionTime": {Type: "string", Format: "date-time"},
									"message":            {Type: "string"},
									"reason":             {Type: "string"},
									"status":             {Type: "string"},
									"type":               {Type: "string"},
								},
							}},
						},
						"users": {Type: "integer", Format: "int64"},
					},
				},
			},
		}

		opts := []cmp.Option{
			// Testing description strings is verbose, fragile, and questionably
			// valuable. We're not too concerned about them.
			cmpopts.IgnoreFields(kextv1.JSONSchemaProps{}, "Description"),

			// We're not opinionated about the schema of a ProviderConfig's spec.
			internal.IgnoreFieldsOfMapKey("spec", "Properties", "Required"),
		}

		served := false
		for _, v := range crd.Spec.Versions {
			if v.Served == false {
				continue
			}
			served = true

			if diff := cmp.Diff(want, v.Schema.OpenAPIV3Schema, opts...); diff != "" {
				t.Errorf("version %q OpenAPI schema: -want, +got:\n%s", v.Name, diff)
			}
		}

		if !served {
			t.Error("CRDs must serve at least one custom resource version")
		}
	}
}

func SubtestForProviderConfigUsage(crd *kextv1.CustomResourceDefinition) func(t *testing.T) {
	return func(t *testing.T) {
		if crd.Spec.Scope != kextv1.ClusterScoped {
			t.Error("provider config usages must be cluster scoped")
		}

		cats := internal.AsSet(crd.Spec.Names.Categories)
		if !cats["crossplane"] {
			t.Error("provider config usages must be in the 'crossplane' CRD category")
		}

		// We're quite opinionated about ProviderConfigUsage schemas, so we simply
		// test that they appear to embed our ProviderConfigUsage type.
		// https://github.com/crossplane/crossplane-runtime/blob/v0.13.0/apis/common/v1/resource.go#L231
		want := &kextv1.JSONSchemaProps{
			Type:     "object",
			Required: []string{"providerConfigRef", "resourceRef"},
			Properties: map[string]kextv1.JSONSchemaProps{
				"apiVersion": {Type: "string"},
				"kind":       {Type: "string"},
				"metadata":   {Type: "object"},
				"providerConfigRef": {
					Type:     "object",
					Required: []string{"name"},
					Properties: map[string]kextv1.JSONSchemaProps{
						"name": {Type: "string"},
					},
				},
				"resourceRef": {
					Type:     "object",
					Required: []string{"apiVersion", "kind", "name"},
					Properties: map[string]kextv1.JSONSchemaProps{
						"apiVersion": {Type: "string"},
						"kind":       {Type: "string"},
						"name":       {Type: "string"},
						"uid":        {Type: "string"},
					},
				},
			},
		}
		opts := []cmp.Option{
			// Testing description strings is verbose, fragile, and questionably
			// valuable. We're not too concerned about them.
			cmpopts.IgnoreFields(kextv1.JSONSchemaProps{}, "Description"),
		}

		served := false
		for _, v := range crd.Spec.Versions {
			if v.Served == false {
				continue
			}
			served = true

			if diff := cmp.Diff(want, v.Schema.OpenAPIV3Schema, opts...); diff != "" {
				t.Errorf("version %q OpenAPI schema: -want, +got:\n%s", v.Name, diff)
			}
		}

		if !served {
			t.Error("CRDs must serve at least one custom resource version")
		}
	}
}

func SubtestForManagedResource(crd *kextv1.CustomResourceDefinition) func(t *testing.T) {
	return func(t *testing.T) {
		if crd.Spec.Scope != kextv1.ClusterScoped {
			t.Error("managed resources must be cluster scoped")
		}

		cats := internal.AsSet(crd.Spec.Names.Categories)
		if !cats["crossplane"] {
			t.Error("managed resources must be in the 'crossplane' CRD category")
		}

		if !cats["managed"] {
			t.Error("managed resources must be in the 'managed' CRD category")
		}

		// We're only concerned that managed resources appear to embed the expected
		// types in their spec and status.
		// https://github.com/crossplane/crossplane-runtime/blob/v0.13.0/apis/common/v1/resource.go#L130
		// https://github.com/crossplane/crossplane-runtime/blob/v0.13.0/apis/common/v1/resource.go#L159
		want := &kextv1.JSONSchemaProps{
			Type:     "object",
			Required: []string{"spec"},
			Properties: map[string]kextv1.JSONSchemaProps{
				"apiVersion": {Type: "string"},
				"kind":       {Type: "string"},
				"metadata":   {Type: "object"},
				"spec": {
					Type: "object",
					Properties: map[string]kextv1.JSONSchemaProps{
						"deletionPolicy": {
							Type: "string",
							Enum: []kextv1.JSON{
								{Raw: []byte(`"Orphan"`)},
								{Raw: []byte(`"Delete"`)},
							},
						},
						// TODO(negz): Ensure that 'name' defaults to 'default'
						// once we expect providers to be built against
						// crossplane-runtime v0.14+, which will includ3
						// https://github.com/crossplane/crossplane-runtime/pull/255
						"providerConfigRef": {
							Type:       "object",
							Required:   []string{"name"},
							Properties: map[string]kextv1.JSONSchemaProps{"name": {Type: "string"}},
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
							Type: "array",
							Items: &kextv1.JSONSchemaPropsOrArray{Schema: &kextv1.JSONSchemaProps{
								Type:     "object",
								Required: []string{"lastTransitionTime", "reason", "status", "type"},
								Properties: map[string]kextv1.JSONSchemaProps{
									"lastTransitionTime": {Type: "string", Format: "date-time"},
									"message":            {Type: "string"},
									"reason":             {Type: "string"},
									"status":             {Type: "string"},
									"type":               {Type: "string"},
								},
							}},
						},
					},
				},
			},
		}

		opts := []cmp.Option{
			// Testing description strings is verbose, fragile, and questionably
			// valuable. We're not too concerned about them.
			cmpopts.IgnoreFields(kextv1.JSONSchemaProps{}, "Description"),

			// We're not concerned with which spec fields are required. None of the
			// spec fields we're concerned with are, but fields like 'forProvider'
			// often are.
			internal.IgnoreFieldsOfMapKey("spec", "Required"),

			// We're only concerned with the spec and status fields that we expect
			// all managed resources to include.
			internal.OnlySubproperties("spec", "deletionPolicy", "providerConfigRef", "writeConnectionSecretToRef"),
			internal.OnlySubproperties("status", "conditions"),
		}

		served := false
		for _, v := range crd.Spec.Versions {
			if v.Served == false {
				continue
			}
			served = true

			if diff := cmp.Diff(want, v.Schema.OpenAPIV3Schema, opts...); diff != "" {
				t.Errorf("version %q OpenAPI schema: -want, +got:\n%s", v.Name, diff)
			}
		}

		if !served {
			t.Error("CRDs must serve at least one custom resource version")
		}
	}
}

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
	"testing"
	"time"

	kextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/crossplane/conformance/internal"
)

func TestCustomResourceDefinitions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	t.Cleanup(cancel)

	kube, err := internal.NewClient()
	if err != nil {
		t.Fatalf("Create client: %v", err)
	}

	t.Log("Testing that all core Crossplane CRDs exist and are well formed.")

	wantCrds := []wantCRD{
		{crd: kextv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "configurations.pkg.crossplane.io"}}},
		{crd: kextv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "configurationrevisions.pkg.crossplane.io"}}},

		{crd: kextv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "providers.pkg.crossplane.io"}}},
		{crd: kextv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "providerrevisions.pkg.crossplane.io"}}},

		{crd: kextv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "functions.pkg.crossplane.io"}}},
		{crd: kextv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "functionrevisions.pkg.crossplane.io"}}},

		{crd: kextv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "deploymentruntimeconfigs.pkg.crossplane.io"}}},
		{crd: kextv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "imageconfigs.pkg.crossplane.io"}}},

		{crd: kextv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "compositeresourcedefinitions.apiextensions.crossplane.io"}}},
		{crd: kextv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "compositions.apiextensions.crossplane.io"}}},
		{crd: kextv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "compositionrevisions.apiextensions.crossplane.io"}}},

		{crd: kextv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "environmentconfigs.apiextensions.crossplane.io"}}},

		{crd: kextv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "managedresourcedefinitions.apiextensions.crossplane.io"}}},
		{crd: kextv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "managedresourceactivationpolicies.apiextensions.crossplane.io"}}},

		{crd: kextv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "usages.protection.crossplane.io"}}, namespaced: true},
		{crd: kextv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "clusterusages.protection.crossplane.io"}}},
		{crd: kextv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "usages.apiextensions.crossplane.io"}}},

		{crd: kextv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "operations.ops.crossplane.io"}}},
		{crd: kextv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "cronoperations.ops.crossplane.io"}}},
		{crd: kextv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "watchoperations.ops.crossplane.io"}}},
	}

	for _, w := range wantCrds {
		crd := w.crd
		if err := kube.Get(ctx, types.NamespacedName{Name: crd.GetName()}, &crd); err != nil {
			t.Errorf("Cannot get CRD %q: %v", crd.GetName(), err)
			continue
		}

		// TODO(negz): Just cmp.Diff the entire CRD spec?
		switch w.namespaced {
		case false:
			if crd.Spec.Scope != kextv1.ClusterScoped {
				t.Errorf("CRD %q should define a cluster scoped resource", crd.GetName())
			}

		case true:
			if crd.Spec.Scope != kextv1.NamespaceScoped {
				t.Errorf("CRD %q should define a namespaced scoped resource", crd.GetName())
			}
		}

		cats := internal.AsSet(crd.Spec.Names.Categories)
		if !cats["crossplane"] {
			t.Errorf("CRD %q must be in category 'crossplane'", crd.GetName())
		}
	}
}

type wantCRD struct {
	crd        kextv1.CustomResourceDefinition
	namespaced bool
}

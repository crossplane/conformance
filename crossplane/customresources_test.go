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
	cfg := kextv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "configurations.pkg.crossplane.io"}}
	cfgRev := kextv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "configurationrevisions.pkg.crossplane.io"}}

	prv := kextv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "providers.pkg.crossplane.io"}}
	prvRev := kextv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "providerrevisions.pkg.crossplane.io"}}

	xrd := kextv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "compositeresourcedefinitions.apiextensions.crossplane.io"}}
	cmp := kextv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "compositions.apiextensions.crossplane.io"}}

	for _, crd := range []kextv1.CustomResourceDefinition{cfg, cfgRev, prv, prvRev, xrd, cmp} {
		crd := crd // Don't take the address of a range var.
		if err := kube.Get(ctx, types.NamespacedName{Name: crd.GetName()}, &crd); err != nil {
			t.Errorf("Cannot get CRD %q: %v", crd.GetName(), err)
			continue
		}

		// TODO(negz): Just cmp.Diff the entire CRD spec?
		if crd.Spec.Scope != kextv1.ClusterScoped {
			t.Errorf("CRD %q must define a cluster scoped resource", crd.GetName())
		}

		cats := internal.AsSet(crd.Spec.Names.Categories)
		if !cats["crossplane"] {
			t.Errorf("CRD %q must be in category 'crossplane'", crd.GetName())
		}
	}
}

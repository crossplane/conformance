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

// Package internal contains internal helper functions used by the conformance tests.
package internal

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kunstructured "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/v2/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource/unstructured"
	extv1 "github.com/crossplane/crossplane/v2/apis/apiextensions/v1"
	extv1alpha1 "github.com/crossplane/crossplane/v2/apis/apiextensions/v1alpha1"
	extv1beta1 "github.com/crossplane/crossplane/v2/apis/apiextensions/v1beta1"
	extv2 "github.com/crossplane/crossplane/v2/apis/apiextensions/v2"
	opsv1alpha1 "github.com/crossplane/crossplane/v2/apis/ops/v1alpha1"
	pkgv1 "github.com/crossplane/crossplane/v2/apis/pkg/v1"
	protectionv1beta1 "github.com/crossplane/crossplane/v2/apis/protection/v1beta1"
)

// SuiteName of the conformance test suite.
const SuiteName = "crossplane-conformance"

// Version of the conformance plugin.
var version = "unknown"

// Version returns the version of the conformance plugin.
func Version() string {
	return version
}

// NewClient returns a Kubernetes API client suitable for use with conformance
// tests. It uses controller-runtime's config loading precedence to figure out
// how to connect to an API server, i.e. it will try in order:
//
// 1. The KUBECONFIG environment variable if it is set to a kubeconfig file path.
// 2. In-cluster config if running in cluster.
// 3. $HOME/.kube/config if it exists.
func NewClient() (client.Client, error) {
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return nil, errors.Wrap(err, "cannot create Kubernetes API REST config")
	}
	cfg.UserAgent = SuiteName + "/" + Version()
	cfg.QPS = 20
	cfg.Burst = 100

	s := runtime.NewScheme()

	if err := corev1.AddToScheme(s); err != nil {
		return nil, errors.Wrap(err, "cannot add Kubernetes core/v1 to scheme")
	}
	if err := kextv1.AddToScheme(s); err != nil {
		return nil, errors.Wrap(err, "cannot add Kubernetes apiextensions/v1 to scheme")
	}
	if err := pkgv1.AddToScheme(s); err != nil {
		return nil, errors.Wrap(err, "cannot add Crossplane pkg/v1 to scheme")
	}
	if err := extv1.AddToScheme(s); err != nil {
		return nil, errors.Wrap(err, "cannot add Crossplane apiextensions/v1 to scheme")
	}
	if err := extv1beta1.AddToScheme(s); err != nil {
		return nil, errors.Wrap(err, "cannot add Crossplane apiextensions/v1beta1 to scheme")
	}
	if err := extv1alpha1.AddToScheme(s); err != nil {
		return nil, errors.Wrap(err, "cannot add Crossplane apiextensions/v1alpha1 to scheme")
	}
	if err := extv2.AddToScheme(s); err != nil {
		return nil, errors.Wrap(err, "cannot add Crossplane apiextensions/v2 to scheme")
	}
	if err := opsv1alpha1.AddToScheme(s); err != nil {
		return nil, errors.Wrap(err, "cannot add Crossplane ops/v1alpha1 to scheme")
	}
	if err := protectionv1beta1.AddToScheme(s); err != nil {
		return nil, errors.Wrap(err, "cannot add Crossplane protection/v1beta1 to scheme")
	}
	if err := appsv1.AddToScheme(s); err != nil {
		return nil, errors.Wrap(err, "cannot add Kubernetes apps/v1 to scheme")
	}
	if err := rbacv1.AddToScheme(s); err != nil {
		return nil, errors.Wrap(err, "cannot add Kubernetes rbac/v1 to scheme")
	}

	c, err := client.New(cfg, client.Options{Scheme: s})
	return unstructured.NewClient(c), errors.Wrap(err, "cannot create Kubernetes API client")
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

// AsSet returns the supplied string slice as a set.
func AsSet(in []string) map[string]bool {
	set := map[string]bool{}
	for _, s := range in {
		set[s] = true
	}
	return set
}

// IgnoreFieldsOfMapKey is like cmpopts.IgnoreFields, but it only ignores fields
// of structs that are the value of a map index with the supplied key. It is
// intended for use with the kextv1.JSONSChemaProps type where IgnoreFields
// would be applied too broadly (e.g. by ignoring fields within a particular
// type of struct regardless of where it appeared in the map).
func IgnoreFieldsOfMapKey(key string, names ...string) cmp.Option {
	return cmp.FilterPath(func(p cmp.Path) bool {
		m, ok := p.Index(-2).(cmp.MapIndex)
		if !ok || m.Key().String() != key {
			return false
		}

		f, ok := p.Index(-1).(cmp.StructField)
		if !ok {
			return false
		}

		for _, n := range names {
			if _, ok := m.Type().FieldByName(n); !ok {
				panic(errors.Errorf("%s field %q does not exist", m.Type(), n))
			}
			if f.Name() == n {
				return true
			}
		}

		return false
	}, cmp.Ignore())
}

// OnlySubproperties is intended for use with the kextv1.JSONSchemaProps type.
// It ignores the supplied keys within any map that is the value of a struct
// field named 'Properties' where the struct is itself a map value under a key
// with the supplied name.
func OnlySubproperties(key string, keys ...string) cmp.Option {
	props := "Properties"
	return cmp.FilterPath(func(p cmp.Path) bool {
		m, ok := p.Index(-3).(cmp.MapIndex)
		if !ok || m.Key().String() != key {
			return false
		}

		if _, ok := m.Type().FieldByName(props); !ok {
			panic(errors.Errorf("%s field %q does not exist", m.Type(), props))
		}

		f, ok := p.Index(-2).(cmp.StructField)
		if !ok || f.Name() != props {
			return false
		}

		m, ok = p.Index(-1).(cmp.MapIndex)
		if !ok {
			return false
		}

		for _, n := range keys {
			if m.Key().String() == n {
				return false
			}
		}

		return true
	}, cmp.Ignore())
}

// CreateNamespace creates a test namespace to use during testing and ensures
// its clean up.
func CreateNamespace(ctx context.Context, t *testing.T, kube client.Client) {
	t.Helper()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: SuiteName}}
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
}

// CreateProvider creates a provider from the given package that will be used during
// testing and ensures its clean up.
func CreateProvider(ctx context.Context, t *testing.T, kube client.Client, pkg string) {
	t.Helper()
	prv := &pkgv1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: SuiteName},
		Spec: pkgv1.ProviderSpec{
			PackageSpec: pkgv1.PackageSpec{
				Package:                     pkg,
				IgnoreCrossplaneConstraints: ptr.To(true),
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
}

// CreateFunction creates a function from the given package that will be used
// during testing and ensures its clean up.
func CreateFunction(ctx context.Context, t *testing.T, kube client.Client, pkg string) *pkgv1.Function {
	fnc := &pkgv1.Function{
		ObjectMeta: metav1.ObjectMeta{Name: SuiteName + "-function"},
		Spec: pkgv1.FunctionSpec{
			PackageSpec: pkgv1.PackageSpec{
				Package:                     pkg,
				IgnoreCrossplaneConstraints: ptr.To(true),
			},
		},
	}

	if err := kube.Create(ctx, fnc); err != nil {
		t.Fatalf("Create function %q: %v", fnc.GetName(), err)
	}
	t.Logf("Created function %q", fnc.GetName())

	t.Cleanup(func() {
		t.Logf("Cleaning up function %q.", fnc.GetName())
		if err := kube.Get(ctx, types.NamespacedName{Name: fnc.GetName()}, fnc); resource.IgnoreNotFound(err) != nil {
			t.Fatalf("Get function %q: %v", fnc.GetName(), err)
		}
		if err := kube.Delete(ctx, fnc); resource.IgnoreNotFound(err) != nil {
			t.Fatalf("Delete function %q: %v", fnc.GetName(), err)
		}
		t.Logf("Deleted function %q", fnc.GetName())
	})

	return fnc
}

// ResourceRef represents an expected resource reference along with the
// set of fields/values to verify
type ResourceRef struct {
	Kind        string
	APIVersion  string
	Namespace   string
	Name        string
	FieldValues map[string]string // Map of field paths to expected values
}

// TestResourceRefs verifies that actual resource references match expected
// ones, including validating their field values
func TestResourceRefs(ctx context.Context, t *testing.T, kube client.Client, actualRefs []ResourceRef, wantRefs []ResourceRef) error {
	t.Helper()

	// verify we have the expected number of resource refs
	if len(actualRefs) != len(wantRefs) {
		return fmt.Errorf("want %d resource refs, got %d", len(wantRefs), len(actualRefs))
	}

	// Verify each expected resource exists and their objects have the expected values
	for _, wantRef := range wantRefs {
		var actualRef *ResourceRef

		// look for this wanted ref in the actual refs (note we just
		// match on kind, so this assumes only 1 resource ref of each
		// kind)
		for i := range actualRefs {
			if wantRef.Kind == actualRefs[i].Kind {
				actualRef = &actualRefs[i]
				break
			}
		}

		if actualRef == nil {
			return fmt.Errorf("missing wanted resource ref with kind %q", wantRef.Kind)
		}

		// retrieve the actual resource using the reference we found
		u := &kunstructured.Unstructured{}
		u.SetAPIVersion(actualRef.APIVersion)
		u.SetKind(actualRef.Kind)
		if err := kube.Get(ctx, types.NamespacedName{Name: actualRef.Name, Namespace: actualRef.Namespace}, u); err != nil {
			if kerrors.IsNotFound(err) {
				return fmt.Errorf("not yet created/applied %s %q", actualRef.Kind, actualRef.Name)
			}
			return fmt.Errorf("failed to get %s %q: %v", actualRef.Kind, actualRef.Name, err)
		}

		// Verify the actual resource's fields match the expected values
		paved := fieldpath.Pave(u.Object)
		for fieldPath, wantValue := range wantRef.FieldValues {
			actualValue, err := paved.GetString(fieldPath)
			if err != nil {
				return fmt.Errorf("resource ref %q failed to get field %q: %v", actualRef.Name, fieldPath, err)
			}
			if actualValue != wantValue {
				return fmt.Errorf("resource ref %q field %q: want %q, got %q", actualRef.Name, fieldPath, wantValue, actualValue)
			}
		}
	}

	return nil
}

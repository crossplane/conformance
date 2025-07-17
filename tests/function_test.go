// Copyright 2024 The Crossplane Authors
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

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/ptr"

	"github.com/crossplane/conformance/internal"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	pkgv1 "github.com/crossplane/crossplane/apis/pkg/v1"
)

func TestFunction(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)

	kube, err := internal.NewClient()
	if err != nil {
		t.Fatalf("Create client: %v", err)
	}

	fnc := &pkgv1.Function{
		ObjectMeta: metav1.ObjectMeta{Name: internal.SuiteName + "-function"},
		Spec: pkgv1.FunctionSpec{
			PackageSpec: pkgv1.PackageSpec{
				Package:                     "xpkg.crossplane.io/crossplane-contrib/function-patch-and-transform:v0.9.0",
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

	t.Run("BecomesInstalledAndHealthy", func(t *testing.T) {
		t.Log("Testing that the function's Healthy and Installed status conditions become 'True'.")
		if err := wait.PollUntilContextTimeout(ctx, 10*time.Second, 90*time.Second, true, func(ctx context.Context) (done bool, err error) {
			if err := kube.Get(ctx, types.NamespacedName{Name: fnc.GetName()}, fnc); err != nil {
				return false, err
			}

			if fnc.GetCondition(pkgv1.TypeHealthy).Status != corev1.ConditionTrue {
				t.Logf("Function %q is not yet Healthy", fnc.GetName())
				return false, nil
			}

			if fnc.GetCondition(pkgv1.TypeInstalled).Status != corev1.ConditionTrue {
				t.Logf("Function %q is not yet Installed", fnc.GetName())
				return false, nil
			}

			t.Logf("Function %q is Healthy and Installed", fnc.GetName())
			return true, nil
		}); err != nil {
			t.Errorf("Function %q never became Healthy and Installed: %v", fnc.GetName(), err)
		}
	})

	t.Run("RevisionBecomesHealthyAndDeploysObjects", func(t *testing.T) {
		t.Log("Testing that the function's revision's Healthy status conditions become 'True', and that it deploys its objects.")

		if err := wait.PollUntilContextTimeout(ctx, 10*time.Second, 90*time.Second, true, func(ctx context.Context) (done bool, err error) {
			l := &pkgv1.FunctionRevisionList{}
			if err := kube.List(ctx, l); err != nil {
				return false, err
			}

			for _, rev := range l.Items {
				for _, o := range rev.GetOwnerReferences() {
					// This is not the revision we're looking for.
					if o.Name != fnc.GetName() {
						continue
					}
					t.Logf("Found revision %q owned by function %q", rev.GetName(), fnc.GetName())

					if rev.GetCondition(pkgv1.TypeRevisionHealthy).Status != corev1.ConditionTrue {
						t.Logf("Revision %q is not yet RevisionHealthy", rev.GetName())
						return false, nil
					}

					if rev.GetCondition(pkgv1.TypeRuntimeHealthy).Status != corev1.ConditionTrue {
						t.Logf("Revision %q is not yet RuntimeHealthy", rev.GetName())
						return false, nil
					}

					t.Logf("Revision %q is RevisionHealthy and RuntimeHealthy", rev.GetName())

					// We expect the revision to deploy one object - the CRD of
					// the Resources function input object.
					if len(rev.Status.ObjectRefs) != 1 {
						t.Logf("Revision %q has deployed %d objects, want %d", rev.GetName(), len(rev.Status.ObjectRefs), 1)
						return false, nil
					}

					for _, ref := range rev.Status.ObjectRefs {
						u := &unstructured.Unstructured{}
						u.SetAPIVersion(ref.APIVersion)
						u.SetKind(ref.Kind)

						if err := kube.Get(ctx, types.NamespacedName{Name: ref.Name}, u); err != nil {
							if kerrors.IsNotFound(err) {
								t.Logf("Revision %q has not yet been created %s %q", rev.GetName(), ref.Kind, ref.Name)
								return false, nil
							}
							return false, err
						}
						t.Logf("Revision %q created %s %q", rev.GetName(), ref.Kind, ref.Name)
					}

					return true, nil
				}
			}

			return false, nil
		}); err != nil {
			t.Errorf("Function %q's revision never became Healthy and deployed its objects: %v", fnc.GetName(), err)
		}
	})
}

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
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/ptr"

	"github.com/crossplane/crossplane-runtime/pkg/resource"
	pkgv1 "github.com/crossplane/crossplane/apis/pkg/v1"

	"github.com/crossplane/conformance/internal"
)

func TestProvider(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)

	kube, err := internal.NewClient()
	if err != nil {
		t.Fatalf("Create client: %v", err)
	}

	prv := &pkgv1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: internal.SuiteName},
		Spec: pkgv1.ProviderSpec{
			PackageSpec: pkgv1.PackageSpec{
				Package:                     "xpkg.upbound.io/crossplane-contrib/provider-nop:v0.2.1",
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

	t.Run("BecomesInstalledAndHealthy", func(t *testing.T) {
		t.Log("Testing that the provider's Healthy and Installed status conditions become 'True'.")
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

	t.Run("RevisionBecomesHealthyAndDeploysObjects", func(t *testing.T) {
		t.Log("Testing that the provider's revision's Healthy status condition becomes 'True', and that it deploys its objects.")

		if err := wait.PollImmediate(10*time.Second, 90*time.Second, func() (done bool, err error) {
			l := &pkgv1.ProviderRevisionList{}
			if err := kube.List(ctx, l); err != nil {
				return false, err
			}

			for _, rev := range l.Items {
				rev := rev // To avoid using the range var in a fn literal.

				for _, o := range rev.GetOwnerReferences() {
					// This is not the revision we're looking for.
					if o.Name != prv.GetName() {
						continue
					}
					t.Logf("Found revision %q owned by provider %q", rev.GetName(), prv.GetName())

					if rev.GetCondition(pkgv1.TypeHealthy).Status != corev1.ConditionTrue {
						t.Logf("Revision %q is not yet Healthy", rev.GetName())
						return false, nil
					}

					t.Logf("Revision %q is Healthy", rev.GetName())

					// We expect the revision to deploy one object - the CRD of
					// the NopResource managed resource.
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
			t.Errorf("Provider %q's revision never became Healthy and deployed its objects: %v", prv.GetName(), err)
		}
	})
}

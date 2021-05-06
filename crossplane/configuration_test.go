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
	"k8s.io/utils/pointer"

	"github.com/crossplane/crossplane-runtime/pkg/resource"
	pkgv1 "github.com/crossplane/crossplane/apis/pkg/v1"

	"github.com/crossplane/conformance/internal"
)

func TestConfiguration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)

	kube, err := internal.NewClient()
	if err != nil {
		t.Fatalf("Create client: %v", err)
	}

	// This configuration is defined in the testdata/configuration directory
	// of this repository. It is built and pushed by CI/CD.
	cfg := &pkgv1.Configuration{
		ObjectMeta: metav1.ObjectMeta{Name: internal.SuiteName},
		Spec: pkgv1.ConfigurationSpec{
			PackageSpec: pkgv1.PackageSpec{
				Package:                     "crossplane/conformance-testdata-configuration:latest",
				IgnoreCrossplaneConstraints: pointer.BoolPtr(true),
			},
		},
	}

	// The crossplane-conformance provider depends on negz/provider-nop.
	prv := &pkgv1.Provider{ObjectMeta: metav1.ObjectMeta{Name: "negz-provider-nop"}}

	if err := kube.Create(ctx, cfg); err != nil {
		t.Fatalf("Create configuration %q: %v", cfg.GetName(), err)
	}
	t.Logf("Created configuration %q", cfg.GetName())

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

	t.Cleanup(func() {
		t.Logf("Cleaning up configuration %q.", cfg.GetName())
		if err := kube.Get(ctx, types.NamespacedName{Name: cfg.GetName()}, cfg); resource.IgnoreNotFound(err) != nil {
			t.Fatalf("Get configuration %q: %v", cfg.GetName(), err)
		}
		if err := kube.Delete(ctx, cfg); resource.IgnoreNotFound(err) != nil {
			t.Fatalf("Delete configuration %q: %v", cfg.GetName(), err)
		}
		t.Logf("Deleted configuration %q", cfg.GetName())
	})

	t.Run("BecomesInstalledAndHealthy", func(t *testing.T) {
		t.Parallel()
		t.Log("Testing that the configuration's Healthy and Installed status conditions become 'True'.")
		if err := wait.PollImmediate(10*time.Second, 90*time.Second, func() (done bool, err error) {
			if err := kube.Get(ctx, types.NamespacedName{Name: cfg.GetName()}, cfg); err != nil {
				return false, err
			}

			if cfg.GetCondition(pkgv1.TypeHealthy).Status != corev1.ConditionTrue {
				t.Logf("Configuration %q is not yet Healthy", cfg.GetName())
				return false, nil
			}

			if cfg.GetCondition(pkgv1.TypeInstalled).Status != corev1.ConditionTrue {
				t.Logf("Configuration %q is not yet Installed", cfg.GetName())
				return false, nil
			}

			t.Logf("Configuration %q is Healthy and Installed", cfg.GetName())
			return true, nil
		}); err != nil {
			t.Errorf("Configuration %q never became Healthy and Installed: %v", cfg.GetName(), err)
		}
	})

	t.Run("RevisionBecomesHealthyAndDeploysObjects", func(t *testing.T) {
		t.Parallel()
		t.Log("Testing that the configuration's revision's Healthy status condition becomes 'True', and that it deploys its objects.")

		if err := wait.PollImmediate(10*time.Second, 90*time.Second, func() (done bool, err error) {
			l := &pkgv1.ConfigurationRevisionList{}
			if err := kube.List(ctx, l); err != nil {
				return false, err
			}

			for _, rev := range l.Items {
				rev := rev // To avoid using the range var in a fn literal.

				for _, o := range rev.GetOwnerReferences() {
					// This is not the revision we're looking for.
					if o.Name != cfg.GetName() {
						continue
					}
					t.Logf("Found revision %q owned by configuration %q", rev.GetName(), cfg.GetName())

					if rev.GetCondition(pkgv1.TypeHealthy).Status != corev1.ConditionTrue {
						t.Logf("Revision %q is not yet Healthy", rev.GetName())
						return false, nil
					}

					t.Logf("Revision %q is Healthy", rev.GetName())

					// We expect the revision to deploy two objects; an XRD
					// and a Composition.
					if len(rev.Status.ObjectRefs) != 2 {
						t.Logf("Revision %q has deployed %d objects, want %d", rev.GetName(), len(rev.Status.ObjectRefs), 2)
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
			t.Errorf("Configuration %q's revision never became Healthy and deployed its objects: %v", cfg.GetName(), err)
		}
	})

	t.Run("DependencyBecomesInstalledAndHealthy", func(t *testing.T) {
		t.Parallel()
		t.Log("Testing that the configuration's dependencies' Healthy and Installed status conditions become 'True'.")
		if err := wait.PollImmediate(10*time.Second, 90*time.Second, func() (done bool, err error) {
			if err := kube.Get(ctx, types.NamespacedName{Name: prv.GetName()}, prv); err != nil {
				// Most likely the provider hasn't been created yet.
				if kerrors.IsNotFound(err) {
					return false, nil
				}
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
}

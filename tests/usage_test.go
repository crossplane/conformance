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
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	kextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"

	extv1beta1 "github.com/crossplane/crossplane/v2/apis/apiextensions/v1beta1"
	protectionv1beta1 "github.com/crossplane/crossplane/v2/apis/protection/v1beta1"

	"github.com/crossplane/conformance/internal"
)

// usageTestConfig holds the configuration for usage protection tests.
type usageTestConfig struct {
	ctx       context.Context
	t         *testing.T
	kube      client.Client
	usage     client.Object
	used      client.Object
	using     client.Object
	usageKind string
	usedKind  string
	usingKind string
}

// TestUsageProtectionNamespace verifies that a namespaced Usage object properly
// protects a used resource from being deleted while still in use.
func TestUsageProtectionNamespaced(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)

	kube, err := internal.NewClient()
	if err != nil {
		t.Fatalf("Create client: %v", err)
	}

	internal.CreateNamespace(ctx, t, kube)

	namespace := internal.SuiteName
	usedName := "used-configmap"
	usingName := "using-secret"
	usageName := "usage"

	used := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: usedName, Namespace: namespace},
		Data: map[string]string{
			"test": "data",
		},
	}

	using := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: usingName, Namespace: namespace},
		Data: map[string][]byte{
			"secret": []byte("value"),
		},
	}

	// usage object that tracks the configmap being used by the secret
	usage := &protectionv1beta1.Usage{
		ObjectMeta: metav1.ObjectMeta{Name: usageName, Namespace: namespace},
		Spec: protectionv1beta1.UsageSpec{
			Of: protectionv1beta1.NamespacedResource{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				ResourceRef: &protectionv1beta1.NamespacedResourceRef{
					Name:      usedName,
					Namespace: &namespace,
				},
			},
			By: &protectionv1beta1.Resource{
				APIVersion: "v1",
				Kind:       "Secret",
				ResourceRef: &protectionv1beta1.ResourceRef{
					Name: usingName,
				},
			},
		},
	}

	config := &usageTestConfig{
		ctx:       ctx,
		t:         t,
		kube:      kube,
		usage:     usage,
		used:      used,
		using:     using,
		usageKind: "Usage",
		usedKind:  "ConfigMap",
		usingKind: "Secret",
	}

	// create the used, using, and usage resources for this test
	createUsageResources(config)

	// verify test scenarios for usage blocking deletion of the used resources
	testUsageBecomesReady(config)
	testDeletionBlocked(config)
	testDeletionAllowedAfterUsingDeleted(config)
}

// TestUsageProtectionCluster verifies that a ClusterUsage object properly
// protects a cluster-scoped resource from being deleted while still in use.
func TestUsageProtectionCluster(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)

	kube, err := internal.NewClient()
	if err != nil {
		t.Fatalf("Create client: %v", err)
	}

	// Create a test CRD as our cluster-scoped "used" resource
	usedCRD := &kextv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "usedresources.test.crossplane.io",
		},
		Spec: kextv1.CustomResourceDefinitionSpec{
			Group: "test.crossplane.io",
			Versions: []kextv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema: &kextv1.CustomResourceValidation{
						OpenAPIV3Schema: &kextv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]kextv1.JSONSchemaProps{
								"spec": {
									Type: "object",
									Properties: map[string]kextv1.JSONSchemaProps{
										"field": {
											Type: "string",
										},
									},
								},
							},
						},
					},
				},
			},
			Scope: kextv1.ClusterScoped,
			Names: kextv1.CustomResourceDefinitionNames{
				Plural:   "usedresources",
				Singular: "usedresource",
				Kind:     "usedresource",
			},
		},
	}

	// Create another test CRD as our cluster-scoped "using" resource
	usingCRD := &kextv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "usingresources.test.crossplane.io",
		},
		Spec: kextv1.CustomResourceDefinitionSpec{
			Group: "test.crossplane.io",
			Versions: []kextv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema: &kextv1.CustomResourceValidation{
						OpenAPIV3Schema: &kextv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]kextv1.JSONSchemaProps{
								"spec": {
									Type: "object",
									Properties: map[string]kextv1.JSONSchemaProps{
										"field": {
											Type: "string",
										},
									},
								},
							},
						},
					},
				},
			},
			Scope: kextv1.ClusterScoped,
			Names: kextv1.CustomResourceDefinitionNames{
				Plural:   "usingresources",
				Singular: "usingresource",
				Kind:     "usingresource",
			},
		},
	}

	// ClusterUsage object that tracks the usage of these two resources
	clusterUsage := &protectionv1beta1.ClusterUsage{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-usage"},
		Spec: protectionv1beta1.ClusterUsageSpec{
			Of: protectionv1beta1.Resource{
				APIVersion: "apiextensions.k8s.io/v1",
				Kind:       "CustomResourceDefinition",
				ResourceRef: &protectionv1beta1.ResourceRef{
					Name: usedCRD.GetName(),
				},
			},
			By: &protectionv1beta1.Resource{
				APIVersion: "apiextensions.k8s.io/v1",
				Kind:       "CustomResourceDefinition",
				ResourceRef: &protectionv1beta1.ResourceRef{
					Name: usingCRD.GetName(),
				},
			},
		},
	}

	config := &usageTestConfig{
		ctx:       ctx,
		t:         t,
		kube:      kube,
		usage:     clusterUsage,
		used:      usedCRD,
		using:     usingCRD,
		usageKind: "ClusterUsage",
		usedKind:  "CustomResourceDefinition",
		usingKind: "CustomResourceDefinition",
	}

	// create the used, using, and usage resources for this test
	createUsageResources(config)

	// verify test scenarios for usage blocking deletion of the used resources
	testUsageBecomesReady(config)
	testDeletionBlocked(config)
	testDeletionAllowedAfterUsingDeleted(config)
}

// TestUsageLegacy verifies that a legacy Usage.apiextensions.crossplane.io
// object properly protects a cluster-scoped resource from being deleted while
// still in use.
func TestUsageLegacy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)

	kube, err := internal.NewClient()
	if err != nil {
		t.Fatalf("Create client: %v", err)
	}

	// Create a test CRD as our cluster-scoped "used" resource
	usedCRD := &kextv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "usedlegacyresources.test.crossplane.io",
		},
		Spec: kextv1.CustomResourceDefinitionSpec{
			Group: "test.crossplane.io",
			Versions: []kextv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema: &kextv1.CustomResourceValidation{
						OpenAPIV3Schema: &kextv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]kextv1.JSONSchemaProps{
								"spec": {
									Type: "object",
									Properties: map[string]kextv1.JSONSchemaProps{
										"field": {
											Type: "string",
										},
									},
								},
							},
						},
					},
				},
			},
			Scope: kextv1.ClusterScoped,
			Names: kextv1.CustomResourceDefinitionNames{
				Plural:   "usedlegacyresources",
				Singular: "usedlegacyresource",
				Kind:     "usedlegacyresource",
			},
		},
	}

	// Create another test CRD as our cluster-scoped "using" resource
	usingCRD := &kextv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "usinglegacyresources.test.crossplane.io",
		},
		Spec: kextv1.CustomResourceDefinitionSpec{
			Group: "test.crossplane.io",
			Versions: []kextv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema: &kextv1.CustomResourceValidation{
						OpenAPIV3Schema: &kextv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]kextv1.JSONSchemaProps{
								"spec": {
									Type: "object",
									Properties: map[string]kextv1.JSONSchemaProps{
										"field": {
											Type: "string",
										},
									},
								},
							},
						},
					},
				},
			},
			Scope: kextv1.ClusterScoped,
			Names: kextv1.CustomResourceDefinitionNames{
				Plural:   "usinglegacyresources",
				Singular: "usinglegacyresource",
				Kind:     "usinglegacyresource",
			},
		},
	}

	// Legacy Usage object that tracks the usage of these two resources.
	//nolint:staticcheck // extv1beta1.Usage is intentionally tested for legacy compatibility.
	legacyUsage := &extv1beta1.Usage{
		ObjectMeta: metav1.ObjectMeta{Name: "legacy-usage"},
		Spec: extv1beta1.UsageSpec{
			Of: extv1beta1.Resource{
				APIVersion: "apiextensions.k8s.io/v1",
				Kind:       "CustomResourceDefinition",
				ResourceRef: &extv1beta1.ResourceRef{
					Name: usedCRD.GetName(),
				},
			},
			By: &extv1beta1.Resource{
				APIVersion: "apiextensions.k8s.io/v1",
				Kind:       "CustomResourceDefinition",
				ResourceRef: &extv1beta1.ResourceRef{
					Name: usingCRD.GetName(),
				},
			},
		},
	}

	config := &usageTestConfig{
		ctx:       ctx,
		t:         t,
		kube:      kube,
		usage:     legacyUsage,
		used:      usedCRD,
		using:     usingCRD,
		usageKind: "Usage",
		usedKind:  "CustomResourceDefinition",
		usingKind: "CustomResourceDefinition",
	}

	// create the used, using, and usage resources for this test
	createUsageResources(config)

	// verify test scenarios for usage blocking deletion of the used resources
	testUsageBecomesReady(config)
	testDeletionBlocked(config)
	testDeletionAllowedAfterUsingDeleted(config)
}

// createUsageResources creates the given using, used, and usage resources and
// schedules their clean up when the test is complete.
func createUsageResources(config *usageTestConfig) {
	ctx := config.ctx
	t := config.t
	kube := config.kube
	used := config.used
	usedKind := config.usedKind
	using := config.using
	usingKind := config.usingKind
	usage := config.usage
	usageKind := config.usageKind

	// schedule clean up for the used, using, and usage resources
	t.Cleanup(func() {
		t.Logf("Cleaning up %s %q", usageKind, usage.GetName())
		if err := kube.Get(ctx, types.NamespacedName{Name: usage.GetName(), Namespace: usage.GetNamespace()}, usage); resource.IgnoreNotFound(err) != nil {
			t.Fatalf("Get %s %q: %v", usageKind, usage.GetName(), err)
		}
		if err := kube.Delete(ctx, usage); resource.IgnoreNotFound(err) != nil {
			t.Fatalf("Delete %s %q: %v", usageKind, usage.GetName(), err)
		}
		t.Logf("Deleted %s %q", usageKind, usage.GetName())

		t.Logf("Cleaning up using %s %q", usingKind, using.GetName())
		if err := kube.Get(ctx, types.NamespacedName{Name: using.GetName(), Namespace: using.GetNamespace()}, using); resource.IgnoreNotFound(err) != nil {
			t.Fatalf("Get using %s %q: %v", usingKind, using.GetName(), err)
		}
		if err := kube.Delete(ctx, using); resource.IgnoreNotFound(err) != nil {
			t.Fatalf("Delete using %s %q: %v", usingKind, using.GetName(), err)
		}
		t.Logf("Deleted using %s %q", usingKind, using.GetName())

		t.Logf("Cleaning up used %s %q", usedKind, used.GetName())
		if err := kube.Get(ctx, types.NamespacedName{Name: used.GetName(), Namespace: used.GetNamespace()}, used); resource.IgnoreNotFound(err) != nil {
			t.Fatalf("Get used %s %q: %v", usedKind, used.GetName(), err)
		}
		if err := kube.Delete(ctx, used); resource.IgnoreNotFound(err) != nil {
			t.Fatalf("Delete used %s %q: %v", usedKind, used.GetName(), err)
		}
		t.Logf("Deleted used %s %q", usedKind, used.GetName())
	})

	// create the used, using, and usage resources
	if err := config.kube.Create(ctx, used); err != nil {
		t.Fatalf("Create used %s %q: %v", usedKind, used.GetName(), err)
	}
	t.Logf("Created used %s %q", usedKind, used.GetName())

	if err := kube.Create(ctx, using); err != nil {
		t.Fatalf("Create using %s %q: %v", usingKind, using.GetName(), err)
	}
	t.Logf("Created using %s %q", usingKind, using.GetName())

	if err := kube.Create(ctx, usage); err != nil {
		t.Fatalf("Create %s %q: %v", usageKind, usage.GetName(), err)
	}
	t.Logf("Created %s %q", usageKind, usage.GetName())
}

// testUsageBecomesReady tests that a usage object becomes ready.
func testUsageBecomesReady(config *usageTestConfig) {
	config.t.Run(fmt.Sprintf("%sBecomesReadyAvailable", config.usageKind), func(t *testing.T) {
		t.Logf("Testing that the %s %q becomes ready and available.", config.usageKind, config.usage.GetName())

		if err := wait.PollUntilContextTimeout(config.ctx, 5*time.Second, 1*time.Minute, true, func(ctx context.Context) (done bool, err error) {
			if err := config.kube.Get(ctx, client.ObjectKeyFromObject(config.usage), config.usage); err != nil {
				config.t.Logf("Unexpected error getting %s %q: %v", config.usageKind, config.usage.GetName(), err)
				return false, nil
			}

			cr, ok := config.usage.(resource.Conditioned)
			if !ok {
				config.t.Logf("%s %q should implement Conditioned interface", config.usageKind, config.usage.GetName())
				return false, nil
			}

			if cr.GetCondition(xpv1.TypeReady).Status != corev1.ConditionTrue {
				config.t.Logf("%s %q is not yet Ready", config.usageKind, config.usage.GetName())
				return false, nil
			}

			return true, nil
		}); err != nil {
			t.Errorf("%s %q did not become ready: %v", config.usageKind, config.usage.GetName(), err)
		}
	})
}

// testDeletionBlocked tests that deletion is blocked while used object still in use.
func testDeletionBlocked(config *usageTestConfig) {
	config.t.Run("DeletionBlockedWhileInUse", func(t *testing.T) {
		t.Logf("Testing that deletion of the used %s is blocked while the using %s exists.", config.usedKind, config.usingKind)

		if err := config.kube.Delete(config.ctx, config.used); err == nil {
			t.Errorf("Expected deletion of used %s %q to be blocked by %s, but it succeeded", config.usedKind, config.used.GetName(), config.usageKind)
		} else {
			if strings.Contains(err.Error(), "admission webhook \"nousages.protection.crossplane.io\" denied the request") {
				t.Logf("Deletion of used %s %q correctly blocked: %v", config.usedKind, config.used.GetName(), err)
			} else {
				t.Errorf("Expected error to mention usage blocking, got: %v", err)
			}
		}

		if err := config.kube.Get(config.ctx, client.ObjectKeyFromObject(config.used), config.used); err != nil {
			t.Errorf("Used %s %q should still exist after blocked deletion: %v", config.usedKind, config.used.GetName(), err)
		}
	})
}

// testDeletionAllowedAfterUsingDeleted tests that deletion of the used object
// is allowed after using resource is deleted.
func testDeletionAllowedAfterUsingDeleted(config *usageTestConfig) {
	config.t.Run("testDeletionAllowedAfterUsingDeleted", func(t *testing.T) {
		t.Log("Testing that deletion of the used object is allowed after using resource is deleted.")

		if err := config.kube.Delete(config.ctx, config.using); err != nil {
			t.Fatalf("Delete using %s %q: %v", config.usingKind, config.using.GetName(), err)
		}
		t.Logf("Deleted using %s %q", config.usingKind, config.using.GetName())

		if err := wait.PollUntilContextTimeout(config.ctx, 5*time.Second, 1*time.Minute, true, func(ctx context.Context) (done bool, err error) {
			if err := config.kube.Delete(ctx, config.used); err != nil {
				t.Logf("Used %s %q deletion still blocked, retrying: %v", config.usedKind, config.used.GetName(), err)
				return false, nil
			}
			t.Logf("Used %s %q deletion succeeded after using %s %q deleted", config.usedKind, config.used.GetName(), config.usingKind, config.using.GetName())
			return true, nil
		}); err != nil {
			t.Errorf("Used %s %q deletion should be allowed after using resource is deleted, but still blocked: %v", config.usedKind, config.used.GetName(), err)
		}

		if err := wait.PollUntilContextTimeout(config.ctx, 5*time.Second, 1*time.Minute, true, func(ctx context.Context) (done bool, err error) {
			if err := config.kube.Get(ctx, client.ObjectKeyFromObject(config.used), config.used); err != nil {
				if kerrors.IsNotFound(err) {
					t.Logf("Confirmed used %s %q no longer exists", config.usedKind, config.used.GetName())
					return true, nil
				}
				t.Logf("Unexpected error getting used %s %q: %v", config.usedKind, config.used.GetName(), err)
				return false, nil
			}
			t.Logf("Used %s %q still exists after deletion...", config.usedKind, config.used.GetName())
			return false, nil
		}); err != nil {
			t.Errorf("Used %s %q still exists after it was deleted: %v", config.usedKind, config.used.GetName(), err)
		}
	})
}

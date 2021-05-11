package internal

import (
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/resource/unstructured"
	extv1 "github.com/crossplane/crossplane/apis/apiextensions/v1"
	pkgv1 "github.com/crossplane/crossplane/apis/pkg/v1"
)

// SuiteName of the conformance test suite.
const SuiteName = "crossplane-conformance"

// Version of the conformance plugin.
var Version = "unknown"

// NewClient returns a Kubernetes API client suitable for use with conformance
// tests. It uses controller-runtime's config loading precedence to figure out
// how to connect to an API server, i.e. it will try in order:
//
// 1. The KUBECONFIG environment variable if it is set to a kubeconfig file path
// 2. In-cluster config if running in cluster
// 3. $HOME/.kube/config if it exists
func NewClient() (client.Client, error) {
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return nil, errors.Wrap(err, "cannot create Kubernetes API REST config")
	}
	cfg.UserAgent = SuiteName + "/" + Version
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
				panic(errors.Errorf("%s field %q does not exist\n", m.Type(), n))
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
			panic(errors.Errorf("%s field %q does not exist\n", m.Type(), props))
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

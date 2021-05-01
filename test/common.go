package conformance

import (
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	extv1 "github.com/crossplane/crossplane/apis/apiextensions/v1"
	pkgv1 "github.com/crossplane/crossplane/apis/pkg/v1"
)

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
	cfg.UserAgent = "crossplane-conformance/" + Version
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
	return c, errors.Wrap(err, "cannot create Kubernetes API client")
}

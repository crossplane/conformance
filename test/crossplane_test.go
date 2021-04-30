package conformance

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/crossplane/crossplane-runtime/pkg/resource"
)

func TestCrossplane(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := NewClient()
	if err != nil {
		t.Fatalf("Create client: %v", err)
	}

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "crossplane-conformance"}}
	if err := client.Create(ctx, ns); resource.Ignore(kerrors.IsAlreadyExists, err) != nil {
		t.Fatalf("Create conformance namespace: %v", err)
	}

	defer func() {
		if err := client.Delete(ctx, ns); resource.Ignore(kerrors.IsNotFound, err) != nil {
			t.Fatalf("Delete conformance namespace: %v", err)
		}
	}()

	t.Run("InstallConfiguration", func(t *testing.T) {
		// TODO
	})
}

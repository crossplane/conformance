package provider

import (
	"testing"

	"github.com/crossplane/conformance/internal"
)

func TestVersion(t *testing.T) {
	// Not really a test, but we want somewhere to log this.
	t.Logf("Conformance test version: %s", internal.Version)
}

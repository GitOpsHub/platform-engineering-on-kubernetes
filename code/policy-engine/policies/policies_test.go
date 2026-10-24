package policies

import (
	"context"
	"testing"

	"github.com/open-policy-agent/opa/v1/tester"
)

// TestRegoSuite runs every *_test.rego in this directory, the same as
// `opa test policies/`, so the policies are verified without the opa CLI.
func TestRegoSuite(t *testing.T) {
	results, err := tester.Run(context.Background(), ".")
	if err != nil {
		t.Fatalf("loading Rego tests: %v", err)
	}
	// labels 6 + registry 4 + limits 6 + privileged 4 = 20 Rego tests.
	const wantAtLeast = 20
	if len(results) < wantAtLeast {
		t.Fatalf("discovered %d Rego tests, want at least %d", len(results), wantAtLeast)
	}
	for _, r := range results {
		switch {
		case r.Error != nil:
			t.Errorf("%s.%s errored: %v", r.Package, r.Name, r.Error)
		case r.Fail:
			t.Errorf("%s.%s failed", r.Package, r.Name)
		}
	}
}

func TestModulesExcludeTests(t *testing.T) {
	mods, err := Modules()
	if err != nil {
		t.Fatal(err)
	}
	for name := range mods {
		if len(name) > len("_test.rego") && name[len(name)-len("_test.rego"):] == "_test.rego" {
			t.Fatalf("test file %s must not be embedded for evaluation", name)
		}
	}
	for _, want := range []string{"labels.rego", "registry.rego", "limits.rego", "privileged.rego"} {
		if _, ok := mods[want]; !ok {
			t.Fatalf("missing policy %s", want)
		}
	}
}

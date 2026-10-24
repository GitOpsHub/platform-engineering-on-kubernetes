// Package policies embeds the Rego policies that the engine loads at start-up.
// Each policy file contributes rules to the package "platform"; the engine
// evaluates data.platform.deny. Files ending in _test.rego are not embedded
// into the engine; they run under `opa test` and in this package's tests.
package policies

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
)

//go:embed *.rego
var files embed.FS

// Modules returns the policy sources the engine evaluates, keyed by file name.
func Modules() (map[string]string, error) {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".rego") || strings.HasSuffix(name, "_test.rego") {
			continue
		}
		raw, err := files.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("reading policy %s: %w", name, err)
		}
		out[name] = string(raw)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no policies embedded")
	}
	return out, nil
}

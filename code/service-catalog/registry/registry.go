// Package registry holds the scaffolding templates available to the CLI: the
// built-in set embedded in the binary, plus any directory of templates a team
// supplies.
package registry

import (
	"fmt"
	"io/fs"
	"sort"

	catalog "github.com/GitOpsHub/platform-engineering-on-kubernetes/code/service-catalog"
	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/service-catalog/templates"
)

// Entry is a template plus the file tree that renders it.
type Entry struct {
	Template catalog.Template
	Files    fs.FS // rooted at the template directory
}

// Registry is a set of templates keyed by name.
type Registry struct {
	entries map[string]Entry
}

// New returns an empty registry.
func New() *Registry {
	return &Registry{entries: map[string]Entry{}}
}

// Add registers an entry. Duplicate names are an error.
func (r *Registry) Add(e Entry) error {
	name := e.Template.Name
	if _, dup := r.entries[name]; dup {
		return fmt.Errorf("template %q is already registered", name)
	}
	r.entries[name] = e
	return nil
}

// Merge copies every entry of other into r, failing on any name clash.
func (r *Registry) Merge(other *Registry) error {
	for _, e := range other.entries {
		if err := r.Add(e); err != nil {
			return err
		}
	}
	return nil
}

// Get returns the named entry.
func (r *Registry) Get(name string) (Entry, error) {
	e, ok := r.entries[name]
	if !ok {
		return Entry{}, fmt.Errorf("unknown template %q (available: %v)", name, r.Names())
	}
	return e, nil
}

// Names returns the sorted template names.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.entries))
	for n := range r.entries {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// List returns the template metadata, sorted by name.
func (r *Registry) List() []catalog.Template {
	out := make([]catalog.Template, 0, len(r.entries))
	for _, n := range r.Names() {
		out = append(out, r.entries[n].Template)
	}
	return out
}

// Builtin returns the templates embedded in the binary.
func Builtin() (*Registry, error) {
	return FromFS(templates.FS)
}

// LoadDir returns the templates found under root. Each immediate subdirectory
// that contains a template.yaml is one template.
func LoadDir(root fs.FS) (*Registry, error) {
	return FromFS(root)
}

// FromFS scans the top-level directories of fsys for template.yaml files.
func FromFS(fsys fs.FS) (*Registry, error) {
	dirs, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("reading template root: %w", err)
	}
	r := New()
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		sub, err := fs.Sub(fsys, d.Name())
		if err != nil {
			return nil, err
		}
		raw, err := fs.ReadFile(sub, catalog.MetadataFile)
		if err != nil {
			continue // not a template directory
		}
		t, err := catalog.ParseTemplate(raw)
		if err != nil {
			return nil, fmt.Errorf("template %q: %w", d.Name(), err)
		}
		if err := r.Add(Entry{Template: t, Files: sub}); err != nil {
			return nil, err
		}
	}
	return r, nil
}

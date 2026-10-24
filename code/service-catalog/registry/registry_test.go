package registry

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func TestBuiltinTemplates(t *testing.T) {
	r, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := r.Names(), []string{"go-http-service", "static-site"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("names = %v, want %v", got, want)
	}
	if _, err := r.Get("nope"); err == nil || !strings.Contains(err.Error(), "go-http-service") {
		t.Fatalf("unknown template error should list available names, got %v", err)
	}
}

func TestFromFSSkipsNonTemplates(t *testing.T) {
	fsys := fstest.MapFS{
		"alpha/template.yaml": {Data: []byte("name: alpha\ntitle: Alpha\nparameters: []\n")},
		"alpha/README.md":     {Data: []byte("x")},
		"notes/todo.txt":      {Data: []byte("not a template")},
	}
	r, err := LoadDir(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Names(); !reflect.DeepEqual(got, []string{"alpha"}) {
		t.Fatalf("names = %v", got)
	}
}

func TestInvalidMetadataIsReported(t *testing.T) {
	fsys := fstest.MapFS{
		"bad/template.yaml": {Data: []byte("name: Bad Name\ntitle: x\nparameters: []\n")},
	}
	if _, err := LoadDir(fsys); err == nil || !strings.Contains(err.Error(), "DNS label") {
		t.Fatalf("expected DNS label error, got %v", err)
	}
}

func TestDuplicateAndMerge(t *testing.T) {
	builtin, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "static-site"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "static-site", "template.yaml"),
		[]byte("name: static-site\ntitle: Custom\nparameters: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	custom, err := LoadDir(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	if err := builtin.Merge(custom); err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("merging a duplicate name must fail, got %v", err)
	}
}

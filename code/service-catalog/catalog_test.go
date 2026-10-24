package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"sigs.k8s.io/yaml"

	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/service-catalog/templates"
)

func TestParseTemplateValidation(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{"ok", "name: svc\ntitle: Svc\nparameters:\n  - name: owner\n    required: true\n", ""},
		{"bad name", "name: Svc_1\ntitle: Svc\n", "DNS label"},
		{"missing title", "name: svc\n", "title is required"},
		{"bad param name", "name: svc\ntitle: S\nparameters:\n  - name: Owner-Team\n", "lowerCamel"},
		{"duplicate param", "name: svc\ntitle: S\nparameters:\n  - name: a\n  - name: a\n", "declared twice"},
		{"bad pattern", "name: svc\ntitle: S\nparameters:\n  - name: a\n    pattern: '['\n", "invalid pattern"},
		{"default violates enum", "name: svc\ntitle: S\nparameters:\n  - name: a\n    default: x\n    enum: [y, z]\n", "default of"},
		{"unknown field", "name: svc\ntitle: S\nbogus: 1\n", "bogus"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseTemplate([]byte(tc.yaml))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	tpl := Template{Name: "svc", Title: "S", Parameters: []Parameter{
		{Name: "owner", Required: true, Pattern: `^[a-z]+$`},
		{Name: "port", Default: "8080", Pattern: `^[0-9]+$`},
		{Name: "lifecycle", Default: "experimental", Enum: []string{"experimental", "production"}},
	}}
	tests := []struct {
		name    string
		values  map[string]string
		want    map[string]string
		wantErr []string
	}{
		{"defaults applied", map[string]string{"owner": "payments"},
			map[string]string{"owner": "payments", "port": "8080", "lifecycle": "experimental"}, nil},
		{"override", map[string]string{"owner": "payments", "port": "9090", "lifecycle": "production"},
			map[string]string{"owner": "payments", "port": "9090", "lifecycle": "production"}, nil},
		{"missing required", map[string]string{}, nil, []string{`"owner" is required`}},
		{"pattern mismatch", map[string]string{"owner": "Pay Ments"}, nil, []string{"does not match"}},
		{"enum mismatch", map[string]string{"owner": "x", "lifecycle": "beta"}, nil, []string{"must be one of"}},
		{"unknown key", map[string]string{"owner": "x", "prot": "1"}, nil, []string{`unknown parameter "prot"`}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tpl.Resolve(tc.values)
			if tc.wantErr != nil {
				if err == nil {
					t.Fatal("expected a validation error")
				}
				ve, ok := err.(*ValidationError)
				if !ok {
					t.Fatalf("error type %T", err)
				}
				joined := strings.Join(ve.Problems, "|")
				for _, w := range tc.wantErr {
					if !strings.Contains(joined, w) {
						t.Fatalf("problems %q missing %q", joined, w)
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Fatalf("%s = %q, want %q (all: %v)", k, got[k], v, got)
				}
			}
		})
	}
}

func TestResolveReportsAllProblems(t *testing.T) {
	tpl := Template{Name: "svc", Title: "S", Parameters: []Parameter{{Name: "owner", Required: true}}}
	_, err := tpl.Resolve(map[string]string{"typo": "x"})
	ve, ok := err.(*ValidationError)
	if !ok || len(ve.Problems) != 2 {
		t.Fatalf("want two problems, got %v", err)
	}
}

func values() map[string]string {
	return map[string]string{
		"name": "checkout", "owner": "payments", "namespace": "payments-prod",
		"description": `Handles "checkout" flows`, "image": "ghcr.io/acme", "lifecycle": "production",
	}
}

func renderByName(t *testing.T, dir string, vals map[string]string) (Template, []File) {
	t.Helper()
	raw, err := templates.FS.ReadFile(dir + "/" + MetadataFile)
	if err != nil {
		t.Fatal(err)
	}
	tpl, err := ParseTemplate(raw)
	if err != nil {
		t.Fatal(err)
	}
	src, err := fsSub(dir)
	if err != nil {
		t.Fatal(err)
	}
	files, err := Render(src, tpl, vals)
	if err != nil {
		t.Fatal(err)
	}
	return tpl, files
}

func findFile(t *testing.T, files []File, p string) string {
	t.Helper()
	for _, f := range files {
		if f.Path == p {
			return string(f.Content)
		}
	}
	t.Fatalf("file %q not rendered; have %v", p, paths(files))
	return ""
}

func paths(files []File) []string {
	var out []string
	for _, f := range files {
		out = append(out, f.Path)
	}
	return out
}

func TestRenderGoHTTPService(t *testing.T) {
	_, files := renderByName(t, "go-http-service", values())
	for _, want := range []string{"Dockerfile", "main.go", "go.mod", "workload.yaml", "catalog-info.yaml"} {
		findFile(t, files, want)
	}
	// Workload and catalog entry must be valid YAML with the expected identity.
	var wl map[string]any
	if err := yaml.Unmarshal([]byte(findFile(t, files, "workload.yaml")), &wl); err != nil {
		t.Fatalf("workload.yaml is not valid YAML: %v", err)
	}
	if wl["kind"] != "Workload" || wl["apiVersion"] != "platform.mycompany.io/v1alpha1" {
		t.Fatalf("bad Workload identity: %v", wl)
	}
	meta := wl["metadata"].(map[string]any)
	if meta["name"] != "checkout" || meta["namespace"] != "payments-prod" {
		t.Fatalf("bad Workload metadata: %v", meta)
	}
	spec := wl["spec"].(map[string]any)
	if spec["image"] != "ghcr.io/acme/checkout:latest" || spec["port"] != float64(8080) {
		t.Fatalf("bad Workload spec: %v", spec)
	}
	var info map[string]any
	if err := yaml.Unmarshal([]byte(findFile(t, files, "catalog-info.yaml")), &info); err != nil {
		t.Fatalf("catalog-info.yaml is not valid YAML: %v", err)
	}
	if info["kind"] != "Component" || info["spec"].(map[string]any)["owner"] != "payments" {
		t.Fatalf("bad catalog entry: %v", info)
	}
	// Quoting: the description contains double quotes and must round-trip.
	if info["metadata"].(map[string]any)["description"] != `Handles "checkout" flows` {
		t.Fatalf("description not escaped correctly: %v", info["metadata"])
	}
	if !strings.Contains(findFile(t, files, "Dockerfile"), "EXPOSE 8080") {
		t.Fatal("Dockerfile must expose the default port")
	}
	if !strings.Contains(findFile(t, files, "go.mod"), "module checkout") {
		t.Fatal("go.mod must name the module after the service")
	}
}

func TestRenderStaticSite(t *testing.T) {
	_, files := renderByName(t, "static-site", values())
	findFile(t, files, "site/index.html")
	if !strings.Contains(findFile(t, files, "Dockerfile"), "nginxinc/nginx-unprivileged") {
		t.Fatal("Dockerfile must use the default base image")
	}
	if !strings.Contains(findFile(t, files, "site/index.html"), "Owned by payments") {
		t.Fatal("index.html must mention the owner")
	}
}

func TestRenderRejectsInvalidValues(t *testing.T) {
	_, err := renderByNameErr("go-http-service", map[string]string{"name": "Bad_Name", "owner": "x"})
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected pattern error, got %v", err)
	}
}

func renderByNameErr(dir string, vals map[string]string) ([]File, error) {
	raw, err := templates.FS.ReadFile(dir + "/" + MetadataFile)
	if err != nil {
		return nil, err
	}
	tpl, err := ParseTemplate(raw)
	if err != nil {
		return nil, err
	}
	src, err := fsSub(dir)
	if err != nil {
		return nil, err
	}
	return Render(src, tpl, vals)
}

func TestRenderMissingKeyIsAnError(t *testing.T) {
	src := fstest.MapFS{"broken.txt.tmpl": {Data: []byte("hello {{ .doesNotExist }}")}}
	tpl := Template{Name: "x", Title: "X", Parameters: []Parameter{}}
	if _, err := Render(src, tpl, map[string]string{}); err == nil {
		t.Fatal("referencing an undeclared value must fail")
	}
}

func TestWriteTree(t *testing.T) {
	dir := t.TempDir()
	files := []File{{Path: "a/b.txt", Content: []byte("one")}, {Path: "c.txt", Content: []byte("two")}}
	if err := WriteTree(dir, files, false); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(filepath.Join(dir, "a", "b.txt")); string(raw) != "one" {
		t.Fatalf("wrote %q", raw)
	}
	if err := WriteTree(dir, files, false); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second write without --force must fail, got %v", err)
	}
	if err := WriteTree(dir, []File{{Path: "c.txt", Content: []byte("new")}}, true); err != nil {
		t.Fatalf("overwrite with force failed: %v", err)
	}
}

func TestWriteTreeRejectsEscapes(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "out")
	for _, bad := range []string{"../evil.txt", "/etc/passwd", "a/../../evil.txt"} {
		if err := WriteTree(dir, []File{{Path: bad, Content: []byte("x")}}, true); err == nil {
			t.Fatalf("path %q must be rejected", bad)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "evil.txt")); err == nil {
		t.Fatal("a rejected path must not be written")
	}
}

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	root := NewRootCmd(&App{Out: &buf, Err: &buf})
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

func TestListShowsBuiltins(t *testing.T) {
	out, err := run(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{"go-http-service", "static-site", "NAME", "TITLE"} {
		if !strings.Contains(out, w) {
			t.Fatalf("list output missing %q:\n%s", w, out)
		}
	}
}

func TestListJSON(t *testing.T) {
	out, err := run(t, "list", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"name": "static-site"`) {
		t.Fatalf("json output:\n%s", out)
	}
}

func TestDescribe(t *testing.T) {
	out, err := run(t, "describe", "go-http-service")
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{"Parameters:", "owner", "one of experimental|production|deprecated", "Dockerfile", "workload.yaml", "catalog-info.yaml"} {
		if !strings.Contains(out, w) {
			t.Fatalf("describe output missing %q:\n%s", w, out)
		}
	}
	if _, err := run(t, "describe", "nope"); err == nil {
		t.Fatal("unknown template must be an error")
	}
}

func TestScaffoldWritesFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "checkout")
	out, err := run(t, "scaffold", "go-http-service", "--set", "name=checkout", "--set", "owner=payments", "--out", dir)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, f := range []string{"Dockerfile", "main.go", "go.mod", "workload.yaml", "catalog-info.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("missing %s: %v", f, err)
		}
	}
	// A second run must not clobber files without --force.
	if _, err := run(t, "scaffold", "go-http-service", "--set", "name=checkout", "--set", "owner=payments", "--out", dir); err == nil {
		t.Fatal("re-scaffolding without --force must fail")
	}
	if _, err := run(t, "scaffold", "go-http-service", "--set", "name=checkout", "--set", "owner=payments", "--out", dir, "--force"); err != nil {
		t.Fatalf("--force should overwrite: %v", err)
	}
}

func TestScaffoldDefaultsDirToName(t *testing.T) {
	wd := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(wd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	if _, err := run(t, "scaffold", "static-site", "--set", "name=landing", "--set", "owner=growth"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(wd, "landing", "site", "index.html")); err != nil {
		t.Fatalf("expected ./landing/site/index.html: %v", err)
	}
}

func TestScaffoldDryRunWritesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "dry")
	out, err := run(t, "scaffold", "static-site", "--set", "name=dry", "--set", "owner=x", "--out", dir, "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "would write") {
		t.Fatalf("dry-run output:\n%s", out)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Fatal("dry run must not create the directory")
	}
}

func TestScaffoldValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"missing required", []string{"scaffold", "go-http-service", "--set", "name=svc", "--out", "x"}, `"owner" is required`},
		{"bad pattern", []string{"scaffold", "go-http-service", "--set", "name=Bad_Name", "--set", "owner=x", "--out", "x"}, "does not match"},
		{"unknown parameter", []string{"scaffold", "go-http-service", "--set", "name=svc", "--set", "owner=x", "--set", "prot=1", "--out", "x"}, `unknown parameter "prot"`},
		{"malformed set", []string{"scaffold", "static-site", "--set", "noequals"}, "want key=value"},
		{"duplicate set", []string{"scaffold", "static-site", "--set", "name=a", "--set", "name=b"}, "more than once"},
		{"needs out or name", []string{"scaffold", "static-site", "--set", "owner=x"}, "--out is required"},
		{"unknown template", []string{"scaffold", "rails", "--set", "name=a"}, `unknown template "rails"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := run(t, tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestExtraTemplatesDirectory(t *testing.T) {
	dir := t.TempDir()
	tplDir := filepath.Join(dir, "cron-job")
	if err := os.MkdirAll(tplDir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := "name: cron-job\ntitle: Cron job\nparameters:\n  - name: name\n    required: true\n"
	if err := os.WriteFile(filepath.Join(tplDir, "template.yaml"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tplDir, "job.yaml.tmpl"), []byte("kind: CronJob\nname: {{ .name }}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, "--templates", dir, "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "cron-job") || !strings.Contains(out, "go-http-service") {
		t.Fatalf("custom and built-in templates should both be listed:\n%s", out)
	}
}

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func envMap(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func writeFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestResolvePrecedence(t *testing.T) {
	file := writeFile(t, "kubeconfig: /file/kube\ncontext: file-ctx\nnamespace: file-ns\noutput: yaml\n")
	tests := []struct {
		name  string
		path  string
		env   map[string]string
		flags Config
		want  Config
	}{
		{"defaults only", filepath.Join(t.TempDir(), "missing.yaml"), nil, Config{},
			Config{Output: "table"}},
		{"file beats defaults", file, nil, Config{},
			Config{Kubeconfig: "/file/kube", Context: "file-ctx", Namespace: "file-ns", Output: "yaml"}},
		{"env beats file", file, map[string]string{EnvNamespace: "env-ns", EnvOutput: "json"}, Config{},
			Config{Kubeconfig: "/file/kube", Context: "file-ctx", Namespace: "env-ns", Output: "json"}},
		{"flags beat env and file", file,
			map[string]string{EnvNamespace: "env-ns", EnvContext: "env-ctx"},
			Config{Namespace: "flag-ns", Output: "table"},
			Config{Kubeconfig: "/file/kube", Context: "env-ctx", Namespace: "flag-ns", Output: "table"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Resolve(tc.path, envMap(tc.env), tc.flags)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestResolveRejectsBadOutput(t *testing.T) {
	_, err := Resolve(filepath.Join(t.TempDir(), "x.yaml"), envMap(map[string]string{EnvOutput: "xml"}), Config{})
	if err == nil || !strings.Contains(err.Error(), "xml") {
		t.Fatalf("expected invalid output error, got %v", err)
	}
}

func TestLoadRejectsUnknownKeys(t *testing.T) {
	_, err := Load(writeFile(t, "nmespace: typo\n"))
	if err == nil {
		t.Fatal("expected an error for an unknown key")
	}
}

func TestSaveThenSetRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.yaml")
	c := Defaults()
	if err := c.Set("namespace", "team-a"); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config permissions = %v, want 0600", info.Mode().Perm())
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Namespace != "team-a" || loaded.Output != "table" {
		t.Fatalf("round trip lost data: %+v", loaded)
	}
}

func TestSetValidation(t *testing.T) {
	tests := []struct {
		key, value string
		wantErr    bool
	}{
		{"namespace", "team-a", false},
		{"output", "json", false},
		{"output", "csv", true},
		{"colour", "blue", true},
	}
	for _, tc := range tests {
		c := Defaults()
		if err := c.Set(tc.key, tc.value); (err != nil) != tc.wantErr {
			t.Fatalf("Set(%q,%q) err = %v, wantErr %v", tc.key, tc.value, err, tc.wantErr)
		}
	}
}

func TestDefaultPathHonoursEnv(t *testing.T) {
	t.Setenv(EnvConfigPath, "/custom/config.yaml")
	p, err := DefaultPath()
	if err != nil || p != "/custom/config.yaml" {
		t.Fatalf("DefaultPath() = %q, %v", p, err)
	}
}

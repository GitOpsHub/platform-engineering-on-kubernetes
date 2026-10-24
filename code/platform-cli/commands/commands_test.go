package commands

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynfake "k8s.io/client-go/dynamic/fake"

	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/platform-cli/config"
)

func cr(kind, ns, name string, spec map[string]any, status map[string]any) *unstructured.Unstructured {
	u := &unstructured.Unstructured{Object: map[string]any{"spec": spec}}
	if status != nil {
		u.Object["status"] = status
	}
	u.SetAPIVersion(APIVersion)
	u.SetKind(kind)
	u.SetName(name)
	u.SetNamespace(ns)
	return u
}

func newFakeCluster(objs ...runtime.Object) *dynfake.FakeDynamicClient {
	return dynfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			TenantGVR: "TenantList", EnvironmentGVR: "EnvironmentList", WorkloadGVR: "WorkloadList",
		}, objs...)
}

func seededCluster() *dynfake.FakeDynamicClient {
	return newFakeCluster(
		cr("Tenant", "", "team-a", map[string]any{"displayName": "Team A"},
			map[string]any{"phase": "Ready", "namespaces": []any{"team-a-dev"}}),
		cr("Tenant", "", "team-b", map[string]any{"displayName": "Team B"}, nil),
		cr("Environment", "team-a", "dev", map[string]any{"tenant": "team-a", "type": "dev"}, nil),
		cr("Environment", "team-b", "prod", map[string]any{"tenant": "team-b", "type": "prod"}, nil),
		cr("Workload", "team-a", "checkout", map[string]any{"image": "ghcr.io/acme/checkout:1", "replicas": int64(2)},
			map[string]any{"availableReplicas": int64(2), "phase": "Running"}),
	)
}

// run executes the CLI against cluster. env is the simulated process environment.
func run(t *testing.T, cluster dynamic.Interface, env map[string]string, args ...string) (string, error) {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	var buf bytes.Buffer
	opts := &Options{
		Build:  BuildInfo{Version: "1.2.3", Commit: "abc", BuildTime: "now"},
		Lookup: func(k string) (string, bool) { v, ok := env[k]; return v, ok },
		NewDynamic: func(config.Config) (dynamic.Interface, error) {
			return cluster, nil
		},
	}
	root := NewRootCmd(opts)
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(append([]string{"--config", cfgPath}, args...))
	err := root.Execute()
	return buf.String(), err
}

// expect checks err and output against a table row's expectations.
func expect(t *testing.T, out string, err error, wantOut []string, wantErr string) {
	t.Helper()
	if wantErr != "" {
		if err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Fatalf("err = %v, want containing %q\n%s", err, wantErr, out)
		}
		return
	}
	if err != nil {
		t.Fatalf("unexpected error: %v\n%s", err, out)
	}
	for _, w := range wantOut {
		if !strings.Contains(out, w) {
			t.Fatalf("output missing %q:\n%s", w, out)
		}
	}
}

func TestTenantCommands(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantOut []string
		wantErr string
	}{
		{"list table", []string{"tenant", "list"}, []string{"NAME", "team-a", "Team B", "Ready"}, ""},
		{"list json", []string{"tenant", "list", "-o", "json"}, []string{`"kind": "TenantList"`, `"name": "team-b"`}, ""},
		{"get yaml", []string{"tenant", "get", "team-a", "-o", "yaml"}, []string{"displayName: Team A"}, ""},
		{"get missing", []string{"tenant", "get", "nope"}, nil, `getting Tenant "nope"`},
		{"create", []string{"tenant", "create", "team-c", "--display-name", "Team C", "--owner", "carol,group:devs", "--quota-cpu", "4"},
			[]string{"team-c"}, ""},
		{"create requires display name", []string{"tenant", "create", "team-c"}, nil, "display-name"},
		{"delete", []string{"tenant", "delete", "team-b"}, []string{`Tenant "team-b" deleted`}, ""},
		{"invalid output", []string{"tenant", "list", "-o", "xml"}, nil, "invalid output format"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := run(t, seededCluster(), nil, tc.args...)
			expect(t, out, err, tc.wantOut, tc.wantErr)
		})
	}
}

func TestTenantCreatePersistsSpec(t *testing.T) {
	cluster := seededCluster()
	if out, err := run(t, cluster, nil, "tenant", "create", "team-c", "--display-name", "Team C",
		"--owner", "carol", "--owner", "group:devs", "--quota-cpu", "4", "--quota-pods", "10", "--network-isolation"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	u, err := cluster.Resource(TenantGVR).Get(context.Background(), "team-c", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	owners, _, _ := unstructured.NestedStringSlice(u.Object, "spec", "owners")
	cpu, _, _ := unstructured.NestedString(u.Object, "spec", "quota", "cpu")
	iso, _, _ := unstructured.NestedBool(u.Object, "spec", "networkIsolation")
	if len(owners) != 2 || owners[1] != "group:devs" || cpu != "4" || !iso {
		t.Fatalf("persisted spec wrong: owners=%v cpu=%q isolation=%v", owners, cpu, iso)
	}
	if u.GetAPIVersion() != APIVersion || u.GetKind() != "Tenant" {
		t.Fatalf("bad type meta: %s %s", u.GetAPIVersion(), u.GetKind())
	}
}

func TestEnvironmentCommands(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		args    []string
		wantOut []string
		wantErr string
	}{
		{"list all namespaces", nil, []string{"env", "list"}, []string{"team-a", "team-b", "prod"}, ""},
		{"list one namespace", nil, []string{"env", "list", "-n", "team-b"}, []string{"prod"}, ""},
		{"namespace from env var", map[string]string{config.EnvNamespace: "team-a"}, []string{"env", "list"}, []string{"dev"}, ""},
		{"get needs namespace", nil, []string{"env", "get", "dev"}, nil, "a namespace is required"},
		{"create bad type", nil, []string{"env", "create", "pr-1", "-n", "team-a", "--tenant", "team-a", "--type", "qa"}, nil, "--type must be one of"},
		{"create needs tenant", nil, []string{"env", "create", "pr-1", "-n", "team-a"}, nil, "tenant"},
		{"create preview", nil, []string{"env", "create", "pr-1", "-n", "team-a", "--tenant", "team-a", "--type", "preview", "--ttl", "24h"}, []string{"pr-1", "preview"}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := run(t, seededCluster(), tc.env, tc.args...)
			expect(t, out, err, tc.wantOut, tc.wantErr)
		})
	}
}

func TestWorkloadCreateBuildsSpec(t *testing.T) {
	cluster := seededCluster()
	out, err := run(t, cluster, nil, "workload", "create", "cart", "-n", "team-a",
		"--image", "ghcr.io/acme/cart:2", "--replicas", "3", "--port", "9090",
		"--env", "LOG_LEVEL=debug", "--env", "FEATURE=on=1",
		"--memory-limit", "512Mi", "--cpu-request", "100m",
		"--trait", "ingress", "--trait", `autoscaling={"maxReplicas":10}`,
		"-o", "json")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	u, err := cluster.Resource(WorkloadGVR).Namespace("team-a").Get(context.Background(), "cart", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	env, _, _ := unstructured.NestedSlice(u.Object, "spec", "env")
	if len(env) != 2 || env[1].(map[string]any)["value"] != "on=1" {
		t.Fatalf("env = %v", env)
	}
	traits, _, _ := unstructured.NestedSlice(u.Object, "spec", "traits")
	if len(traits) != 2 || traits[1].(map[string]any)["config"].(map[string]any)["maxReplicas"] != float64(10) {
		t.Fatalf("traits = %v", traits)
	}
	limit, _, _ := unstructured.NestedString(u.Object, "spec", "resources", "limits", "memory")
	req, _, _ := unstructured.NestedString(u.Object, "spec", "resources", "requests", "cpu")
	if limit != "512Mi" || req != "100m" {
		t.Fatalf("resources limit=%q request=%q", limit, req)
	}
	if port, _, _ := unstructured.NestedInt64(u.Object, "spec", "port"); port != 9090 {
		t.Fatalf("port = %d", port)
	}
}

func TestWorkloadCreateRejectsBadInput(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"bad env", []string{"--env", "NOEQUALS"}, "KEY=VALUE"},
		{"bad trait json", []string{"--trait", `autoscaling={nope`}, "JSON object"},
		{"bad port", []string{"--port", "70000"}, "--port"},
		{"missing image", []string{"--image", ""}, "--image is required"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"workload", "create", "cart", "-n", "team-a", "--image", "img"}, tc.args...)
			_, err := run(t, seededCluster(), nil, args...)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestWorkloadListHonoursFlagOverEnv(t *testing.T) {
	env := map[string]string{config.EnvNamespace: "team-a"}
	out, err := run(t, seededCluster(), env, "workload", "list", "-n", "team-b")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "checkout") {
		t.Fatalf("flag -n team-b should win over env namespace team-a:\n%s", out)
	}
	out, err = run(t, seededCluster(), env, "workload", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "checkout") {
		t.Fatalf("env namespace not applied:\n%s", out)
	}
}

func TestConfigSetAndView(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	cluster := seededCluster()
	exec := func(args ...string) (string, error) {
		var buf bytes.Buffer
		root := NewRootCmd(&Options{Lookup: func(string) (string, bool) { return "", false },
			NewDynamic: func(config.Config) (dynamic.Interface, error) { return cluster, nil }})
		root.SetOut(&buf)
		root.SetErr(&buf)
		root.SetArgs(append([]string{"--config", cfgPath}, args...))
		err := root.Execute()
		return buf.String(), err
	}
	if _, err := exec("config", "set", "namespace", "team-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := exec("config", "set", "output", "xml"); err == nil {
		t.Fatal("invalid output must be rejected")
	}
	out, err := exec("config", "view")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "namespace: team-a") || !strings.Contains(out, "output: table") {
		t.Fatalf("view output:\n%s", out)
	}
	out, err = exec("config", "path")
	if err != nil || !strings.Contains(out, cfgPath) {
		t.Fatalf("path = %q, %v", out, err)
	}
}

func TestVersionWorksWithoutCluster(t *testing.T) {
	out, err := run(t, seededCluster(), nil, "version", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"version": "1.2.3"`) {
		t.Fatalf("version output:\n%s", out)
	}
}

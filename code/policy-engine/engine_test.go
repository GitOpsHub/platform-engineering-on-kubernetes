package engine

import (
	"context"
	"strings"
	"testing"
)

func testConfig() Config {
	return Config{AllowedRegistries: []string{"ghcr.io/acme/"}, MaxReplicas: 10}
}

// workload returns a compliant Workload object; tests mutate fresh copies.
func workload() map[string]any {
	return map[string]any{
		"apiVersion": "platform.mycompany.io/v1alpha1",
		"kind":       "Workload",
		"metadata": map[string]any{
			"name": "checkout",
			"labels": map[string]any{
				"platform.mycompany.io/owner": "payments",
				"app.kubernetes.io/name":      "checkout",
			},
		},
		"spec": map[string]any{
			"image":    "ghcr.io/acme/checkout:1.2.3",
			"replicas": 2,
			"resources": map[string]any{
				"limits": map[string]any{"cpu": "500m", "memory": "256Mi"},
			},
		},
	}
}

func TestEvaluate(t *testing.T) {
	eng, err := NewEmbedded(testConfig())
	if err != nil {
		t.Fatal(err)
	}
	mutate := func(f func(o map[string]any)) map[string]any {
		o := workload()
		f(o)
		return o
	}
	spec := func(o map[string]any) map[string]any { return o["spec"].(map[string]any) }
	labels := func(o map[string]any) map[string]any {
		return o["metadata"].(map[string]any)["labels"].(map[string]any)
	}

	tests := []struct {
		name       string
		kind       string
		obj        map[string]any
		wantPolicy []string // policies that must fire, in order
		wantSubstr string   // substring that must appear in the messages
	}{
		{"compliant workload", "Workload", workload(), nil, ""},
		{"missing owner label", "Workload", mutate(func(o map[string]any) { delete(labels(o), "platform.mycompany.io/owner") }),
			[]string{"required-labels"}, "platform.mycompany.io/owner"},
		{"unapproved registry", "Workload", mutate(func(o map[string]any) { spec(o)["image"] = "docker.io/library/nginx" }),
			[]string{"registry-allowlist"}, "not under an allowed registry"},
		{"privileged container", "Workload", mutate(func(o map[string]any) {
			spec(o)["sidecar"] = map[string]any{"securityContext": map[string]any{"privileged": true}}
		}), []string{"no-privileged"}, `at ["spec", "sidecar", "securityContext"]`},
		{"no cpu limit", "Workload", mutate(func(o map[string]any) {
			delete(spec(o)["resources"].(map[string]any)["limits"].(map[string]any), "cpu")
		}), []string{"resource-limits"}, "limits.cpu"},
		{"replicas over ceiling", "Workload", mutate(func(o map[string]any) { spec(o)["replicas"] = 99 }),
			[]string{"resource-limits"}, "maximum is 10"},
		{"several violations are all reported", "Workload", mutate(func(o map[string]any) {
			delete(labels(o), "app.kubernetes.io/name")
			spec(o)["image"] = "quay.io/x/y"
		}), []string{"registry-allowlist", "required-labels"}, ""},
		{"environment needs owner", "Environment", map[string]any{
			"metadata": map[string]any{"name": "pr-1", "labels": map[string]any{}},
			"spec":     map[string]any{"tenant": "payments", "type": "preview"},
		}, []string{"required-labels"}, "Environment"},
		{"tenant with owner passes", "Tenant", map[string]any{
			"metadata": map[string]any{"name": "payments", "labels": map[string]any{"platform.mycompany.io/owner": "payments"}},
		}, nil, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := eng.Evaluate(context.Background(), Request{
				Operation: "CREATE", Kind: tc.kind, Namespace: "payments", Name: "checkout", Object: tc.obj,
			})
			if err != nil {
				t.Fatal(err)
			}
			var policies, msgs []string
			for _, v := range got {
				policies = append(policies, v.Policy)
				msgs = append(msgs, v.Message)
			}
			if !equal(policies, tc.wantPolicy) {
				t.Fatalf("policies = %v, want %v\nmessages: %v", policies, tc.wantPolicy, msgs)
			}
			if tc.wantSubstr != "" && !strings.Contains(strings.Join(msgs, "\n"), tc.wantSubstr) {
				t.Fatalf("messages %v do not contain %q", msgs, tc.wantSubstr)
			}
		})
	}
}

func TestEvaluateUsesConfiguredRegistries(t *testing.T) {
	eng, err := NewEmbedded(Config{AllowedRegistries: []string{"quay.io/partner/"}, MaxReplicas: 3})
	if err != nil {
		t.Fatal(err)
	}
	got, err := eng.Evaluate(context.Background(), Request{Kind: "Workload", Name: "checkout", Object: workload()})
	if err != nil {
		t.Fatal(err)
	}
	// ghcr.io/acme is not in this config; 2 replicas is still under the ceiling of 3.
	if len(got) != 1 || got[0].Policy != "registry-allowlist" {
		t.Fatalf("violations = %+v", got)
	}
}

func TestNewValidatesConfig(t *testing.T) {
	ok := map[string]string{"a.rego": "package platform\n"}
	tests := []struct {
		name string
		cfg  Config
		src  map[string]string
		want string
	}{
		{"no registries fails closed", Config{MaxReplicas: 1}, ok, "allowed registry"},
		{"no replica ceiling", Config{AllowedRegistries: []string{"x/"}}, ok, "MaxReplicas"},
		{"no modules", testConfig(), nil, "no policy modules"},
		{"bad rego", testConfig(), map[string]string{"bad.rego": "package platform\ndeny contains x if {"}, "compiling policies"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.cfg, tc.src)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

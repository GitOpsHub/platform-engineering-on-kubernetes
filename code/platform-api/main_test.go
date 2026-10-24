package main

import (
	"io"
	"log/slog"
	"reflect"
	"testing"

	"k8s.io/client-go/rest"

	mw "github.com/GitOpsHub/platform-engineering-on-kubernetes/code/platform-api/middleware"
)

func TestParseTokens(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    mw.StaticTokens
		wantErr bool
	}{
		{"single", "abc,alice", mw.StaticTokens{"abc": {Subject: "alice"}}, false},
		{"groups and comments", "# comment\n\nabc, alice , admins;devs\nxyz,bob\n",
			mw.StaticTokens{"abc": {Subject: "alice", Groups: []string{"admins", "devs"}}, "xyz": {Subject: "bob"}}, false},
		{"missing subject", "abc", nil, true},
		{"empty", "# nothing\n", nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseTokens(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestOpenAPISpecEmbedded(t *testing.T) {
	if len(openAPISpec) == 0 {
		t.Fatal("openapi.yaml was not embedded")
	}
}

func TestParseFlagsDefaults(t *testing.T) {
	t.Setenv("PLATFORM_API_ADDR", "")
	c := parseFlags([]string{"--log-level=debug", "--cors-origins=https://a.example, https://b.example"})
	if c.Addr != ":8080" || c.LogLevel != "debug" || !c.EnableMetrics || c.EnableProfiling {
		t.Fatalf("unexpected config: %+v", c)
	}
	if got := splitCSV(c.CORSOrigins); !reflect.DeepEqual(got, []string{"https://a.example", "https://b.example"}) {
		t.Fatalf("cors origins = %v", got)
	}
}

func TestImpersonationConfigScopesToCaller(t *testing.T) {
	base := &rest.Config{Host: "https://kube.example", BearerToken: "server-sa-token"}
	p := &mw.Principal{Subject: "alice", Groups: []string{"team-a"}}

	got, dropped := impersonationConfig(base, p, []string{"team-a"})
	if got.Impersonate.UserName != "alice" || !reflect.DeepEqual(got.Impersonate.Groups, []string{"team-a"}) {
		t.Fatalf("impersonation = %+v", got.Impersonate)
	}
	if len(dropped) != 0 {
		t.Fatalf("dropped = %v, want none", dropped)
	}
	if got.BearerToken != "server-sa-token" || got.Host != base.Host {
		t.Fatal("impersonation config must keep the server's connection settings")
	}
	if base.Impersonate.UserName != "" {
		t.Fatal("base config must not be mutated")
	}
}

func TestImpersonationDropsGroupsOffAllowlist(t *testing.T) {
	base := &rest.Config{Host: "https://kube.example"}
	p := &mw.Principal{Subject: "alice", Groups: []string{"team-a", "engineering-all", "platform-admins"}}

	got, dropped := impersonationConfig(base, p, []string{"team-a", "platform-admins"})
	if !reflect.DeepEqual(got.Impersonate.Groups, []string{"team-a", "platform-admins"}) {
		t.Fatalf("kept = %v", got.Impersonate.Groups)
	}
	if !reflect.DeepEqual(dropped, []string{"engineering-all"}) {
		t.Fatalf("dropped = %v, want [engineering-all]", dropped)
	}
}

func TestImpersonationNeverForwardsSystemGroups(t *testing.T) {
	base := &rest.Config{Host: "https://kube.example"}
	// Even if the allowlist somehow named a system group, it must not be forwarded.
	p := &mw.Principal{Subject: "alice", Groups: []string{"system:masters", "team-a"}}
	got, dropped := impersonationConfig(base, p, []string{"system:masters", "team-a"})
	if !reflect.DeepEqual(got.Impersonate.Groups, []string{"team-a"}) {
		t.Fatalf("kept = %v", got.Impersonate.Groups)
	}
	if !reflect.DeepEqual(dropped, []string{"system:masters"}) {
		t.Fatalf("dropped = %v", dropped)
	}
}

func TestImpersonatingClientRefusesReservedIdentities(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	f := impersonatingClientFor(&rest.Config{Host: "https://kube.example"}, []string{"team-a"}, logger)
	tests := []struct {
		name string
		p    *mw.Principal
	}{
		{"empty subject", &mw.Principal{}},
		{"nil principal", nil},
		{"system subject", &mw.Principal{Subject: "system:serviceaccount:kube-system:x", Groups: []string{"team-a"}}},
		{"system:masters group", &mw.Principal{Subject: "alice", Groups: []string{"system:masters"}}},
		{"system:nodes group", &mw.Principal{Subject: "alice", Groups: []string{"system:nodes"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := f(tc.p); err == nil {
				t.Fatal("expected refusal")
			}
		})
	}
}

func TestBuildAllowlist(t *testing.T) {
	got, err := buildAllowlist("team-a, team-b")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"team-a", "team-b"}) {
		t.Fatalf("allowlist = %v", got)
	}
	if _, err := buildAllowlist("team-a,system:masters"); err == nil {
		t.Fatal("a system: entry must be refused at start-up")
	}
}

package main

import (
	"reflect"
	"testing"

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

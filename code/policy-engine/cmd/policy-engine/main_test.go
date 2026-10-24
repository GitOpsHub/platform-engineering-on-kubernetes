package main

import "testing"

func TestParseFlagsDefaults(t *testing.T) {
	o, err := parseFlags(nil)
	if err != nil {
		t.Fatal(err)
	}
	if o.addr != ":8443" || o.metricsAddr != ":8080" || o.maxReplicas != 10 || o.allowedRegistries != "ghcr.io/acme/" {
		t.Fatalf("defaults = %+v", o)
	}
}

func TestParseFlagsRejectsUnknown(t *testing.T) {
	if _, err := parseFlags([]string{"--nope"}); err == nil {
		t.Fatal("unknown flag must be an error")
	}
}

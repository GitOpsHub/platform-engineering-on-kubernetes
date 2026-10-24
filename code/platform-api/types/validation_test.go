package types

import (
	"errors"
	"strings"
	"testing"
)

func int32p(v int32) *int32 { return &v }

func TestValidate(t *testing.T) {
	validTenant := func() *Tenant {
		return &Tenant{
			Metadata: ObjectMeta{Name: "team-a"},
			Spec: TenantSpec{
				DisplayName: "Team A",
				Owners:      []string{"alice@example.com"},
				Namespaces:  []string{"team-a-dev"},
				Quota:       TenantQuota{CPU: "10", Memory: "20Gi", Storage: "100Gi", Pods: 50},
			},
		}
	}
	validEnv := func() *Environment {
		return &Environment{
			Metadata: ObjectMeta{Name: "pr-42", Namespace: "team-a"},
			Spec:     EnvironmentSpec{Tenant: "team-a", Type: EnvironmentPreview, TTL: "72h"},
		}
	}
	validWorkload := func() *Workload {
		return &Workload{
			Metadata: ObjectMeta{Name: "checkout", Namespace: "team-a"},
			Spec: WorkloadSpec{
				Image: "ghcr.io/acme/checkout:1.0.0", Replicas: int32p(2), Port: 8080,
				Env:       []EnvVar{{Name: "LOG_LEVEL", Value: "info"}},
				Resources: ResourceRequirements{Requests: map[string]string{"cpu": "100m", "memory": "128Mi"}},
				Traits:    []Trait{{Type: TraitIngress, Config: map[string]any{"host": "checkout.example.com"}}},
			},
		}
	}

	tests := []struct {
		name    string
		obj     Validator
		wantErr string // substring; empty means valid
	}{
		{"tenant valid", validTenant(), ""},
		{"tenant bad name", func() Validator { t := validTenant(); t.Metadata.Name = "Team_A"; return t }(), "metadata.name"},
		{"tenant namespaced", func() Validator { t := validTenant(); t.Metadata.Namespace = "x"; return t }(), "cluster-scoped"},
		{"tenant no owners", func() Validator { t := validTenant(); t.Spec.Owners = nil; return t }(), "spec.owners"},
		{"tenant no display name", func() Validator { t := validTenant(); t.Spec.DisplayName = " "; return t }(), "spec.displayName"},
		{"tenant bad quota", func() Validator { t := validTenant(); t.Spec.Quota.Memory = "lots"; return t }(), "spec.quota.memory"},
		{"tenant bad namespace", func() Validator { t := validTenant(); t.Spec.Namespaces = []string{"-bad"}; return t }(), "spec.namespaces[0]"},
		{"env valid", validEnv(), ""},
		{"env bad type", func() Validator { e := validEnv(); e.Spec.Type = "qa"; return e }(), "spec.type"},
		{"env missing tenant", func() Validator { e := validEnv(); e.Spec.Tenant = ""; return e }(), "spec.tenant is required"},
		{"env bad ttl", func() Validator { e := validEnv(); e.Spec.TTL = "3 days"; return e }(), "spec.ttl"},
		{"env prod with ttl", func() Validator { e := validEnv(); e.Spec.Type = EnvironmentProd; return e }(), "not allowed for prod"},
		{"workload valid", validWorkload(), ""},
		{"workload no image", func() Validator { w := validWorkload(); w.Spec.Image = ""; return w }(), "spec.image"},
		{"workload negative replicas", func() Validator { w := validWorkload(); w.Spec.Replicas = int32p(-1); return w }(), "spec.replicas"},
		{"workload bad port", func() Validator { w := validWorkload(); w.Spec.Port = 70000; return w }(), "spec.port"},
		{"workload bad env", func() Validator { w := validWorkload(); w.Spec.Env = []EnvVar{{Name: "1BAD"}}; return w }(), "spec.env[0]"},
		{"workload unknown trait", func() Validator { w := validWorkload(); w.Spec.Traits = []Trait{{Type: "canary"}}; return w }(), "spec.traits[0]"},
		{"workload duplicate trait", func() Validator {
			w := validWorkload()
			w.Spec.Traits = []Trait{{Type: TraitPDB}, {Type: TraitPDB}}
			return w
		}(), "duplicated"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.obj.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected valid, got %v", err)
				}
				return
			}
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("expected *ValidationError, got %v", err)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err, tc.wantErr)
			}
		})
	}
}

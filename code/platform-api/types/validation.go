package types

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Validator is implemented by request bodies that can check themselves.
type Validator interface {
	Validate() error
}

// ValidationError aggregates field-level problems.
type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string {
	return "validation failed: " + strings.Join(e.Problems, "; ")
}

type problems []string

func (p *problems) addf(format string, args ...any) { *p = append(*p, fmt.Sprintf(format, args...)) }

func (p problems) err() error {
	if len(p) == 0 {
		return nil
	}
	return &ValidationError{Problems: p}
}

var (
	dns1123Label = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	quantity     = regexp.MustCompile(`^([0-9]+(\.[0-9]+)?)(m|k|M|G|T|P|E|Ki|Mi|Gi|Ti|Pi|Ei)?$`)
	envName      = regexp.MustCompile(`^[-._a-zA-Z][-._a-zA-Z0-9]*$`)
)

// IsDNSLabel reports whether s is a valid RFC 1123 label (max 63 chars).
func IsDNSLabel(s string) bool {
	return len(s) > 0 && len(s) <= 63 && dns1123Label.MatchString(s)
}

func checkName(p *problems, field, name string) {
	if !IsDNSLabel(name) {
		p.addf("%s %q must be a lowercase RFC 1123 label (a-z, 0-9, '-', max 63 chars)", field, name)
	}
}

func checkQuantity(p *problems, field, q string) {
	if q != "" && !quantity.MatchString(q) {
		p.addf("%s %q is not a valid resource quantity", field, q)
	}
}

// Validate implements Validator.
func (t *Tenant) Validate() error {
	var p problems
	checkName(&p, "metadata.name", t.Metadata.Name)
	if t.Metadata.Namespace != "" {
		p.addf("metadata.namespace must be empty: Tenant is cluster-scoped")
	}
	if strings.TrimSpace(t.Spec.DisplayName) == "" {
		p.addf("spec.displayName is required")
	}
	if len(t.Spec.Owners) == 0 {
		p.addf("spec.owners must contain at least one owner")
	}
	for i, ns := range t.Spec.Namespaces {
		checkName(&p, fmt.Sprintf("spec.namespaces[%d]", i), ns)
	}
	checkQuantity(&p, "spec.quota.cpu", t.Spec.Quota.CPU)
	checkQuantity(&p, "spec.quota.memory", t.Spec.Quota.Memory)
	checkQuantity(&p, "spec.quota.storage", t.Spec.Quota.Storage)
	if t.Spec.Quota.Pods < 0 {
		p.addf("spec.quota.pods must be >= 0")
	}
	return p.err()
}

// Validate implements Validator.
func (e *Environment) Validate() error {
	var p problems
	checkName(&p, "metadata.name", e.Metadata.Name)
	if e.Spec.Tenant == "" {
		p.addf("spec.tenant is required")
	} else {
		checkName(&p, "spec.tenant", e.Spec.Tenant)
	}
	switch e.Spec.Type {
	case EnvironmentDev, EnvironmentStaging, EnvironmentProd, EnvironmentPreview:
	default:
		p.addf("spec.type %q must be one of dev, staging, prod, preview", e.Spec.Type)
	}
	if e.Spec.TTL != "" {
		d, err := time.ParseDuration(e.Spec.TTL)
		if err != nil || d <= 0 {
			p.addf("spec.ttl %q must be a positive Go duration (e.g. 72h)", e.Spec.TTL)
		}
		if e.Spec.Type == EnvironmentProd {
			p.addf("spec.ttl is not allowed for prod environments")
		}
	}
	return p.err()
}

// Validate implements Validator.
func (w *Workload) Validate() error {
	var p problems
	checkName(&p, "metadata.name", w.Metadata.Name)
	if strings.TrimSpace(w.Spec.Image) == "" {
		p.addf("spec.image is required")
	}
	if w.Spec.Replicas != nil && *w.Spec.Replicas < 0 {
		p.addf("spec.replicas must be >= 0")
	}
	if w.Spec.Port < 0 || w.Spec.Port > 65535 {
		p.addf("spec.port %d must be between 1 and 65535", w.Spec.Port)
	}
	for i, ev := range w.Spec.Env {
		if !envName.MatchString(ev.Name) {
			p.addf("spec.env[%d].name %q is not a valid environment variable name", i, ev.Name)
		}
	}
	for k, v := range w.Spec.Resources.Requests {
		checkQuantity(&p, "spec.resources.requests."+k, v)
	}
	for k, v := range w.Spec.Resources.Limits {
		checkQuantity(&p, "spec.resources.limits."+k, v)
	}
	seen := map[string]bool{}
	for i, tr := range w.Spec.Traits {
		switch tr.Type {
		case TraitIngress, TraitAutoscaling, TraitPDB:
		default:
			p.addf("spec.traits[%d].type %q must be one of ingress, autoscaling, pdb", i, tr.Type)
		}
		if seen[tr.Type] {
			p.addf("spec.traits[%d].type %q is duplicated", i, tr.Type)
		}
		seen[tr.Type] = true
	}
	return p.err()
}

// Package engine evaluates platform policies (Rego, via OPA's Go API) against
// Kubernetes objects and reports every violation.
package engine

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/open-policy-agent/opa/v1/rego"

	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/policy-engine/policies"
)

// Query is the entry point. Every policy file adds objects to this set.
const Query = "data.platform.deny"

// Config holds the parameters policies read through input.parameters.
type Config struct {
	// AllowedRegistries are image name prefixes Workloads may use, for example "ghcr.io/acme/".
	AllowedRegistries []string
	// MaxReplicas is the replica ceiling for Workloads.
	MaxReplicas int
}

// Request is one object to evaluate.
type Request struct {
	Operation string         // CREATE, UPDATE, ...
	Kind      string         // e.g. Workload
	Namespace string         // may be empty for cluster-scoped kinds
	Name      string         // object name
	Object    map[string]any // the object as decoded JSON
}

// Violation is one failed rule.
type Violation struct {
	Policy  string `json:"policy"`
	Message string `json:"message"`
}

// Engine is a compiled, reusable policy set. It is safe for concurrent use.
type Engine struct {
	query rego.PreparedEvalQuery
	cfg   Config
}

// New compiles the given Rego modules (file name -> source).
func New(cfg Config, modules map[string]string) (*Engine, error) {
	if len(cfg.AllowedRegistries) == 0 {
		return nil, errors.New("engine: at least one allowed registry is required")
	}
	if cfg.MaxReplicas < 1 {
		return nil, errors.New("engine: MaxReplicas must be at least 1")
	}
	if len(modules) == 0 {
		return nil, errors.New("engine: no policy modules")
	}
	names := make([]string, 0, len(modules))
	for n := range modules {
		names = append(names, n)
	}
	sort.Strings(names)

	opts := []func(*rego.Rego){rego.Query(Query)}
	for _, n := range names {
		opts = append(opts, rego.Module(n, modules[n]))
	}
	pq, err := rego.New(opts...).PrepareForEval(context.Background())
	if err != nil {
		return nil, fmt.Errorf("compiling policies: %w", err)
	}
	return &Engine{query: pq, cfg: cfg}, nil
}

// NewEmbedded builds an engine from the policies compiled into the binary.
func NewEmbedded(cfg Config) (*Engine, error) {
	mods, err := policies.Modules()
	if err != nil {
		return nil, err
	}
	return New(cfg, mods)
}

// Evaluate returns every violation for req, sorted for stable output. An empty
// slice means the object is compliant.
func (e *Engine) Evaluate(ctx context.Context, req Request) ([]Violation, error) {
	registries := make([]any, 0, len(e.cfg.AllowedRegistries))
	for _, r := range e.cfg.AllowedRegistries {
		registries = append(registries, r)
	}
	input := map[string]any{
		"operation": req.Operation,
		"kind":      req.Kind,
		"namespace": req.Namespace,
		"name":      req.Name,
		"object":    req.Object,
		"parameters": map[string]any{
			"allowedRegistries": registries,
			"maxReplicas":       e.cfg.MaxReplicas,
		},
	}
	rs, err := e.query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return nil, fmt.Errorf("evaluating policies: %w", err)
	}
	var out []Violation
	for _, result := range rs {
		for _, expr := range result.Expressions {
			items, ok := expr.Value.([]any)
			if !ok {
				return nil, fmt.Errorf("unexpected policy result type %T", expr.Value)
			}
			for _, item := range items {
				m, ok := item.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("unexpected violation type %T", item)
				}
				policy, _ := m["policy"].(string)
				msg, _ := m["message"].(string)
				out = append(out, Violation{Policy: policy, Message: msg})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Policy != out[j].Policy {
			return out[i].Policy < out[j].Policy
		}
		return out[i].Message < out[j].Message
	})
	return out, nil
}

package commands

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// workloadAccess describes Workload (namespaced).
var workloadAccess = resourceAccess{
	kind:       "Workload",
	gvr:        WorkloadGVR,
	namespaced: true,
	cols: columns{
		headers: []string{"NAMESPACE", "NAME", "IMAGE", "REPLICAS", "AVAILABLE", "PHASE", "URL"},
		row: func(u *unstructured.Unstructured) []string {
			return []string{
				u.GetNamespace(),
				u.GetName(),
				dash(str(u, "spec", "image")),
				dash(int64Field(u, "spec", "replicas")),
				dash(int64Field(u, "status", "availableReplicas")),
				dash(str(u, "status", "phase")),
				dash(str(u, "status", "url")),
			}
		},
	},
}

// newWorkloadCmd builds `platform-cli workload ...`.
func newWorkloadCmd(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "workload",
		Aliases: []string{"workloads", "wl"},
		Short:   "Manage Workloads (namespaced)",
		Long:    "Workloads are listed across all namespaces unless -n/--namespace is set.",
	}
	var selector string
	addCommonSubcommands(cmd, opts, workloadAccess, &selector)
	cmd.AddCommand(newWorkloadCreateCmd(opts))
	return cmd
}

func newWorkloadCreateCmd(opts *Options) *cobra.Command {
	var (
		image, cpuReq, memReq, cpuLim, memLim string
		replicas, port                        int64
		env, traits                           []string
	)
	cmd := &cobra.Command{
		Use:   "create NAME --image IMAGE [flags]",
		Short: "Create a Workload in the namespace given by -n",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := nameArg(args)
			if err != nil {
				return err
			}
			if image == "" {
				return fmt.Errorf("--image is required")
			}
			if replicas < 0 {
				return fmt.Errorf("--replicas must be >= 0")
			}
			if port < 1 || port > 65535 {
				return fmt.Errorf("--port must be between 1 and 65535")
			}
			ns, err := opts.requireNamespace()
			if err != nil {
				return err
			}
			spec := map[string]any{"image": image, "replicas": replicas, "port": port}
			if len(env) > 0 {
				spec["env"], err = parseEnv(env)
				if err != nil {
					return err
				}
			}
			if res := resourcesSpec(cpuReq, memReq, cpuLim, memLim); res != nil {
				spec["resources"] = res
			}
			if len(traits) > 0 {
				spec["traits"], err = parseTraits(traits)
				if err != nil {
					return err
				}
			}
			c, err := opts.dynamicClient()
			if err != nil {
				return err
			}
			created, err := workloadAccess.create(cmd.Context(), c, ns, workloadAccess.newObject(name, ns, spec))
			if err != nil {
				return err
			}
			p, err := opts.printer(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			return workloadAccess.renderOne(p, created)
		},
	}
	f := cmd.Flags()
	f.StringVar(&image, "image", "", "Container image (required)")
	f.Int64Var(&replicas, "replicas", 1, "Desired replica count")
	f.Int64Var(&port, "port", 8080, "Container port")
	f.StringArrayVar(&env, "env", nil, "Environment variable KEY=VALUE (repeatable)")
	f.StringVar(&cpuReq, "cpu-request", "", "CPU request, e.g. 100m")
	f.StringVar(&memReq, "memory-request", "", "Memory request, e.g. 128Mi")
	f.StringVar(&cpuLim, "cpu-limit", "", "CPU limit, e.g. 500m")
	f.StringVar(&memLim, "memory-limit", "", "Memory limit, e.g. 512Mi")
	f.StringArrayVar(&traits, "trait", nil, "Trait as TYPE or TYPE=JSON-CONFIG, e.g. ingress or autoscaling={\"maxReplicas\":10} (repeatable)")
	_ = cmd.MarkFlagRequired("image")
	return cmd
}

// parseEnv turns KEY=VALUE pairs into the Workload env list of {name, value}.
func parseEnv(pairs []string) ([]any, error) {
	out := make([]any, 0, len(pairs))
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("--env %q: want KEY=VALUE", p)
		}
		out = append(out, map[string]any{"name": k, "value": v})
	}
	return out, nil
}

// parseTraits turns TYPE or TYPE=JSON into the Workload traits list.
func parseTraits(specs []string) ([]any, error) {
	out := make([]any, 0, len(specs))
	for _, s := range specs {
		typ, raw, hasCfg := strings.Cut(s, "=")
		if typ == "" {
			return nil, fmt.Errorf("--trait %q: missing type", s)
		}
		t := map[string]any{"type": typ}
		if hasCfg {
			var cfg map[string]any
			if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
				return nil, fmt.Errorf("--trait %q: config must be a JSON object: %w", s, err)
			}
			t["config"] = cfg
		}
		out = append(out, t)
	}
	return out, nil
}

// resourcesSpec builds {requests:{...}, limits:{...}} from the given flags, or
// nil when none are set.
func resourcesSpec(cpuReq, memReq, cpuLim, memLim string) map[string]any {
	requests := map[string]any{}
	limits := map[string]any{}
	if cpuReq != "" {
		requests["cpu"] = cpuReq
	}
	if memReq != "" {
		requests["memory"] = memReq
	}
	if cpuLim != "" {
		limits["cpu"] = cpuLim
	}
	if memLim != "" {
		limits["memory"] = memLim
	}
	if len(requests) == 0 && len(limits) == 0 {
		return nil
	}
	res := map[string]any{}
	if len(requests) > 0 {
		res["requests"] = requests
	}
	if len(limits) > 0 {
		res["limits"] = limits
	}
	return res
}

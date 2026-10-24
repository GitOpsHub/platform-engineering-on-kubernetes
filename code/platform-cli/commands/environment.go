package commands

import (
	"fmt"
	"slices"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// environmentTypes are the values accepted for Environment.spec.type.
var environmentTypes = []string{"dev", "staging", "prod", "preview"}

// environmentAccess describes Environment (namespaced).
var environmentAccess = resourceAccess{
	kind:       "Environment",
	gvr:        EnvironmentGVR,
	namespaced: true,
	cols: columns{
		headers: []string{"NAMESPACE", "NAME", "TENANT", "TYPE", "PHASE", "URL", "EXPIRES"},
		row: func(u *unstructured.Unstructured) []string {
			return []string{
				u.GetNamespace(),
				u.GetName(),
				dash(str(u, "spec", "tenant")),
				dash(str(u, "spec", "type")),
				dash(str(u, "status", "phase")),
				dash(str(u, "status", "url")),
				dash(str(u, "status", "expiresAt")),
			}
		},
	},
}

// newEnvironmentCmd builds `platform-cli env ...`.
func newEnvironmentCmd(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "env",
		Aliases: []string{"environment", "environments", "envs"},
		Short:   "Manage Environments (namespaced)",
		Long:    "Environments are listed across all namespaces unless -n/--namespace is set.",
	}
	var selector string
	addCommonSubcommands(cmd, opts, environmentAccess, &selector)
	cmd.AddCommand(newEnvironmentCreateCmd(opts))
	return cmd
}

func newEnvironmentCreateCmd(opts *Options) *cobra.Command {
	var tenant, envType, ttl, template, cluster string
	cmd := &cobra.Command{
		Use:   "create NAME --tenant TENANT [flags]",
		Short: "Create an Environment in the namespace given by -n",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := nameArg(args)
			if err != nil {
				return err
			}
			if tenant == "" {
				return fmt.Errorf("--tenant is required")
			}
			if !slices.Contains(environmentTypes, envType) {
				return fmt.Errorf("--type must be one of %v", environmentTypes)
			}
			ns, err := opts.requireNamespace()
			if err != nil {
				return err
			}
			spec := map[string]any{"tenant": tenant, "type": envType}
			if ttl != "" {
				spec["ttl"] = ttl
			}
			if template != "" {
				spec["template"] = template
			}
			if cluster != "" {
				spec["cluster"] = cluster
			}
			c, err := opts.dynamicClient()
			if err != nil {
				return err
			}
			created, err := environmentAccess.create(cmd.Context(), c, ns, environmentAccess.newObject(name, ns, spec))
			if err != nil {
				return err
			}
			p, err := opts.printer(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			return environmentAccess.renderOne(p, created)
		},
	}
	f := cmd.Flags()
	f.StringVar(&tenant, "tenant", "", "Owning tenant (required)")
	f.StringVar(&envType, "type", "dev", "Environment type: dev, staging, prod or preview")
	f.StringVar(&ttl, "ttl", "", "Time to live, e.g. 24h (typically for preview environments)")
	f.StringVar(&template, "template", "", "Environment template to instantiate")
	f.StringVar(&cluster, "cluster", "", "Target cluster name")
	_ = cmd.MarkFlagRequired("tenant")
	return cmd
}

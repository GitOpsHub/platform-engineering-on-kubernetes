package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// tenantAccess describes Tenant (cluster-scoped).
var tenantAccess = resourceAccess{
	kind: "Tenant",
	gvr:  TenantGVR,
	cols: columns{
		headers: []string{"NAME", "DISPLAY NAME", "PHASE", "NAMESPACES", "AGE"},
		row: func(u *unstructured.Unstructured) []string {
			return []string{
				u.GetName(),
				dash(str(u, "spec", "displayName")),
				dash(str(u, "status", "phase")),
				fmt.Sprint(len(strSlice(u, "status", "namespaces"))),
				ageOf(u),
			}
		},
	},
}

// newTenantCmd builds `platform-cli tenant ...`.
func newTenantCmd(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "tenant",
		Aliases: []string{"tenants", "tn"},
		Short:   "Manage Tenants (cluster-scoped)",
	}
	var selector string
	addCommonSubcommands(cmd, opts, tenantAccess, &selector)
	cmd.AddCommand(newTenantCreateCmd(opts))
	return cmd
}

func newTenantCreateCmd(opts *Options) *cobra.Command {
	var (
		displayName string
		owners      []string
		namespaces  []string
		cpu, memory string
		storage     string
		pods        int64
		isolation   bool
	)
	cmd := &cobra.Command{
		Use:   "create NAME --display-name NAME [flags]",
		Short: "Create a Tenant (platform-admin group only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := nameArg(args)
			if err != nil {
				return err
			}
			if strings.TrimSpace(displayName) == "" {
				return fmt.Errorf("--display-name is required")
			}
			spec := map[string]any{"displayName": displayName}
			if len(owners) > 0 {
				spec["owners"] = toAny(owners)
			}
			if len(namespaces) > 0 {
				spec["namespaces"] = toAny(namespaces)
			}
			quota := map[string]any{}
			if cpu != "" {
				quota["cpu"] = cpu
			}
			if memory != "" {
				quota["memory"] = memory
			}
			if storage != "" {
				quota["storage"] = storage
			}
			if pods > 0 {
				quota["pods"] = pods
			}
			if len(quota) > 0 {
				spec["quota"] = quota
			}
			if isolation {
				spec["networkIsolation"] = true
			}
			c, err := opts.dynamicClient()
			if err != nil {
				return err
			}
			created, err := tenantAccess.create(cmd.Context(), c, "", tenantAccess.newObject(name, "", spec))
			if err != nil {
				return err
			}
			p, err := opts.printer(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			return tenantAccess.renderOne(p, created)
		},
	}
	f := cmd.Flags()
	f.StringVar(&displayName, "display-name", "", "Human-friendly tenant name (required)")
	f.StringSliceVar(&owners, "owner", nil, "Owner user, or group prefixed with group: (repeatable)")
	f.StringSliceVar(&namespaces, "namespaces", nil, "Namespaces to provision for the tenant (comma-separated or repeated)")
	f.StringVar(&cpu, "quota-cpu", "", "Aggregate CPU quota per namespace, e.g. 4")
	f.StringVar(&memory, "quota-memory", "", "Aggregate memory quota per namespace, e.g. 8Gi")
	f.StringVar(&storage, "quota-storage", "", "Aggregate storage quota per namespace, e.g. 100Gi")
	f.Int64Var(&pods, "quota-pods", 0, "Maximum pods per namespace")
	f.BoolVar(&isolation, "network-isolation", false, "Install default-deny NetworkPolicies")
	_ = cmd.MarkFlagRequired("display-name")
	return cmd
}

func toAny(in []string) []any {
	out := make([]any, 0, len(in))
	for _, s := range in {
		out = append(out, s)
	}
	return out
}

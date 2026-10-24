// Package commands implements the platform-cli command tree.
package commands

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/client-go/dynamic"

	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/platform-cli/config"
	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/platform-cli/output"
)

// BuildInfo is injected at build time with -ldflags.
type BuildInfo struct {
	Version   string
	Commit    string
	BuildTime string
}

// Options carries process-wide state shared by all commands. Tests replace
// NewDynamic to run commands against a fake cluster.
type Options struct {
	Out    io.Writer
	Err    io.Writer
	Build  BuildInfo
	Lookup func(string) (string, bool) // environment lookup; os.LookupEnv when nil

	// NewDynamic builds the dynamic client for the resolved configuration.
	NewDynamic func(c config.Config) (dynamic.Interface, error)

	// Resolved is set by PersistentPreRunE: the effective configuration.
	Resolved config.Config
	// ConfigPath is the file that holds persistent settings.
	ConfigPath string

	client dynamic.Interface
}

// flagValues holds the persistent flags. Each is applied only when the user
// set it explicitly, so unset flags do not mask the file or the environment.
type flagValues struct {
	configPath, kubeconfig, context, namespace, output string
}

// NewRootCmd builds the full command tree.
func NewRootCmd(opts *Options) *cobra.Command {
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	if opts.Err == nil {
		opts.Err = os.Stderr
	}
	if opts.Lookup == nil {
		opts.Lookup = os.LookupEnv
	}
	if opts.NewDynamic == nil {
		opts.NewDynamic = newDynamicClient
	}

	var fv flagValues
	root := &cobra.Command{
		Use:           "platform-cli",
		Short:         "Command-line interface for the Internal Developer Platform",
		Long:          "platform-cli manages Tenants, Environments and Workloads (platform.mycompany.io/v1alpha1).\n\nSettings resolve as: flags > environment (PLATFORM_*) > ~/.platform/config.yaml.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Name() == "help" {
				return nil
			}
			err := opts.resolve(cmd, fv)
			if err != nil && cmd.Name() == "version" {
				// version must work even when the config file is broken.
				return nil
			}
			return err
		},
	}
	pf := root.PersistentFlags()
	pf.StringVar(&fv.configPath, "config", "", "Config file (default ~/.platform/config.yaml or $PLATFORM_CONFIG)")
	pf.StringVar(&fv.kubeconfig, "kubeconfig", "", "Path to kubeconfig ($PLATFORM_KUBECONFIG, else $KUBECONFIG)")
	pf.StringVar(&fv.context, "context", "", "Kubeconfig context to use ($PLATFORM_CONTEXT)")
	pf.StringVarP(&fv.namespace, "namespace", "n", "", "Namespace for namespaced resources ($PLATFORM_NAMESPACE)")
	pf.StringVarP(&fv.output, "output", "o", "", "Output format: table, json or yaml ($PLATFORM_OUTPUT, default table)")

	root.AddCommand(newTenantCmd(opts), newEnvironmentCmd(opts), newWorkloadCmd(opts),
		newConfigCmd(opts), newVersionCmd(opts))
	return root
}

// resolve computes the effective configuration. It must run before any command
// that talks to the cluster or reads settings.
func (o *Options) resolve(cmd *cobra.Command, fv flagValues) error {
	pf := cmd.Root().PersistentFlags()
	changed := func(name, value string) string {
		if pf.Changed(name) {
			return value
		}
		return ""
	}
	flags := config.Config{
		Kubeconfig: changed("kubeconfig", fv.kubeconfig),
		Context:    changed("context", fv.context),
		Namespace:  changed("namespace", fv.namespace),
		Output:     changed("output", fv.output),
	}
	path := fv.configPath
	if path == "" {
		p, err := config.DefaultPath()
		if err != nil {
			return err
		}
		path = p
	}
	eff, err := config.Resolve(path, o.Lookup, flags)
	if err != nil {
		return err
	}
	o.Resolved = eff
	o.ConfigPath = path
	return nil
}

// dynamicClient returns the (lazily built) client for the resolved config.
func (o *Options) dynamicClient() (dynamic.Interface, error) {
	if o.client != nil {
		return o.client, nil
	}
	c, err := o.NewDynamic(o.Resolved)
	if err != nil {
		return nil, fmt.Errorf("connecting to cluster: %w", err)
	}
	o.client = c
	return c, nil
}

// printer returns a Printer for the resolved output format.
func (o *Options) printer(w io.Writer) (output.Printer, error) {
	f, err := output.ParseFormat(o.Resolved.Output)
	if err != nil {
		return output.Printer{}, err
	}
	return output.Printer{Out: w, Format: f}, nil
}

// requireNamespace returns the namespace for a namespaced get/create/delete.
func (o *Options) requireNamespace() (string, error) {
	if o.Resolved.Namespace == "" {
		return "", fmt.Errorf("a namespace is required: pass -n/--namespace or set namespace in the config")
	}
	return o.Resolved.Namespace, nil
}

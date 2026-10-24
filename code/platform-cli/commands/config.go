package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/platform-cli/config"
	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/platform-cli/output"
)

// newConfigCmd builds `platform-cli config ...`.
func newConfigCmd(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "View and edit platform-cli settings",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "view",
		Short: "Show the effective configuration (flags > env > file)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			f, err := output.ParseFormat(opts.Resolved.Output)
			if err != nil || f == output.Table {
				f = output.YAML // a table of settings is less useful than YAML
			}
			return output.Printer{Out: cmd.OutOrStdout(), Format: f}.Render(opts.Resolved, nil, nil)
		},
	}, &cobra.Command{
		Use:   "path",
		Short: "Print the config file path",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), opts.ConfigPath)
			return err
		},
	}, &cobra.Command{
		Use:   "set KEY VALUE",
		Short: fmt.Sprintf("Persist a setting to the config file (keys: %v)", config.Keys),
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Edit only the file: flags and environment must not leak into it.
			file, err := config.Load(opts.ConfigPath)
			if err != nil {
				return err
			}
			if err := file.Set(args[0], args[1]); err != nil {
				return err
			}
			if err := config.Save(opts.ConfigPath, file); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "set %s=%s in %s\n", args[0], args[1], opts.ConfigPath)
			return err
		},
	})
	return cmd
}

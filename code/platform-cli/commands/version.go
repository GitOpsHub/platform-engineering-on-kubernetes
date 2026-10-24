package commands

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/platform-cli/output"
)

// newVersionCmd builds `platform-cli version`. It does not read configuration,
// so it works even when the config file is broken.
func newVersionCmd(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := map[string]string{
				"version":   opts.Build.Version,
				"commit":    opts.Build.Commit,
				"buildTime": opts.Build.BuildTime,
				"goVersion": runtime.Version(),
				"platform":  runtime.GOOS + "/" + runtime.GOARCH,
			}
			format := opts.Resolved.Output
			if format == "" {
				format = "table"
			}
			f, err := output.ParseFormat(format)
			if err != nil {
				f = output.Table
			}
			if f == output.Table {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "platform-cli %s (commit %s, built %s, %s, %s)\n",
					info["version"], info["commit"], info["buildTime"], info["goVersion"], info["platform"])
				return err
			}
			return output.Printer{Out: cmd.OutOrStdout(), Format: f}.Render(info, nil, nil)
		},
	}
}

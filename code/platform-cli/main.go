// Command platform-cli is the command-line interface for the Internal Developer
// Platform. It manages Tenants, Environments and Workloads
// (platform.mycompany.io/v1alpha1) through the Kubernetes API, so RBAC applies
// to the user running it.
package main

import (
	"fmt"
	"os"

	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/platform-cli/commands"
)

// Injected at build time with -ldflags "-X main.version=... -X main.commit=... -X main.buildTime=...".
var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func main() {
	opts := &commands.Options{
		Build: commands.BuildInfo{Version: version, Commit: commit, BuildTime: buildTime},
	}
	root := commands.NewRootCmd(opts)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

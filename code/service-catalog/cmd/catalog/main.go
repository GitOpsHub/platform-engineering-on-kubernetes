// Command catalog browses and scaffolds services from the platform service catalog.
package main

import (
	"fmt"
	"os"

	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/service-catalog/cli"
)

func main() {
	if err := cli.NewRootCmd(&cli.App{}).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

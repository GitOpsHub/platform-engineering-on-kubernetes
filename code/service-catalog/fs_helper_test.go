package catalog

import (
	"io/fs"

	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/service-catalog/templates"
)

// fsSub returns the embedded template directory dir.
func fsSub(dir string) (fs.FS, error) {
	return fs.Sub(templates.FS, dir)
}

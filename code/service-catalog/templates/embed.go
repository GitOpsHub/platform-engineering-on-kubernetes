// Package templates embeds the built-in scaffolding templates. The skeletons are
// data files; this package exists only because go:embed cannot reach outside its
// own package directory.
package templates

import "embed"

// FS holds every built-in template directory.
//
//go:embed go-http-service static-site
var FS embed.FS

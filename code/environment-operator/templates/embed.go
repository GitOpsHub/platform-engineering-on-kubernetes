// Package templates embeds the baseline object templates rendered for each Environment.
//
// Layout: <template-set>/<environment-type>/<kind>.yaml.tmpl, e.g. baseline/prod/networkpolicy.yaml.tmpl.
package templates

import "embed"

// FS holds every embedded template set.
//
//go:embed baseline
var FS embed.FS

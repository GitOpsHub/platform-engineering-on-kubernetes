// Package config resolves platform-cli settings from three layers, lowest to
// highest precedence:
//
//  1. the config file (~/.platform/config.yaml, or $PLATFORM_CONFIG)
//  2. environment variables (PLATFORM_KUBECONFIG, PLATFORM_CONTEXT,
//     PLATFORM_NAMESPACE, PLATFORM_OUTPUT)
//  3. command-line flags (only flags the user actually passed)
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"
)

// Environment variables that override the config file.
const (
	EnvConfigPath = "PLATFORM_CONFIG"
	EnvKubeconfig = "PLATFORM_KUBECONFIG"
	EnvContext    = "PLATFORM_CONTEXT"
	EnvNamespace  = "PLATFORM_NAMESPACE"
	EnvOutput     = "PLATFORM_OUTPUT"
)

// Output formats accepted by the -o flag.
const (
	OutputTable = "table"
	OutputJSON  = "json"
	OutputYAML  = "yaml"
)

// Config is the set of user-tunable settings. Empty fields mean "not set".
type Config struct {
	Kubeconfig string `json:"kubeconfig,omitempty"`
	Context    string `json:"context,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Output     string `json:"output,omitempty"`
}

// Keys lists the settable keys, in display order.
var Keys = []string{"kubeconfig", "context", "namespace", "output"}

// Defaults returns the built-in settings.
func Defaults() Config {
	return Config{Output: OutputTable}
}

// DefaultPath returns $PLATFORM_CONFIG if set, else ~/.platform/config.yaml.
func DefaultPath() (string, error) {
	if p := os.Getenv(EnvConfigPath); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating home directory: %w", err)
	}
	return filepath.Join(home, ".platform", "config.yaml"), nil
}

// Load reads the file at path. A missing file is not an error and yields an
// empty Config. Unknown keys are rejected so typos do not pass silently.
func Load(path string) (Config, error) {
	var c Config
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, fmt.Errorf("reading %s: %w", path, err)
	}
	if err := yaml.UnmarshalStrict(raw, &c); err != nil {
		return c, fmt.Errorf("parsing %s: %w", path, err)
	}
	return c, nil
}

// Save writes c to path with owner-only permissions, creating the directory.
func Save(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	raw, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

// FromEnv reads the PLATFORM_* variables through lookup (usually os.LookupEnv).
func FromEnv(lookup func(string) (string, bool)) Config {
	get := func(k string) string { v, _ := lookup(k); return v }
	return Config{
		Kubeconfig: get(EnvKubeconfig),
		Context:    get(EnvContext),
		Namespace:  get(EnvNamespace),
		Output:     get(EnvOutput),
	}
}

// Merge returns base overlaid with over: every non-empty field of over wins.
func (c Config) Merge(over Config) Config {
	pick := func(lo, hi string) string {
		if hi != "" {
			return hi
		}
		return lo
	}
	return Config{
		Kubeconfig: pick(c.Kubeconfig, over.Kubeconfig),
		Context:    pick(c.Context, over.Context),
		Namespace:  pick(c.Namespace, over.Namespace),
		Output:     pick(c.Output, over.Output),
	}
}

// Validate checks values that have a fixed set of legal options.
func (c Config) Validate() error {
	_, err := ParseOutput(c.Output)
	return err
}

// Set assigns value to the named key. Only the key being set is validated, so
// an unset output in an otherwise empty file is not an error.
func (c *Config) Set(key, value string) error {
	switch key {
	case "kubeconfig":
		c.Kubeconfig = value
	case "context":
		c.Context = value
	case "namespace":
		c.Namespace = value
	case "output":
		if _, err := ParseOutput(value); err != nil {
			return err
		}
		c.Output = value
	default:
		return fmt.Errorf("unknown key %q (valid: %v)", key, Keys)
	}
	return nil
}

// ParseOutput checks that s is a supported output format.
func ParseOutput(s string) (string, error) {
	switch s {
	case OutputTable, OutputJSON, OutputYAML:
		return s, nil
	default:
		return "", fmt.Errorf("invalid output format %q (want table, json or yaml)", s)
	}
}

// Resolve builds the effective configuration: defaults, then the file at path,
// then the environment (via lookup), then flags. flags must hold only the
// values the user set explicitly.
func Resolve(path string, lookup func(string) (string, bool), flags Config) (Config, error) {
	file, err := Load(path)
	if err != nil {
		return Config{}, err
	}
	eff := Defaults().Merge(file).Merge(FromEnv(lookup)).Merge(flags)
	if err := eff.Validate(); err != nil {
		return Config{}, err
	}
	return eff, nil
}

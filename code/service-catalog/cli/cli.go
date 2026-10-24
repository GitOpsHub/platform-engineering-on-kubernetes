// Package cli implements the catalog command: list, describe and scaffold.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	catalog "github.com/GitOpsHub/platform-engineering-on-kubernetes/code/service-catalog"
	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/service-catalog/registry"
)

// App wires the commands to their I/O and to the template registry.
type App struct {
	Out io.Writer
	Err io.Writer
	// Builtin is true unless overridden; tests may replace Load.
	Load func(dir string) (*registry.Registry, error)
}

// NewRootCmd builds `catalog`.
func NewRootCmd(app *App) *cobra.Command {
	if app.Out == nil {
		app.Out = os.Stdout
	}
	if app.Err == nil {
		app.Err = os.Stderr
	}
	if app.Load == nil {
		app.Load = loadRegistry
	}
	var (
		dir    string
		reg    *registry.Registry
		format string
	)
	root := &cobra.Command{
		Use:           "catalog",
		Short:         "Browse and scaffold services from the platform service catalog",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			r, err := app.Load(dir)
			if err != nil {
				return err
			}
			reg = r
			return nil
		},
	}
	root.PersistentFlags().StringVar(&dir, "templates", "", "Directory of extra templates, added to the built-in set")
	root.PersistentFlags().StringVarP(&format, "output", "o", "table", "Output format for list: table or json")

	root.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List available templates",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return list(cmd.OutOrStdout(), reg.List(), format)
			},
		},
		&cobra.Command{
			Use:   "describe TEMPLATE",
			Short: "Show a template's parameters and the files it produces",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				e, err := reg.Get(args[0])
				if err != nil {
					return err
				}
				return describe(cmd.OutOrStdout(), e)
			},
		},
		newScaffoldCmd(&reg),
	)
	return root
}

func loadRegistry(extra string) (*registry.Registry, error) {
	r, err := registry.Builtin()
	if err != nil {
		return nil, err
	}
	if extra == "" {
		return r, nil
	}
	custom, err := registry.LoadDir(os.DirFS(extra))
	if err != nil {
		return nil, fmt.Errorf("loading %s: %w", extra, err)
	}
	if err := r.Merge(custom); err != nil {
		return nil, err
	}
	return r, nil
}

func list(w io.Writer, ts []catalog.Template, format string) error {
	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(ts)
	}
	if format != "table" {
		return fmt.Errorf("unsupported output format %q (want table or json)", format)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "NAME\tTITLE\tOWNER\tPARAMETERS\tDESCRIPTION")
	for _, t := range ts {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n", t.Name, t.Title, dash(t.Owner), len(t.Parameters), t.Description)
	}
	return tw.Flush()
}

func describe(w io.Writer, e registry.Entry) error {
	t := e.Template
	fmt.Fprintf(w, "%s (%s)\n", t.Title, t.Name)
	if t.Description != "" {
		fmt.Fprintln(w, t.Description)
	}
	if len(t.Tags) > 0 {
		fmt.Fprintf(w, "Tags: %s\n", strings.Join(t.Tags, ", "))
	}
	fmt.Fprintln(w, "\nParameters:")
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "  NAME\tREQUIRED\tDEFAULT\tCONSTRAINT\tDESCRIPTION")
	for _, p := range t.Parameters {
		constraint := "-"
		switch {
		case len(p.Enum) > 0:
			constraint = "one of " + strings.Join(p.Enum, "|")
		case p.Pattern != "":
			constraint = "matches " + p.Pattern
		}
		fmt.Fprintf(tw, "  %s\t%t\t%s\t%s\t%s\n", p.Name, p.Required, dash(p.Default), constraint, p.Description)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	paths, err := catalog.OutputPaths(e.Files)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "\nProduces:")
	for _, p := range paths {
		fmt.Fprintf(w, "  %s\n", p)
	}
	return nil
}

func newScaffoldCmd(reg **registry.Registry) *cobra.Command {
	var (
		sets   []string
		out    string
		force  bool
		dryRun bool
	)
	cmd := &cobra.Command{
		Use:   "scaffold TEMPLATE --set name=... --set owner=... [--out DIR]",
		Short: "Render a template into a new service directory",
		Long: "Renders the template into DIR (default ./<name>). Existing files are left alone " +
			"unless --force is given. Use --dry-run to list the files without writing.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := (*reg).Get(args[0])
			if err != nil {
				return err
			}
			values, err := parseSets(sets)
			if err != nil {
				return err
			}
			target := out
			if target == "" {
				if values["name"] == "" {
					return fmt.Errorf("--out is required when --set name=... is not given")
				}
				target = "./" + values["name"]
			}
			files, err := catalog.Render(e.Files, e.Template, values)
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if dryRun {
				fmt.Fprintf(w, "would write %d files to %s:\n", len(files), target)
				for _, f := range files {
					fmt.Fprintf(w, "  %s\n", f.Path)
				}
				return nil
			}
			if err := catalog.WriteTree(target, files, force); err != nil {
				return err
			}
			fmt.Fprintf(w, "scaffolded %s into %s (%d files)\n", e.Template.Name, target, len(files))
			return nil
		},
	}
	f := cmd.Flags()
	f.StringArrayVar(&sets, "set", nil, "Parameter value as key=value (repeatable)")
	f.StringVar(&out, "out", "", "Target directory (default ./<name>)")
	f.BoolVar(&force, "force", false, "Overwrite existing files")
	f.BoolVar(&dryRun, "dry-run", false, "Print the files that would be written and exit")
	return cmd
}

// parseSets turns key=value pairs into a map, rejecting malformed entries and
// duplicate keys.
func parseSets(pairs []string) (map[string]string, error) {
	values := make(map[string]string, len(pairs))
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("--set %q: want key=value", p)
		}
		if _, dup := values[k]; dup {
			return nil, fmt.Errorf("--set %s given more than once", k)
		}
		values[k] = v
	}
	return values, nil
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

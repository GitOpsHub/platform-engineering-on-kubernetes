// Package output renders CLI results as a table, JSON or YAML.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"sigs.k8s.io/yaml"
)

// Format is a supported output format.
type Format string

// Supported formats.
const (
	Table Format = "table"
	JSON  Format = "json"
	YAML  Format = "yaml"
)

// ParseFormat validates a -o value.
func ParseFormat(s string) (Format, error) {
	switch f := Format(strings.ToLower(strings.TrimSpace(s))); f {
	case Table, JSON, YAML:
		return f, nil
	default:
		return "", fmt.Errorf("unsupported output format %q (want table, json or yaml)", s)
	}
}

// Printer writes results to Out in Format.
type Printer struct {
	Out    io.Writer
	Format Format
}

// Render writes data for json and yaml, and headers plus rows as an aligned
// table. Callers pass the same logical content in both forms: data is the
// structured value (a single object or a list), and headers/rows are its
// human-readable projection.
func (p Printer) Render(data any, headers []string, rows [][]string) error {
	switch p.Format {
	case JSON:
		raw, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(p.Out, string(raw))
		return err
	case YAML:
		raw, err := yaml.Marshal(data)
		if err != nil {
			return err
		}
		_, err = io.WriteString(p.Out, string(raw))
		return err
	case Table, "":
		return writeTable(p.Out, headers, rows)
	default:
		return fmt.Errorf("unsupported output format %q", p.Format)
	}
}

func writeTable(w io.Writer, headers []string, rows [][]string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	if len(headers) > 0 {
		if _, err := fmt.Fprintln(tw, strings.Join(headers, "\t")); err != nil {
			return err
		}
	}
	for _, r := range rows {
		if _, err := fmt.Fprintln(tw, strings.Join(r, "\t")); err != nil {
			return err
		}
	}
	return tw.Flush()
}

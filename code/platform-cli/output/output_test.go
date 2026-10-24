package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseFormat(t *testing.T) {
	tests := []struct {
		in      string
		want    Format
		wantErr bool
	}{
		{"table", Table, false},
		{"JSON", JSON, false},
		{" yaml ", YAML, false},
		{"xml", "", true},
	}
	for _, tc := range tests {
		got, err := ParseFormat(tc.in)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Fatalf("ParseFormat(%q) = %q, %v", tc.in, got, err)
		}
	}
}

func TestRender(t *testing.T) {
	data := map[string]any{"items": []string{"team-a"}}
	headers := []string{"NAME", "PHASE"}
	rows := [][]string{{"team-a", "Ready"}, {"team-bb", "Pending"}}
	tests := []struct {
		name   string
		format Format
		want   []string // substrings that must appear
	}{
		{"table", Table, []string{"NAME", "PHASE", "team-a", "team-bb   Pending"}},
		{"json", JSON, []string{`"items": [`, `"team-a"`}},
		{"yaml", YAML, []string{"items:", "- team-a"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := (Printer{Out: &buf, Format: tc.format}).Render(data, headers, rows); err != nil {
				t.Fatal(err)
			}
			for _, w := range tc.want {
				if !strings.Contains(buf.String(), w) {
					t.Fatalf("output missing %q:\n%s", w, buf.String())
				}
			}
		})
	}
}

func TestRenderTableWithoutRows(t *testing.T) {
	var buf bytes.Buffer
	if err := (Printer{Out: &buf, Format: Table}).Render(nil, []string{"NAME"}, nil); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(buf.String()) != "NAME" {
		t.Fatalf("unexpected output %q", buf.String())
	}
}

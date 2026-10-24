// Package catalog defines service templates (metadata, parameters and text
// skeletons), validates parameter values and renders templates into file trees.
//
// A template is a directory containing a template.yaml with its metadata and
// parameters, plus files whose names end in ".tmpl". Rendering executes every
// ".tmpl" file with the resolved parameter values and writes the result without
// the suffix, e.g. Dockerfile.tmpl becomes Dockerfile.
package catalog

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"sigs.k8s.io/yaml"
)

// MetadataFile is the name of the metadata file inside each template directory.
const MetadataFile = "template.yaml"

// TemplateSuffix marks files that are executed as text/template.
const TemplateSuffix = ".tmpl"

var (
	dnsLabel       = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	identifierName = regexp.MustCompile(`^[a-z][a-zA-Z0-9]*$`)
)

// Parameter is one input a template accepts.
type Parameter struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Default     string   `json:"default,omitempty"`
	Pattern     string   `json:"pattern,omitempty"` // regular expression the value must match
	Enum        []string `json:"enum,omitempty"`    // allowed values, if restricted
}

// Template is the metadata of a scaffolding template.
type Template struct {
	Name        string      `json:"name"`
	Title       string      `json:"title"`
	Description string      `json:"description,omitempty"`
	Owner       string      `json:"owner,omitempty"`
	Tags        []string    `json:"tags,omitempty"`
	Parameters  []Parameter `json:"parameters"`
}

// File is one rendered output file, with a slash-separated relative path.
type File struct {
	Path    string
	Content []byte
}

// ValidationError lists every problem found, not just the first.
type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string {
	return "invalid parameters: " + strings.Join(e.Problems, "; ")
}

// ParseTemplate decodes and checks template metadata.
func ParseTemplate(raw []byte) (Template, error) {
	var t Template
	if err := yaml.UnmarshalStrict(raw, &t); err != nil {
		return Template{}, fmt.Errorf("parsing %s: %w", MetadataFile, err)
	}
	if err := t.check(); err != nil {
		return Template{}, err
	}
	return t, nil
}

func (t Template) check() error {
	var problems []string
	if !dnsLabel.MatchString(t.Name) {
		problems = append(problems, fmt.Sprintf("template name %q must be a DNS label", t.Name))
	}
	if strings.TrimSpace(t.Title) == "" {
		problems = append(problems, "template title is required")
	}
	seen := map[string]bool{}
	for _, p := range t.Parameters {
		switch {
		case !identifierName.MatchString(p.Name):
			problems = append(problems, fmt.Sprintf("parameter %q must be a lowerCamel identifier", p.Name))
		case seen[p.Name]:
			problems = append(problems, fmt.Sprintf("parameter %q is declared twice", p.Name))
		}
		seen[p.Name] = true
		if p.Pattern != "" {
			if _, err := regexp.Compile(p.Pattern); err != nil {
				problems = append(problems, fmt.Sprintf("parameter %q has an invalid pattern: %v", p.Name, err))
			}
		}
		if p.Default != "" {
			if err := checkValue(p, p.Default); err != nil {
				problems = append(problems, fmt.Sprintf("default of %v", err))
			}
		}
	}
	if len(problems) > 0 {
		return &ValidationError{Problems: problems}
	}
	return nil
}

// Parameter returns the named parameter.
func (t Template) Parameter(name string) (Parameter, bool) {
	for _, p := range t.Parameters {
		if p.Name == name {
			return p, true
		}
	}
	return Parameter{}, false
}

// Resolve checks supplied values against the template's parameters, fills in
// defaults and returns the complete value set. Unknown keys are errors, so a
// misspelled --set flag cannot be ignored.
func (t Template) Resolve(values map[string]string) (map[string]string, error) {
	var problems []string
	for k := range values {
		if _, ok := t.Parameter(k); !ok {
			problems = append(problems, fmt.Sprintf("unknown parameter %q", k))
		}
	}
	out := make(map[string]string, len(t.Parameters))
	for _, p := range t.Parameters {
		v := values[p.Name]
		if v == "" {
			if p.Required {
				problems = append(problems, fmt.Sprintf("parameter %q is required", p.Name))
				continue
			}
			v = p.Default
		}
		if v != "" {
			if err := checkValue(p, v); err != nil {
				problems = append(problems, err.Error())
			}
		}
		out[p.Name] = v
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return nil, &ValidationError{Problems: problems}
	}
	return out, nil
}

// checkValue applies a parameter's pattern and enum to one value.
func checkValue(p Parameter, v string) error {
	if p.Pattern != "" {
		ok, err := regexp.MatchString(p.Pattern, v)
		if err != nil {
			return fmt.Errorf("parameter %q has an invalid pattern: %v", p.Name, err)
		}
		if !ok {
			return fmt.Errorf("parameter %q value %q does not match %s", p.Name, v, p.Pattern)
		}
	}
	if len(p.Enum) > 0 {
		for _, e := range p.Enum {
			if e == v {
				return nil
			}
		}
		return fmt.Errorf("parameter %q value %q must be one of %v", p.Name, v, p.Enum)
	}
	return nil
}

var funcs = template.FuncMap{
	"lower": strings.ToLower,
	"upper": strings.ToUpper,
	// quote renders a YAML/Dockerfile-safe double-quoted string.
	"quote": strconv.Quote,
}

// Render resolves values and executes every file under src (except the
// metadata file) into a sorted list of output files. Templates run with
// missingkey=error so a typo in a skeleton fails loudly.
func Render(src fs.FS, t Template, values map[string]string) ([]File, error) {
	data, err := t.Resolve(values)
	if err != nil {
		return nil, err
	}
	var files []File
	err = fs.WalkDir(src, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || p == MetadataFile {
			return nil
		}
		raw, err := fs.ReadFile(src, p)
		if err != nil {
			return err
		}
		out := raw
		outPath := p
		if strings.HasSuffix(p, TemplateSuffix) {
			outPath = strings.TrimSuffix(p, TemplateSuffix)
			tpl, err := template.New(p).Option("missingkey=error").Funcs(funcs).Parse(string(raw))
			if err != nil {
				return fmt.Errorf("parsing %s: %w", p, err)
			}
			var buf bytes.Buffer
			if err := tpl.Execute(&buf, data); err != nil {
				return fmt.Errorf("rendering %s: %w", p, err)
			}
			out = buf.Bytes()
		}
		files = append(files, File{Path: outPath, Content: out})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// OutputPaths lists the file paths a template produces, without rendering.
func OutputPaths(src fs.FS) ([]string, error) {
	var out []string
	err := fs.WalkDir(src, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || p == MetadataFile {
			return nil
		}
		out = append(out, strings.TrimSuffix(p, TemplateSuffix))
		return nil
	})
	sort.Strings(out)
	return out, err
}

// ErrExists is returned by WriteTree when a target file already exists.
var ErrExists = errors.New("file already exists (use --force to overwrite)")

// WriteTree writes files beneath dir. It refuses paths that escape dir and, unless
// overwrite is set, refuses to replace existing files. Nothing is written if any
// check fails.
func WriteTree(dir string, files []File, overwrite bool) error {
	targets := make([]string, len(files))
	for i, f := range files {
		clean := path.Clean(f.Path)
		if path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
			return fmt.Errorf("refusing to write outside %s: %q", dir, f.Path)
		}
		targets[i] = filepath.Join(dir, filepath.FromSlash(clean))
		if _, err := os.Stat(targets[i]); err == nil && !overwrite {
			return fmt.Errorf("%s: %w", targets[i], ErrExists)
		}
	}
	for i, f := range files {
		if err := os.MkdirAll(filepath.Dir(targets[i]), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(targets[i], f.Content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

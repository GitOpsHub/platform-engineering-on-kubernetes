package controller

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"text/template"

	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	platformv1alpha1 "github.com/GitOpsHub/platform-engineering-on-kubernetes/code/environment-operator/api/v1alpha1"
)

// DefaultTemplateSet is used when spec.template is empty.
const DefaultTemplateSet = "baseline"

const templateSuffix = ".yaml.tmpl"

// templateData is the context every embedded template is rendered with.
type templateData struct {
	Name      string
	Namespace string
	Tenant    string
	Type      string
	URL       string
	OwnerUID  string
}

// renderBaseline renders every template in <set>/<type>/ into typed objects.
// Objects are returned in file-name order so reconciles are deterministic.
func renderBaseline(fsys fs.FS, env *platformv1alpha1.Environment, namespace, url string) ([]client.Object, error) {
	set := env.Spec.Template
	if set == "" {
		set = DefaultTemplateSet
	}
	dir := path.Join(set, string(env.Spec.Type))
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("template set %q has no templates for type %q: %w", set, env.Spec.Type, err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	data := templateData{
		Name:      env.Name,
		Namespace: namespace,
		Tenant:    env.Spec.Tenant,
		Type:      string(env.Spec.Type),
		URL:       url,
		OwnerUID:  string(env.UID),
	}

	codecs := scheme.Codecs.UniversalDeserializer()
	var out []client.Object
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), templateSuffix) {
			continue
		}
		raw, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		tmpl, err := template.New(e.Name()).Option("missingkey=error").Parse(string(raw))
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", e.Name(), err)
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err != nil {
			return nil, fmt.Errorf("render %s: %w", e.Name(), err)
		}
		obj, _, err := codecs.Decode(buf.Bytes(), nil, nil)
		if err != nil {
			return nil, fmt.Errorf("decode %s: %w", e.Name(), err)
		}
		co, ok := obj.(client.Object)
		if !ok {
			return nil, fmt.Errorf("template %s did not produce a Kubernetes object", e.Name())
		}
		out = append(out, co)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("template set %q type %q has no templates", set, env.Spec.Type)
	}
	return out, nil
}

// templateSetExists reports whether a template set is embedded.
func templateSetExists(fsys fs.FS, set string) bool {
	if set == "" {
		set = DefaultTemplateSet
	}
	info, err := fs.Stat(fsys, set)
	return err == nil && info.IsDir()
}

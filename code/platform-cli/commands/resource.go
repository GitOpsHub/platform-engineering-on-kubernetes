package commands

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/platform-cli/output"
)

// columns describes the table projection of one resource kind.
type columns struct {
	headers []string
	row     func(u *unstructured.Unstructured) []string
}

// resourceAccess is the generic list/get/create/delete for one kind.
type resourceAccess struct {
	kind       string // e.g. "Tenant"
	gvr        schema.GroupVersionResource
	namespaced bool
	cols       columns
}

// ri returns the resource interface for the given namespace ("" = all, for lists).
func (ra resourceAccess) ri(c dynamic.Interface, namespace string) dynamic.ResourceInterface {
	r := c.Resource(ra.gvr)
	if ra.namespaced {
		return r.Namespace(namespace)
	}
	return r
}

func (ra resourceAccess) list(ctx context.Context, c dynamic.Interface, namespace, selector string) ([]unstructured.Unstructured, error) {
	l, err := ra.ri(c, namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("listing %s: %w", ra.gvr.Resource, err)
	}
	return l.Items, nil
}

func (ra resourceAccess) get(ctx context.Context, c dynamic.Interface, namespace, name string) (*unstructured.Unstructured, error) {
	u, err := ra.ri(c, namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting %s %q: %w", ra.kind, name, err)
	}
	return u, nil
}

func (ra resourceAccess) create(ctx context.Context, c dynamic.Interface, namespace string, obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	created, err := ra.ri(c, namespace).Create(ctx, obj, metav1.CreateOptions{FieldManager: "platform-cli"})
	if err != nil {
		return nil, fmt.Errorf("creating %s %q: %w", ra.kind, obj.GetName(), err)
	}
	return created, nil
}

func (ra resourceAccess) delete(ctx context.Context, c dynamic.Interface, namespace, name string) error {
	if err := ra.ri(c, namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("deleting %s %q: %w", ra.kind, name, err)
	}
	return nil
}

// newObject builds a CR ready to create.
func (ra resourceAccess) newObject(name, namespace string, spec map[string]any) *unstructured.Unstructured {
	u := &unstructured.Unstructured{Object: map[string]any{"spec": spec}}
	u.SetAPIVersion(APIVersion)
	u.SetKind(ra.kind)
	u.SetName(name)
	if ra.namespaced {
		u.SetNamespace(namespace)
	}
	return u
}

// renderList prints a list of objects in the configured format.
func (ra resourceAccess) renderList(p output.Printer, items []unstructured.Unstructured) error {
	rows := make([][]string, 0, len(items))
	raw := make([]map[string]any, 0, len(items))
	for i := range items {
		rows = append(rows, ra.cols.row(&items[i]))
		raw = append(raw, items[i].Object)
	}
	return p.Render(map[string]any{"apiVersion": APIVersion, "kind": ra.kind + "List", "items": raw}, ra.cols.headers, rows)
}

// renderOne prints a single object in the configured format.
func (ra resourceAccess) renderOne(p output.Printer, u *unstructured.Unstructured) error {
	return p.Render(u.Object, ra.cols.headers, [][]string{ra.cols.row(u)})
}

// nameArg validates the single NAME argument.
func nameArg(args []string) (string, error) {
	if len(args) != 1 || args[0] == "" {
		return "", fmt.Errorf("exactly one name argument is required")
	}
	return args[0], nil
}

// str returns a nested string field or "" when missing.
func str(u *unstructured.Unstructured, fields ...string) string {
	v, _, _ := unstructured.NestedString(u.Object, fields...)
	return v
}

// strSlice returns a nested string-slice field or nil when missing.
func strSlice(u *unstructured.Unstructured, fields ...string) []string {
	v, _, _ := unstructured.NestedStringSlice(u.Object, fields...)
	return v
}

// int64Field returns a nested integer field as a string, or "" when missing.
func int64Field(u *unstructured.Unstructured, fields ...string) string {
	v, found, _ := unstructured.NestedFieldNoCopy(u.Object, fields...)
	if !found || v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// ageOf renders the creation date, or "-" when unknown.
func ageOf(u *unstructured.Unstructured) string {
	ts := u.GetCreationTimestamp()
	if ts.IsZero() {
		return "-"
	}
	return ts.Format("2006-01-02")
}

// dash replaces an empty value with "-" for table output.
func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// addCommonSubcommands wires get/list/delete into a resource group. create is
// added by each kind because its flags differ.
func addCommonSubcommands(parent *cobra.Command, opts *Options, ra resourceAccess, selector *string) {
	list := &cobra.Command{
		Use:   "list",
		Short: fmt.Sprintf("List %s objects", ra.gvr.Resource),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ns := opts.Resolved.Namespace
			if !ra.namespaced {
				ns = ""
			}
			c, err := opts.dynamicClient()
			if err != nil {
				return err
			}
			items, err := ra.list(cmd.Context(), c, ns, *selector)
			if err != nil {
				return err
			}
			p, err := opts.printer(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			return ra.renderList(p, items)
		},
	}
	list.Flags().StringVarP(selector, "selector", "l", "", "Label selector, e.g. team=payments")

	get := &cobra.Command{
		Use:   "get NAME",
		Short: fmt.Sprintf("Show one %s", ra.kind),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ns, err := scopeNamespace(opts, ra)
			if err != nil {
				return err
			}
			c, err := opts.dynamicClient()
			if err != nil {
				return err
			}
			u, err := ra.get(cmd.Context(), c, ns, args[0])
			if err != nil {
				return err
			}
			p, err := opts.printer(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			return ra.renderOne(p, u)
		},
	}

	del := &cobra.Command{
		Use:   "delete NAME",
		Short: fmt.Sprintf("Delete a %s", ra.kind),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ns, err := scopeNamespace(opts, ra)
			if err != nil {
				return err
			}
			c, err := opts.dynamicClient()
			if err != nil {
				return err
			}
			if err := ra.delete(cmd.Context(), c, ns, args[0]); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s %q deleted\n", ra.kind, args[0])
			return err
		},
	}
	parent.AddCommand(list, get, del)
}

// scopeNamespace returns the namespace for get/delete/create, or "" for
// cluster-scoped kinds.
func scopeNamespace(opts *Options, ra resourceAccess) (string, error) {
	if !ra.namespaced {
		return "", nil
	}
	return opts.requireNamespace()
}

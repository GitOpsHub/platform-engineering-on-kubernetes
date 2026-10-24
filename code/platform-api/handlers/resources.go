package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/dynamic"

	mw "github.com/GitOpsHub/platform-engineering-on-kubernetes/code/platform-api/middleware"
	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/platform-api/types"
)

// GroupVersionResources of the platform CRDs (the shared API contract).
var (
	TenantGVR      = schema.GroupVersionResource{Group: types.Group, Version: types.Version, Resource: "tenants"}
	EnvironmentGVR = schema.GroupVersionResource{Group: types.Group, Version: types.Version, Resource: "environments"}
	WorkloadGVR    = schema.GroupVersionResource{Group: types.Group, Version: types.Version, Resource: "workloads"}
)

// CreatedByAnnotation records the authenticated subject that created an object.
const CreatedByAnnotation = types.Group + "/created-by"

// maxListLimit caps ?limit= on list endpoints.
const maxListLimit = 500

// objPtr constrains PT to be *T and a types.Object, so generic code can both
// allocate a T and call methods on it.
type objPtr[T any] interface {
	*T
	types.Object
}

// resource implements list/get/create/delete for one CRD kind.
type resource[T any, PT objPtr[T]] struct {
	clientFor  func(*http.Request) (dynamic.Interface, error) // caller-scoped client
	gvr        schema.GroupVersionResource
	kind       string
	namespaced bool
}

func (rs *resource[T, PT]) ri(client dynamic.Interface, namespace string) dynamic.ResourceInterface {
	nri := client.Resource(rs.gvr)
	if rs.namespaced {
		return nri.Namespace(namespace) // "" lists across all namespaces
	}
	return nri
}

// client resolves the caller-scoped client for r, writing the error response
// itself when there is none.
func (rs *resource[T, PT]) client(w http.ResponseWriter, r *http.Request) (dynamic.Interface, bool) {
	c, err := rs.clientFor(r)
	if err != nil {
		writeClientError(w, r, err)
		return nil, false
	}
	return c, true
}

// namespace extracts and validates the {namespace} path segment (if any).
func (rs *resource[T, PT]) namespace(r *http.Request) (string, error) {
	if !rs.namespaced {
		return "", nil
	}
	ns := r.PathValue("namespace")
	if ns != "" && !types.IsDNSLabel(ns) {
		return "", fmt.Errorf("invalid namespace %q", ns)
	}
	return ns, nil
}

func (rs *resource[T, PT]) list(w http.ResponseWriter, r *http.Request) {
	ns, err := rs.namespace(r)
	if err != nil {
		mw.WriteError(w, r, http.StatusBadRequest, "BadRequest", err.Error())
		return
	}
	q := r.URL.Query()
	if q.Get("watch") == "true" {
		rs.watch(w, r, ns)
		return
	}
	opts := metav1.ListOptions{LabelSelector: q.Get("labelSelector"), Continue: q.Get("continue")}
	if l := q.Get("limit"); l != "" {
		n, err := strconv.ParseInt(l, 10, 64)
		if err != nil || n < 1 || n > maxListLimit {
			mw.WriteError(w, r, http.StatusBadRequest, "BadRequest", fmt.Sprintf("limit must be an integer between 1 and %d", maxListLimit))
			return
		}
		opts.Limit = n
	}
	client, ok := rs.client(w, r)
	if !ok {
		return
	}
	ul, err := rs.ri(client, ns).List(r.Context(), opts)
	if err != nil {
		writeKubeError(w, r, err)
		return
	}
	out := types.List[T]{Items: make([]T, 0, len(ul.Items)), Continue: ul.GetContinue()}
	for i := range ul.Items {
		obj, err := fromUnstructured[T](&ul.Items[i])
		if err != nil {
			writeKubeError(w, r, err)
			return
		}
		out.Items = append(out.Items, *obj)
	}
	mw.WriteJSON(w, http.StatusOK, out)
}

func (rs *resource[T, PT]) get(w http.ResponseWriter, r *http.Request) {
	ns, err := rs.namespace(r)
	if err != nil {
		mw.WriteError(w, r, http.StatusBadRequest, "BadRequest", err.Error())
		return
	}
	client, ok := rs.client(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	u, err := rs.ri(client, ns).Get(r.Context(), name, metav1.GetOptions{})
	if err != nil {
		writeKubeError(w, r, err)
		return
	}
	obj, err := fromUnstructured[T](u)
	if err != nil {
		writeKubeError(w, r, err)
		return
	}
	mw.WriteJSON(w, http.StatusOK, obj)
}

func (rs *resource[T, PT]) create(w http.ResponseWriter, r *http.Request) {
	ns, err := rs.namespace(r)
	if err != nil {
		mw.WriteError(w, r, http.StatusBadRequest, "BadRequest", err.Error())
		return
	}
	in := PT(new(T))
	// For namespaced kinds the URL namespace is authoritative (checked below);
	// cluster-scoped kinds reject a body namespace during Validate.
	if err := mw.DecodeAndValidate(w, r, in); err != nil {
		var de *mw.DecodeError
		if errors.As(err, &de) {
			mw.WriteError(w, r, de.Status, de.Reason, de.Message, de.Details...)
			return
		}
		mw.WriteError(w, r, http.StatusBadRequest, "BadRequest", err.Error())
		return
	}
	meta := in.GetMeta()
	if rs.namespaced {
		if meta.Namespace != "" && meta.Namespace != ns {
			mw.WriteError(w, r, http.StatusBadRequest, "BadRequest",
				fmt.Sprintf("metadata.namespace %q does not match URL namespace %q", meta.Namespace, ns))
			return
		}
		meta.Namespace = ns
	}
	for k := range meta.Labels {
		if errs := validation.IsQualifiedName(k); len(errs) > 0 {
			mw.WriteError(w, r, http.StatusUnprocessableEntity, "Invalid", "invalid label key "+strconv.Quote(k), errs...)
			return
		}
	}
	if p := mw.PrincipalFrom(r.Context()); p != nil {
		if meta.Annotations == nil {
			meta.Annotations = map[string]string{}
		}
		meta.Annotations[CreatedByAnnotation] = p.Subject
	}

	u, err := toUnstructured(in, rs.kind)
	if err != nil {
		writeKubeError(w, r, err)
		return
	}
	client, ok := rs.client(w, r)
	if !ok {
		return
	}
	created, err := rs.ri(client, ns).Create(r.Context(), u, metav1.CreateOptions{FieldManager: "platform-api"})
	if err != nil {
		writeKubeError(w, r, err)
		return
	}
	out, err := fromUnstructured[T](created)
	if err != nil {
		writeKubeError(w, r, err)
		return
	}
	loc := "/api/" + types.Version + "/" + rs.gvr.Resource + "/" + meta.Name
	if rs.namespaced {
		loc = "/api/" + types.Version + "/namespaces/" + ns + "/" + rs.gvr.Resource + "/" + meta.Name
	}
	w.Header().Set("Location", loc)
	mw.WriteJSON(w, http.StatusCreated, out)
}

func (rs *resource[T, PT]) delete(w http.ResponseWriter, r *http.Request) {
	ns, err := rs.namespace(r)
	if err != nil {
		mw.WriteError(w, r, http.StatusBadRequest, "BadRequest", err.Error())
		return
	}
	client, ok := rs.client(w, r)
	if !ok {
		return
	}
	policy := metav1.DeletePropagationForeground
	if err := rs.ri(client, ns).Delete(r.Context(), r.PathValue("name"), metav1.DeleteOptions{PropagationPolicy: &policy}); err != nil {
		writeKubeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// toUnstructured converts an API object into a CR ready for Create: TypeMeta
// is forced, and status and server-populated metadata are dropped.
func toUnstructured[PT types.Object](in PT, kind string) (*unstructured.Unstructured, error) {
	m := in.GetMeta()
	m.UID, m.ResourceVersion, m.CreationTimestamp, m.Generation = "", "", "", 0
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	obj := map[string]any{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	delete(obj, "status")
	u := &unstructured.Unstructured{Object: obj}
	u.SetAPIVersion(types.APIVersion)
	u.SetKind(kind)
	return u, nil
}

func fromUnstructured[T any](u *unstructured.Unstructured) (*T, error) {
	raw, err := u.MarshalJSON()
	if err != nil {
		return nil, err
	}
	out := new(T)
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, fmt.Errorf("decoding %s %q: %w", u.GetKind(), u.GetName(), err)
	}
	return out, nil
}

// writeKubeError maps Kubernetes API errors onto HTTP responses.
func writeKubeError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		mw.WriteError(w, r, http.StatusGatewayTimeout, "Timeout", "timed out talking to the Kubernetes API")
		return
	}
	var status apierrors.APIStatus
	if errors.As(err, &status) {
		s := status.Status()
		code := int(s.Code)
		if code == 0 {
			code = http.StatusInternalServerError
		}
		var details []string
		if s.Details != nil {
			for _, c := range s.Details.Causes {
				details = append(details, fmt.Sprintf("%s: %s", c.Field, c.Message))
			}
		}
		reason := string(s.Reason)
		if reason == "" {
			reason = http.StatusText(code)
		}
		mw.WriteError(w, r, code, reason, s.Message, details...)
		return
	}
	mw.WriteError(w, r, http.StatusInternalServerError, "InternalError", err.Error())
}

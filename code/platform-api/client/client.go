// Package client is a typed Go SDK for the platform-api REST API.
//
//	c, err := client.New("https://platform-api.example.com", client.WithToken(os.Getenv("PLATFORM_TOKEN")))
//	tenants, err := c.Tenants().List(ctx, client.ListOptions{})
//	wl, err := c.Workloads("team-a").Get(ctx, "checkout")
//
// It depends only on the standard library and the types package.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/platform-api/types"
)

// Client talks to platform-api.
type Client struct {
	base      *url.URL
	http      *http.Client
	token     string
	userAgent string
}

// Option customises a Client.
type Option func(*Client)

// WithToken sets the bearer token sent on every request.
func WithToken(token string) Option { return func(c *Client) { c.token = token } }

// WithHTTPClient replaces the default *http.Client (30s timeout).
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithUserAgent overrides the User-Agent header.
func WithUserAgent(ua string) Option { return func(c *Client) { c.userAgent = ua } }

// New returns a client for the API rooted at baseURL (e.g. http://localhost:8080).
func New(baseURL string, opts ...Option) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing base URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("base URL %q must use http or https", baseURL)
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	c := &Client{base: u, http: &http.Client{Timeout: 30 * time.Second}, userAgent: "platform-api-go-client"}
	for _, o := range opts {
		o(c)
	}
	return c, nil
}

// APIError is returned for any non-2xx response.
type APIError struct {
	StatusCode int
	types.ErrorDetail
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("platform-api: %d %s: %s", e.StatusCode, e.Reason, e.Message)
	if len(e.Details) > 0 {
		msg += " (" + strings.Join(e.Details, "; ") + ")"
	}
	if e.RequestID != "" {
		msg += " [request-id " + e.RequestID + "]"
	}
	return msg
}

// IsNotFound reports whether err is a 404 from the API.
func IsNotFound(err error) bool { return statusIs(err, http.StatusNotFound) }

// IsConflict reports whether err is a 409 (e.g. already exists).
func IsConflict(err error) bool { return statusIs(err, http.StatusConflict) }

// IsUnauthorized reports whether err is a 401.
func IsUnauthorized(err error) bool { return statusIs(err, http.StatusUnauthorized) }

func statusIs(err error, code int) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.StatusCode == code
}

// ListOptions filter list calls.
type ListOptions struct {
	LabelSelector string
	Limit         int
	Continue      string
}

func (o ListOptions) query() url.Values {
	q := url.Values{}
	if o.LabelSelector != "" {
		q.Set("labelSelector", o.LabelSelector)
	}
	if o.Limit > 0 {
		q.Set("limit", strconv.Itoa(o.Limit))
	}
	if o.Continue != "" {
		q.Set("continue", o.Continue)
	}
	return q
}

func (c *Client) do(ctx context.Context, method, path string, q url.Values, in, out any) error {
	u := *c.base
	u.Path += path
	u.RawQuery = q.Encode()

	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		ae := &APIError{StatusCode: resp.StatusCode}
		var eb types.ErrorBody
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if json.Unmarshal(raw, &eb) == nil && eb.Error.Message != "" {
			ae.ErrorDetail = eb.Error
		} else {
			ae.Reason = http.StatusText(resp.StatusCode)
			ae.Message = strings.TrimSpace(string(raw))
		}
		if ae.RequestID == "" {
			ae.RequestID = resp.Header.Get("X-Request-ID")
		}
		return ae
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding %s %s response: %w", method, path, err)
	}
	return nil
}

// Version returns server build information.
func (c *Client) Version(ctx context.Context) (*types.VersionInfo, error) {
	var v types.VersionInfo
	return &v, c.do(ctx, http.MethodGet, "/version", nil, nil, &v)
}

// Platform returns platform metadata.
func (c *Client) Platform(ctx context.Context) (*types.PlatformInfo, error) {
	var p types.PlatformInfo
	return &p, c.do(ctx, http.MethodGet, "/api/"+types.Version+"/platform", nil, nil, &p)
}

// ResourceClient is a typed CRUD client for one resource kind.
type ResourceClient[T any] struct {
	c          *Client
	collection string // e.g. /api/v1alpha1/namespaces/team-a/workloads
}

// List lists resources.
func (r *ResourceClient[T]) List(ctx context.Context, opts ListOptions) (*types.List[T], error) {
	var l types.List[T]
	if err := r.c.do(ctx, http.MethodGet, r.collection, opts.query(), nil, &l); err != nil {
		return nil, err
	}
	return &l, nil
}

// Get fetches one resource by name.
func (r *ResourceClient[T]) Get(ctx context.Context, name string) (*T, error) {
	var out T
	if err := r.c.do(ctx, http.MethodGet, r.collection+"/"+url.PathEscape(name), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Create creates a resource and returns the stored object.
func (r *ResourceClient[T]) Create(ctx context.Context, obj *T) (*T, error) {
	var out T
	if err := r.c.do(ctx, http.MethodPost, r.collection, nil, obj, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Delete deletes a resource by name.
func (r *ResourceClient[T]) Delete(ctx context.Context, name string) error {
	return r.c.do(ctx, http.MethodDelete, r.collection+"/"+url.PathEscape(name), nil, nil, nil)
}

const apiPrefix = "/api/" + types.Version

func namespaced(ns, resource string) string {
	if ns == "" {
		return apiPrefix + "/" + resource // list across all namespaces
	}
	return apiPrefix + "/namespaces/" + url.PathEscape(ns) + "/" + resource
}

// Tenants returns a client for the cluster-scoped Tenant resource.
func (c *Client) Tenants() *ResourceClient[types.Tenant] {
	return &ResourceClient[types.Tenant]{c: c, collection: apiPrefix + "/tenants"}
}

// Environments returns a client for Environments in namespace ns. An empty
// namespace is only valid for List (all namespaces).
func (c *Client) Environments(ns string) *ResourceClient[types.Environment] {
	return &ResourceClient[types.Environment]{c: c, collection: namespaced(ns, "environments")}
}

// Workloads returns a client for Workloads in namespace ns. An empty
// namespace is only valid for List (all namespaces).
func (c *Client) Workloads(ns string) *ResourceClient[types.Workload] {
	return &ResourceClient[types.Workload]{c: c, collection: namespaced(ns, "workloads")}
}

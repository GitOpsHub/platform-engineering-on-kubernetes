package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/platform-api/types"
)

func ok() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p := PrincipalFrom(r.Context()); p != nil {
			w.Header().Set("X-Subject", p.Subject)
		}
		w.WriteHeader(http.StatusOK)
	})
}

func TestAuth(t *testing.T) {
	h := Chain(ok(), RequestID(), Auth(AuthConfig{
		Verifier:    StaticTokens{"s3cret": {Subject: "alice"}},
		PublicPaths: []string{"/healthz", "/debug/"},
	}))
	tests := []struct {
		name, path, header string
		wantStatus         int
		wantSubject        string
	}{
		{"valid token", "/api/v1alpha1/tenants", "Bearer s3cret", 200, "alice"},
		{"case-insensitive scheme", "/api/v1alpha1/tenants", "bearer s3cret", 200, "alice"},
		{"missing header", "/api/v1alpha1/tenants", "", 401, ""},
		{"wrong scheme", "/api/v1alpha1/tenants", "Basic s3cret", 401, ""},
		{"wrong token", "/api/v1alpha1/tenants", "Bearer nope", 401, ""},
		{"empty token", "/api/v1alpha1/tenants", "Bearer ", 401, ""},
		{"public exact", "/healthz", "", 200, ""},
		{"public prefix", "/debug/pprof/heap", "", 200, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rr.Code, tc.wantStatus)
			}
			if got := rr.Header().Get("X-Subject"); got != tc.wantSubject {
				t.Fatalf("subject = %q, want %q", got, tc.wantSubject)
			}
			if tc.wantStatus == 401 {
				var body types.ErrorBody
				if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if body.Error.RequestID == "" || rr.Header().Get("WWW-Authenticate") == "" {
					t.Fatalf("401 should carry request id and WWW-Authenticate: %+v", body)
				}
			}
		})
	}
}

func TestAuthDisabledWithNilVerifier(t *testing.T) {
	rr := httptest.NewRecorder()
	Auth(AuthConfig{})(ok()).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rr.Code != 200 {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestRequestID(t *testing.T) {
	tests := []struct {
		name, incoming string
		keep           bool
	}{
		{"generated when absent", "", false},
		{"kept when safe", "abc-123.DEF_4", true},
		{"replaced when unsafe", "bad id\n<script>", false},
		{"replaced when too long", strings.Repeat("a", 200), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var seen string
			h := RequestID()(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				seen = RequestIDFrom(r.Context())
			}))
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.incoming != "" {
				req.Header.Set(RequestIDHeader, tc.incoming)
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if seen == "" || rr.Header().Get(RequestIDHeader) != seen {
				t.Fatalf("context id %q, header %q", seen, rr.Header().Get(RequestIDHeader))
			}
			if (seen == tc.incoming) != tc.keep {
				t.Fatalf("keep=%v but id=%q incoming=%q", tc.keep, seen, tc.incoming)
			}
		})
	}
}

func TestCORS(t *testing.T) {
	tests := []struct {
		name       string
		cfg        CORSConfig
		origin     string
		preflight  bool
		wantOrigin string
		wantCreds  string
		wantStatus int
	}{
		{"disabled", CORSConfig{}, "https://evil.example", false, "", "", 200},
		{"allowed origin reflected with creds", CORSConfig{AllowedOrigins: []string{"https://portal.example"}, AllowCredentials: true},
			"https://portal.example", false, "https://portal.example", "true", 200},
		{"disallowed origin", CORSConfig{AllowedOrigins: []string{"https://portal.example"}, AllowCredentials: true},
			"https://evil.example", false, "", "", 200},
		{"wildcard never sends credentials", CORSConfig{AllowedOrigins: []string{"*"}, AllowCredentials: true},
			"https://any.example", false, "*", "", 200},
		{"preflight allowed", CORSConfig{AllowedOrigins: []string{"https://portal.example"}, MaxAge: 10 * time.Minute},
			"https://portal.example", true, "https://portal.example", "", 204},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			method := http.MethodGet
			if tc.preflight {
				method = http.MethodOptions
			}
			req := httptest.NewRequest(method, "/api", nil)
			req.Header.Set("Origin", tc.origin)
			if tc.preflight {
				req.Header.Set("Access-Control-Request-Method", "POST")
			}
			rr := httptest.NewRecorder()
			CORS(tc.cfg)(ok()).ServeHTTP(rr, req)
			if rr.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rr.Code, tc.wantStatus)
			}
			if got := rr.Header().Get("Access-Control-Allow-Origin"); got != tc.wantOrigin {
				t.Fatalf("allow-origin = %q, want %q", got, tc.wantOrigin)
			}
			if got := rr.Header().Get("Access-Control-Allow-Credentials"); got != tc.wantCreds {
				t.Fatalf("allow-credentials = %q, want %q", got, tc.wantCreds)
			}
			if tc.preflight && rr.Header().Get("Access-Control-Max-Age") != "600" {
				t.Fatalf("max-age = %q", rr.Header().Get("Access-Control-Max-Age"))
			}
		})
	}
}

func TestRecoverAndLogging(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	h := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }),
		RequestID(), Logging(logger), Recover(logger))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/explode", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rr.Code)
	}
	if !strings.Contains(buf.String(), `"status":500`) || !strings.Contains(buf.String(), "panic serving request") {
		t.Fatalf("unexpected logs: %s", buf.String())
	}
}

type payload struct {
	Name string `json:"name"`
}

func (p *payload) Validate() error {
	if p.Name == "" {
		return &types.ValidationError{Problems: []string{"name is required"}}
	}
	return nil
}

func TestDecodeAndValidate(t *testing.T) {
	tests := []struct {
		name, contentType, body string
		wantStatus              int // 0 means success
	}{
		{"ok", "application/json", `{"name":"x"}`, 0},
		{"ok with charset", "application/json; charset=utf-8", `{"name":"x"}`, 0},
		{"wrong content type", "text/plain", `{"name":"x"}`, 415},
		{"empty", "application/json", ``, 400},
		{"malformed", "application/json", `{"name":`, 400},
		{"unknown field", "application/json", `{"name":"x","extra":1}`, 400},
		{"trailing data", "application/json", `{"name":"x"}{"name":"y"}`, 400},
		{"invalid", "application/json", `{"name":""}`, 422},
		{"too large", "application/json", `{"name":"` + strings.Repeat("a", MaxBodyBytes) + `"}`, 413},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.contentType)
			err := DecodeAndValidate(httptest.NewRecorder(), req, &payload{})
			if tc.wantStatus == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			de, isDecode := err.(*DecodeError)
			if !isDecode || de.Status != tc.wantStatus {
				t.Fatalf("err = %v, want status %d", err, tc.wantStatus)
			}
		})
	}
}

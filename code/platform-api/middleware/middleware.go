// Package middleware contains the net/http middleware chain for platform-api:
// request IDs, structured logging, panic recovery, CORS, authentication and
// request-body validation helpers.
package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"regexp"
	"runtime/debug"
	"slices"
	"strconv"
	"time"
)

// Middleware wraps an http.Handler.
type Middleware func(http.Handler) http.Handler

// Chain applies middlewares so that the first one listed is the outermost.
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// ---------------------------------------------------------------------------
// Request ID
// ---------------------------------------------------------------------------

// RequestIDHeader is the header used to propagate request IDs.
const RequestIDHeader = "X-Request-ID"

var safeRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// RequestID accepts a well-formed incoming X-Request-ID or generates one,
// stores it on the request context and echoes it on the response.
func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(RequestIDHeader)
			if !safeRequestID.MatchString(id) {
				id = newRequestID()
			}
			w.Header().Set(RequestIDHeader, id)
			next.ServeHTTP(w, r.WithContext(withRequestID(r.Context(), id)))
		})
	}
}

func newRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error on supported platforms
	return hex.EncodeToString(b)
}

// ---------------------------------------------------------------------------
// Logging + recovery
// ---------------------------------------------------------------------------

// statusRecorder captures the status code and bytes written.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// Logging emits one structured log line per request. Health probes are logged
// at debug level to keep logs quiet.
func Logging(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)
			if rec.status == 0 {
				rec.status = http.StatusOK
			}
			level := slog.LevelInfo
			switch {
			case rec.status >= 500:
				level = slog.LevelError
			case r.URL.Path == "/healthz" || r.URL.Path == "/readyz":
				level = slog.LevelDebug
			}
			attrs := []slog.Attr{
				slog.String("requestId", RequestIDFrom(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Int("bytes", rec.bytes),
				slog.Duration("latency", time.Since(start)),
				slog.String("remote", r.RemoteAddr),
				slog.String("userAgent", r.UserAgent()),
			}
			if p := PrincipalFrom(r.Context()); p != nil {
				attrs = append(attrs, slog.String("subject", p.Subject))
			}
			logger.LogAttrs(r.Context(), level, "http request", attrs...)
		})
	}
}

// Recover converts panics into 500 responses instead of killing the
// connection, and logs the stack trace.
func Recover(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					if v == http.ErrAbortHandler { //nolint:errorlint // sentinel compared by identity per net/http docs
						panic(v)
					}
					logger.Error("panic serving request",
						slog.Any("panic", v),
						slog.String("requestId", RequestIDFrom(r.Context())),
						slog.String("stack", string(debug.Stack())))
					WriteError(w, r, http.StatusInternalServerError, "InternalError", "internal server error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// ---------------------------------------------------------------------------
// CORS
// ---------------------------------------------------------------------------

// CORSConfig configures cross-origin access. An empty AllowedOrigins list
// disables CORS headers entirely (same-origin only).
type CORSConfig struct {
	AllowedOrigins   []string // exact origins, e.g. https://portal.example.com; "*" allows any origin WITHOUT credentials
	AllowCredentials bool     // never combined with a wildcard origin
	MaxAge           time.Duration
}

// CORS reflects the request Origin only when it is explicitly allowed. The
// browser spec forbids "Access-Control-Allow-Origin: *" together with
// credentials, so a wildcard never sends Allow-Credentials.
func CORS(cfg CORSConfig) Middleware {
	wildcard := slices.Contains(cfg.AllowedOrigins, "*")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" || len(cfg.AllowedOrigins) == 0 {
				next.ServeHTTP(w, r)
				return
			}
			h := w.Header()
			h.Add("Vary", "Origin")
			allowed := slices.Contains(cfg.AllowedOrigins, origin)
			switch {
			case allowed:
				h.Set("Access-Control-Allow-Origin", origin)
				if cfg.AllowCredentials {
					h.Set("Access-Control-Allow-Credentials", "true")
				}
			case wildcard:
				h.Set("Access-Control-Allow-Origin", "*")
			}
			preflight := r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != ""
			if preflight {
				if allowed || wildcard {
					h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
					h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, "+RequestIDHeader)
					if cfg.MaxAge > 0 {
						h.Set("Access-Control-Max-Age", strconv.Itoa(int(cfg.MaxAge.Seconds())))
					}
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
			h.Set("Access-Control-Expose-Headers", RequestIDHeader)
			next.ServeHTTP(w, r)
		})
	}
}

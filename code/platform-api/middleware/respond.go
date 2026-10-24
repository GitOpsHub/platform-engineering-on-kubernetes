package middleware

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"

	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/platform-api/types"
)

// MaxBodyBytes bounds JSON request bodies.
const MaxBodyBytes = 1 << 20 // 1 MiB

// WriteJSON writes v as JSON with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError writes the standard error envelope, tagged with the request ID.
func WriteError(w http.ResponseWriter, r *http.Request, status int, reason, msg string, details ...string) {
	WriteJSON(w, status, types.ErrorBody{Error: types.ErrorDetail{
		Code:      status,
		Reason:    reason,
		Message:   msg,
		Details:   details,
		RequestID: RequestIDFrom(r.Context()),
	}})
}

// DecodeError is returned by DecodeAndValidate; it carries the HTTP status
// the handler should respond with.
type DecodeError struct {
	Status  int
	Reason  string
	Message string
	Details []string
}

func (e *DecodeError) Error() string { return e.Message }

// DecodeAndValidate reads a JSON body into dst (rejecting unknown fields and
// oversized bodies) and runs dst.Validate().
func DecodeAndValidate[T types.Validator](w http.ResponseWriter, r *http.Request, dst T) error {
	if ct := r.Header.Get("Content-Type"); ct != "" {
		mt, _, err := mime.ParseMediaType(ct)
		if err != nil || mt != "application/json" {
			return &DecodeError{Status: http.StatusUnsupportedMediaType, Reason: "UnsupportedMediaType",
				Message: "Content-Type must be application/json"}
		}
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return &DecodeError{Status: http.StatusRequestEntityTooLarge, Reason: "RequestEntityTooLarge",
				Message: fmt.Sprintf("request body exceeds %d bytes", MaxBodyBytes)}
		}
		if errors.Is(err, io.EOF) {
			return &DecodeError{Status: http.StatusBadRequest, Reason: "BadRequest", Message: "request body is empty"}
		}
		return &DecodeError{Status: http.StatusBadRequest, Reason: "BadRequest", Message: "invalid JSON: " + err.Error()}
	}
	if dec.More() {
		return &DecodeError{Status: http.StatusBadRequest, Reason: "BadRequest", Message: "request body must contain a single JSON object"}
	}
	if err := dst.Validate(); err != nil {
		var ve *types.ValidationError
		if errors.As(err, &ve) {
			return &DecodeError{Status: http.StatusUnprocessableEntity, Reason: "Invalid",
				Message: "request failed validation", Details: ve.Problems}
		}
		return &DecodeError{Status: http.StatusUnprocessableEntity, Reason: "Invalid", Message: err.Error()}
	}
	return nil
}

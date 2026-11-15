// Package api serves the rmhub API under /api/v1/ on the API listener.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/brianmcgee/rmhub/internal/ratelimit"
	"github.com/brianmcgee/rmhub/internal/tokens"
)

// MaxBody is the maximum size of a JSON request body.
const MaxBody = 1 << 20

// Error codes and their status codes.
const (
	CodeBadRequest         = "bad_request"
	CodeUnauthorized       = "unauthorized"
	CodeForbidden          = "forbidden"
	CodeNotFound           = "not_found"
	CodeConflict           = "conflict"
	CodePreconditionFailed = "precondition_failed"
	CodeRateLimited        = "rate_limited"
	CodeInternal           = "internal"
)

var statusOf = map[string]int{
	CodeBadRequest:         http.StatusBadRequest,
	CodeUnauthorized:       http.StatusUnauthorized,
	CodeForbidden:          http.StatusForbidden,
	CodeNotFound:           http.StatusNotFound,
	CodeConflict:           http.StatusConflict,
	CodePreconditionFailed: http.StatusPreconditionFailed,
	CodeRateLimited:        http.StatusTooManyRequests,
	CodeInternal:           http.StatusInternalServerError,
}

// Error is an API error with a code and a message.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// RetryAfter is the wait time of an error with the code
	// rate_limited. It goes into the Retry-After header.
	RetryAfter time.Duration `json:"-"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Status returns the HTTP status of the error code.
func (e *Error) Status() int {
	if s, ok := statusOf[e.Code]; ok {
		return s
	}
	return http.StatusInternalServerError
}

// Errorf returns an API error.
func Errorf(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// take takes one unit from the bucket of the key. If the bucket is
// empty, it returns an error with the code rate_limited. The text what
// names the limit, for example "API requests of this token".
func take(l *ratelimit.Limiter, key, what string) error {
	ok, wait := l.Allow(key)
	if ok {
		return nil
	}
	e := Errorf(CodeRateLimited, "too many %s in one minute", what)
	e.RetryAfter = wait
	return e
}

// ErrorBody is the body of each error response.
type ErrorBody struct {
	Error *Error `json:"error"`
}

// WriteJSON writes v as a JSON response.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	_ = enc.Encode(v)
}

// WriteError writes an error response. An error that is not an *Error
// becomes code internal, and the log gets the details.
func WriteError(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	var e *Error
	if !errors.As(err, &e) {
		log.Error("api request failed", "method", r.Method, "path", r.URL.Path, "err", err)
		e = Errorf(CodeInternal, "internal error")
	}
	if e.Code == CodeRateLimited {
		w.Header().Set("Retry-After", ratelimit.RetryAfter(e.RetryAfter))
	}
	WriteJSON(w, e.Status(), ErrorBody{Error: e})
}

// DecodeJSON reads a JSON request body into v. A body that is not JSON,
// has unknown fields or has the wrong media type gives code bad_request.
func DecodeJSON(r *http.Request, v any) error {
	if ct := r.Header.Get("Content-Type"); ct != "" {
		mt, _, err := mime.ParseMediaType(ct)
		if err != nil || mt != "application/json" {
			return Errorf(CodeBadRequest, "the request body must have the media type application/json")
		}
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, MaxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return Errorf(CodeBadRequest, "the request body is empty")
		}
		return Errorf(CodeBadRequest, "the request body is not valid JSON: %v", err)
	}
	if dec.More() {
		return Errorf(CodeBadRequest, "the request body has more than one JSON value")
	}
	return nil
}

// Principal is the identity of an authenticated request.
type Principal struct {
	// Admin is true for the admin token.
	Admin bool
	// Account and TokenID identify an account token.
	Account string
	TokenID string
	// Chain is the token and its ancestors, from tokens.Store.Check.
	// Chain[0] is the token itself.
	Chain []tokens.Token
}

// Scoped tells if the principal is a scoped token.
func (p *Principal) Scoped() bool { return len(p.Chain) > 0 && p.Chain[0].Scoped() }

// TokenKind authenticates one kind of bearer token.
type TokenKind interface {
	// Match tells if the token has the form of this kind.
	Match(token string) bool
	// Authenticate returns the principal, or an *Error with the code
	// unauthorized. For a valid token above its request limit, it
	// returns an *Error with the code rate_limited.
	Authenticate(ctx context.Context, token string) (*Principal, error)
}

type principalKey struct{}

// PrincipalFrom returns the principal of an authenticated request.
func PrincipalFrom(ctx context.Context) *Principal {
	p, _ := ctx.Value(principalKey{}).(*Principal)
	return p
}

// Auth checks the bearer token of each request with the token kinds, in
// order, and gives the principal to h through the request context.
func Auth(kinds []TokenKind, h HandlerFunc) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		hdr := r.Header.Get("Authorization")
		if hdr == "" {
			return Errorf(CodeUnauthorized, "the request has no Authorization header")
		}
		if len(hdr) < 7 || !strings.EqualFold(hdr[:7], "bearer ") {
			return Errorf(CodeUnauthorized, "the Authorization header must be Bearer <token>")
		}
		tok := strings.TrimSpace(hdr[7:])
		for _, k := range kinds {
			if !k.Match(tok) {
				continue
			}
			p, err := k.Authenticate(r.Context(), tok)
			if err != nil {
				return err
			}
			return h(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, p)))
		}
		return Errorf(CodeUnauthorized, "the token is not valid")
	}
}

// Mux routes the API paths. Each path that has no handler gets an error
// body with code not_found.
type Mux struct {
	mux *http.ServeMux
	log *slog.Logger
}

// NewMux returns an empty API mux.
func NewMux(log *slog.Logger) *Mux {
	m := &Mux{mux: http.NewServeMux(), log: log}
	m.mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, log, Errorf(CodeNotFound, "no endpoint %s %s", r.Method, r.URL.Path))
	})
	return m
}

// HandlerFunc is an API handler that returns an error.
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

// Handle adds a handler for a pattern such as "GET /api/v1/account".
func (m *Mux) Handle(pattern string, h HandlerFunc) {
	m.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			WriteError(w, r, m.log, err)
		}
	})
}

// HandleRaw adds an http.Handler for a pattern.
func (m *Mux) HandleRaw(pattern string, h http.Handler) { m.mux.Handle(pattern, h) }

// ServeHTTP routes the request. The catch-all pattern "/api/" matches
// each method, so a wrong method also gets code not_found and never the
// plain-text 405 response of the standard mux.
func (m *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) { m.mux.ServeHTTP(w, r) }

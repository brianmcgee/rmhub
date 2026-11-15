package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/brianmcgee/rmhub/internal/xdg"
)

// Exit codes of the client.
const (
	exitOK         = 0
	exitUnexpected = 1
	exitUsage      = 2
	exitAuth       = 3
	exitNotFound   = 4
	exitConflict   = 5
	// exitRateLimited is for status 429. The client does not send the
	// request again: the caller decides to wait or to stop.
	exitRateLimited = 6
)

// exitError is an error with an exit code.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) Unwrap() error { return e.err }

func usageErr(format string, args ...any) error {
	return &exitError{code: exitUsage, err: fmt.Errorf(format, args...)}
}

// apiError is an error response of the server.
type apiError struct {
	status  int
	code    string
	message string
	body    []byte
	// retryAfter is the number of seconds of the Retry-After header of
	// a response with status 429, or 0.
	retryAfter int
}

func (e *apiError) Error() string {
	text := fmt.Sprintf("server answered with status %d", e.status)
	if e.code != "" {
		text = fmt.Sprintf("%s (%s)", e.message, e.code)
	}
	if e.retryAfter > 0 {
		text += fmt.Sprintf("; try again in %d seconds", e.retryAfter)
	}
	return text
}

// exitCode maps an error to the exit code of the client.
func exitCode(err error) int {
	var ee *exitError
	if errors.As(err, &ee) {
		return ee.code
	}
	var ae *apiError
	if errors.As(err, &ae) {
		switch ae.status {
		case http.StatusUnauthorized, http.StatusForbidden:
			return exitAuth
		case http.StatusNotFound:
			return exitNotFound
		case http.StatusConflict, http.StatusPreconditionFailed:
			return exitConflict
		case http.StatusTooManyRequests:
			return exitRateLimited
		}
	}
	return exitUnexpected
}

// clientFile is the form of client.toml.
type clientFile struct {
	URL   string `toml:"url"`
	Token string `toml:"token"`
}

// settings are the client settings and their sources.
type settings struct {
	URL   string
	Token string
}

func clientFilePath() string { return filepath.Join(xdg.ConfigHome(), "rmhub", "client.toml") }

// resolveSettings gives the settings in this order: the options, the
// environment, client.toml.
func resolveSettings(flagURL, flagToken string, getenv func(string) string, stderr io.Writer) (settings, error) {
	s := settings{URL: flagURL, Token: flagToken}
	if s.URL == "" {
		s.URL = getenv("RMHUB_URL")
	}
	if s.Token == "" {
		s.Token = getenv("RMHUB_TOKEN")
	}
	if s.URL == "" || s.Token == "" {
		path := clientFilePath()
		f, err := readClientFile(path, stderr)
		if err != nil {
			return s, err
		}
		if s.URL == "" {
			s.URL = f.URL
		}
		if s.Token == "" {
			s.Token = f.Token
		}
	}
	if s.URL == "" {
		return s, usageErr("no server URL: set RMHUB_URL, the option --url or url in %s", clientFilePath())
	}
	if s.Token == "" {
		return s, usageErr("no token: set RMHUB_TOKEN, the option --token or token in %s", clientFilePath())
	}
	s.URL = strings.TrimRight(s.URL, "/")
	return s, nil
}

func readClientFile(path string, stderr io.Writer) (clientFile, error) {
	var f clientFile
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return f, usageErr("%s: %v", path, err)
	}
	if info.Mode().Perm()&0o044 != 0 {
		_, _ = fmt.Fprintf(stderr, "rmhub: warning: other users can read %s; run chmod 600 %s\n", path, path)
	}
	md, err := toml.DecodeFile(path, &f)
	if err != nil {
		return f, usageErr("%s: %v", path, err)
	}
	if und := md.Undecoded(); len(und) > 0 {
		return f, usageErr("%s: unknown key %s", path, und[0].String())
	}
	return f, nil
}

// client calls the rmhub API.
type client struct {
	s    settings
	http *http.Client
}

func newClient(s settings) *client {
	return &client{s: s, http: &http.Client{Timeout: 60 * time.Second}}
}

// do sends a request. With out set, it decodes the JSON response into out
// and returns the raw body.
func (c *client) do(ctx context.Context, method, path string, body, out any) ([]byte, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.s.URL+path, rd)
	if err != nil {
		return nil, usageErr("server URL %q: %v", c.s.URL, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.s.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 {
		return nil, parseAPIError(res, b)
	}
	if out != nil && len(b) > 0 {
		if err := json.Unmarshal(b, out); err != nil {
			return nil, fmt.Errorf("server answer is not valid JSON: %w", err)
		}
	}
	return b, nil
}

// parseAPIError reads the error body of a response.
func parseAPIError(res *http.Response, b []byte) error {
	e := &apiError{status: res.StatusCode}
	if e.status == http.StatusTooManyRequests {
		// A value that is not a number of seconds gives 0.
		e.retryAfter, _ = strconv.Atoi(res.Header.Get("Retry-After"))
	}
	var eb struct {
		Error struct{ Code, Message string }
	}
	if json.Unmarshal(b, &eb) == nil && eb.Error.Code != "" {
		e.code, e.message, e.body = eb.Error.Code, eb.Error.Message, bytes.TrimSpace(b)
	}
	return e
}

// parseExpires reads an expiry time as RFC 3339 time or as a duration
// from now, for example 720h.
func parseExpires(s string, now time.Time) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t, nil
	}
	if d, err := time.ParseDuration(s); err == nil && d > 0 {
		t := now.Add(d).UTC().Truncate(time.Second)
		return &t, nil
	}
	return nil, usageErr("--expires %q: give an RFC 3339 time (2027-01-31T00:00:00Z) or a duration (720h)", s)
}

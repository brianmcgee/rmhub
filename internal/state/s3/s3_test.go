package s3_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/brianmcgee/rmhub/internal/s3conn"
	"github.com/brianmcgee/rmhub/internal/s3conn/s3test"
	"github.com/brianmcgee/rmhub/internal/state"
	states3 "github.com/brianmcgee/rmhub/internal/state/s3"
	"github.com/brianmcgee/rmhub/internal/state/statetest"
)

func TestContract(t *testing.T) {
	statetest.Run(t, func(t *testing.T) state.Store {
		c, err := s3conn.New(context.Background(), s3test.Settings(t))
		if err != nil {
			t.Fatal(err)
		}
		return states3.New(c)
	})
}

func fakeStore(t *testing.T, h http.HandlerFunc) *states3.Store {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	t.Setenv("AWS_ACCESS_KEY_ID", "x")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "y")
	c, err := s3conn.New(context.Background(), s3conn.Settings{
		Endpoint: srv.URL, Bucket: "b", PathStyle: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return states3.New(c)
}

// TestLostConnection checks that a write whose answer is lost is a
// failure and not a conflict.
func TestLostConnection(t *testing.T) {
	calls := 0
	s := fakeStore(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.(*net.TCPConn).Close()
		}
	})
	_, err := s.PutIfAbsent(context.Background(), "k", []byte("v"))
	if err == nil || errors.Is(err, state.ErrConflict) {
		t.Fatalf("err = %v, want a failure that is not a conflict", err)
	}
	if calls != 1 {
		t.Fatalf("server got %d requests, want 1 (no retry)", calls)
	}
}

func TestStatusMapping(t *testing.T) {
	for _, tc := range []struct {
		status  int
		code    string
		ifMatch bool
		want    error
	}{
		{412, "PreconditionFailed", false, state.ErrConflict},
		{412, "PreconditionFailed", true, state.ErrConflict},
		{404, "NoSuchKey", true, state.ErrConflict},
	} {
		s := fakeStore(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(`<?xml version="1.0"?><Error><Code>` + tc.code + `</Code><Message>x</Message></Error>`))
		})
		var err error
		if tc.ifMatch {
			_, err = s.PutIfMatch(context.Background(), "k", []byte("v"), `"etag"`)
		} else {
			_, err = s.PutIfAbsent(context.Background(), "k", []byte("v"))
		}
		if !errors.Is(err, tc.want) {
			t.Errorf("status %d ifMatch=%v: err = %v, want %v", tc.status, tc.ifMatch, err, tc.want)
		}
	}
}

func TestRepeated409IsFailure(t *testing.T) {
	calls := 0
	s := fakeStore(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(409)
		_, _ = w.Write([]byte(`<?xml version="1.0"?><Error><Code>ConditionalRequestConflict</Code><Message>x</Message></Error>`))
	})
	_, err := s.PutIfAbsent(context.Background(), "k", []byte("v"))
	if err == nil || errors.Is(err, state.ErrConflict) {
		t.Fatalf("err = %v, want a failure", err)
	}
	if calls != 2 {
		t.Fatalf("server got %d requests, want 2", calls)
	}
}

package app

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"
)

// pairFrom sends a pairing request with an X-Forwarded-For header.
func (e *env) pairFrom(addr, c, id string) tresp {
	e.t.Helper()
	b, _ := json.Marshal(map[string]string{"code": c, "deviceDesc": "remarkable", "deviceID": id})
	var hdr map[string]string
	if addr != "" {
		hdr = map[string]string{"X-Forwarded-For": addr}
	}
	return e.tab("POST", "/token/json/2/device/new", "", b, hdr)
}

// limited checks that the response is the answer above a limit.
func limited(t *testing.T, status int, retryAfter string) {
	t.Helper()
	if status != 429 {
		t.Fatalf("status %d, want 429", status)
	}
	if n, err := strconv.Atoi(retryAfter); err != nil || n < 1 || n > 60 {
		t.Errorf("Retry-After = %q, want an integer from 1 to 60", retryAfter)
	}
}

func TestPairingLimit(t *testing.T) {
	e := newEnv(t, true)
	_, owner := e.user("ada@example.org")
	c := e.code(owner)
	for i := range 10 {
		if r := e.pairFrom("", "zzzzzzzz", "dev"); r.status != 400 {
			t.Fatalf("request %d: status %d, want 400", i+1, r.status)
		}
	}
	// A valid code does not help above the limit, and the code stays
	// unused. The header of a peer that is not trusted has no effect.
	r := e.pairFrom("203.0.113.9", c.Code, "dev")
	limited(t, r.status, r.header.Get("Retry-After"))
	if len(r.body) != 0 {
		t.Errorf("body %q, want an empty body", r.body)
	}
	// The other tablet endpoints have no limit.
	if r := e.tab("GET", "/healthz", "", nil, nil); r.status != 200 {
		t.Errorf("healthz: status %d", r.status)
	}
}

func TestPairingLimitOff(t *testing.T) {
	e := newEnvWith(t, true, "[limits]\npairing_per_minute = -1\n")
	for i := range 30 {
		if r := e.pairFrom("", "zzzzzzzz", "dev"); r.status != 400 {
			t.Fatalf("request %d: status %d, want 400", i+1, r.status)
		}
	}
}

func TestPairingLimitBehindProxy(t *testing.T) {
	e := newEnvWith(t, true, "[listen.tablet]\ntrusted_proxies = [\"127.0.0.1\", \"::1\"]\n")
	_, owner := e.user("ada@example.org")
	for range 10 {
		e.pairFrom("198.51.100.7, 203.0.113.9", "zzzzzzzz", "dev")
	}
	r := e.pairFrom("203.0.113.9", "zzzzzzzz", "dev")
	limited(t, r.status, r.header.Get("Retry-After"))
	// The leftmost address comes from the client. It must not give the
	// client a new bucket.
	if r := e.pairFrom("192.0.2.50, 203.0.113.9", "zzzzzzzz", "dev"); r.status != 429 {
		t.Errorf("false address on the left: status %d, want 429", r.status)
	}
	if r := e.pairFrom("198.51.100.7", e.code(owner).Code, "dev"); r.status != 200 {
		t.Errorf("second address: status %d, want 200", r.status)
	}
}

func (e *env) pairingKeys() []string {
	e.t.Helper()
	keys, err := e.st.List(context.Background(), "pairing/", "", 0)
	if err != nil {
		e.t.Fatal(err)
	}
	return keys
}

func TestPairingRecordsAreDeleted(t *testing.T) {
	e := newEnv(t, true)
	_, owner := e.user("ada@example.org")
	c := e.code(owner)
	if r := e.pair(c.Code, "dev-a"); r.status != 200 {
		t.Fatalf("pair: status %d", r.status)
	}
	if keys := e.pairingKeys(); len(keys) != 0 {
		t.Errorf("records after the use of the code: %v", keys)
	}
	if r := e.pair(c.Code, "dev-b"); r.status != 400 {
		t.Errorf("second use: status %d, want 400", r.status)
	}

	e.code(owner)
	e.code(owner)
	e.advance(4 * time.Minute)
	live := e.code(owner)
	e.advance(2 * time.Minute)
	// Two codes are expired now. The next code removes them.
	last := e.code(owner)
	keys := e.pairingKeys()
	if len(keys) != 2 {
		t.Fatalf("records after a new code: %v, want 2", keys)
	}
	for _, c := range []code{live, last} {
		if r := e.pair(c.Code, "dev-"+c.Code); r.status != 200 {
			t.Errorf("code %s: status %d", c.Code, r.status)
		}
	}
}

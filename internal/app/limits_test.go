package app

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestAPILimit(t *testing.T) {
	e := newEnvWith(t, true, "[limits]\napi_per_minute = 5\n")
	acct, owner := e.user("ada@example.org")
	tr := e.tree(acct)
	tr.Folder("agent", "Agent", "")
	tr.Commit(4)
	_, tok := tokenText(t, e.scoped(owner, map[string]any{"label": "agent", "grants": grantsOf("agent", "read")}))

	// A request with a token that is not valid takes no unit.
	bad := tok[:len(tok)-1] + "a"
	if tok == bad {
		bad = tok[:len(tok)-1] + "b"
	}
	for range 8 {
		if r := e.req("GET", "/api/v1/library", bad, nil); r.status != 401 {
			t.Fatalf("bad token: status %d, want 401", r.status)
		}
	}
	for i := range 5 {
		if r := e.req("GET", "/api/v1/library", tok, nil); r.status != 200 {
			t.Fatalf("request %d: status %d %s", i+1, r.status, r.body)
		}
	}
	// A refused request does not change data.
	r := e.jreq("POST", "/api/v1/account/tokens", tok, map[string]any{"grants": grantsOf("agent", "read")})
	limited(t, r.status, r.header.Get("Retry-After"))
	var body struct {
		Error struct{ Code, Message string }
	}
	r.json(t, &body)
	if body.Error.Code != "rate_limited" || body.Error.Message == "" {
		t.Errorf("error body %s", r.body)
	}
	if got := r.header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
	// The owner token has its own bucket. It used one unit for the
	// scoped token.
	var all []struct{ ID string }
	lr := e.req("GET", "/api/v1/account/tokens", owner, nil)
	if lr.status != 200 {
		t.Fatalf("owner token: status %d %s", lr.status, lr.body)
	}
	lr.json(t, &all)
	if len(all) != 2 {
		t.Errorf("tokens %s, want the owner token and the first scoped token", lr.body)
	}
	for i := range 10 {
		if r := e.req("GET", "/healthz", "", nil); r.status != 200 {
			t.Fatalf("healthz %d: status %d", i+1, r.status)
		}
	}
}

func TestRenderWorkers(t *testing.T) {
	e := newEnvWith(t, true, "[render]\nmax_workers = 2\n")
	if got := e.app.Library.Render.Workers(); got != 2 {
		t.Errorf("Workers = %d, want 2", got)
	}
}

func TestAPILimitOff(t *testing.T) {
	e := newEnvWith(t, true, "[limits]\napi_per_minute = -1\n")
	_, owner := e.user("ada@example.org")
	for i := range 1000 {
		if r := e.req("GET", "/api/v1/account", owner, nil); r.status != 200 {
			t.Fatalf("request %d: status %d", i+1, r.status)
		}
	}
}

func TestRenderLimit(t *testing.T) {
	e := newEnvWith(t, true, "[limits]\nrender_per_minute = 2\n")
	acct, owner := e.user("ada@example.org")
	tr := e.tree(acct)
	tr.Folder("agent", "Agent", "")
	putNotebook(tr, "nb", "Sketch", "agent", 3)
	tr.Commit(4)
	_, tok := tokenText(t, e.scoped(owner, map[string]any{"label": "agent", "grants": grantsOf("agent", "read")}))
	page := func(tok string, n int, hdr ...string) hresp {
		return e.req("GET", fmt.Sprintf("/api/v1/library/nb/pages/%d/render", n), tok, nil, hdr...)
	}
	first := page(owner, 1)
	if first.status != 200 {
		t.Fatalf("page 1: status %d %s", first.status, first.body)
	}
	if r := page(owner, 2); r.status != 200 {
		t.Fatalf("page 2: status %d %s", r.status, r.body)
	}
	// The limit is for the account, so a second token has no bucket of
	// its own.
	r := page(tok, 3)
	limited(t, r.status, r.header.Get("Retry-After"))
	if r := page(owner, 3); r.status != 429 {
		t.Errorf("page 3 with the owner token: status %d, want 429", r.status)
	}
	// The cache and the ETag check need no render.
	if r := page(tok, 1); r.status != 200 || string(r.body) != string(first.body) {
		t.Errorf("page 1 from the cache: status %d", r.status)
	}
	if r := page(tok, 3, "If-None-Match", r.header.Get("ETag")); r.status != 304 {
		t.Errorf("page 3 with its ETag: status %d, want 304", r.status)
	}
	// A different width is a new render.
	if r := e.req("GET", "/api/v1/library/nb/pages/1/render?width=300", owner, nil); r.status != 429 {
		t.Errorf("page 1 with a new width: status %d, want 429", r.status)
	}
}

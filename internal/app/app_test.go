package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/brianmcgee/rmhub/internal/api"
	blobfs "github.com/brianmcgee/rmhub/internal/blob/fs"
	"github.com/brianmcgee/rmhub/internal/config"
	"github.com/brianmcgee/rmhub/internal/devices"
	"github.com/brianmcgee/rmhub/internal/server"
	"github.com/brianmcgee/rmhub/internal/state"
	"github.com/brianmcgee/rmhub/internal/state/sqlite"
	"github.com/brianmcgee/rmhub/internal/tablet"
	"github.com/brianmcgee/rmhub/internal/tokens"
)

type env struct {
	t      *testing.T
	app    *App
	tablet *httptest.Server
	api    *httptest.Server
	admin  string
	now    time.Time
	st     state.Store
	logs   *syncBuffer
	mu     sync.Mutex
}

func (e *env) clock() time.Time {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.now
}

func (e *env) advance(d time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.now = e.now.Add(d)
}

// newEnv starts both handlers on a new SQLite state store. With admin
// false, no admin token is configured.
func newEnv(t *testing.T, admin bool) *env { return newEnvWith(t, admin, "") }

// newEnvWith is newEnv with more TOML configuration text.
func newEnvWith(t *testing.T, admin bool, extra string) *env {
	t.Helper()
	dir := t.TempDir()
	cfg, err := config.Parse("data_dir = " + `"` + dir + `"` + "\n" + extra)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Render.CacheDir = filepath.Join(dir, "render")
	e := &env{t: t, now: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)}
	if admin {
		e.admin = tokens.GenerateAdmin()
		cfg.Admin.TokenFile = filepath.Join(dir, "admin-token")
		if err := os.WriteFile(cfg.Admin.TokenFile, []byte(e.admin+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	st, err := sqlite.Open(cfg.State.Path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	blobs, err := blobfs.New(cfg.Blob.Dir)
	if err != nil {
		t.Fatal(err)
	}
	e.logs = &syncBuffer{}
	log := slog.New(slog.NewTextHandler(e.logs, nil))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	a, err := New(ctx, cfg, &server.Stores{State: st, Blob: blobs, TempDir: blobs.TempDir()}, log)
	if err != nil {
		t.Fatal(err)
	}
	a.Pairing.SetClock(e.clock)
	a.Devices.SetClock(e.clock)
	a.Issuer.SetClock(e.clock)
	a.Tokens.SetClock(e.clock)
	a.Tablet.Now = e.clock
	e.app = a
	e.st = st

	tab := a.TabletHandler().(*http.ServeMux)
	// A stand-in for a sync endpoint, for the tests of the middleware.
	tab.Handle("GET /sync/test", a.Tablet.RequireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, tablet.UserFrom(r.Context()).DeviceID)
	})))
	e.tablet = httptest.NewServer(tab)
	e.api = httptest.NewServer(a.APIHandler())
	t.Cleanup(e.tablet.Close)
	t.Cleanup(e.api.Close)
	return e
}

// syncBuffer is a log buffer that the server goroutines can share.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

type resp struct {
	status int
	ctype  string
	body   []byte
}

func (r resp) json(t *testing.T, v any) {
	t.Helper()
	if err := json.Unmarshal(r.body, v); err != nil {
		t.Fatalf("decode %q: %v", r.body, err)
	}
}

func (r resp) errCode(t *testing.T) string {
	t.Helper()
	var b struct {
		Error struct{ Code, Message string }
	}
	r.json(t, &b)
	if b.Error.Message == "" {
		t.Fatalf("error body without message: %s", r.body)
	}
	return b.Error.Code
}

func (e *env) do(method, url, token string, body any) resp {
	e.t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rd = strings.NewReader(b)
	default:
		buf, _ := json.Marshal(b)
		rd = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, url, rd)
	if err != nil {
		e.t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	return resp{status: res.StatusCode, ctype: res.Header.Get("Content-Type"), body: b}
}

func (e *env) apiDo(method, path, token string, body any) resp {
	return e.do(method, e.api.URL+path, token, body)
}

// user creates an account and an owner token.
func (e *env) user(email string) (id, token string) {
	e.t.Helper()
	r := e.apiDo("POST", "/api/v1/admin/users", e.admin, map[string]string{"email": email, "name": "Ada"})
	if r.status != 201 {
		e.t.Fatalf("create user: %d %s", r.status, r.body)
	}
	var u struct{ ID string }
	r.json(e.t, &u)
	r = e.apiDo("POST", "/api/v1/admin/users/"+u.ID+"/tokens", e.admin, map[string]string{"label": "test"})
	if r.status != 201 {
		e.t.Fatalf("issue token: %d %s", r.status, r.body)
	}
	var tok struct{ Token string }
	r.json(e.t, &tok)
	return u.ID, tok.Token
}

func TestErrorStatus(t *testing.T) {
	want := map[string]int{
		"bad_request": 400, "unauthorized": 401, "forbidden": 403, "not_found": 404,
		"conflict": 409, "precondition_failed": 412, "internal": 500,
	}
	for code, status := range want {
		if got := api.Errorf(code, "x").Status(); got != status {
			t.Errorf("%s: status %d, want %d", code, got, status)
		}
	}
}

func TestAPIBase(t *testing.T) {
	e := newEnv(t, true)
	r := e.apiDo("POST", "/api/v1/admin/users", e.admin, "name=x")
	if r.status != 400 || r.errCode(t) != "bad_request" || r.ctype != "application/json" {
		t.Fatalf("body that is not JSON: %d %s %s", r.status, r.ctype, r.body)
	}
	r = e.apiDo("GET", "/api/v1/nothing", e.admin, nil)
	if r.status != 404 || r.errCode(t) != "not_found" {
		t.Fatalf("unknown path: %d %s", r.status, r.body)
	}
	r = e.apiDo("PUT", "/api/v1/account", e.admin, nil)
	if r.status != 404 || r.errCode(t) != "not_found" {
		t.Fatalf("wrong method: %d %s", r.status, r.body)
	}
	// The API is not on the tablet listener.
	r = e.do("GET", e.tablet.URL+"/api/v1/account/devices", e.admin, nil)
	if r.status != 404 {
		t.Fatalf("API path on the tablet listener: %d", r.status)
	}
	for _, l := range []string{e.api.URL, e.tablet.URL} {
		if r := e.do("GET", l+"/healthz", "", nil); r.status != 200 {
			t.Fatalf("healthz on %s: %d", l, r.status)
		}
	}
}

func TestBearer(t *testing.T) {
	e := newEnv(t, true)
	acct, _ := e.user("ada@example.org")
	unknown := "rmh_" + acct + "_" + strings.Repeat("a", 16) + "_" + strings.Repeat("b", 52)
	for name, tok := range map[string]string{"missing": "", "malformed": "nonsense", "unknown": unknown, "wrong admin": tokens.GenerateAdmin()} {
		r := e.apiDo("GET", "/api/v1/account", tok, nil)
		if r.status != 401 || r.errCode(t) != "unauthorized" {
			t.Errorf("%s token: %d %s", name, r.status, r.body)
		}
	}
	req, _ := http.NewRequest("GET", e.api.URL+"/api/v1/account", nil)
	req.Header.Set("Authorization", "Basic eDp5")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatalf("basic auth: %d", res.StatusCode)
	}
}

func TestAdmin(t *testing.T) {
	e := newEnv(t, true)
	if r := e.apiDo("GET", "/api/v1/admin/users", e.admin, nil); r.status != 200 || strings.TrimSpace(string(r.body)) != "[]" {
		t.Fatalf("list: %d %s", r.status, r.body)
	}
	r := e.apiDo("POST", "/api/v1/admin/users", e.admin, map[string]string{"email": "Ada@Example.org"})
	var u struct{ ID, Email, Name string }
	r.json(t, &u)
	if r.status != 201 || len(u.ID) != 26 || u.Email != "Ada@Example.org" {
		t.Fatalf("create: %d %s", r.status, r.body)
	}
	if r := e.apiDo("POST", "/api/v1/admin/users", e.admin, map[string]string{"email": "ADA@example.org"}); r.status != 409 || r.errCode(t) != "conflict" {
		t.Fatalf("email in use: %d %s", r.status, r.body)
	}
	if r := e.apiDo("POST", "/api/v1/admin/users", e.admin, map[string]string{"name": "x"}); r.status != 400 || r.errCode(t) != "bad_request" {
		t.Fatalf("email missing: %d %s", r.status, r.body)
	}
	if r := e.apiDo("GET", "/api/v1/admin/users/"+u.ID, e.admin, nil); r.status != 200 {
		t.Fatalf("get: %d %s", r.status, r.body)
	}
	if r := e.apiDo("GET", "/api/v1/admin/users/"+strings.Repeat("a", 26), e.admin, nil); r.status != 404 || r.errCode(t) != "not_found" {
		t.Fatalf("missing user: %d %s", r.status, r.body)
	}
	if r := e.apiDo("POST", "/api/v1/admin/users/"+strings.Repeat("a", 26)+"/tokens", e.admin, map[string]string{}); r.status != 404 {
		t.Fatalf("token for missing user: %d %s", r.status, r.body)
	}
	_, owner := e.user("bob@example.org")
	if r := e.apiDo("GET", "/api/v1/admin/users", owner, nil); r.status != 403 || r.errCode(t) != "forbidden" {
		t.Fatalf("owner on admin endpoint: %d %s", r.status, r.body)
	}
	if r := e.apiDo("GET", "/api/v1/account", e.admin, nil); r.status != 403 {
		t.Fatalf("admin on owner endpoint: %d %s", r.status, r.body)
	}
	var list []struct{ ID string }
	e.apiDo("GET", "/api/v1/admin/users", e.admin, nil).json(t, &list)
	if len(list) != 2 || list[0].ID > list[1].ID {
		t.Fatalf("list %+v", list)
	}
}

func TestNoAdminToken(t *testing.T) {
	e := newEnv(t, false)
	if r := e.apiDo("GET", "/api/v1/admin/users", tokens.GenerateAdmin(), nil); r.status != 401 {
		t.Fatalf("admin request without admin.token_file: %d", r.status)
	}
}

func TestAdminTokenFileErrors(t *testing.T) {
	dir := t.TempDir()
	st, err := sqlite.Open(filepath.Join(dir, "s.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	short := filepath.Join(dir, "short")
	_ = os.WriteFile(short, []byte("rmh_admin_abc"), 0o600)
	for _, path := range []string{short, filepath.Join(dir, "missing")} {
		cfg, _ := config.Parse("data_dir = " + `"` + dir + `"`)
		cfg.Admin.TokenFile = path
		_, err := New(context.Background(), cfg, &server.Stores{State: st}, slog.Default())
		if err == nil || !strings.Contains(err.Error(), "admin.token_file") {
			t.Fatalf("token file %s: %v", path, err)
		}
	}
}

func TestOwnerTokens(t *testing.T) {
	e := newEnv(t, true)
	acct, owner := e.user("ada@example.org")
	if !strings.HasPrefix(owner, "rmh_"+acct+"_") {
		t.Fatalf("token %q", owner)
	}
	var me struct{ ID string }
	e.apiDo("GET", "/api/v1/account", owner, nil).json(t, &me)
	if me.ID != acct {
		t.Fatalf("who am I: %q", me.ID)
	}

	exp := e.now.Add(time.Hour).Format(time.RFC3339)
	r := e.apiDo("POST", "/api/v1/account/tokens", owner, map[string]string{"label": "laptop", "expiresAt": exp})
	var created struct{ ID, Token, Label, ExpiresAt string }
	r.json(t, &created)
	if r.status != 201 || created.Label != "laptop" || created.ExpiresAt != exp {
		t.Fatalf("create: %d %s", r.status, r.body)
	}
	r = e.apiDo("GET", "/api/v1/account/tokens", owner, nil)
	if r.status != 200 || bytes.Contains(r.body, []byte(strings.Split(created.Token, "_")[3])) || bytes.Contains(r.body, []byte("secret")) {
		t.Fatalf("list has a secret: %s", r.body)
	}
	var list []map[string]any
	r.json(t, &list)
	if len(list) != 2 {
		t.Fatalf("list %s", r.body)
	}

	// Expired token.
	if r := e.apiDo("GET", "/api/v1/account", created.Token, nil); r.status != 200 {
		t.Fatalf("before expiry: %d", r.status)
	}
	e.advance(time.Hour + time.Minute)
	if r := e.apiDo("GET", "/api/v1/account", created.Token, nil); r.status != 401 {
		t.Fatalf("expired: %d", r.status)
	}

	// Another account cannot revoke the token.
	_, other := e.user("bob@example.org")
	if r := e.apiDo("DELETE", "/api/v1/account/tokens/"+created.ID, other, nil); r.status != 404 {
		t.Fatalf("foreign revoke: %d %s", r.status, r.body)
	}
	if r := e.apiDo("DELETE", "/api/v1/account/tokens/"+created.ID, owner, nil); r.status != 204 {
		t.Fatalf("revoke: %d %s", r.status, r.body)
	}
	ownID := strings.Split(owner, "_")[2]
	if r := e.apiDo("DELETE", "/api/v1/account/tokens/"+ownID, owner, nil); r.status != 204 {
		t.Fatalf("revoke own token: %d %s", r.status, r.body)
	}
	if r := e.apiDo("GET", "/api/v1/account", owner, nil); r.status != 401 {
		t.Fatalf("revoked token: %d", r.status)
	}
}

func TestJWTKeyShared(t *testing.T) {
	st, err := sqlite.Open(filepath.Join(t.TempDir(), "s.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	ctx := context.Background()
	k1, err := tablet.LoadKey(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	k2, err := tablet.LoadKey(ctx, st)
	if err != nil || !bytes.Equal(k1, k2) || len(k1) != 32 {
		t.Fatalf("keys differ: %x %x %v", k1, k2, err)
	}
}

type code struct {
	Code      string
	ExpiresAt time.Time
}

func (e *env) code(owner string) code {
	e.t.Helper()
	r := e.apiDo("POST", "/api/v1/account/pairing-codes", owner, nil)
	if r.status != 201 {
		e.t.Fatalf("pairing code: %d %s", r.status, r.body)
	}
	var c code
	r.json(e.t, &c)
	return c
}

func (e *env) pair(c, id string) resp {
	return e.do("POST", e.tablet.URL+"/token/json/2/device/new", "", map[string]string{"code": c, "deviceDesc": "remarkable", "deviceID": id})
}

func decodeClaims(t *testing.T, tok string) map[string]any {
	t.Helper()
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("token %q", tok)
	}
	var hdr map[string]any
	b, _ := base64.RawURLEncoding.DecodeString(parts[0])
	_ = json.Unmarshal(b, &hdr)
	if hdr["alg"] != "HS256" || hdr["kid"] != "1" {
		t.Fatalf("header %v", hdr)
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var c map[string]any
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestPairing(t *testing.T) {
	e := newEnv(t, true)
	acct, owner := e.user("ada@example.org")
	c := e.code(owner)
	if len(c.Code) != 8 || strings.ToLower(c.Code) != c.Code || !c.ExpiresAt.Equal(e.now.Add(5*time.Minute)) {
		t.Fatalf("code %+v", c)
	}

	// A bad device ID does not use the code.
	if r := e.pair(c.Code, "../x"); r.status != 400 {
		t.Fatalf("device ID ../x: %d", r.status)
	}
	long := map[string]string{"code": c.Code, "deviceDesc": strings.Repeat("d", 257), "deviceID": "RM110-abc-123"}
	if r := e.do("POST", e.tablet.URL+"/token/json/2/device/new", "", long); r.status != 400 {
		t.Fatalf("device description of 257 bytes: %d", r.status)
	}
	if r := e.pair(c.Code, strings.Repeat("a", 129)); r.status != 400 {
		t.Fatalf("long device ID: %d", r.status)
	}
	r := e.pair(strings.ToUpper(c.Code), "RM110-abc-123")
	if r.status != 200 || r.ctype != "text/plain" {
		t.Fatalf("pair: %d %s %s", r.status, r.ctype, r.body)
	}
	claims := decodeClaims(t, string(r.body))
	want := map[string]any{"auth0-userid": "auth0|" + acct, "device-desc": "remarkable", "device-id": "RM110-abc-123", "aud": "api"}
	for k, v := range want {
		if claims[k] != v {
			t.Errorf("device claim %s = %v, want %v", k, claims[k], v)
		}
	}
	if _, ok := claims["exp"]; ok {
		t.Error("device token has an expiry time")
	}

	// Used code.
	if r := e.pair(c.Code, "other"); r.status != 400 || len(r.body) != 0 {
		t.Fatalf("used code: %d %q", r.status, r.body)
	}
	// Unknown code.
	if r := e.pair("zzzzzzzz", "other"); r.status != 400 || len(r.body) != 0 {
		t.Fatalf("unknown code: %d %q", r.status, r.body)
	}
	// Expired code.
	c2 := e.code(owner)
	e.advance(6 * time.Minute)
	if r := e.pair(c2.Code, "other"); r.status != 400 {
		t.Fatalf("expired code: %d", r.status)
	}

	var devs []struct {
		DeviceID string
		Desc     string
		PairedAt time.Time
	}
	e.apiDo("GET", "/api/v1/account/devices", owner, nil).json(t, &devs)
	if len(devs) != 1 || devs[0].DeviceID != "RM110-abc-123" || devs[0].Desc != "remarkable" || devs[0].PairedAt.IsZero() {
		t.Fatalf("devices %+v", devs)
	}
}

func TestParallelPairing(t *testing.T) {
	e := newEnv(t, true)
	_, owner := e.user("ada@example.org")
	c := e.code(owner)
	var wg sync.WaitGroup
	status := make([]int, 2)
	for i := range status {
		wg.Go(func() { status[i] = e.pair(c.Code, []string{"dev-a", "dev-b"}[i]).status })
	}
	wg.Wait()
	if (status[0] != 200 || status[1] != 400) && (status[0] != 400 || status[1] != 200) {
		t.Fatalf("statuses %v, want one 200 and one 400", status)
	}
}

// paired pairs a device and returns its device token.
func (e *env) paired(owner, id string) string {
	e.t.Helper()
	r := e.pair(e.code(owner).Code, id)
	if r.status != 200 {
		e.t.Fatalf("pair: %d", r.status)
	}
	return string(r.body)
}

func (e *env) userToken(device string) resp {
	return e.do("POST", e.tablet.URL+"/token/json/2/user/new", device, nil)
}

func TestUserToken(t *testing.T) {
	e := newEnv(t, true)
	acct, owner := e.user("ada@example.org")
	device := e.paired(owner, "dev-1")
	r := e.userToken(device)
	if r.status != 200 || r.ctype != "text/plain" {
		t.Fatalf("user token: %d %s", r.status, r.body)
	}
	c := decodeClaims(t, string(r.body))
	now := float64(e.now.Unix())
	want := map[string]any{
		"device-desc": "remarkable", "device-id": "dev-1",
		"scopes": "intgr screenshare docedit sync:tortoise", "level": "connect", "tectonic": "eu",
		"version": float64(10), "iss": "rM WebApp", "sub": "rM User Token",
		"iat": now, "nbf": now, "exp": now + 3*3600,
	}
	for k, v := range want {
		if c[k] != v {
			t.Errorf("claim %s = %v, want %v", k, c[k], v)
		}
	}
	if jti, _ := c["jti"].(string); jti == "" {
		t.Error("claim jti is empty")
	}
	p, _ := c["auth0-profile"].(map[string]any)
	wantP := map[string]any{"UserID": "auth0|" + acct, "Email": "ada@example.org", "Name": "Ada", "EmailVerified": true}
	for k, v := range wantP {
		if p[k] != v {
			t.Errorf("auth0-profile.%s = %v, want %v", k, p[k], v)
		}
	}
	for _, k := range []string{"CreatedAt", "UpdatedAt"} {
		if s, _ := p[k].(string); s == "" {
			t.Errorf("auth0-profile.%s is empty", k)
		} else if _, err := time.Parse(time.RFC3339, s); err != nil {
			t.Errorf("auth0-profile.%s = %q: %v", k, s, err)
		}
	}

	// The user token works on a sync endpoint.
	sync := e.do("GET", e.tablet.URL+"/sync/test", string(r.body), nil)
	if sync.status != 200 || string(sync.body) != "dev-1" {
		t.Fatalf("sync request: %d %s", sync.status, sync.body)
	}
}

func TestTabletAuth(t *testing.T) {
	e := newEnv(t, true)
	acct, owner := e.user("ada@example.org")
	device := e.paired(owner, "dev-1")
	user := string(e.userToken(device).body)

	unauth := func(name, tok string) {
		t.Helper()
		r := e.do("GET", e.tablet.URL+"/sync/test", tok, nil)
		if r.status != 401 || string(r.body) != "Unauthorized" {
			t.Errorf("%s: %d %q", name, r.status, r.body)
		}
	}
	unauth("device token", device)
	unauth("no token", "")
	unauth("garbage", "abc.def.ghi")

	// A token without version 10.
	a, _ := e.app.Accounts.Get(context.Background(), acct)
	other := tablet.NewIssuer([]byte(strings.Repeat("k", 32)))
	foreign, _ := other.UserToken(a, "remarkable", "dev-1", "")
	unauth("token with another key", foreign)
	noVersion := signWithout(t, e, user, "version")
	unauth("token without version", noVersion)

	// Expired user token.
	e.advance(3*time.Hour + time.Second)
	unauth("expired user token", user)
}

// signWithout signs the claims of tok again with the server key, without
// the named claim.
func signWithout(t *testing.T, e *env, tok, claim string) string {
	t.Helper()
	c := decodeClaims(t, tok)
	delete(c, claim)
	key, err := tablet.LoadKey(context.Background(), e.st)
	if err != nil {
		t.Fatal(err)
	}
	jt := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims(c))
	jt.Header["kid"] = "1"
	s, err := jt.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestDeviceRemoval(t *testing.T) {
	e := newEnv(t, true)
	_, owner := e.user("ada@example.org")
	device := e.paired(owner, "dev-1")
	user := string(e.userToken(device).body)
	if r := e.do("GET", e.tablet.URL+"/sync/test", user, nil); r.status != 200 {
		t.Fatalf("before removal: %d", r.status)
	}
	if r := e.apiDo("DELETE", "/api/v1/account/devices/nope", owner, nil); r.status != 404 || r.errCode(t) != "not_found" {
		t.Fatalf("remove unknown device: %d %s", r.status, r.body)
	}
	// Removal in another process: the cache of this process keeps the
	// old result for up to 30 seconds.
	if err := e.st.Delete(context.Background(), deviceKey(e, owner, "dev-1")); err != nil {
		t.Fatal(err)
	}
	if r := e.do("GET", e.tablet.URL+"/sync/test", user, nil); r.status != 200 {
		t.Fatalf("within the cache time: %d", r.status)
	}
	e.advance(31 * time.Second)
	if r := e.do("GET", e.tablet.URL+"/sync/test", user, nil); r.status != 401 {
		t.Fatalf("after the cache time: %d", r.status)
	}
	if r := e.userToken(device); r.status != 401 {
		t.Fatalf("user token for a removed device: %d", r.status)
	}

	// Removal through the API in this process takes effect at once.
	device2 := e.paired(owner, "dev-2")
	user2 := string(e.userToken(device2).body)
	if r := e.do("GET", e.tablet.URL+"/sync/test", user2, nil); r.status != 200 {
		t.Fatalf("dev-2: %d", r.status)
	}
	if r := e.apiDo("DELETE", "/api/v1/account/devices/dev-2", owner, nil); r.status != 204 {
		t.Fatalf("remove: %d %s", r.status, r.body)
	}
	if r := e.do("GET", e.tablet.URL+"/sync/test", user2, nil); r.status != 401 {
		t.Fatalf("after API removal: %d", r.status)
	}
}

// A device that pairs again gets new tokens. The tokens of the earlier
// pairing stay invalid, so a removed device cannot come back with them.
func TestTokensOfAnEarlierPairing(t *testing.T) {
	e := newEnv(t, true)
	_, owner := e.user("ada@example.org")
	device := e.paired(owner, "dev-1")
	user := string(e.userToken(device).body)
	if r := e.apiDo("DELETE", "/api/v1/account/devices/dev-1", owner, nil); r.status != 204 {
		t.Fatalf("remove: %d %s", r.status, r.body)
	}
	device2 := e.paired(owner, "dev-1")
	if r := e.userToken(device); r.status != 401 {
		t.Fatalf("device token of the earlier pairing: %d", r.status)
	}
	if r := e.do("GET", e.tablet.URL+"/sync/test", user, nil); r.status != 401 {
		t.Fatalf("user token of the earlier pairing: %d", r.status)
	}
	r := e.userToken(device2)
	if r.status != 200 {
		t.Fatalf("device token of the new pairing: %d", r.status)
	}
	if r := e.do("GET", e.tablet.URL+"/sync/test", string(r.body), nil); r.status != 200 {
		t.Fatalf("user token of the new pairing: %d", r.status)
	}
}

// A device that paired with an earlier version of rmhub has a record and
// a token without a pairing ID. It continues to sync.
func TestTokenWithoutPairingID(t *testing.T) {
	e := newEnv(t, true)
	acct, _ := e.user("ada@example.org")
	ctx := context.Background()
	rec := `{"deviceId":"dev-1","desc":"remarkable","pairedAt":"2026-01-02T03:04:05Z"}`
	if _, err := e.st.PutIfAbsent(ctx, devices.Key(acct, "dev-1"), []byte(rec)); err != nil {
		t.Fatal(err)
	}
	device, err := e.app.Tablet.Issuer.DeviceToken(acct, "remarkable", "dev-1", "")
	if err != nil {
		t.Fatal(err)
	}
	r := e.userToken(device)
	if r.status != 200 {
		t.Fatalf("user token: %d", r.status)
	}
	if r := e.do("GET", e.tablet.URL+"/sync/test", string(r.body), nil); r.status != 200 {
		t.Fatalf("sync: %d", r.status)
	}
	withID, _ := e.app.Tablet.Issuer.DeviceToken(acct, "remarkable", "dev-1", "abcdefghijklmnop")
	if r := e.userToken(withID); r.status != 401 {
		t.Fatalf("token with a pairing ID that the record does not have: %d", r.status)
	}
}

func deviceKey(e *env, owner, id string) string {
	var me struct{ ID string }
	e.apiDo("GET", "/api/v1/account", owner, nil).json(e.t, &me)
	return devices.Key(me.ID, id)
}

func TestDeviceDelete(t *testing.T) {
	for _, v := range []string{"2", "3"} {
		t.Run(v, func(t *testing.T) {
			e := newEnv(t, true)
			_, owner := e.user("ada@example.org")
			device := e.paired(owner, "dev-1")
			if r := e.do("POST", e.tablet.URL+"/token/json/"+v+"/device/delete", "bad", nil); r.status != 401 {
				t.Fatalf("invalid token: %d", r.status)
			}
			if r := e.do("POST", e.tablet.URL+"/token/json/"+v+"/device/delete", device, nil); r.status != 204 {
				t.Fatalf("delete: %d %s", r.status, r.body)
			}
			r := e.apiDo("GET", "/api/v1/account/devices", owner, nil)
			if strings.TrimSpace(string(r.body)) != "[]" {
				t.Fatalf("devices after delete: %s", r.body)
			}
		})
	}
}

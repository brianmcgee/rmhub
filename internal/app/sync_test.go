package app

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/brianmcgee/rmhub/internal/hashtree"
)

type tresp struct {
	status int
	header http.Header
	body   []byte
}

// tab sends a request to the tablet listener.
func (e *env) tab(method, path, token string, body []byte, hdr map[string]string) tresp {
	e.t.Helper()
	req, err := http.NewRequest(method, e.tablet.URL+path, bytes.NewReader(body))
	if err != nil {
		e.t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	return tresp{status: res.StatusCode, header: res.Header, body: b}
}

// device pairs a device with the account of the owner token and returns
// a user token.
func (e *env) device(owner, id string) string {
	e.t.Helper()
	r := e.userToken(e.paired(owner, id))
	if r.status != 200 {
		e.t.Fatalf("user token: %d", r.status)
	}
	return string(r.body)
}

func crcOf(b []byte) string {
	var s [4]byte
	binary.BigEndian.PutUint32(s[:], crc32.Checksum(b, crc32.MakeTable(crc32.Castagnoli)))
	return "crc32c=" + base64.StdEncoding.EncodeToString(s[:])
}

func (e *env) putBlob(user, hash string, body []byte) {
	e.t.Helper()
	r := e.tab("PUT", "/sync/v3/files/"+hash, user, body, map[string]string{"x-goog-hash": crcOf(body)})
	if r.status != 200 {
		e.t.Fatalf("put blob %s: %d %s", hash, r.status, r.body)
	}
}

// doc uploads the files and the document index of a document, and returns
// its root entry.
func (e *env) doc(user, id string) hashtree.Entry {
	e.t.Helper()
	var files []hashtree.Entry
	var size int64
	for _, ext := range []string{".metadata", ".content", ".pdf"} {
		body := []byte(id + ext + " body")
		h := hashtree.FileHash(body)
		e.putBlob(user, h, body)
		files = append(files, hashtree.Entry{Hash: h, Name: id + ext, Size: int64(len(body))})
		size += int64(len(body))
	}
	idx := hashtree.WriteDocument(files)
	dh := hashtree.EntryListHash(files)
	e.putBlob(user, dh, idx)
	return hashtree.Entry{Hash: dh, Name: id, Subfiles: len(files), Size: size}
}

// root uploads a root index and returns its hash.
func (e *env) root(user string, schema int, docs ...hashtree.Entry) string {
	e.t.Helper()
	raw := hashtree.WriteRoot(schema, docs)
	ix, err := hashtree.Parse(raw)
	if err != nil {
		e.t.Fatal(err)
	}
	h := ix.RootHash(raw)
	e.putBlob(user, h, raw)
	return h
}

func (e *env) putRoot(user string, gen uint64, hash string, broadcast bool) tresp {
	e.t.Helper()
	b, _ := json.Marshal(map[string]any{"generation": gen, "hash": hash, "broadcast": broadcast})
	return e.tab("PUT", "/sync/v3/root", user, b, nil)
}

func TestDiscovery(t *testing.T) {
	e := newEnv(t, true)
	for path, want := range map[string]string{
		"/discovery/v1/endpoints":          `{"notifications":"local.appspot.com","webapp":"local.appspot.com"}`,
		"/discovery/v1/webapp":             `{"Host":"local.appspot.com","Status":"OK"}`,
		"/service/json/1/blob-storage":     `{"Host":"https://local.appspot.com","Status":"OK"}`,
		"/service/json/1/document-storage": `{"Host":"local.appspot.com","Status":"OK"}`,
	} {
		r := e.tab("GET", path, "", nil, nil)
		if r.status != 200 || string(r.body) != want {
			t.Errorf("%s: %d %s, want %s", path, r.status, r.body, want)
		}
	}
	e2 := newEnvWith(t, true, "[tablet]\nannounced_host = \"rmhub.example.org:8443\"\n")
	r := e2.tab("GET", "/service/json/1/blob-storage", "", nil, nil)
	if string(r.body) != `{"Host":"https://rmhub.example.org:8443","Status":"OK"}` {
		t.Fatalf("announced host: %s", r.body)
	}
}

func TestTelemetry(t *testing.T) {
	e := newEnv(t, true)
	_, owner := e.user("ada@example.org")
	user := e.device(owner, "RM110-1")

	r := e.tab("POST", "/analytics/v2/events", "", []byte(`{"events":[1,2,3]}`), nil)
	if r.status != 201 || string(r.body) != `{"message":"Success"}` {
		t.Fatalf("analytics: %d %s", r.status, r.body)
	}
	for _, p := range []string{"/v1/reports", "/v2/reports", "/report/v1", "/v2/events"} {
		if r := e.tab("POST", p, "", []byte("crash data"), nil); r.status != 200 || len(r.body) != 0 {
			t.Errorf("%s: %d %q", p, r.status, r.body)
		}
	}
	if r := e.tab("POST", "/sync/reports/v1", "", []byte("{}"), nil); r.status != 401 {
		t.Fatalf("sync report without token: %d", r.status)
	}
	if r := e.tab("POST", "/sync/reports/v1", user, []byte("{}"), nil); r.status != 200 || len(r.body) != 0 {
		t.Fatalf("sync report: %d %q", r.status, r.body)
	}
	r = e.tab("GET", "/settings/v1/beta", "", nil, nil)
	if r.status != 200 || string(r.body) != `{"available":true,"enrolled":false}` && string(r.body) != `{"enrolled":false,"available":true}` {
		t.Fatalf("beta: %d %s", r.status, r.body)
	}
	if r := e.tab("POST", "/settings/v1/beta", "", []byte(`{"enrolled":true}`), nil); r.status != 200 {
		t.Fatalf("beta post: %d", r.status)
	}
	// The state store holds nothing of the telemetry.
	keys, err := e.st.List(t.Context(), "", "", 1000)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range keys {
		if strings.Contains(k, "report") || strings.Contains(k, "analytics") {
			t.Fatalf("telemetry stored: %s", k)
		}
	}
}

// countingReader counts the bytes that the server reads.
type countingReader struct {
	n, size int
}

func (c *countingReader) Read(p []byte) (int, error) {
	if c.n >= c.size {
		return 0, io.EOF
	}
	k := min(len(p), c.size-c.n)
	for i := range k {
		p[i] = 'x'
	}
	c.n += k
	return k, nil
}

func TestLargeReport(t *testing.T) {
	e := newEnv(t, true)
	body := &countingReader{size: 10 << 20}
	req := httptest.NewRequest("POST", "/v2/reports", body)
	rec := httptest.NewRecorder()
	e.app.TabletHandler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	if body.n > 1<<20+64<<10 {
		t.Fatalf("server read %d bytes", body.n)
	}
}

func TestUnknownTabletPath(t *testing.T) {
	e := newEnv(t, true)
	r := e.tab("POST", "/screenshare/v1/rooms?secret=query-value", "token-value", []byte("body-value"), nil)
	if r.status != 404 {
		t.Fatalf("status %d", r.status)
	}
	logs := e.logs.String()
	if !strings.Contains(logs, "level=WARN") || !strings.Contains(logs, "method=POST") || !strings.Contains(logs, "path=/screenshare/v1/rooms") {
		t.Fatalf("log: %s", logs)
	}
	for _, secret := range []string{"query-value", "token-value", "body-value"} {
		if strings.Contains(logs, secret) {
			t.Fatalf("log has %q: %s", secret, logs)
		}
	}
}

func TestRootRead(t *testing.T) {
	e := newEnv(t, true)
	_, owner := e.user("ada@example.org")
	user := e.device(owner, "RM110-1")

	if r := e.tab("GET", "/sync/v4/root", "", nil, nil); r.status != 401 {
		t.Fatalf("no token: %d", r.status)
	}
	r := e.tab("GET", "/sync/v4/root", user, nil, nil)
	if r.status != 200 || string(r.body) != `{"generation":0,"schemaVersion":3}` || r.header.Get("x-goog-hash") != crcOf(r.body) {
		t.Fatalf("v4 new account: %d %s %v", r.status, r.body, r.header)
	}
	r = e.tab("GET", "/sync/v3/root", user, nil, nil)
	if r.status != 404 || string(r.body) != `{"message":"root not found"}` {
		t.Fatalf("v3 new account: %d %s", r.status, r.body)
	}
}

func TestSyncFlow(t *testing.T) {
	e := newEnv(t, true)
	_, owner := e.user("ada@example.org")
	user := e.device(owner, "RM110-1")

	d1 := e.doc(user, "d1")
	h1 := e.root(user, 4, d1)
	r := e.putRoot(user, 0, h1, false)
	if r.status != 200 || string(r.body) != fmt.Sprintf(`{"generation":1,"hash":"%s"}`, h1) {
		t.Fatalf("first root: %d %s", r.status, r.body)
	}
	r = e.tab("GET", "/sync/v4/root", user, nil, nil)
	if string(r.body) != fmt.Sprintf(`{"generation":1,"hash":"%s","schemaVersion":3}`, h1) || r.header.Get("x-goog-hash") != crcOf(r.body) {
		t.Fatalf("v4: %s", r.body)
	}
	r = e.tab("GET", "/sync/v3/root", user, nil, nil)
	if r.status != 200 || string(r.body) != fmt.Sprintf(`{"generation":1,"hash":"%s"}`, h1) {
		t.Fatalf("v3: %d %s", r.status, r.body)
	}
	// The root log has the device as the source.
	cur, err := e.app.Roots.Current(t.Context(), mustAccount(t, e, owner))
	if err != nil || cur.Source.ID != "RM110-1" || cur.Source.Type != "device" {
		t.Fatalf("source: %+v %v", cur, err)
	}

	// Stale write.
	d2 := e.doc(user, "d2")
	h2 := e.root(user, 4, d1, d2)
	r = e.putRoot(user, 0, h2, false)
	if r.status != 412 || len(r.body) != 0 {
		t.Fatalf("stale: %d %q", r.status, r.body)
	}
	// Body that is not JSON.
	if r := e.tab("PUT", "/sync/v3/root", user, []byte("generation=1"), nil); r.status != 400 {
		t.Fatalf("not JSON: %d", r.status)
	}
	// A root that names a missing document index.
	missing := hashtree.Entry{Hash: strings.Repeat("e", 64), Name: "gone", Subfiles: 1, Size: 1}
	hm := e.root(user, 4, d1, missing)
	if r := e.putRoot(user, 1, hm, false); r.status != 400 {
		t.Fatalf("missing document index: %d", r.status)
	}
	// A document index that names a missing file.
	files := []hashtree.Entry{{Hash: strings.Repeat("f", 64), Name: "d3.pdf", Size: 3}}
	dh := hashtree.EntryListHash(files)
	e.putBlob(user, dh, hashtree.WriteDocument(files))
	hf := e.root(user, 4, d1, hashtree.Entry{Hash: dh, Name: "d3", Subfiles: 1, Size: 3})
	if r := e.putRoot(user, 1, hf, false); r.status != 400 {
		t.Fatalf("missing file: %d", r.status)
	}
	// A root hash that names a file blob.
	pdf := []byte("%PDF-1.7 not an index")
	e.putBlob(user, hashtree.FileHash(pdf), pdf)
	if r := e.putRoot(user, 1, hashtree.FileHash(pdf), false); r.status != 400 {
		t.Fatalf("file as root: %d", r.status)
	}
	// A root hash without a blob.
	if r := e.putRoot(user, 1, strings.Repeat("a", 64), false); r.status != 400 {
		t.Fatalf("missing root blob: %d", r.status)
	}
	if cur, _ := e.app.Roots.Current(t.Context(), mustAccount(t, e, owner)); cur.Generation != 1 {
		t.Fatalf("generation changed to %d", cur.Generation)
	}

	// Schema setting: 4, then a schema 3 root still works.
	if r := e.apiDo("PATCH", "/api/v1/account", owner, map[string]int{"schemaVersion": 5}); r.status != 400 {
		t.Fatalf("schema 5: %d", r.status)
	}
	if r := e.apiDo("PATCH", "/api/v1/account", owner, map[string]int{"schemaVersion": 4}); r.status != 200 {
		t.Fatalf("schema 4: %d %s", r.status, r.body)
	}
	r = e.tab("GET", "/sync/v4/root", user, nil, nil)
	if !strings.Contains(string(r.body), `"schemaVersion":4`) {
		t.Fatalf("announced schema: %s", r.body)
	}
	h3 := e.root(user, 3, d1, d2)
	r = e.putRoot(user, 1, h3, true)
	if r.status != 200 || !strings.Contains(string(r.body), `"generation":2`) {
		t.Fatalf("schema 3 root: %d %s", r.status, r.body)
	}
	// The blob read gives the bytes and the CRC32C.
	r = e.tab("GET", "/sync/v3/files/"+h3, user, nil, nil)
	if r.status != 200 || r.header.Get("Content-Type") != "application/octet-stream" || r.header.Get("x-goog-hash") != crcOf(r.body) {
		t.Fatalf("blob read: %d %v", r.status, r.header)
	}
	if ix, err := hashtree.Parse(r.body); err != nil || ix.Schema != 3 {
		t.Fatalf("stored root: %v", err)
	}
}

func mustAccount(t *testing.T, e *env, owner string) string {
	t.Helper()
	var a struct{ ID string }
	e.apiDo("GET", "/api/v1/account", owner, nil).json(t, &a)
	return a.ID
}

func TestBlobWrite(t *testing.T) {
	e := newEnv(t, true)
	_, owner := e.user("ada@example.org")
	user := e.device(owner, "RM110-1")

	body := []byte("test")
	h := hashtree.FileHash(body)
	// Without x-goog-hash.
	if r := e.tab("PUT", "/sync/v3/files/"+h, user, body, nil); r.status != 200 || len(r.body) != 0 {
		t.Fatalf("file: %d %q", r.status, r.body)
	}
	if r := e.tab("GET", "/sync/v3/files/"+h, user, nil, nil); r.status != 200 || string(r.body) != "test" {
		t.Fatalf("read: %d %q", r.status, r.body)
	}
	// Document index and roots of both schemas.
	d := e.doc(user, "d1")
	e.root(user, 3, d)
	e.root(user, 4, d)

	// Mismatch.
	bad := strings.Repeat("b", 64)
	if r := e.tab("PUT", "/sync/v3/files/"+bad, user, []byte("other"), nil); r.status != 400 {
		t.Fatalf("mismatch: %d", r.status)
	}
	if r := e.tab("GET", "/sync/v3/files/"+bad, user, nil, nil); r.status != 404 {
		t.Fatalf("read after mismatch: %d", r.status)
	}
	// An index under a wrong hash.
	idx := hashtree.WriteDocument([]hashtree.Entry{{Hash: h, Name: "x.pdf", Size: 4}})
	if r := e.tab("PUT", "/sync/v3/files/"+bad, user, idx, nil); r.status != 400 {
		t.Fatalf("index mismatch: %d", r.status)
	}
	// CRC32C mismatch.
	body2 := []byte("test2")
	h2 := hashtree.FileHash(body2)
	if r := e.tab("PUT", "/sync/v3/files/"+h2, user, body2, map[string]string{"x-goog-hash": crcOf([]byte("other"))}); r.status != 400 {
		t.Fatalf("crc mismatch: %d", r.status)
	}
	if r := e.tab("GET", "/sync/v3/files/"+h2, user, nil, nil); r.status != 404 {
		t.Fatalf("stored after crc mismatch: %d", r.status)
	}
	// The header can hold more than one value.
	if r := e.tab("PUT", "/sync/v3/files/"+h2, user, body2, map[string]string{"x-goog-hash": "md5=abc, " + crcOf(body2)}); r.status != 200 {
		t.Fatalf("two hash values: %d", r.status)
	}
	// Invalid hashes.
	if r := e.tab("PUT", "/sync/v3/files/ABC", user, body, nil); r.status != 400 {
		t.Fatalf("put invalid hash: %d", r.status)
	}
	if r := e.tab("GET", "/sync/v3/files/"+strings.ToUpper(h), user, nil, nil); r.status != 404 {
		t.Fatalf("get invalid hash: %d", r.status)
	}
}

func TestLargeBlobRead(t *testing.T) {
	e := newEnv(t, true)
	_, owner := e.user("ada@example.org")
	user := e.device(owner, "RM110-1")
	body := bytes.Repeat([]byte("0123456789abcdef"), (9<<20)/16)
	h := hashtree.FileHash(body)
	e.putBlob(user, h, body)
	r := e.tab("GET", "/sync/v3/files/"+h, user, nil, nil)
	if r.status != 200 || !bytes.Equal(r.body, body) || r.header.Get("x-goog-hash") != crcOf(body) || r.header.Get("Content-Length") != fmt.Sprint(len(body)) {
		t.Fatalf("large read: %d %d %v", r.status, len(r.body), r.header)
	}
}

func checkFiles(e *env, user string, files ...string) string {
	b, _ := json.Marshal(map[string]any{"filename": "root.docSchema", "files": files, "reason": "sync"})
	r := e.tab("POST", "/sync/v3/check-files", user, b, nil)
	if r.status != 200 {
		e.t.Fatalf("check-files: %d", r.status)
	}
	return string(r.body)
}

func TestCheckFiles(t *testing.T) {
	for _, mode := range []string{"truthful", "none"} {
		t.Run(mode, func(t *testing.T) {
			e := newEnvWith(t, true, "[tablet]\ncheck_files = \""+mode+"\"\n")
			_, owner := e.user("ada@example.org")
			user := e.device(owner, "RM110-1")
			body := []byte("test")
			h1 := hashtree.FileHash(body)
			e.putBlob(user, h1, body)
			h2 := strings.Repeat("2", 64)
			got := checkFiles(e, user, h1, h2)
			want := `{"missingFiles":["` + h2 + `"]}`
			if mode == "none" {
				want = `{"missingFiles":null}`
			}
			if got != want {
				t.Fatalf("got %s, want %s", got, want)
			}
		})
	}
}

func TestMissingAndSyncComplete(t *testing.T) {
	e := newEnv(t, true)
	_, owner := e.user("ada@example.org")
	user := e.device(owner, "RM110-1")
	if r := e.tab("GET", "/sync/v3/missing", user, nil, nil); r.status != 200 || string(r.body) != `{"hashes":[]}` {
		t.Fatalf("missing: %d %s", r.status, r.body)
	}
	var a, b struct {
		ID         string `json:"id"`
		Generation int    `json:"generation"`
	}
	r := e.tab("POST", "/sync/v2/sync-complete", user, []byte(`{"generation":3}`), nil)
	if r.status != 200 || json.Unmarshal(r.body, &a) != nil || a.ID == "" || a.Generation != 0 {
		t.Fatalf("sync-complete: %d %s", r.status, r.body)
	}
	r = e.tab("POST", "/sync/v2/sync-complete", user, []byte(`{"generation":3}`), nil)
	if json.Unmarshal(r.body, &b) != nil || b.ID == a.ID {
		t.Fatalf("message IDs: %s %s", a.ID, b.ID)
	}
}

func TestAccountIsolation(t *testing.T) {
	e := newEnv(t, true)
	_, ownerA := e.user("ada@example.org")
	_, ownerB := e.user("bob@example.org")
	userA := e.device(ownerA, "RM110-A")
	userB := e.device(ownerB, "RM110-B")

	d := e.doc(userA, "d1")
	if r := e.tab("GET", "/sync/v3/files/"+d.Hash, userB, nil, nil); r.status != 404 {
		t.Fatalf("read of another account: %d", r.status)
	}
	if got := checkFiles(e, userB, d.Hash); got != `{"missingFiles":["`+d.Hash+`"]}` {
		t.Fatalf("check-files of another account: %s", got)
	}
	hb := e.root(userB, 4, d)
	if r := e.putRoot(userB, 0, hb, false); r.status != 400 {
		t.Fatalf("root of another account: %d", r.status)
	}
}

// ws opens the notification websocket of a user token.
func (e *env) ws(user string) *websocket.Conn {
	e.t.Helper()
	url := "ws" + strings.TrimPrefix(e.tablet.URL, "http") + "/notifications/ws/json/1"
	c, res, err := websocket.DefaultDialer.Dial(url, http.Header{"Authorization": {"Bearer " + user}})
	if err != nil {
		e.t.Fatalf("websocket: %v (%v)", err, res)
	}
	e.t.Cleanup(func() { _ = c.Close() })
	return c
}

type notification struct {
	Message struct {
		Attributes struct {
			Auth0UserID      string `json:"auth0UserID"`
			Event            string `json:"event"`
			SourceDeviceDesc string `json:"sourceDeviceDesc"`
			SourceDeviceID   string `json:"sourceDeviceID"`
		} `json:"attributes"`
		MessageID string `json:"messageid"`
	} `json:"message"`
}

// client reads a notification websocket in a goroutine.
type client struct {
	conn   *websocket.Conn
	msgs   chan []byte
	closed chan struct{}
}

// listen opens the websocket of a user token and reads it. With pongs
// false, the client does not answer pings.
func (e *env) listen(user string, pongs bool) *client {
	e.t.Helper()
	c := &client{conn: e.ws(user), msgs: make(chan []byte, 256), closed: make(chan struct{})}
	if !pongs {
		c.conn.SetPingHandler(func(string) error { return nil })
	}
	go func() {
		defer close(c.closed)
		for {
			_, b, err := c.conn.ReadMessage()
			if err != nil {
				return
			}
			c.msgs <- b
		}
	}()
	return c
}

// next returns the next message, or false after the timeout.
func (c *client) next(timeout time.Duration) ([]byte, bool) {
	select {
	case b := <-c.msgs:
		return b, true
	case <-time.After(timeout):
		return nil, false
	}
}

// closedWithin tells if the server closes the websocket within d.
func (c *client) closedWithin(d time.Duration) bool {
	select {
	case <-c.closed:
		return true
	case <-time.After(d):
		return false
	}
}

func TestNotifications(t *testing.T) {
	e := newEnv(t, true)
	acctA, ownerA := e.user("ada@example.org")
	_, ownerB := e.user("bob@example.org")
	u1 := e.device(ownerA, "RM110-1")
	u2 := e.device(ownerA, "RM110-2")
	u3 := e.device(ownerB, "RM110-3")

	hdr := map[string]string{"Connection": "Upgrade", "Upgrade": "websocket", "Sec-WebSocket-Version": "13", "Sec-WebSocket-Key": "dGhlIHNhbXBsZSBub25jZQ=="}
	if r := e.tab("GET", "/notifications/ws/json/1", "", nil, hdr); r.status != 401 {
		t.Fatalf("no token: %d", r.status)
	}
	c1, c2, c3 := e.listen(u1, true), e.listen(u2, true), e.listen(u3, true)
	time.Sleep(50 * time.Millisecond) // let the hub register the websockets

	d := e.doc(u1, "d1")
	h := e.root(u1, 4, d)
	if r := e.putRoot(u1, 0, h, true); r.status != 200 {
		t.Fatalf("root: %d", r.status)
	}
	b, ok := c2.next(2 * time.Second)
	if !ok {
		t.Fatal("device 2 got no message")
	}
	var n notification
	if err := json.Unmarshal(b, &n); err != nil {
		t.Fatal(err)
	}
	a := n.Message.Attributes
	if a.Auth0UserID != acctA || a.Event != "SyncComplete" || a.SourceDeviceDesc != "remarkable" || a.SourceDeviceID != "RM110-1" || len(n.Message.MessageID) == 0 {
		t.Fatalf("message: %s", b)
	}
	want := fmt.Sprintf(`{"message":{"attributes":{"auth0UserID":"%s","event":"SyncComplete","sourceDeviceDesc":"remarkable","sourceDeviceID":"RM110-1"},"messageid":"%s"}}`, acctA, n.Message.MessageID)
	if string(b) != want {
		t.Fatalf("message\n%s\nwant\n%s", b, want)
	}

	// Without broadcast, no message.
	if r := e.putRoot(u1, 1, h, false); r.status != 200 {
		t.Fatalf("root: %d", r.status)
	}
	// sync-complete gives a message with a new ID.
	e.tab("POST", "/sync/v2/sync-complete", u1, []byte(`{"generation":2}`), nil)
	b2, ok := c2.next(2 * time.Second)
	var n2 notification
	if !ok || json.Unmarshal(b2, &n2) != nil || n2.Message.MessageID == n.Message.MessageID {
		t.Fatalf("sync-complete message: %s", b2)
	}
	if b, ok := c2.next(200 * time.Millisecond); ok {
		t.Fatalf("extra message (root write without broadcast?): %s", b)
	}
	if b, ok := c1.next(100 * time.Millisecond); ok {
		t.Fatalf("source device got %s", b)
	}
	if b, ok := c3.next(100 * time.Millisecond); ok {
		t.Fatalf("other account got %s", b)
	}
}

func TestStalledWebsocket(t *testing.T) {
	e := newEnv(t, true)
	_, owner := e.user("ada@example.org")
	u1 := e.device(owner, "RM110-1")
	u2 := e.device(owner, "RM110-2")
	_ = e.ws(u2) // never reads
	time.Sleep(50 * time.Millisecond)

	d := e.doc(u1, "d1")
	h := e.root(u1, 4, d)
	start := time.Now()
	for g := range uint64(100) {
		if r := e.putRoot(u1, g, h, true); r.status != 200 {
			t.Fatalf("root write %d: %d", g, r.status)
		}
	}
	if d := time.Since(start); d > 20*time.Second {
		t.Fatalf("root writes took %v", d)
	}
}

func TestKeepaliveAndRemoval(t *testing.T) {
	e := newEnv(t, true)
	e.app.Hub.PingInterval = 50 * time.Millisecond
	e.app.Hub.PongWait = 200 * time.Millisecond
	_, owner := e.user("ada@example.org")

	silent := e.listen(e.device(owner, "RM110-1"), false)
	live := e.listen(e.device(owner, "RM110-2"), true)
	if !silent.closedWithin(2 * time.Second) {
		t.Fatal("server did not close a websocket without pongs")
	}
	if live.closedWithin(300 * time.Millisecond) {
		t.Fatal("server closed a live websocket")
	}
	if r := e.apiDo("DELETE", "/api/v1/account/devices/RM110-2", owner, nil); r.status != 204 {
		t.Fatalf("remove: %d", r.status)
	}
	if !live.closedWithin(2 * time.Second) {
		t.Fatal("server did not close the websocket of a removed device")
	}
}

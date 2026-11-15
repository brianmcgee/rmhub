package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/brianmcgee/rmhub/internal/library"
	"github.com/brianmcgee/rmhub/internal/library/libtest"
)

// account makes a user and an owner token, and returns the account ID
// and the environment of the owner.
func (ts *testSrv) account(t *testing.T) (string, map[string]string) {
	t.Helper()
	env := map[string]string{"RMHUB_URL": ts.url, "RMHUB_TOKEN": ts.admin}
	r := rmhub(env, "admin", "user", "add", "--email", "ada@example.org", "--name", "Ada", "--json")
	var u userView
	if r.code != 0 || json.Unmarshal([]byte(r.stdout), &u) != nil {
		t.Fatalf("user add: %+v", r)
	}
	r = rmhub(env, "admin", "token", "issue", u.ID)
	if r.code != 0 {
		t.Fatalf("token issue: %+v", r)
	}
	return u.ID, map[string]string{"RMHUB_URL": ts.url, "RMHUB_TOKEN": strings.TrimSpace(r.stdout)}
}

func (ts *testSrv) tree(t *testing.T, account string) *libtest.Env {
	return libtest.On(t, ts.blobs, ts.app.Roots, account)
}

func jsonOut(t *testing.T, r result, v any) {
	t.Helper()
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	if err := json.Unmarshal([]byte(r.stdout), v); err != nil {
		t.Fatalf("output is not one JSON value: %q", r.stdout)
	}
}

func TestLibraryCommands(t *testing.T) {
	isolate(t)
	ts := newTestServer(t)
	acct, env := ts.account(t)
	tr := ts.tree(t, acct)
	tr.Folder("work", "Work", "")
	tr.PDF("paper", "Paper", "work", []byte("%PDF-1.7 paper"))
	tr.PDF("n1", "Notes", "", []byte("%PDF-1.7 a"))
	tr.PDF("n2", "Notes", "", []byte("%PDF-1.7 b"))
	tr.Notebook("nb", "Sketch", "work", 2)
	tr.Commit(4)

	// ls
	r := rmhub(env, "ls")
	if r.code != 0 || !strings.Contains(r.stdout, "Work") || !strings.Contains(r.stdout, "n1") {
		t.Fatalf("ls: %+v", r)
	}
	var es []entryView
	jsonOut(t, rmhub(env, "ls", "/Work", "--json"), &es)
	if len(es) != 2 || es[0].Name != "Paper" || es[1].Type != "notebook" {
		t.Fatalf("ls /Work: %+v", es)
	}
	// stat with a path and an ID.
	var e entryView
	jsonOut(t, rmhub(env, "stat", "/Work/Paper", "--json"), &e)
	if e.ID != "paper" || e.Path != "/Work/Paper" || e.Hash == "" {
		t.Fatalf("stat: %+v", e)
	}
	if r := rmhub(env, "stat", "paper"); r.code != 0 || !strings.Contains(r.stdout, "/Work/Paper") {
		t.Fatalf("stat by ID: %+v", r)
	}
	r = rmhub(env, "stat", "/Notes")
	if r.code != exitConflict || !strings.Contains(r.stderr, "n1") || !strings.Contains(r.stderr, "n2") {
		t.Fatalf("ambiguous path: %+v", r)
	}
	if r := rmhub(env, "stat", "/Nothing"); r.code != exitNotFound {
		t.Fatalf("missing path: %+v", r)
	}
	// find
	jsonOut(t, rmhub(env, "find", "NOTE", "--json"), &es)
	if len(es) != 2 {
		t.Fatalf("find: %+v", es)
	}
	if r := rmhub(env, "find", "sketch"); r.code != 0 || !strings.Contains(r.stdout, "nb") {
		t.Fatalf("find text: %+v", r)
	}

	// get: the default file name, no replace, -o -, --archive.
	dir := t.TempDir()
	t.Chdir(dir)
	if r := rmhub(env, "get", "/Work/Paper"); r.code != 0 || strings.TrimSpace(r.stdout) != "Paper.pdf" {
		t.Fatalf("get: %+v", r)
	}
	if b, _ := os.ReadFile("Paper.pdf"); string(b) != "%PDF-1.7 paper" {
		t.Fatalf("downloaded %q", b)
	}
	if err := os.WriteFile("Paper.pdf", []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	if r := rmhub(env, "get", "/Work/Paper"); r.code != exitUnexpected {
		t.Fatalf("get over a file: %+v", r)
	}
	if b, _ := os.ReadFile("Paper.pdf"); string(b) != "mine" {
		t.Fatal("get replaced the file")
	}
	if r := rmhub(env, "get", "paper", "-o", "-"); r.stdout != "%PDF-1.7 paper" {
		t.Fatalf("get -o -: %+v", r)
	}
	var got struct {
		File string
		Size int64
	}
	jsonOut(t, rmhub(env, "get", "/Work/Sketch", "--archive", "--json"), &got)
	if got.File != "Sketch.zip" || got.Size == 0 {
		t.Fatalf("get --archive: %+v", got)
	}
	if r := rmhub(env, "get", "/Work/Sketch", "-o", "x.pdf"); r.code != exitNotFound {
		t.Fatalf("get notebook content: %+v", r)
	}

	// upload, mkdir, mv, rename, trash.
	if err := os.WriteFile("paper.pdf", []byte("%PDF-1.7 new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("notes.txt", []byte("text"), 0o600); err != nil {
		t.Fatal(err)
	}
	if r := rmhub(env, "upload", "notes.txt"); r.code != exitUsage {
		t.Fatalf("upload .txt: %+v", r)
	}
	jsonOut(t, rmhub(env, "mkdir", "Inbox", "--json"), &e)
	if e.Type != "folder" || e.Path != "/Inbox" {
		t.Fatalf("mkdir: %+v", e)
	}
	jsonOut(t, rmhub(env, "mkdir", "Sub", "--in", "/Inbox", "--json"), &e)
	if e.Path != "/Inbox/Sub" {
		t.Fatalf("mkdir --in: %+v", e)
	}
	if r := rmhub(env, "upload", "paper.pdf", "--to", "/Inbox"); r.code != 0 || !strings.Contains(r.stdout, "/Inbox/paper") {
		t.Fatalf("upload: %+v", r)
	}
	jsonOut(t, rmhub(env, "upload", "paper.pdf", "--name", "Named", "--json"), &e)
	if e.Name != "Named" || e.Type != "pdf" || e.Parent != "" {
		t.Fatalf("upload --name: %+v", e)
	}
	jsonOut(t, rmhub(env, "stat", "/Inbox/paper", "--json"), &e)
	hash := e.Hash
	jsonOut(t, rmhub(env, "mv", "/Inbox/paper", "/Work", "--if-match", hash, "--json"), &e)
	if e.Path != "/Work/paper" {
		t.Fatalf("mv: %+v", e)
	}
	if r := rmhub(env, "rename", e.ID, "Other", "--if-match", hash); r.code != exitConflict {
		t.Fatalf("rename with a stale hash: %+v", r)
	}
	if r := rmhub(env, "rename", e.ID, "Other"); r.code != 0 || !strings.Contains(r.stdout, "/Work/Other") {
		t.Fatalf("rename: %+v", r)
	}
	if r := rmhub(env, "mv", "/Work/Other", "/"); r.code != 0 || !strings.Contains(r.stdout, "/Other") {
		t.Fatalf("mv to the top level: %+v", r)
	}
	jsonOut(t, rmhub(env, "trash", "/Other", "--json"), &e)
	if !e.Trashed {
		t.Fatalf("trash: %+v", e)
	}
	if r := rmhub(env, "trash", "/Inbox", "--if-match", "nope"); r.code != exitConflict {
		t.Fatalf("trash with a stale hash: %+v", r)
	}
}

func TestScopedTokenCommand(t *testing.T) {
	isolate(t)
	ts := newTestServer(t)
	acct, env := ts.account(t)
	tr := ts.tree(t, acct)
	tr.Folder("agent", "Agent", "")
	tr.Folder("ref", "Ref", "")
	tr.PDF("private", "Private", "", []byte("%PDF-1.7 p"))
	tr.Commit(4)

	if r := rmhub(env, "tokens", "create", "--grant", "/Agent:admin"); r.code != exitUsage {
		t.Fatalf("bad access: %+v", r)
	}
	var tok issuedView
	jsonOut(t, rmhub(env, "tokens", "create", "--label", "agent", "--grant", "/Agent:write", "--grant", "ref:read", "--expires", "24h", "--json"), &tok)
	if len(tok.Grants) != 2 || tok.Grants[0] != (grantView{Folder: "agent", Access: "write"}) || tok.ExpiresAt == nil || tok.Parent == "" {
		t.Fatalf("scoped token: %+v", tok)
	}
	agent := map[string]string{"RMHUB_URL": ts.url, "RMHUB_TOKEN": tok.Token}
	var es []entryView
	jsonOut(t, rmhub(agent, "ls", "--json"), &es)
	if len(es) != 2 || es[0].Name != "Agent" || es[1].Name != "Ref" {
		t.Fatalf("scoped ls: %+v", es)
	}
	if r := rmhub(agent, "stat", "private"); r.code != exitNotFound {
		t.Fatalf("scoped stat outside: %+v", r)
	}
	if r := rmhub(agent, "pair"); r.code != exitAuth {
		t.Fatalf("scoped pair: %+v", r)
	}
	if r := rmhub(agent, "mkdir", "Notes", "--in", "/Agent"); r.code != 0 {
		t.Fatalf("scoped mkdir: %+v", r)
	}
	if r := rmhub(agent, "mkdir", "Notes", "--in", "/Ref"); r.code != exitAuth {
		t.Fatalf("scoped mkdir in read-only: %+v", r)
	}
	if r := rmhub(env, "tokens", "list"); r.code != 0 || !strings.Contains(r.stdout, "agent:write,ref:read") {
		t.Fatalf("tokens list: %+v", r)
	}
}

// lines is a writer that the watch goroutine and the test can share.
type lines struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lines) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lines) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func (l *lines) generations(t *testing.T) []uint64 {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []uint64
	sc := bufio.NewScanner(bytes.NewReader(l.b.Bytes()))
	for sc.Scan() {
		var ev struct{ Generation uint64 }
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil || ev.Generation == 0 {
			t.Fatalf("watch line %q is not an event", sc.Text())
		}
		out = append(out, ev.Generation)
	}
	return out
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout: %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestWatchReconnect(t *testing.T) {
	isolate(t)
	ts := newTestServer(t)
	acct, env := ts.account(t)
	tr := ts.tree(t, acct)
	tr.PDF("d", "D", "", []byte("%PDF-1.7 d"))
	start := tr.Commit(4).Generation

	out, errb := &lines{}, &lines{}
	done := make(chan int, 1)
	go func() {
		done <- run([]string{"watch", "--after", "0"}, out, errb, func(k string) string { return env[k] })
	}()
	waitFor(t, "first event", func() bool { return len(out.generations(t)) == 1 })

	tr.PDF("d", "D2", "", []byte("%PDF-1.7 d"))
	tr.Commit(4)
	waitFor(t, "second event", func() bool { return len(out.generations(t)) == 2 })

	// Stop the server. Two commits happen while it is down.
	ts.stop()
	tr.PDF("d", "D3", "", []byte("%PDF-1.7 d"))
	tr.Commit(4)
	tr.PDF("d", "D4", library.Trash, []byte("%PDF-1.7 d"))
	last := tr.Commit(4).Generation
	ts.start()
	waitFor(t, "events after the restart", func() bool { return len(out.generations(t)) == 4 })
	gens := out.generations(t)
	for i, g := range gens {
		if g != start+uint64(i) {
			t.Fatalf("generations %v, want %d to %d without a gap", gens, start, last)
		}
	}
	if !strings.Contains(errb.String(), "connecting again") {
		t.Fatalf("no reconnect message: %q", errb.String())
	}

	// Revocation of the token ends the watch with exit code 3.
	var list []tokenView
	jsonOut(t, rmhub(env, "tokens", "list", "--json"), &list)
	rmhub(env, "tokens", "revoke", list[0].ID)
	ts.stop()
	ts.start()
	select {
	case code := <-done:
		if code != exitAuth {
			t.Fatalf("watch exit %d", code)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("watch did not end after revocation")
	}
}

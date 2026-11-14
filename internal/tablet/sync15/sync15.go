// Package sync15 serves the root and blob endpoints of the reMarkable
// Sync 1.5 protocol on the tablet listener.
//
// Each blob write is checked against its key before the server stores
// it. Each root write is checked against the blobs before the commit. So
// no client can store a broken tree.
package sync15

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/brianmcgee/rmhub/internal/accounts"
	"github.com/brianmcgee/rmhub/internal/blob"
	"github.com/brianmcgee/rmhub/internal/hashtree"
	"github.com/brianmcgee/rmhub/internal/rootlog"
	"github.com/brianmcgee/rmhub/internal/tablet"
)

const (
	// maxIndex is the size limit below which a blob write that is not a
	// file is parsed as an index.
	maxIndex = 4 << 20
	// maxRoot is the maximum size of a root index that a root write can
	// name.
	maxRoot = 64 << 20
	// maxJSON is the maximum size of a JSON request body.
	maxJSON = 1 << 20
	// memBlob is the size up to which a blob read stays in memory. A
	// larger blob goes through a temporary file, because the CRC32C
	// header comes before the body.
	memBlob = 8 << 20
	// checkWorkers is the number of document indexes that a root check
	// reads at the same time.
	checkWorkers = 16
)

var castagnoli = crc32.MakeTable(crc32.Castagnoli)

// Notifier sends a SyncComplete message for a sync-complete request.
type Notifier interface {
	// Notify sends the message to the devices of the account except the
	// source device, and returns the message ID.
	Notify(account string, src rootlog.Source) string
}

// Service holds the dependencies of the Sync 1.5 endpoints.
type Service struct {
	Blobs    blob.Store
	Roots    *rootlog.Log
	Accounts *accounts.Store
	Notifier Notifier
	Log      *slog.Logger
	// TempDir is the directory for temporary files.
	TempDir string
	// CheckFiles is false when tablet.check_files is "none".
	CheckFiles bool

	cacheOnce sync.Once
	cache     *rootCache
}

// Register adds the endpoints to the mux. requireUser checks the user
// token of each request.
func (s *Service) Register(mux *http.ServeMux, requireUser func(http.Handler) http.Handler) {
	h := func(pattern string, fn func(http.ResponseWriter, *http.Request, tablet.User, blob.Namespace) error) {
		mux.Handle(pattern, requireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := tablet.UserFrom(r.Context())
			ns, err := blob.NewNamespace(u.Account)
			if err == nil {
				err = fn(w, r, u, ns)
			}
			var he httpError
			switch {
			case err == nil:
			case errors.As(err, &he):
				s.Log.Info("tablet sync request refused", "method", r.Method, "path", r.URL.Path,
					"account", u.Account, "device", u.DeviceID, "status", he.status, "reason", he.msg)
				http.Error(w, he.msg, he.status)
			default:
				s.Log.Error("tablet sync request failed", "method", r.Method, "path", r.URL.Path,
					"account", u.Account, "err", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		})))
	}
	h("GET /sync/v4/root", s.rootV4)
	h("GET /sync/v3/root", s.rootV3)
	h("PUT /sync/v3/root", s.putRoot)
	h("GET /sync/v3/files/{hash}", s.getBlob)
	h("PUT /sync/v3/files/{hash}", s.putBlob)
	h("POST /sync/v3/check-files", s.checkFiles)
	h("GET /sync/v3/missing", func(w http.ResponseWriter, _ *http.Request, _ tablet.User, _ blob.Namespace) error {
		writeJSON(w, http.StatusOK, map[string][]string{"hashes": {}})
		return nil
	})
	h("POST /sync/v2/sync-complete", s.syncComplete)
}

// httpError is a refusal with a status and a plain text message.
type httpError struct {
	status int
	msg    string
}

func (e httpError) Error() string { return e.msg }

func badRequest(format string, args ...any) error {
	return httpError{status: http.StatusBadRequest, msg: fmt.Sprintf(format, args...)}
}

func crcHeader(sum uint32) string {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], sum)
	return "crc32c=" + base64.StdEncoding.EncodeToString(b[:])
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	writeBody(w, status, "application/json", b)
}

// writeBody writes the body with the CRC32C header.
func writeBody(w http.ResponseWriter, status int, ctype string, b []byte) {
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("x-goog-hash", crcHeader(crc32.Checksum(b, castagnoli)))
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

func decodeJSON(r *http.Request, v any) error {
	if err := json.NewDecoder(io.LimitReader(r.Body, maxJSON)).Decode(v); err != nil {
		return badRequest("request body is not valid JSON")
	}
	return nil
}

type rootV4Response struct {
	Generation    uint64 `json:"generation"`
	Hash          string `json:"hash,omitempty"`
	SchemaVersion int    `json:"schemaVersion"`
}

func (s *Service) rootV4(w http.ResponseWriter, r *http.Request, u tablet.User, _ blob.Namespace) error {
	e, err := s.Roots.Current(r.Context(), u.Account)
	if err != nil {
		return err
	}
	a, err := s.Accounts.Get(r.Context(), u.Account)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, rootV4Response{Generation: e.Generation, Hash: e.Hash, SchemaVersion: a.SchemaVersion})
	return nil
}

type rootV3Response struct {
	Generation uint64 `json:"generation"`
	Hash       string `json:"hash"`
}

func (s *Service) rootV3(w http.ResponseWriter, r *http.Request, u tablet.User, _ blob.Namespace) error {
	e, err := s.Roots.Current(r.Context(), u.Account)
	if err != nil {
		return err
	}
	if e.Generation == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "root not found"})
		return nil
	}
	writeJSON(w, http.StatusOK, rootV3Response{Generation: e.Generation, Hash: e.Hash})
	return nil
}

type putRootRequest struct {
	Generation *uint64 `json:"generation"`
	Hash       string  `json:"hash"`
	Broadcast  bool    `json:"broadcast"`
}

func (s *Service) putRoot(w http.ResponseWriter, r *http.Request, u tablet.User, ns blob.Namespace) error {
	var req putRootRequest
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	if req.Generation == nil || !hashtree.ValidHash(req.Hash) {
		return badRequest("root write needs a generation and a valid hash")
	}
	ctx := r.Context()
	cur, err := s.Roots.Current(ctx, u.Account)
	if err != nil {
		return err
	}
	if cur.Generation != *req.Generation {
		w.WriteHeader(http.StatusPreconditionFailed)
		return nil
	}
	if err := s.checkTree(ctx, ns, cur.Hash, req.Hash); err != nil {
		return err
	}
	src := rootlog.Source{Type: rootlog.SourceDevice, ID: u.DeviceID, Desc: u.Desc}
	e, err := s.Roots.Commit(ctx, u.Account, *req.Generation, req.Hash, src, req.Broadcast)
	if errors.Is(err, rootlog.ErrStale) {
		w.WriteHeader(http.StatusPreconditionFailed)
		return nil
	}
	if err != nil {
		return err
	}
	s.Log.Info("root committed", "account", u.Account, "device", u.DeviceID, "generation", e.Generation, "hash", e.Hash)
	writeJSON(w, http.StatusOK, rootV3Response{Generation: e.Generation, Hash: e.Hash})
	return nil
}

func (s *Service) roots() *rootCache {
	s.cacheOnce.Do(func() { s.cache = newRootCache(64) })
	return s.cache
}

// readAll reads a blob of at most limit bytes. A missing blob gives
// blob.ErrNotFound.
func (s *Service) readAll(ctx context.Context, ns blob.Namespace, hash string, limit int64) ([]byte, error) {
	rc, size, err := s.Blobs.Get(ctx, ns, hash)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	if size > limit {
		return nil, badRequest("blob %s is larger than %d bytes", hash, limit)
	}
	return io.ReadAll(io.LimitReader(rc, limit))
}

// loadRoot reads and parses the root index of the hash.
func (s *Service) loadRoot(ctx context.Context, ns blob.Namespace, hash string) (*hashtree.Index, error) {
	key := ns.String() + "/" + hash
	if ix, ok := s.roots().get(key); ok {
		return ix, nil
	}
	raw, err := s.readAll(ctx, ns, hash, maxRoot)
	if errors.Is(err, blob.ErrNotFound) {
		return nil, badRequest("root index %s does not exist", hash)
	}
	if err != nil {
		return nil, err
	}
	ix, err := hashtree.Parse(raw)
	if err != nil {
		return nil, badRequest("root %s is not a root index: %v", hash, err)
	}
	if !ix.MatchesRoot(raw, hash) {
		return nil, badRequest("root %s is not a root index with this hash", hash)
	}
	s.roots().put(key, ix)
	return ix, nil
}

// checkTree checks the new root against the current root. It reads each
// document index that the current root does not have, and checks that
// its files exist.
func (s *Service) checkTree(ctx context.Context, ns blob.Namespace, curHash, newHash string) error {
	next, err := s.loadRoot(ctx, ns, newHash)
	if err != nil {
		return err
	}
	known := map[string]bool{}
	if curHash != "" {
		if cur, err := s.loadRoot(ctx, ns, curHash); err == nil {
			for _, e := range cur.Entries {
				known[e.Hash] = true
			}
		} else {
			s.Log.Warn("current root cannot be read, the check reads each document", "namespace", ns.String(), "err", err)
		}
	}
	var todo []hashtree.Entry
	for _, e := range next.Entries {
		if !known[e.Hash] {
			todo = append(todo, e)
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan hashtree.Entry)
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
	)
	fail := func(err error) {
		mu.Lock()
		if firstErr == nil {
			firstErr = err
			cancel()
		}
		mu.Unlock()
	}
	for range min(checkWorkers, len(todo)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for e := range jobs {
				if err := s.checkDocument(ctx, ns, e); err != nil {
					fail(err)
				}
			}
		}()
	}
feed:
	for _, e := range todo {
		select {
		case jobs <- e:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}
	return ctx.Err()
}

func (s *Service) checkDocument(ctx context.Context, ns blob.Namespace, e hashtree.Entry) error {
	raw, err := s.readAll(ctx, ns, e.Hash, maxIndex)
	if errors.Is(err, blob.ErrNotFound) {
		return badRequest("document index %s of %s does not exist", e.Hash, e.Name)
	}
	if err != nil {
		return err
	}
	doc, err := hashtree.Parse(raw)
	if err != nil {
		return badRequest("document index %s of %s does not parse: %v", e.Hash, e.Name, err)
	}
	keys := make([]string, len(doc.Entries))
	for i, f := range doc.Entries {
		keys[i] = f.Hash
	}
	if len(keys) == 0 {
		return nil
	}
	have, err := s.Blobs.Exists(ctx, ns, keys)
	if err != nil {
		return err
	}
	for _, f := range doc.Entries {
		if !have[f.Hash] {
			return badRequest("file %s (%s) of document %s does not exist", f.Hash, f.Name, e.Name)
		}
	}
	return nil
}

func (s *Service) getBlob(w http.ResponseWriter, r *http.Request, _ tablet.User, ns blob.Namespace) error {
	hash := r.PathValue("hash")
	if !hashtree.ValidHash(hash) {
		return httpError{status: http.StatusNotFound, msg: "blob not found"}
	}
	rc, size, err := s.Blobs.Get(r.Context(), ns, hash)
	if errors.Is(err, blob.ErrNotFound) {
		return httpError{status: http.StatusNotFound, msg: "blob not found"}
	}
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	if size <= memBlob {
		b, err := io.ReadAll(rc)
		if err != nil {
			return err
		}
		writeBody(w, http.StatusOK, "application/octet-stream", b)
		return nil
	}
	f, err := os.CreateTemp(s.TempDir, "get-*")
	if err != nil {
		return err
	}
	defer func() { _ = f.Close(); _ = os.Remove(f.Name()) }()
	crc := crc32.New(castagnoli)
	n, err := io.Copy(io.MultiWriter(f, crc), rc)
	if err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprint(n))
	w.Header().Set("x-goog-hash", crcHeader(crc.Sum32()))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f)
	return nil
}

// headerCRC returns the crc32c value of an x-goog-hash header, if any.
func headerCRC(h string) (uint32, bool, error) {
	for _, part := range strings.Split(h, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || k != "crc32c" {
			continue
		}
		b, err := base64.StdEncoding.DecodeString(v)
		if err != nil || len(b) != 4 {
			return 0, false, badRequest("x-goog-hash has a crc32c value that is not valid")
		}
		return binary.BigEndian.Uint32(b), true, nil
	}
	return 0, false, nil
}

// fileStore is a blob store that can take a temporary file without a
// copy.
type fileStore interface {
	PutFile(ns blob.Namespace, key, path string) error
}

func (s *Service) putBlob(w http.ResponseWriter, r *http.Request, _ tablet.User, ns blob.Namespace) error {
	hash := r.PathValue("hash")
	if !hashtree.ValidHash(hash) {
		return badRequest("hash %q is not 64 lowercase hexadecimal characters", hash)
	}
	want, hasCRC, err := headerCRC(r.Header.Get("x-goog-hash"))
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(s.TempDir, "put-*")
	if err != nil {
		return err
	}
	path := f.Name()
	defer func() { _ = os.Remove(path) }()
	sha := sha256.New()
	crc := crc32.New(castagnoli)
	n, err := io.Copy(io.MultiWriter(f, sha, crc), r.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return badRequest("request body cannot be read: %v", err)
	}
	if hasCRC && crc.Sum32() != want {
		return badRequest("crc32c of the body does not match x-goog-hash")
	}
	if hex.EncodeToString(sha.Sum(nil)) != hash {
		if n >= maxIndex {
			return badRequest("body does not match the hash %s", hash)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		ix, err := hashtree.Parse(raw)
		if err != nil || hashtree.EntryListHash(ix.Entries) != hash {
			return badRequest("body does not match the hash %s", hash)
		}
	}
	if fs, ok := s.Blobs.(fileStore); ok {
		err = fs.PutFile(ns, hash, path)
	} else {
		var rf *os.File
		rf, err = os.Open(path)
		if err == nil {
			err = s.Blobs.Put(r.Context(), ns, hash, rf)
			_ = rf.Close()
		}
	}
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusOK)
	return nil
}

type checkFilesRequest struct {
	Filename string   `json:"filename"`
	Files    []string `json:"files"`
	Reason   string   `json:"reason"`
}

func (s *Service) checkFiles(w http.ResponseWriter, r *http.Request, _ tablet.User, ns blob.Namespace) error {
	var req checkFilesRequest
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	if !s.CheckFiles {
		writeJSON(w, http.StatusOK, map[string]any{"missingFiles": nil})
		return nil
	}
	var valid []string
	for _, h := range req.Files {
		if hashtree.ValidHash(h) {
			valid = append(valid, h)
		}
	}
	have := map[string]bool{}
	if len(valid) > 0 {
		var err error
		if have, err = s.Blobs.Exists(r.Context(), ns, valid); err != nil {
			return err
		}
	}
	missing := []string{}
	for _, h := range req.Files {
		if !have[h] {
			missing = append(missing, h)
		}
	}
	writeJSON(w, http.StatusOK, map[string][]string{"missingFiles": missing})
	return nil
}

type syncCompleteRequest struct {
	Generation uint64 `json:"generation"`
}

func (s *Service) syncComplete(w http.ResponseWriter, r *http.Request, u tablet.User, _ blob.Namespace) error {
	var req syncCompleteRequest
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	id := s.Notifier.Notify(u.Account, rootlog.Source{Type: rootlog.SourceDevice, ID: u.DeviceID, Desc: u.Desc})
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "generation": 0})
	return nil
}

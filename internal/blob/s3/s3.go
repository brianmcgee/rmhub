// Package s3 is the S3 backend of the blob store.
//
// The object name is <prefix><namespace>/<key>. A write uses
// If-None-Match: *, so the first write wins.
package s3

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/brianmcgee/rmhub/internal/blob"
	"github.com/brianmcgee/rmhub/internal/s3conn"
)

// existsParallel is the maximum number of HeadObject requests at one time.
const existsParallel = 16

// Store keeps blobs in an S3 bucket.
type Store struct {
	c       *s3conn.Conn
	tempDir string
}

var _ blob.Store = (*Store)(nil)

// New returns a store on the connection. A write of a body that is not
// an io.ReadSeeker goes to a temporary file in tempDir first. An empty
// tempDir means os.TempDir().
func New(c *s3conn.Conn, tempDir string) *Store {
	return &Store{c: c, tempDir: tempDir}
}

func (s *Store) name(ns blob.Namespace, key string) *string {
	return aws.String(s.c.Prefix + ns.String() + "/" + key)
}

// Put writes the blob with If-None-Match: *.
func (s *Store) Put(ctx context.Context, ns blob.Namespace, key string, r io.Reader) error {
	if err := blob.Check(ns, key); err != nil {
		return err
	}
	body, size, cleanup, err := s.seekable(r)
	if err != nil {
		return err
	}
	defer cleanup()

	err = s.put(ctx, ns, key, body, size)
	if s3conn.Status(err) == http.StatusConflict {
		// A parallel write of the same key is in progress. Try once more.
		if _, serr := body.Seek(0, io.SeekStart); serr != nil {
			return serr
		}
		err = s.put(ctx, ns, key, body, size)
		if err != nil {
			ex, xerr := s.Exists(ctx, ns, []string{key})
			if xerr == nil && ex[key] {
				return nil
			}
		}
	}
	return err
}

func (s *Store) put(ctx context.Context, ns blob.Namespace, key string, body io.ReadSeeker, size int64) error {
	_, err := s.c.Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.c.Bucket),
		Key:           s.name(ns, key),
		Body:          body,
		ContentLength: aws.Int64(size),
		IfNoneMatch:   aws.String("*"),
	})
	if s3conn.Status(err) == http.StatusPreconditionFailed {
		// The blob exists. Blobs never change, so the first blob stays.
		return nil
	}
	return err
}

// seekable returns the body as a seekable reader with its size. It
// copies a reader that cannot seek into a temporary file, so that a
// failing source never reaches the bucket.
func (s *Store) seekable(r io.Reader) (io.ReadSeeker, int64, func(), error) {
	if rs, ok := r.(io.ReadSeeker); ok {
		size, err := rs.Seek(0, io.SeekEnd)
		if err == nil {
			_, err = rs.Seek(0, io.SeekStart)
		}
		if err == nil {
			return rs, size, func() {}, nil
		}
	}
	f, err := os.CreateTemp(s.tempDir, "rmhub-blob-*")
	if err != nil {
		return nil, 0, nil, err
	}
	cleanup := func() {
		_ = f.Close()
		_ = os.Remove(f.Name())
	}
	size, err := io.Copy(f, r)
	if err == nil {
		_, err = f.Seek(0, io.SeekStart)
	}
	if err != nil {
		cleanup()
		return nil, 0, nil, err
	}
	return f, size, cleanup, nil
}

// Get reads the blob.
func (s *Store) Get(ctx context.Context, ns blob.Namespace, key string) (io.ReadCloser, int64, error) {
	if err := blob.Check(ns, key); err != nil {
		return nil, 0, err
	}
	out, err := s.c.Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.c.Bucket),
		Key:    s.name(ns, key),
	})
	if err != nil {
		if s3conn.IsNotFound(err) {
			return nil, 0, blob.ErrNotFound
		}
		return nil, 0, err
	}
	return out.Body, aws.ToInt64(out.ContentLength), nil
}

// Exists checks each key with HeadObject.
func (s *Store) Exists(ctx context.Context, ns blob.Namespace, keys []string) (map[string]bool, error) {
	for _, k := range keys {
		if err := blob.Check(ns, k); err != nil {
			return nil, err
		}
	}
	out := make(map[string]bool, len(keys))
	var (
		mu       sync.Mutex
		firstErr error
		wg       sync.WaitGroup
	)
	sem := make(chan struct{}, existsParallel)
	for _, k := range keys {
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()
			_, err := s.c.Client.HeadObject(ctx, &s3.HeadObjectInput{
				Bucket: aws.String(s.c.Bucket),
				Key:    s.name(ns, k),
			})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				out[k] = true
			case s3conn.IsNotFound(err):
				out[k] = false
			case firstErr == nil:
				firstErr = fmt.Errorf("head %s: %w", k, err)
			}
		})
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

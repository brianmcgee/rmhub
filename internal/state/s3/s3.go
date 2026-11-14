// Package s3 is the S3 backend of the state store.
//
// The object name is <prefix><key>. The version is the ETag. Conditional
// writes use If-None-Match: * and If-Match.
package s3

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/brianmcgee/rmhub/internal/s3conn"
	"github.com/brianmcgee/rmhub/internal/state"
)

// Store keeps values in an S3 bucket.
type Store struct {
	c *s3conn.Conn
}

var _ state.Store = (*Store)(nil)

// New returns a store on the connection.
func New(c *s3conn.Conn) *Store { return &Store{c: c} }

func (s *Store) name(key string) *string { return aws.String(s.c.Prefix + key) }

// Get reads a key.
func (s *Store) Get(ctx context.Context, key string) ([]byte, string, error) {
	if err := state.CheckKey(key); err != nil {
		return nil, "", err
	}
	out, err := s.c.Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.c.Bucket),
		Key:    s.name(key),
	})
	if err != nil {
		if s3conn.IsNotFound(err) {
			return nil, "", state.ErrNotFound
		}
		return nil, "", err
	}
	defer func() { _ = out.Body.Close() }()
	b, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, "", err
	}
	return b, aws.ToString(out.ETag), nil
}

// PutIfAbsent writes with If-None-Match: *.
func (s *Store) PutIfAbsent(ctx context.Context, key string, value []byte) (string, error) {
	if err := state.CheckKey(key); err != nil {
		return "", err
	}
	return s.put(ctx, key, value, func(in *s3.PutObjectInput) {
		in.IfNoneMatch = aws.String("*")
	}, false)
}

// PutIfMatch writes with If-Match.
func (s *Store) PutIfMatch(ctx context.Context, key string, value []byte, version string) (string, error) {
	if err := state.CheckKey(key); err != nil {
		return "", err
	}
	if version == "" {
		return "", state.ErrConflict
	}
	return s.put(ctx, key, value, func(in *s3.PutObjectInput) {
		in.IfMatch = aws.String(version)
	}, true)
}

// put sends the conditional write. Status 412 is a conflict. With
// If-Match, status 404 is also a conflict: some services answer 404 for
// a missing key. Status 409 means a parallel conditional write. The write
// is tried once more. A second 409 is a failure, because the outcome is
// not known.
func (s *Store) put(ctx context.Context, key string, value []byte, cond func(*s3.PutObjectInput), ifMatch bool) (string, error) {
	var err error
	for range 2 {
		in := &s3.PutObjectInput{
			Bucket:        aws.String(s.c.Bucket),
			Key:           s.name(key),
			Body:          bytes.NewReader(value),
			ContentLength: aws.Int64(int64(len(value))),
		}
		cond(in)
		var out *s3.PutObjectOutput
		out, err = s.c.Client.PutObject(ctx, in, s3conn.NoRetry)
		switch st := s3conn.Status(err); {
		case err == nil:
			return aws.ToString(out.ETag), nil
		case st == http.StatusPreconditionFailed:
			return "", state.ErrConflict
		case st == http.StatusNotFound && ifMatch && s3conn.Code(err) != "NoSuchBucket":
			return "", state.ErrConflict
		case st == http.StatusConflict:
			continue
		default:
			return "", err
		}
	}
	return "", err
}

// Delete removes a key.
func (s *Store) Delete(ctx context.Context, key string) error {
	if err := state.CheckKey(key); err != nil {
		return err
	}
	_, err := s.c.Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.c.Bucket),
		Key:    s.name(key),
	})
	if s3conn.IsNotFound(err) {
		return nil
	}
	return err
}

// List lists keys with ListObjectsV2 and StartAfter.
func (s *Store) List(ctx context.Context, prefix, after string, limit int) ([]string, error) {
	if err := state.CheckPrefix(prefix); err != nil {
		return nil, err
	}
	in := &s3.ListObjectsV2Input{
		Bucket: aws.String(s.c.Bucket),
		Prefix: aws.String(s.c.Prefix + prefix),
	}
	if after != "" {
		in.StartAfter = aws.String(s.c.Prefix + after)
	}
	var out []string
	p := s3.NewListObjectsV2Paginator(s.c.Client, in)
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, o := range page.Contents {
			k := strings.TrimPrefix(aws.ToString(o.Key), s.c.Prefix)
			if after != "" && k <= after {
				continue
			}
			out = append(out, k)
			if limit > 0 && len(out) == limit {
				return out, nil
			}
		}
	}
	return out, nil
}

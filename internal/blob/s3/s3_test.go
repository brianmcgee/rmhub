package s3_test

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/brianmcgee/rmhub/internal/blob"
	"github.com/brianmcgee/rmhub/internal/blob/blobtest"
	blobs3 "github.com/brianmcgee/rmhub/internal/blob/s3"
	"github.com/brianmcgee/rmhub/internal/s3conn"
	"github.com/brianmcgee/rmhub/internal/s3conn/s3test"
)

func TestContract(t *testing.T) {
	blobtest.Run(t, func(t *testing.T) blob.Store {
		c, err := s3conn.New(context.Background(), s3test.Settings(t))
		if err != nil {
			t.Fatal(err)
		}
		return blobs3.New(c, t.TempDir())
	})
}

// TestStorageFailure needs no S3 server: the endpoint does not answer.
func TestStorageFailure(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	t.Setenv("AWS_ACCESS_KEY_ID", "x")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "y")
	c, err := s3conn.New(context.Background(), s3conn.Settings{
		Endpoint: "http://" + addr, Bucket: "b", PathStyle: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	s := blobs3.New(c, t.TempDir())
	ns := blobtest.NewNamespace(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, _, err = s.Get(ctx, ns, blobtest.HashOf([]byte("x")))
	if err == nil || errors.Is(err, blob.ErrNotFound) {
		t.Fatalf("err = %v, want a storage failure", err)
	}
}

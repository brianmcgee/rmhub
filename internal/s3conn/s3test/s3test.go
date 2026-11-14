// Package s3test gives S3 settings for tests from the environment.
//
// The tests run only when RMHUB_TEST_S3_ENDPOINT is set. These variables
// are also read:
//
//   - RMHUB_TEST_S3_BUCKET (default "rmhub-test")
//   - RMHUB_TEST_S3_ACCESS_KEY_ID
//   - RMHUB_TEST_S3_SECRET_ACCESS_KEY
//   - RMHUB_TEST_S3_REGION (default "us-east-1")
package s3test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/brianmcgee/rmhub/internal/ids"
	"github.com/brianmcgee/rmhub/internal/s3conn"
)

var bucketOnce sync.Map

// Settings returns settings with a new random prefix, or skips the test
// when RMHUB_TEST_S3_ENDPOINT is not set. It makes the bucket if it does
// not exist.
func Settings(t *testing.T) s3conn.Settings {
	t.Helper()
	endpoint := os.Getenv("RMHUB_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("RMHUB_TEST_S3_ENDPOINT is not set")
	}
	s := s3conn.Settings{
		Endpoint:    endpoint,
		Region:      envOr("RMHUB_TEST_S3_REGION", "us-east-1"),
		Bucket:      envOr("RMHUB_TEST_S3_BUCKET", "rmhub-test"),
		Prefix:      "test-" + ids.Random(10) + "/",
		PathStyle:   true,
		AccessKeyID: os.Getenv("RMHUB_TEST_S3_ACCESS_KEY_ID"),
	}
	if s.AccessKeyID != "" {
		p := filepath.Join(t.TempDir(), "secret")
		if err := os.WriteFile(p, []byte(os.Getenv("RMHUB_TEST_S3_SECRET_ACCESS_KEY")), 0o600); err != nil {
			t.Fatal(err)
		}
		s.SecretAccessKeyFile = p
	}
	if _, done := bucketOnce.LoadOrStore(s.Bucket, true); !done {
		c, err := s3conn.New(context.Background(), s)
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.Client.CreateBucket(context.Background(), &s3.CreateBucketInput{Bucket: aws.String(s.Bucket)})
		var owned *types.BucketAlreadyOwnedByYou
		var exists *types.BucketAlreadyExists
		if err != nil && !errors.As(err, &owned) && !errors.As(err, &exists) {
			t.Fatalf("create bucket %s: %v", s.Bucket, err)
		}
	}
	return s
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

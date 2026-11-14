// Package s3conn makes the S3 client for the S3 backends.
package s3conn

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

// Settings are the S3 settings of one backend.
type Settings struct {
	Endpoint            string `toml:"endpoint"`
	Region              string `toml:"region"`
	Bucket              string `toml:"bucket"`
	Prefix              string `toml:"prefix"`
	PathStyle           bool   `toml:"path_style"`
	AccessKeyID         string `toml:"access_key_id"`
	SecretAccessKeyFile string `toml:"secret_access_key_file"`
}

// Conn is an S3 client for one bucket and prefix.
type Conn struct {
	Client *s3.Client
	Bucket string
	Prefix string
}

// New makes the client. Without AccessKeyID, the client uses the
// standard AWS credential chain.
func New(ctx context.Context, s Settings) (*Conn, error) {
	if s.Bucket == "" {
		return nil, errors.New("s3: bucket is not set")
	}
	opts := []func(*config.LoadOptions) error{}
	region := s.Region
	if region == "" {
		region = "us-east-1"
	}
	opts = append(opts, config.WithRegion(region))
	if s.AccessKeyID != "" {
		secret, err := ReadSecret(s.SecretAccessKeyFile)
		if err != nil {
			return nil, err
		}
		opts = append(opts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(s.AccessKeyID, secret, "")))
	}
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("s3: %w", err)
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if s.Endpoint != "" {
			o.BaseEndpoint = aws.String(s.Endpoint)
		}
		o.UsePathStyle = s.PathStyle
		// Compute checksums only when an operation needs them. Some
		// S3-compatible services do not accept the default checksums.
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	})
	return &Conn{Client: client, Bucket: s.Bucket, Prefix: s.Prefix}, nil
}

// ReadSecret reads a secret from a file and removes the trailing line
// break.
func ReadSecret(path string) (string, error) {
	if path == "" {
		return "", errors.New("s3: access_key_id is set, but secret_access_key_file is not set")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("s3: secret_access_key_file: %w", err)
	}
	secret := strings.TrimRight(string(b), "\r\n")
	if secret == "" {
		return "", fmt.Errorf("s3: secret_access_key_file %s is empty", path)
	}
	return secret, nil
}

// NoRetry turns off the retries of the SDK for one operation. A retry of
// a conditional write after a lost answer would report the first write
// of the same request as a conflict.
func NoRetry(o *s3.Options) { o.Retryer = aws.NopRetryer{} }

// Status returns the HTTP status of an error from the SDK, or 0.
func Status(err error) int {
	var re *awshttp.ResponseError
	if errors.As(err, &re) {
		return re.HTTPStatusCode()
	}
	return 0
}

// Code returns the S3 error code of an error from the SDK, or "".
func Code(err error) string {
	var ae smithy.APIError
	if errors.As(err, &ae) {
		return ae.ErrorCode()
	}
	return ""
}

// IsNotFound tells if the error is status 404.
func IsNotFound(err error) bool { return Status(err) == http.StatusNotFound }

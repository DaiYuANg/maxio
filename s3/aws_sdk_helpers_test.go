package s3_test

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/index"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/store"
	"github.com/lyonbrown4d/maxio/object"
	maxios3 "github.com/lyonbrown4d/maxio/s3"
)

func newAWSSDKTestClient(t *testing.T) (*awss3.Client, func()) {
	t.Helper()

	objects := newAWSSDKObjectService(t)
	service := maxios3.NewService(objects, slog.New(slog.DiscardHandler), maxios3.Config{
		DataDir:   t.TempDir(),
		AccessKey: awsSDKAccessKey,
		SecretKey: awsSDKSigningMaterial(),
		Region:    awsSDKRegion,
	})
	server := httptest.NewServer(service)
	client := awss3.New(awss3.Options{
		BaseEndpoint:               aws.String(server.URL + "/s3"),
		Credentials:                aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(awsSDKAccessKey, awsSDKSigningMaterial(), "")),
		HTTPClient:                 server.Client(),
		Region:                     awsSDKRegion,
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		UsePathStyle:               true,
	})
	return client, server.Close
}

func awsSDKSigningMaterial() string {
	return strings.Repeat("m", 32)
}

func putAWSSDKObject(ctx context.Context, t *testing.T, client *awss3.Client, key, content string) {
	t.Helper()
	put, err := client.PutObject(ctx, &awss3.PutObjectInput{
		Bucket:      aws.String(awsSDKBucket),
		Key:         aws.String(key),
		Body:        strings.NewReader(content),
		ContentType: aws.String("text/plain"),
		Metadata: map[string]string{
			"owner": "sdk",
		},
	})
	if err != nil {
		t.Fatalf("aws sdk put object: %v", err)
	}
	if strings.TrimSpace(aws.ToString(put.ETag)) == "" {
		t.Fatal("aws sdk put object returned empty etag")
	}
}

func assertAWSSDKHeadObject(ctx context.Context, t *testing.T, client *awss3.Client, key string, wantLength int) {
	t.Helper()
	head, err := client.HeadObject(ctx, &awss3.HeadObjectInput{
		Bucket: aws.String(awsSDKBucket),
		Key:    aws.String(key),
	})
	if err != nil {
		t.Fatalf("aws sdk head object: %v", err)
	}
	if got := aws.ToInt64(head.ContentLength); got != int64(wantLength) {
		t.Fatalf("head content length = %d, want %d", got, wantLength)
	}
	if got := head.Metadata["owner"]; got != "sdk" {
		t.Fatalf("head metadata owner = %q, want sdk", got)
	}
}

func assertAWSSDKGetObject(ctx context.Context, t *testing.T, client *awss3.Client, key, want string) {
	t.Helper()
	get, err := client.GetObject(ctx, &awss3.GetObjectInput{
		Bucket: aws.String(awsSDKBucket),
		Key:    aws.String(key),
	})
	if err != nil {
		t.Fatalf("aws sdk get object: %v", err)
	}
	data := readAWSBody(t, get.Body)
	if string(data) != want {
		t.Fatalf("get body = %q, want %q", string(data), want)
	}
}

func assertAWSSDKListPrefix(ctx context.Context, t *testing.T, client *awss3.Client, prefix, want string) {
	t.Helper()
	list, err := client.ListObjectsV2(ctx, &awss3.ListObjectsV2Input{
		Bucket: aws.String(awsSDKBucket),
		Prefix: aws.String(prefix),
	})
	if err != nil {
		t.Fatalf("aws sdk list objects: %v", err)
	}
	if len(list.Contents) != 1 || aws.ToString(list.Contents[0].Key) != want {
		t.Fatalf("list contents = %+v, want %s", list.Contents, want)
	}
}

func deleteAWSSDKObject(ctx context.Context, t *testing.T, client *awss3.Client, key string) {
	t.Helper()
	if _, err := client.DeleteObject(ctx, &awss3.DeleteObjectInput{
		Bucket: aws.String(awsSDKBucket),
		Key:    aws.String(key),
	}); err != nil {
		t.Fatalf("aws sdk delete object: %v", err)
	}
}

func newAWSSDKObjectService(t *testing.T) *object.Service {
	t.Helper()

	storage, err := store.NewStore(t.TempDir(), metadata.NewInMemoryMetadata(), nil)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	objects := object.NewService(storage, index.NewInMemorySearchEngine(), nil, slog.New(slog.DiscardHandler), config.Config{})
	if err := objects.CreateBucket(context.Background(), awsSDKBucket); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	return objects
}

func readAWSBody(t *testing.T, body io.ReadCloser) []byte {
	t.Helper()
	defer closeAWSBody(t, body)
	return readHTTPBody(t, body)
}

func readHTTPBody(t *testing.T, body io.Reader) []byte {
	t.Helper()
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return data
}

func closeAWSBody(t *testing.T, body io.Closer) {
	t.Helper()
	if err := body.Close(); err != nil {
		t.Fatalf("close body: %v", err)
	}
}

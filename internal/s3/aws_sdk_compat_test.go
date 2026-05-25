package s3_test

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const (
	awsSDKAccessKey = "maxio-sdk-client"
	awsSDKRegion    = "us-east-1"
	awsSDKBucket    = "bucket"
)

func TestAWSSDKObjectOperations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	client, cleanup := newAWSSDKTestClient(t)
	defer cleanup()

	putAWSSDKObject(ctx, t, client, "sdk/object.txt", "sdk object content")
	assertAWSSDKHeadObject(ctx, t, client, "sdk/object.txt", len("sdk object content"))
	assertAWSSDKGetObject(ctx, t, client, "sdk/object.txt", "sdk object content")
	assertAWSSDKListPrefix(ctx, t, client, "sdk/", "sdk/object.txt")
	deleteAWSSDKObject(ctx, t, client, "sdk/object.txt")
}

func TestAWSSDKRangeAndPresign(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	client, cleanup := newAWSSDKTestClient(t)
	defer cleanup()

	if _, err := client.PutObject(ctx, &awss3.PutObjectInput{
		Bucket: aws.String(awsSDKBucket),
		Key:    aws.String("sdk/range.txt"),
		Body:   strings.NewReader("0123456789"),
	}); err != nil {
		t.Fatalf("aws sdk put object: %v", err)
	}

	ranged, err := client.GetObject(ctx, &awss3.GetObjectInput{
		Bucket: aws.String(awsSDKBucket),
		Key:    aws.String("sdk/range.txt"),
		Range:  aws.String("bytes=2-6"),
	})
	if err != nil {
		t.Fatalf("aws sdk range get: %v", err)
	}
	if got := string(readAWSBody(t, ranged.Body)); got != "23456" {
		t.Fatalf("range body = %q, want 23456", got)
	}
	if got := aws.ToString(ranged.ContentRange); got != "bytes 2-6/10" {
		t.Fatalf("content range = %q, want bytes 2-6/10", got)
	}

	presigned, err := awss3.NewPresignClient(client).PresignGetObject(
		ctx,
		&awss3.GetObjectInput{
			Bucket: aws.String(awsSDKBucket),
			Key:    aws.String("sdk/range.txt"),
		},
		awss3.WithPresignExpires(time.Minute),
	)
	if err != nil {
		t.Fatalf("aws sdk presign get object: %v", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, presigned.URL, http.NoBody)
	if err != nil {
		t.Fatalf("build presigned request: %v", err)
	}
	response, err := client.Options().HTTPClient.Do(request)
	if err != nil {
		t.Fatalf("http get presigned url: %v", err)
	}
	defer closeAWSBody(t, response.Body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("presigned status = %d", response.StatusCode)
	}
	if got := string(readHTTPBody(t, response.Body)); got != "0123456789" {
		t.Fatalf("presigned body = %q, want 0123456789", got)
	}
}

func TestAWSSDKMultipartUpload(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	client, cleanup := newAWSSDKTestClient(t)
	defer cleanup()

	create, err := client.CreateMultipartUpload(ctx, &awss3.CreateMultipartUploadInput{
		Bucket:      aws.String(awsSDKBucket),
		Key:         aws.String("sdk/multipart.txt"),
		ContentType: aws.String("text/plain"),
	})
	if err != nil {
		t.Fatalf("aws sdk create multipart upload: %v", err)
	}
	uploadID := aws.ToString(create.UploadId)
	if uploadID == "" {
		t.Fatal("aws sdk create multipart upload returned empty upload id")
	}

	firstPart := strings.Repeat("a", 5*1024*1024)
	partOne, err := client.UploadPart(ctx, &awss3.UploadPartInput{
		Bucket:     aws.String(awsSDKBucket),
		Key:        aws.String("sdk/multipart.txt"),
		UploadId:   aws.String(uploadID),
		PartNumber: aws.Int32(1),
		Body:       strings.NewReader(firstPart),
	})
	if err != nil {
		t.Fatalf("aws sdk upload part 1: %v", err)
	}
	partTwo, err := client.UploadPart(ctx, &awss3.UploadPartInput{
		Bucket:     aws.String(awsSDKBucket),
		Key:        aws.String("sdk/multipart.txt"),
		UploadId:   aws.String(uploadID),
		PartNumber: aws.Int32(2),
		Body:       strings.NewReader("tail"),
	})
	if err != nil {
		t.Fatalf("aws sdk upload part 2: %v", err)
	}

	complete, err := client.CompleteMultipartUpload(ctx, &awss3.CompleteMultipartUploadInput{
		Bucket:   aws.String(awsSDKBucket),
		Key:      aws.String("sdk/multipart.txt"),
		UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: []types.CompletedPart{
				{ETag: partOne.ETag, PartNumber: aws.Int32(1)},
				{ETag: partTwo.ETag, PartNumber: aws.Int32(2)},
			},
		},
	})
	if err != nil {
		t.Fatalf("aws sdk complete multipart upload: %v", err)
	}
	if !strings.Contains(aws.ToString(complete.ETag), "-2") {
		t.Fatalf("complete etag = %q, want multipart suffix", aws.ToString(complete.ETag))
	}

	get, err := client.GetObject(ctx, &awss3.GetObjectInput{
		Bucket: aws.String(awsSDKBucket),
		Key:    aws.String("sdk/multipart.txt"),
	})
	if err != nil {
		t.Fatalf("aws sdk get completed multipart object: %v", err)
	}
	data := readAWSBody(t, get.Body)
	if len(data) != len(firstPart)+len("tail") || !bytes.HasSuffix(data, []byte("tail")) {
		t.Fatalf("multipart object length = %d, want suffix tail and length %d", len(data), len(firstPart)+len("tail"))
	}
}

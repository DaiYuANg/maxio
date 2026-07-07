package proxy

import "testing"

func TestParseS3ObjectPathKeepsObjectKeyWhitespace(t *testing.T) {
	bucket, key, ok := parseS3ObjectPath("/docs/ record.txt")
	if !ok {
		t.Fatal("expected object path to parse")
	}
	if bucket != "docs" {
		t.Fatalf("bucket = %q, want docs", bucket)
	}
	if key != " record.txt" {
		t.Fatalf("key = %q, want leading-space key", key)
	}
}
func TestParseS3ObjectPathKeepsObjectKeyPathSegments(t *testing.T) {
	_, key, ok := parseS3ObjectPath("/docs/a//../b.txt")
	if !ok {
		t.Fatal("expected object path to parse")
	}
	if key != "a//../b.txt" {
		t.Fatalf("key = %q, want uncleaned key", key)
	}
}

func TestS3ObjectPathKeepsLeadingSlashInObjectKey(t *testing.T) {
	path := s3ObjectPath("docs", "/nested.txt")
	if path != "/docs//nested.txt" {
		t.Fatalf("path = %q, want /docs//nested.txt", path)
	}
}
func TestParseS3ObjectPathToleratesExtraLeadingSlashBeforeBucket(t *testing.T) {
	bucket, key, ok := parseS3ObjectPath("//docs/file.txt")
	if !ok {
		t.Fatal("expected object path to parse")
	}
	if bucket != "docs" || key != "file.txt" {
		t.Fatalf("bucket/key = %q/%q, want docs/file.txt", bucket, key)
	}
}

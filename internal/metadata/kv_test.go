package metadata_test

import (
	"context"
	"testing"

	"github.com/lyonbrown4d/maxio/internal/metadata"
)

func TestInMemoryMetadataBlobRefStoresCoreFields(t *testing.T) {
	meta := metadata.NewInMemoryMetadata()

	err := meta.CreateBlobRef(context.Background(), "hash", "blob-dir", 2048)
	if err != nil {
		t.Fatalf("create blob ref: %v", err)
	}

	ref, ok, err := meta.GetBlobRef(context.Background(), "hash")
	if err != nil {
		t.Fatalf("get blob ref: %v", err)
	}
	if !ok {
		t.Fatal("blob ref not found")
	}
	if ref.Hash != "hash" {
		t.Fatalf("blob ref hash = %q, want hash", ref.Hash)
	}
	if ref.Path != "blob-dir" {
		t.Fatalf("blob ref path = %q, want %q", ref.Path, "blob-dir")
	}
	if ref.Size != 2048 {
		t.Fatalf("blob ref size = %d, want %d", ref.Size, 2048)
	}
	if ref.RefCount != 1 {
		t.Fatalf("blob ref ref count = %d, want %d", ref.RefCount, 1)
	}
}

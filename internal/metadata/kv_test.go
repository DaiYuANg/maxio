package metadata_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/model"
)

func TestInMemoryMetadataBlobRefStoresShardPlacements(t *testing.T) {
	meta := metadata.NewInMemoryMetadata()

	placements := []model.ShardPlacement{
		{Index: 0, NodeID: "node-a", NodeAddress: "127.0.0.1:9001", Local: true},
		{Index: 1, NodeID: "node-b", NodeAddress: "127.0.0.1:9002", Local: true},
	}
	checksums := []string{"checksum-a", "checksum-b"}
	sizes := []int64{128, 256}
	err := meta.CreateBlobRef(context.Background(), "hash", "shard-dir", 2048, placements, checksums, sizes)
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
	if !reflect.DeepEqual(ref.ShardPlacements, placements) {
		t.Fatalf("stored shard placements %#v, want %#v", ref.ShardPlacements, placements)
	}
	if !reflect.DeepEqual(ref.ShardChecksums, checksums) {
		t.Fatalf("stored shard checksums %#v, want %#v", ref.ShardChecksums, checksums)
	}
	if !reflect.DeepEqual(ref.ShardSizes, sizes) {
		t.Fatalf("stored shard sizes %#v, want %#v", ref.ShardSizes, sizes)
	}

	placements[0].NodeID = "changed"
	checksums[0] = "changed"
	sizes[0] = 999
	if ref.ShardPlacements[0].NodeID != "node-a" {
		t.Fatalf("stored shard placements mutated by caller: %q", ref.ShardPlacements[0].NodeID)
	}
	if ref.ShardChecksums[0] != "checksum-a" {
		t.Fatalf("stored shard checksums mutated by caller: %q", ref.ShardChecksums[0])
	}
	if ref.ShardSizes[0] != 128 {
		t.Fatalf("stored shard sizes mutated by caller: %d", ref.ShardSizes[0])
	}
}

func TestInMemoryMetadataUpdatesBlobRefPlacements(t *testing.T) {
	meta := metadata.NewInMemoryMetadata()
	original := []model.ShardPlacement{{Index: 0, NodeID: "node-a"}}
	updated := []model.ShardPlacement{{Index: 0, NodeID: "node-b"}}
	err := meta.CreateBlobRef(context.Background(), "hash", "shard-dir", 2048, original, nil)
	if err != nil {
		t.Fatalf("create blob ref: %v", err)
	}

	updateErr := meta.UpdateBlobRefPlacements(context.Background(), "hash", updated)
	if updateErr != nil {
		t.Fatalf("update blob ref placements: %v", updateErr)
	}

	ref, ok, err := meta.GetBlobRef(context.Background(), "hash")
	if err != nil {
		t.Fatalf("get blob ref: %v", err)
	}
	if !ok {
		t.Fatal("blob ref not found")
	}
	if !reflect.DeepEqual(ref.ShardPlacements, updated) {
		t.Fatalf("updated shard placements %#v, want %#v", ref.ShardPlacements, updated)
	}
}

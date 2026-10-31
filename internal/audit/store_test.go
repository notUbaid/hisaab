package audit_test

import (
	"context"
	"testing"
	"time"

	"github.com/ubaid/hisaab/internal/audit"
)

func TestMemoryRootStore_SaveAndGet(t *testing.T) {
	store := audit.NewMemoryRootStore()
	ctx := context.Background()

	root := audit.SignedRoot{
		BatchID:          "BATCH-TEST-001",
		MerkleRoot:       "sha256:abcdef1234567890abcdef1234567890",
		LeafCount:        4,
		Algorithm:        "SHA-256",
		NPPairID:         "BAP-01:BPP-01",
		SignatureEd25519: "ed25519-sig-mock-test",
		SigningKeyID:     "sa-key-1",
		SignedAt:         time.Now(),
		CreatedAt:        time.Now(),
	}

	if err := store.SaveRoot(ctx, root); err != nil {
		t.Fatalf("SaveRoot failed: %v", err)
	}

	retrieved, err := store.GetRoot(ctx, "BATCH-TEST-001")
	if err != nil {
		t.Fatalf("GetRoot failed: %v", err)
	}

	if retrieved.MerkleRoot != root.MerkleRoot {
		t.Errorf("expected root %s, got %s", root.MerkleRoot, retrieved.MerkleRoot)
	}
	if retrieved.LeafCount != 4 {
		t.Errorf("expected leaf count 4, got %d", retrieved.LeafCount)
	}

	// Non-existent batch
	_, err = store.GetRoot(ctx, "BATCH-NONEXISTENT")
	if err != audit.ErrRootNotFound {
		t.Errorf("expected ErrRootNotFound, got %v", err)
	}
}

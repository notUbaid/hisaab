package audit_test

import (
	"testing"

	"github.com/ubaid/hisaab/internal/audit"
	"github.com/ubaid/hisaab/internal/ledger"
)

func TestMerkleTree_DeterministicRootAndProof(t *testing.T) {
	entries := [][]byte{
		audit.CanonicalEntryBytes(1, ledger.EntryLine{AccountName: "collector", Direction: ledger.Debit, AmountPaise: 100000}),
		audit.CanonicalEntryBytes(1, ledger.EntryLine{AccountName: "receiver", Direction: ledger.Credit, AmountPaise: 96000}),
		audit.CanonicalEntryBytes(1, ledger.EntryLine{AccountName: "tax:tds", Direction: ledger.Credit, AmountPaise: 1000}),
		audit.CanonicalEntryBytes(1, ledger.EntryLine{AccountName: "tax:tcs", Direction: ledger.Credit, AmountPaise: 3000}),
	}

	tree, err := audit.BuildMerkleTree(entries)
	if err != nil {
		t.Fatalf("BuildMerkleTree failed: %v", err)
	}

	if tree.RootHex == "" {
		t.Fatal("Expected non-empty root hex")
	}

	// Verify inclusion proof for leaf 1 (receiver)
	proof, err := tree.GenerateInclusionProof(1)
	if err != nil {
		t.Fatalf("GenerateInclusionProof failed: %v", err)
	}

	valid, err := audit.VerifyInclusionProof(proof, entries[1])
	if err != nil || !valid {
		t.Fatalf("Expected valid inclusion proof, got valid=%v, err=%v", valid, err)
	}
}

func TestMerkleTree_TamperedEntry_Detection(t *testing.T) {
	originalEntry := audit.CanonicalEntryBytes(1, ledger.EntryLine{
		AccountName: "collector",
		Direction:   ledger.Debit,
		AmountPaise: 100000,
	})
	entries := [][]byte{
		originalEntry,
		audit.CanonicalEntryBytes(1, ledger.EntryLine{AccountName: "receiver", Direction: ledger.Credit, AmountPaise: 100000}),
	}

	tree, err := audit.BuildMerkleTree(entries)
	if err != nil {
		t.Fatalf("BuildMerkleTree failed: %v", err)
	}

	proof, err := tree.GenerateInclusionProof(0)
	if err != nil {
		t.Fatalf("GenerateInclusionProof failed: %v", err)
	}

	// Tamper: change 100000 to 100001 (1 paise tamper)
	tamperedEntry := audit.CanonicalEntryBytes(1, ledger.EntryLine{
		AccountName: "collector",
		Direction:   ledger.Debit,
		AmountPaise: 100001,
	})

	valid, err := audit.VerifyInclusionProof(proof, tamperedEntry)
	if valid || err == nil {
		t.Fatal("Expected verification failure on tampered entry, got valid=true")
	}
}

func TestBatchRootChain_UnbrokenAndTampered(t *testing.T) {
	// Construct 3 sequential batch roots
	root1 := "1111111111111111111111111111111111111111111111111111111111111111"
	chain1 := audit.ComputeChainHash(1, "", root1)

	root2 := "2222222222222222222222222222222222222222222222222222222222222222"
	chain2 := audit.ComputeChainHash(2, root1, root2)

	root3 := "3333333333333333333333333333333333333333333333333333333333333333"
	chain3 := audit.ComputeChainHash(3, root2, root3)

	chain := []audit.BatchRootChain{
		{BatchID: 1, RootHash: root1, PrevRootHash: "", ChainHash: chain1},
		{BatchID: 2, RootHash: root2, PrevRootHash: root1, ChainHash: chain2},
		{BatchID: 3, RootHash: root3, PrevRootHash: root2, ChainHash: chain3},
	}

	ok, breakIdx := audit.VerifyBatchChain(chain)
	if !ok || breakIdx != -1 {
		t.Fatalf("Expected valid batch chain, got ok=%v, breakIdx=%d", ok, breakIdx)
	}

	// Tamper batch 2 root
	chain[1].RootHash = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	okTampered, breakIdxTampered := audit.VerifyBatchChain(chain)
	if okTampered || breakIdxTampered != 1 {
		t.Fatalf("Expected tamper detected at index 1, got ok=%v, breakIdx=%d", okTampered, breakIdxTampered)
	}
}

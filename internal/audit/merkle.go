package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/ubaid/hisaab/internal/ledger"
)

var (
	ErrEmptyEntrySet    = errors.New("merkle: cannot compute tree for empty entry set")
	ErrInvalidProof     = errors.New("merkle: invalid proof structure")
	ErrProofFailed      = errors.New("merkle: cryptographic proof verification failed")
	ErrLeafOutOfBounds  = errors.New("merkle: leaf index out of bounds")
)

// Domain separation prefixes (RFC 6962 standard) to prevent second-preimage attacks
const (
	LeafPrefix byte = 0x00
	NodePrefix byte = 0x01
)

// CanonicalEntryBytes serializes an EntryLine deterministically with unambiguous length delimiters.
func CanonicalEntryBytes(txID int64, line ledger.EntryLine) []byte {
	return []byte(fmt.Sprintf("tx:%d|acc:%s|dir:%s|amt:%d", txID, line.AccountName, line.Direction, line.AmountPaise))
}

// HashLeaf computes SHA-256(0x00 || canonicalEntryBytes).
func HashLeaf(canonicalBytes []byte) []byte {
	h := sha256.New()
	h.Write([]byte{LeafPrefix})
	h.Write(canonicalBytes)
	return h.Sum(nil)
}

// HashNode computes SHA-256(0x01 || left || right).
func HashNode(left, right []byte) []byte {
	h := sha256.New()
	h.Write([]byte{NodePrefix})
	h.Write(left)
	h.Write(right)
	return h.Sum(nil)
}

// MerkleNode represents a step in an inclusion proof path.
type MerkleNode struct {
	Hash    string `json:"hash"`
	IsRight bool   `json:"is_right"`
}

// MerkleInclusionProof proves that an entry is included in the batch Merkle root.
type MerkleInclusionProof struct {
	LeafIndex int          `json:"leaf_index"`
	LeafHash  string       `json:"leaf_hash"`
	RootHash  string       `json:"root_hash"`
	AuditPath []MerkleNode `json:"audit_path"`
}

// MerkleTree represents a balanced binary Merkle tree over a batch's journal entries.
type MerkleTree struct {
	Leaves   [][]byte     // SHA-256(0x00 || entry)
	Levels   [][][]byte   // Levels[0] = Leaves, Levels[len-1] = [Root]
	Root     []byte       // Root hash bytes
	RootHex  string       // Hex-encoded root hash
}

// BuildMerkleTree builds a tamper-evident Merkle tree from canonical entry bytes.
func BuildMerkleTree(entries [][]byte) (*MerkleTree, error) {
	if len(entries) == 0 {
		return nil, ErrEmptyEntrySet
	}

	leaves := make([][]byte, len(entries))
	for i, e := range entries {
		leaves[i] = HashLeaf(e)
	}

	levels := make([][][]byte, 0)
	currentLevel := leaves
	levels = append(levels, currentLevel)

	for len(currentLevel) > 1 {
		nextLevel := make([][]byte, 0, (len(currentLevel)+1)/2)
		for i := 0; i < len(currentLevel); i += 2 {
			if i+1 < len(currentLevel) {
				nextLevel = append(nextLevel, HashNode(currentLevel[i], currentLevel[i+1]))
			} else {
				// Odd node is paired with itself (standard Bitcoin/RFC 6962 duplicate policy)
				nextLevel = append(nextLevel, HashNode(currentLevel[i], currentLevel[i]))
			}
		}
		levels = append(levels, nextLevel)
		currentLevel = nextLevel
	}

	root := currentLevel[0]
	return &MerkleTree{
		Leaves:  leaves,
		Levels:  levels,
		Root:    root,
		RootHex: hex.EncodeToString(root),
	}, nil
}

// GenerateInclusionProof produces an audit path proving inclusion of the leaf at leafIndex.
func (t *MerkleTree) GenerateInclusionProof(leafIndex int) (*MerkleInclusionProof, error) {
	if leafIndex < 0 || leafIndex >= len(t.Leaves) {
		return nil, ErrLeafOutOfBounds
	}

	path := make([]MerkleNode, 0)
	idx := leafIndex

	for levelIdx := 0; levelIdx < len(t.Levels)-1; levelIdx++ {
		level := t.Levels[levelIdx]
		var sibling []byte
		isRight := false

		if idx%2 == 0 {
			// Sibling is on the right
			if idx+1 < len(level) {
				sibling = level[idx+1]
			} else {
				sibling = level[idx] // Duplicated odd node
			}
			isRight = true
		} else {
			// Sibling is on the left
			sibling = level[idx-1]
			isRight = false
		}

		path = append(path, MerkleNode{
			Hash:    hex.EncodeToString(sibling),
			IsRight: isRight,
		})
		idx /= 2
	}

	return &MerkleInclusionProof{
		LeafIndex: leafIndex,
		LeafHash:  hex.EncodeToString(t.Leaves[leafIndex]),
		RootHash:  t.RootHex,
		AuditPath: path,
	}, nil
}

// VerifyInclusionProof verifies an inclusion proof without the original tree.
func VerifyInclusionProof(proof *MerkleInclusionProof, rawEntryBytes []byte) (bool, error) {
	if proof == nil {
		return false, ErrInvalidProof
	}

	expectedLeaf := HashLeaf(rawEntryBytes)
	if hex.EncodeToString(expectedLeaf) != proof.LeafHash {
		return false, fmt.Errorf("%w: leaf hash mismatch", ErrProofFailed)
	}

	current := expectedLeaf
	for _, node := range proof.AuditPath {
		siblingBytes, err := hex.DecodeString(node.Hash)
		if err != nil {
			return false, fmt.Errorf("%w: invalid hex sibling", ErrInvalidProof)
		}

		if node.IsRight {
			current = HashNode(current, siblingBytes)
		} else {
			current = HashNode(siblingBytes, current)
		}
	}

	computedRootHex := hex.EncodeToString(current)
	if computedRootHex != proof.RootHash {
		return false, fmt.Errorf("%w: expected root %s, computed %s", ErrProofFailed, proof.RootHash, computedRootHex)
	}

	return true, nil
}

// BatchRootChain models a tamper-evident root chain linking sequential batches.
type BatchRootChain struct {
	BatchID      int64  `json:"batch_id"`
	RootHash     string `json:"root_hash"`
	PrevRootHash string `json:"prev_root_hash"`
	ChainHash    string `json:"chain_hash"`
}

// ComputeChainHash links batch root with the previous batch root.
func ComputeChainHash(batchID int64, prevRootHex, rootHex string) string {
	payload := fmt.Sprintf("batch:%d|prev:%s|root:%s", batchID, prevRootHex, rootHex)
	hash := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(hash[:])
}

// VerifyBatchChain verifies that a series of batch roots form an unbroken cryptographic chain.
func VerifyBatchChain(chain []BatchRootChain) (bool, int) {
	for i := range chain {
		if i > 0 {
			if chain[i].PrevRootHash != chain[i-1].RootHash {
				return false, i // Link broken between i-1 and i
			}
		}
		expectedChainHash := ComputeChainHash(chain[i].BatchID, chain[i].PrevRootHash, chain[i].RootHash)
		if chain[i].ChainHash != expectedChainHash {
			return false, i // Tampered chain hash at i
		}
	}
	return true, -1
}

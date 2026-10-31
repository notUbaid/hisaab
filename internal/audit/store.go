package audit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrRootNotFound = errors.New("audit: batch root record not found")

// SignedRoot represents a cryptographically signed batch Merkle root stored in PostgreSQL.
type SignedRoot struct {
	RootID           int64     `json:"root_id"`
	BatchID          string    `json:"batch_id"`
	MerkleRoot       string    `json:"merkle_root"`
	LeafCount        int       `json:"leaf_count"`
	Algorithm        string    `json:"algorithm"`
	NPPairID         string    `json:"np_pair_id"`
	SignatureEd25519 string    `json:"signature_ed25519,omitempty"`
	SigningKeyID     string    `json:"signing_key_id,omitempty"`
	SignedAt         time.Time `json:"signed_at,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// RootStore provides transactional persistence for signed batch Merkle roots.
type RootStore interface {
	SaveRoot(ctx context.Context, root SignedRoot) error
	GetRoot(ctx context.Context, batchID string) (*SignedRoot, error)
	ListRoots(ctx context.Context, limit int) ([]SignedRoot, error)
}

// PostgresRootStore implements RootStore using PostgreSQL batch_roots table.
type PostgresRootStore struct {
	pool *pgxpool.Pool
}

// NewPostgresRootStore initializes a Postgres-backed batch root store.
func NewPostgresRootStore(pool *pgxpool.Pool) *PostgresRootStore {
	return &PostgresRootStore{pool: pool}
}

// SaveRoot persists or updates a signed batch Merkle root record.
func (s *PostgresRootStore) SaveRoot(ctx context.Context, root SignedRoot) error {
	if s.pool == nil {
		return nil
	}

	algo := root.Algorithm
	if algo == "" {
		algo = "SHA-256"
	}
	now := root.CreatedAt
	if now.IsZero() {
		now = time.Now()
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO batch_roots (
			batch_id, merkle_root, leaf_count, algorithm,
			np_pair_id, signature_ed25519, signing_key_id, signed_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (batch_id) DO UPDATE SET
			merkle_root = EXCLUDED.merkle_root,
			leaf_count = EXCLUDED.leaf_count,
			signature_ed25519 = EXCLUDED.signature_ed25519,
			signing_key_id = EXCLUDED.signing_key_id,
			signed_at = EXCLUDED.signed_at
	`, root.BatchID, root.MerkleRoot, root.LeafCount, algo,
		root.NPPairID, root.SignatureEd25519, root.SigningKeyID, root.SignedAt, now)
	if err != nil {
		return fmt.Errorf("audit_store: save batch root %s failed: %w", root.BatchID, err)
	}

	return nil
}

// GetRoot retrieves a batch root by its unique batch identifier.
func (s *PostgresRootStore) GetRoot(ctx context.Context, batchID string) (*SignedRoot, error) {
	if s.pool == nil {
		return nil, ErrRootNotFound
	}

	var r SignedRoot
	var sig, keyID *string
	var signedAt *time.Time

	err := s.pool.QueryRow(ctx, `
		SELECT root_id, batch_id, merkle_root, leaf_count, algorithm,
		       np_pair_id, signature_ed25519, signing_key_id, signed_at, created_at
		FROM batch_roots
		WHERE batch_id = $1
	`, batchID).Scan(
		&r.RootID, &r.BatchID, &r.MerkleRoot, &r.LeafCount, &r.Algorithm,
		&r.NPPairID, &sig, &keyID, &signedAt, &r.CreatedAt,
	)
	if err != nil {
		return nil, ErrRootNotFound
	}

	if sig != nil {
		r.SignatureEd25519 = *sig
	}
	if keyID != nil {
		r.SigningKeyID = *keyID
	}
	if signedAt != nil {
		r.SignedAt = *signedAt
	}

	return &r, nil
}

// ListRoots returns the most recent batch roots up to limit.
func (s *PostgresRootStore) ListRoots(ctx context.Context, limit int) ([]SignedRoot, error) {
	if s.pool == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.pool.Query(ctx, `
		SELECT root_id, batch_id, merkle_root, leaf_count, algorithm,
		       np_pair_id, signature_ed25519, signing_key_id, signed_at, created_at
		FROM batch_roots
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("audit_store: list batch roots failed: %w", err)
	}
	defer rows.Close()

	var roots []SignedRoot
	for rows.Next() {
		var r SignedRoot
		var sig, keyID *string
		var signedAt *time.Time

		if err := rows.Scan(
			&r.RootID, &r.BatchID, &r.MerkleRoot, &r.LeafCount, &r.Algorithm,
			&r.NPPairID, &sig, &keyID, &signedAt, &r.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("audit_store: scan root failed: %w", err)
		}

		if sig != nil {
			r.SignatureEd25519 = *sig
		}
		if keyID != nil {
			r.SigningKeyID = *keyID
		}
		if signedAt != nil {
			r.SignedAt = *signedAt
		}
		roots = append(roots, r)
	}

	return roots, nil
}

// MemoryRootStore implements RootStore in memory for tests.
type MemoryRootStore struct {
	mu    sync.RWMutex
	roots map[string]SignedRoot
}

// NewMemoryRootStore initializes an in-memory root store.
func NewMemoryRootStore() *MemoryRootStore {
	return &MemoryRootStore{
		roots: make(map[string]SignedRoot),
	}
}

func (m *MemoryRootStore) SaveRoot(_ context.Context, root SignedRoot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.roots[root.BatchID] = root
	return nil
}

func (m *MemoryRootStore) GetRoot(_ context.Context, batchID string) (*SignedRoot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	root, ok := m.roots[batchID]
	if !ok {
		return nil, ErrRootNotFound
	}
	return &root, nil
}

func (m *MemoryRootStore) ListRoots(_ context.Context, limit int) ([]SignedRoot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []SignedRoot
	for _, r := range m.roots {
		list = append(list, r)
		if limit > 0 && len(list) >= limit {
			break
		}
	}
	return list, nil
}

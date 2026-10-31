package recon

import (
	"context"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ubaid/hisaab/internal/ledger"
)

// Repository defines transactional persistence for reconciliation exceptions.
type Repository interface {
	SaveExceptions(ctx context.Context, exceptions []ReconException) error
	ListExceptions(ctx context.Context, status string) ([]ReconException, error)
	GetException(ctx context.Context, id string) (*ReconException, error)
	ResolveException(ctx context.Context, id string, remedy string) error
}

// PostgresRepository persists recon exceptions to PostgreSQL.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository initializes a Postgres-backed reconciliation repository.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// SaveExceptions transactionally upserts detected reconciliation breaks.
func (r *PostgresRepository) SaveExceptions(ctx context.Context, exceptions []ReconException) error {
	if r.pool == nil || len(exceptions) == 0 {
		return nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("recon_repo: begin tx failed: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	for _, exc := range exceptions {
		variance := exc.ActualPaise - exc.ExpectedPaise
		_, err := tx.Exec(ctx, `
			INSERT INTO recon_exceptions (
				exception_id, network_order_id, exception_class,
				expected_paise, actual_paise, variance_paise,
				ageing_days, status, details, detected_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (exception_id) DO UPDATE SET
				actual_paise = EXCLUDED.actual_paise,
				variance_paise = EXCLUDED.variance_paise,
				ageing_days = EXCLUDED.ageing_days,
				details = EXCLUDED.details
		`, exc.ID, exc.NetworkOrderID, string(exc.Class),
			int64(exc.ExpectedPaise), int64(exc.ActualPaise), int64(variance),
			exc.AgeingDays, string(exc.Status), exc.Details, exc.DetectedAt)
		if err != nil {
			return fmt.Errorf("recon_repo: insert exception %s failed: %w", exc.ID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("recon_repo: commit tx failed: %w", err)
	}
	return nil
}

// ListExceptions queries exceptions, optionally filtered by status ("OPEN", "RESOLVED").
func (r *PostgresRepository) ListExceptions(ctx context.Context, status string) ([]ReconException, error) {
	if r.pool == nil {
		return nil, nil
	}

	var query string
	var args []any

	if status != "" && status != "ALL" {
		query = `
			SELECT exception_id, network_order_id, exception_class,
			       expected_paise, actual_paise, ageing_days, status,
			       details, detected_at
			FROM recon_exceptions
			WHERE status = $1
			ORDER BY detected_at DESC
		`
		args = append(args, status)
	} else {
		query = `
			SELECT exception_id, network_order_id, exception_class,
			       expected_paise, actual_paise, ageing_days, status,
			       details, detected_at
			FROM recon_exceptions
			ORDER BY detected_at DESC
		`
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("recon_repo: query exceptions failed: %w", err)
	}
	defer rows.Close()

	var exceptions []ReconException
	for rows.Next() {
		var exc ReconException
		var expPaise, actPaise int64
		var classStr, statusStr string

		if err := rows.Scan(
			&exc.ID, &exc.NetworkOrderID, &classStr,
			&expPaise, &actPaise, &exc.AgeingDays, &statusStr,
			&exc.Details, &exc.DetectedAt,
		); err != nil {
			return nil, fmt.Errorf("recon_repo: scan exception failed: %w", err)
		}

		exc.Class = ExceptionClass(classStr)
		exc.Status = ReconStatus(statusStr)
		exc.ExpectedPaise = ledger.AmountPaise(expPaise)
		exc.ActualPaise = ledger.AmountPaise(actPaise)
		exceptions = append(exceptions, exc)
	}

	return exceptions, nil
}

// GetException retrieves an exception by ID.
func (r *PostgresRepository) GetException(ctx context.Context, id string) (*ReconException, error) {
	if r.pool == nil {
		return nil, ErrExceptionNotFound
	}

	var exc ReconException
	var expPaise, actPaise int64
	var classStr, statusStr string

	err := r.pool.QueryRow(ctx, `
		SELECT exception_id, network_order_id, exception_class,
		       expected_paise, actual_paise, ageing_days, status,
		       details, detected_at
		FROM recon_exceptions
		WHERE exception_id = $1
	`, id).Scan(
		&exc.ID, &exc.NetworkOrderID, &classStr,
		&expPaise, &actPaise, &exc.AgeingDays, &statusStr,
		&exc.Details, &exc.DetectedAt,
	)
	if err != nil {
		return nil, ErrExceptionNotFound
	}

	exc.Class = ExceptionClass(classStr)
	exc.Status = ReconStatus(statusStr)
	exc.ExpectedPaise = ledger.AmountPaise(expPaise)
	exc.ActualPaise = ledger.AmountPaise(actPaise)
	return &exc, nil
}

// ResolveException marks an exception resolved with an associated remedy action.
func (r *PostgresRepository) ResolveException(ctx context.Context, id string, remedy string) error {
	if r.pool == nil {
		return ErrExceptionNotFound
	}

	cmd, err := r.pool.Exec(ctx, `
		UPDATE recon_exceptions
		SET status = 'RESOLVED',
		    remedy_action = $2,
		    resolved_at = NOW()
		WHERE exception_id = $1
	`, id, remedy)
	if err != nil {
		return fmt.Errorf("recon_repo: resolve exception failed: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrExceptionNotFound
	}
	return nil
}

// MemoryRepository provides thread-safe in-memory exception persistence.
type MemoryRepository struct {
	mu         sync.RWMutex
	exceptions map[string]ReconException
}

// NewMemoryRepository initializes an in-memory repository for local simulations and tests.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		exceptions: make(map[string]ReconException),
	}
}

func (m *MemoryRepository) SaveExceptions(_ context.Context, exceptions []ReconException) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, exc := range exceptions {
		m.exceptions[exc.ID] = exc
	}
	return nil
}

func (m *MemoryRepository) ListExceptions(_ context.Context, status string) ([]ReconException, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []ReconException
	for _, exc := range m.exceptions {
		if status == "" || status == "ALL" || string(exc.Status) == status {
			list = append(list, exc)
		}
	}
	return list, nil
}

func (m *MemoryRepository) GetException(_ context.Context, id string) (*ReconException, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	exc, ok := m.exceptions[id]
	if !ok {
		return nil, ErrExceptionNotFound
	}
	return &exc, nil
}

func (m *MemoryRepository) ResolveException(_ context.Context, id string, remedy string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	exc, ok := m.exceptions[id]
	if !ok {
		return ErrExceptionNotFound
	}
	exc.Status = ReconStatusResolved
	m.exceptions[id] = exc
	return nil
}

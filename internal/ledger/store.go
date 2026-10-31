package ledger

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrStoreUninitialized = errors.New("ledger_store: store connection uninitialized")
)

// Store defines persistence operations for ledger accounts, transactions, and balances.
type Store interface {
	GetAccount(ctx context.Context, name string) (Account, error)
	GetBalance(ctx context.Context, accountName string) (AmountPaise, error)
	ListAccounts(ctx context.Context) ([]Account, error)
	RegisterAccount(ctx context.Context, name string, accType AccountType, currency string, constrained bool) error
	Post(ctx context.Context, req PostRequest) (*PostResult, error)
}

// SQLStore implements Store backed by PostgreSQL.
type SQLStore struct {
	ledger *Ledger
	pool   *pgxpool.Pool
}

// NewSQLStore creates a new PostgreSQL ledger store.
func NewSQLStore(pool *pgxpool.Pool) *SQLStore {
	return &SQLStore{
		ledger: New(pool),
		pool:   pool,
	}
}

// GetAccount retrieves an account by name from PostgreSQL.
func (s *SQLStore) GetAccount(ctx context.Context, name string) (Account, error) {
	if s.pool == nil {
		return Account{}, ErrAccountNotFound
	}
	var acc Account
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, type, currency, constrained, normal_balance, created_at
		FROM accounts
		WHERE name = $1
	`, name).Scan(&acc.ID, &acc.Name, &acc.Type, &acc.Currency, &acc.Constrained, &acc.NormalBalance, &acc.CreatedAt)
	if err != nil {
		return Account{}, ErrAccountNotFound
	}
	return acc, nil
}

// GetBalance returns the current balance in integer paise for a given account.
func (s *SQLStore) GetBalance(ctx context.Context, accountName string) (AmountPaise, error) {
	if s.ledger == nil {
		return 0, ErrStoreUninitialized
	}
	return s.ledger.GetBalance(ctx, accountName)
}

// ListAccounts returns all registered accounts from PostgreSQL.
func (s *SQLStore) ListAccounts(ctx context.Context) ([]Account, error) {
	if s.pool == nil {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, type, currency, constrained, normal_balance, created_at
		FROM accounts
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("ledger_store: list accounts failed: %w", err)
	}
	defer rows.Close()

	var accounts []Account
	for rows.Next() {
		var acc Account
		if err := rows.Scan(&acc.ID, &acc.Name, &acc.Type, &acc.Currency, &acc.Constrained, &acc.NormalBalance, &acc.CreatedAt); err == nil {
			accounts = append(accounts, acc)
		}
	}
	return accounts, nil
}

// RegisterAccount creates an account in PostgreSQL.
func (s *SQLStore) RegisterAccount(ctx context.Context, name string, accType AccountType, currency string, constrained bool) error {
	if s.ledger == nil {
		return ErrStoreUninitialized
	}
	return s.ledger.RegisterAccount(ctx, name, accType, currency, constrained)
}

// Post atomically records a double-entry transaction into PostgreSQL.
func (s *SQLStore) Post(ctx context.Context, req PostRequest) (*PostResult, error) {
	if s.ledger == nil {
		return nil, ErrStoreUninitialized
	}
	return s.ledger.Post(ctx, req)
}

// MemoryStore implements Store backed by in-memory journal.
type MemoryStore struct {
	mem *MemoryLedger
}

// NewMemoryStore creates a new in-memory ledger store.
func NewMemoryStore(mem *MemoryLedger) *MemoryStore {
	if mem == nil {
		mem = NewMemoryLedger()
	}
	return &MemoryStore{mem: mem}
}

func (m *MemoryStore) GetAccount(ctx context.Context, name string) (Account, error) {
	acc, ok := m.mem.GetAccount(name)
	if !ok {
		return Account{}, ErrAccountNotFound
	}
	return acc, nil
}

func (m *MemoryStore) GetBalance(ctx context.Context, accountName string) (AmountPaise, error) {
	return m.mem.Balance(ctx, accountName)
}

func (m *MemoryStore) ListAccounts(ctx context.Context) ([]Account, error) {
	m.mem.mu.RLock()
	defer m.mem.mu.RUnlock()
	res := make([]Account, 0, len(m.mem.accounts))
	for _, a := range m.mem.accounts {
		res = append(res, a)
	}
	return res, nil
}

func (m *MemoryStore) RegisterAccount(ctx context.Context, name string, accType AccountType, currency string, constrained bool) error {
	_, err := m.mem.RegisterAccount(name, accType, currency, constrained)
	return err
}

func (m *MemoryStore) Post(ctx context.Context, req PostRequest) (*PostResult, error) {
	return m.mem.Post(ctx, req)
}

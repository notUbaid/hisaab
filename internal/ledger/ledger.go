package ledger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBTX is an interface covering pgxpool.Pool and pgx.Tx for database interactions.
type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Ledger provides the PostgreSQL financial double-entry ledger engine.
type Ledger struct {
	pool *pgxpool.Pool
}

// New creates a new Ledger engine instance connected to the given PostgreSQL pool.
func New(pool *pgxpool.Pool) *Ledger {
	return &Ledger{pool: pool}
}

// ValidateLines ensures that a set of posting lines satisfies double-entry constraints.
// It checks line counts, amounts, per-line limits, and prevents 64-bit integer overflow (B5).
func ValidateLines(lines []EntryLine) (AmountPaise, error) {
	if len(lines) < 2 {
		return 0, ErrInsufficientLines
	}
	if len(lines) > 1000 {
		return 0, errors.New("ledger: transaction exceeds maximum allowable lines (1000)")
	}

	var sumDebit AmountPaise
	var sumCredit AmountPaise

	for _, line := range lines {
		if strings.TrimSpace(line.AccountName) == "" {
			return 0, ErrEmptyAccountName
		}
		if line.AmountPaise <= 0 {
			return 0, ErrZeroOrNegativeEntry
		}
		if line.AmountPaise > MaxLineAmountPaise {
			return 0, fmt.Errorf("%w: line amount %d exceeds maximum allowable limit %d",
				ErrAmountOverflow, line.AmountPaise, MaxLineAmountPaise)
		}

		switch line.Direction {
		case Debit:
			if math.MaxInt64-int64(sumDebit) < int64(line.AmountPaise) {
				return 0, fmt.Errorf("%w: debit sum overflows int64", ErrAmountOverflow)
			}
			sumDebit += line.AmountPaise
		case Credit:
			if math.MaxInt64-int64(sumCredit) < int64(line.AmountPaise) {
				return 0, fmt.Errorf("%w: credit sum overflows int64", ErrAmountOverflow)
			}
			sumCredit += line.AmountPaise
		default:
			return 0, fmt.Errorf("ledger: unknown direction %q for account %q", line.Direction, line.AccountName)
		}
	}

	if sumDebit != sumCredit {
		return 0, fmt.Errorf("%w: total debits (%d paise) != total credits (%d paise)",
			ErrUnbalancedTransaction, sumDebit, sumCredit)
	}

	return sumDebit, nil
}

// ComputeRequestHash generates a deterministic SHA-256 hash of the post request content.
// Length prefixes and delimiters are strictly applied to prevent boundary collisions (B5).
func ComputeRequestHash(req PostRequest) string {
	hasher := sha256.New()
	writeField := func(tag string, val string) {
		_, _ = fmt.Fprintf(hasher, "%s:%d:%s|", tag, len(val), val)
	}

	writeField("key", req.IdempotencyKey)
	writeField("cause_type", string(req.CauseType))
	writeField("cause_id", fmt.Sprintf("%d", req.CauseID))
	writeField("desc", req.Description)

	sortedLines := make([]EntryLine, len(req.Lines))
	copy(sortedLines, req.Lines)
	sort.Slice(sortedLines, func(i, j int) bool {
		if sortedLines[i].AccountName == sortedLines[j].AccountName {
			if sortedLines[i].Direction == sortedLines[j].Direction {
				return sortedLines[i].AmountPaise < sortedLines[j].AmountPaise
			}
			return sortedLines[i].Direction < sortedLines[j].Direction
		}
		return sortedLines[i].AccountName < sortedLines[j].AccountName
	})

	for _, l := range sortedLines {
		writeField("acc", l.AccountName)
		writeField("dir", string(l.Direction))
		writeField("amt", fmt.Sprintf("%d", l.AmountPaise))
	}

	return hex.EncodeToString(hasher.Sum(nil))
}

// Post atomically records a balanced double-entry transaction into PostgreSQL.
// It enforces Invariant 1 (zero-sum), Invariant 3 (atomic inside-tx idempotency),
// and Invariant 4 (lock-free non-negative balance check constraint).
func (l *Ledger) Post(ctx context.Context, req PostRequest) (*PostResult, error) {
	if l.pool == nil {
		return nil, errors.New("ledger: postgres pool is nil")
	}

	totalPaise, err := ValidateLines(req.Lines)
	if err != nil {
		return nil, fmt.Errorf("ledger.Post: validation failed: %w", err)
	}

	reqHash := ComputeRequestHash(req)
	if req.RequestHash != "" && req.RequestHash != reqHash {
		return nil, fmt.Errorf("%w: caller supplied request hash mismatch", ErrDuplicateIdempotency)
	}

	// 1. Begin Database Transaction (READ COMMITTED)
	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("ledger.Post: begin tx failed: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	// 2. Insert-First Idempotency Check INSIDE Transaction (Appendix B.2)
	// Concurrent duplicate requests block on the unique index until this transaction commits or rolls back.
	tag, err := tx.Exec(ctx, `
		INSERT INTO idempotency_keys (key, request_hash, status)
		VALUES ($1, $2, 'in_progress')
		ON CONFLICT (key) DO NOTHING
	`, req.IdempotencyKey, reqHash)
	if err != nil {
		return nil, fmt.Errorf("ledger.Post: idempotency insert failed: %w", err)
	}

	if tag.RowsAffected() == 0 {
		// Key was already recorded. Query stored hash and response.
		var storedHash string
		var storedResponse []byte
		var status string
		err = tx.QueryRow(ctx, `
			SELECT request_hash, response, status
			FROM idempotency_keys
			WHERE key = $1
		`, req.IdempotencyKey).Scan(&storedHash, &storedResponse, &status)
		if err != nil {
			return nil, fmt.Errorf("ledger.Post: read existing idempotency key failed: %w", err)
		}

		if storedHash != reqHash {
			return nil, fmt.Errorf("%w: key %q has previous hash %s, new hash %s",
				ErrDuplicateIdempotency, req.IdempotencyKey, storedHash, reqHash)
		}

		if len(storedResponse) > 0 {
			var storedResult PostResult
			if err := json.Unmarshal(storedResponse, &storedResult); err != nil {
				return nil, fmt.Errorf("ledger.Post: unmarshal cached response failed: %w", err)
			}
			return &storedResult, nil
		}

		return nil, fmt.Errorf("ledger.Post: key %q is currently in progress", req.IdempotencyKey)
	}

	// 3. Resolve Accounts and build ID lookup
	accountNames := make([]string, 0, len(req.Lines))
	for _, line := range req.Lines {
		accountNames = append(accountNames, line.AccountName)
	}

	rows, err := tx.Query(ctx, `
		SELECT id, name, type, currency, constrained, created_at
		FROM accounts
		WHERE name = ANY($1)
	`, accountNames)
	if err != nil {
		return nil, fmt.Errorf("ledger.Post: account query failed: %w", err)
	}
	defer rows.Close()

	accountMap := make(map[string]Account)
	accountMapByID := make(map[int64]Account)
	for rows.Next() {
		var acc Account
		if err := rows.Scan(&acc.ID, &acc.Name, &acc.Type, &acc.Currency, &acc.Constrained, &acc.CreatedAt); err != nil {
			return nil, fmt.Errorf("ledger.Post: account scan failed: %w", err)
		}
		accountMap[acc.Name] = acc
		accountMapByID[acc.ID] = acc
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ledger.Post: account rows iteration error: %w", err)
	}

	for _, name := range accountNames {
		if _, ok := accountMap[name]; !ok {
			return nil, fmt.Errorf("%w: %q", ErrAccountNotFound, name)
		}
	}

	// 4. Calculate Net Balance Deltas per Account
	accountDeltas := make(map[int64]int64)
	for _, line := range req.Lines {
		acc := accountMap[line.AccountName]
		if line.Direction == Credit {
			accountDeltas[acc.ID] += int64(line.AmountPaise)
		} else {
			accountDeltas[acc.ID] -= int64(line.AmountPaise)
		}
	}

	// 5. Update Balances in Ascending account_id order (Deadlock-Free Row Locks & CHECK constraint)
	touchedIDs := make([]int64, 0, len(accountDeltas))
	for id := range accountDeltas {
		touchedIDs = append(touchedIDs, id)
	}
	sort.Slice(touchedIDs, func(i, j int) bool {
		return touchedIDs[i] < touchedIDs[j]
	})

	for _, accID := range touchedIDs {
		delta := accountDeltas[accID]
		acc := accountMapByID[accID]

		_, err := tx.Exec(ctx, `
			INSERT INTO account_balances (account_id, constrained, balance_paise)
			VALUES ($1, $2, $3)
			ON CONFLICT (account_id) DO UPDATE
			SET balance_paise = account_balances.balance_paise + EXCLUDED.balance_paise
		`, accID, acc.Constrained, delta)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23514" { // check_violation
				return nil, fmt.Errorf("%w: account %q constrained balance would become negative",
					ErrNegativeBalance, acc.Name)
			}
			return nil, fmt.Errorf("ledger.Post: balance update failed for %s: %w", acc.Name, err)
		}
	}

	// 6. Record Journal Transaction & Entries
	postedAt := req.PostedAt
	if postedAt.IsZero() {
		postedAt = time.Now().UTC()
	}

	var txID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO journal_transactions (idempotency_key, description, cause_type, cause_id, created_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, req.IdempotencyKey, req.Description, string(req.CauseType), req.CauseID, postedAt).Scan(&txID)
	if err != nil {
		return nil, fmt.Errorf("ledger.Post: insert journal_transactions failed: %w", err)
	}

	for _, line := range req.Lines {
		acc := accountMap[line.AccountName]
		_, err := tx.Exec(ctx, `
			INSERT INTO journal_entries (transaction_id, account_id, direction, amount_paise, created_at)
			VALUES ($1, $2, $3, $4, $5)
		`, txID, acc.ID, string(line.Direction), int64(line.AmountPaise), postedAt)
		if err != nil {
			return nil, fmt.Errorf("ledger.Post: insert journal_entries failed: %w", err)
		}
	}

	result := &PostResult{
		TransactionID:  txID,
		IdempotencyKey: req.IdempotencyKey,
		LinesCount:     len(req.Lines),
		TotalPaise:     totalPaise,
		CreatedAt:      postedAt,
	}

	responseBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("ledger.Post: marshal result failed: %w", err)
	}

	// 7. Update Idempotency Keys with cached response
	_, err = tx.Exec(ctx, `
		UPDATE idempotency_keys
		SET status = 'done', response = $1
		WHERE key = $2
	`, responseBytes, req.IdempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("ledger.Post: update idempotency key failed: %w", err)
	}

	// 8. Commit Transaction Atomically
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("ledger.Post: commit failed: %w", err)
	}

	return result, nil
}

// GetBalance returns the current balance in integer paise for a given account name.
func (l *Ledger) GetBalance(ctx context.Context, accountName string) (AmountPaise, error) {
	if l.pool == nil {
		return 0, errors.New("ledger: postgres pool is nil")
	}

	var balance int64
	err := l.pool.QueryRow(ctx, `
		SELECT COALESCE(ab.balance_paise, 0)
		FROM accounts a
		LEFT JOIN account_balances ab ON a.id = ab.account_id
		WHERE a.name = $1
	`, accountName).Scan(&balance)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("%w: %s", ErrAccountNotFound, accountName)
		}
		return 0, fmt.Errorf("ledger.GetBalance: query failed: %w", err)
	}

	return AmountPaise(balance), nil
}

// Balance implements the unified Engine interface for PostgreSQL Ledger.
func (l *Ledger) Balance(ctx context.Context, accountName string) (AmountPaise, error) {
	return l.GetBalance(ctx, accountName)
}

// RegisterAccount creates or registers an account in PostgreSQL chart of accounts.
func (l *Ledger) RegisterAccount(ctx context.Context, name string, accType AccountType, currency string, constrained bool) error {
	if l.pool == nil {
		return errors.New("ledger: postgres pool is nil")
	}

	normalBalance := "credit"
	if accType == AccountAsset || accType == AccountExpense || accType == AccountExternal {
		normalBalance = "debit"
	}

	_, err := l.pool.Exec(ctx, `
		INSERT INTO accounts (name, type, currency, constrained, normal_balance)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (name) DO NOTHING
	`, name, string(accType), currency, constrained, normalBalance)
	return err
}

// SeedStandardAccounts initializes standard ONDC settlement accounts into PostgreSQL.
func (l *Ledger) SeedStandardAccounts(ctx context.Context) error {
	standardAccounts := []struct {
		name        string
		accType     AccountType
		constrained bool
	}{
		{"collector:ondc_settlement", AccountAsset, true},
		{"receiver:ondc_settlement", AccountLiability, true},
		{"receiver:operative", AccountAsset, false},
		{"collector:operative", AccountAsset, false},
		{"ext:buyer_clearing", AccountExternal, false},
		{"ext:nodal_clearing", AccountExternal, false},
		{"fees:ondc_network", AccountIncome, false},
		{"fees:withholding_tax", AccountLiability, false},
		{"sa:suspense", AccountAsset, false},
		{"sa:tolerance_adjustment", AccountAsset, false},
	}

	for _, sa := range standardAccounts {
		currency := "INR"
		if err := l.RegisterAccount(ctx, sa.name, sa.accType, currency, sa.constrained); err != nil {
			return err
		}
	}
	return nil
}

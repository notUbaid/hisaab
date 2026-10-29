package ledger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

// Ledger provides the core financial ledger engine.
type Ledger struct {
	pool *pgxpool.Pool
}

// New creates a new Ledger engine instance connected to the given PostgreSQL pool.
func New(pool *pgxpool.Pool) *Ledger {
	return &Ledger{pool: pool}
}

// ValidateLines ensures that a set of posting lines satisfies the basic double-entry constraints.
func ValidateLines(lines []EntryLine) (AmountPaise, error) {
	if len(lines) < 2 {
		return 0, ErrInsufficientLines
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
		switch line.Direction {
		case Debit:
			sumDebit += line.AmountPaise
		case Credit:
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
func ComputeRequestHash(req PostRequest) string {
	hasher := sha256.New()
	hasher.Write([]byte(req.IdempotencyKey))
	hasher.Write([]byte(string(req.CauseType)))
	hasher.Write([]byte(fmt.Sprintf("%d", req.CauseID)))
	hasher.Write([]byte(req.Description))

	// Sort lines by account name and direction for canonical hashing
	sortedLines := make([]EntryLine, len(req.Lines))
	copy(sortedLines, req.Lines)
	sort.Slice(sortedLines, func(i, j int) bool {
		if sortedLines[i].AccountName == sortedLines[j].AccountName {
			return sortedLines[i].Direction < sortedLines[j].Direction
		}
		return sortedLines[i].AccountName < sortedLines[j].AccountName
	})

	for _, l := range sortedLines {
		hasher.Write([]byte(l.AccountName))
		hasher.Write([]byte(string(l.Direction)))
		hasher.Write([]byte(fmt.Sprintf("%d", l.AmountPaise)))
	}

	return hex.EncodeToString(hasher.Sum(nil))
}

// Post atomically records a balanced double-entry transaction into the journal.
// It guarantees Invariant 1 (zero-sum), Invariant 3 (effectively-once idempotency),
// and Invariant 4 (non-negative constrained balance).
func (l *Ledger) Post(ctx context.Context, req PostRequest) (*PostResult, error) {
	totalPaise, err := ValidateLines(req.Lines)
	if err != nil {
		return nil, fmt.Errorf("ledger.Post: validation failed: %w", err)
	}

	reqHash := req.RequestHash
	if reqHash == "" {
		reqHash = ComputeRequestHash(req)
	}

	// 1. Insert-first Idempotency Check
	var existingStatus string
	var existingHash string
	var existingResponse []byte

	err = l.pool.QueryRow(ctx, `
		INSERT INTO idempotency_keys (key, request_hash, status)
		VALUES ($1, $2, 'in_progress')
		ON CONFLICT (key) DO UPDATE
			SET key = EXCLUDED.key
		RETURNING status, request_hash, response
	`, req.IdempotencyKey, reqHash).Scan(&existingStatus, &existingHash, &existingResponse)
	if err != nil {
		return nil, fmt.Errorf("ledger.Post: idempotency record lookup failed: %w", err)
	}

	// If already completed previously, verify payload hash
	if existingStatus == "done" {
		if existingHash != reqHash {
			return nil, fmt.Errorf("%w: key %q has previous hash %s, new hash %s",
				ErrDuplicateIdempotency, req.IdempotencyKey, existingHash, reqHash)
		}
		var storedResult PostResult
		if err := json.Unmarshal(existingResponse, &storedResult); err != nil {
			return nil, fmt.Errorf("ledger.Post: failed to unmarshal cached response: %w", err)
		}
		return &storedResult, nil
	}

	// 2. Begin Database Transaction
	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("ledger.Post: begin tx failed: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	// 3. Resolve Account IDs and check constraints
	accountMap := make(map[string]Account)
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
		return nil, fmt.Errorf("ledger.Post: account resolution query failed: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var acc Account
		if err := rows.Scan(&acc.ID, &acc.Name, &acc.Type, &acc.Currency, &acc.Constrained, &acc.CreatedAt); err != nil {
			return nil, fmt.Errorf("ledger.Post: account scan failed: %w", err)
		}
		accountMap[acc.Name] = acc
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ledger.Post: account rows iteration error: %w", err)
	}

	// Ensure all accounts exist
	for _, name := range accountNames {
		if _, ok := accountMap[name]; !ok {
			return nil, fmt.Errorf("%w: %q", ErrAccountNotFound, name)
		}
	}

	// 4. Lock constrained accounts in ascending ID order to avoid deadlocks
	constrainedAccounts := make([]Account, 0)
	for _, acc := range accountMap {
		if acc.Constrained {
			constrainedAccounts = append(constrainedAccounts, acc)
		}
	}
	sort.Slice(constrainedAccounts, func(i, j int) bool {
		return constrainedAccounts[i].ID < constrainedAccounts[j].ID
	})

	for _, acc := range constrainedAccounts {
		var currentBalance int64
		err := tx.QueryRow(ctx, `
			SELECT COALESCE(SUM(
				CASE WHEN direction = 'credit' THEN amount_paise
				     WHEN direction = 'debit'  THEN -amount_paise
				     ELSE 0 END
			), 0)
			FROM journal_entries
			WHERE account_id = $1
		`, acc.ID).Scan(&currentBalance)
		if err != nil {
			return nil, fmt.Errorf("ledger.Post: balance check query failed for %s: %w", acc.Name, err)
		}

		// Calculate net delta for this account in this transaction
		var delta int64
		for _, line := range req.Lines {
			if line.AccountName == acc.Name {
				if line.Direction == Credit {
					delta += int64(line.AmountPaise)
				} else {
					delta -= int64(line.AmountPaise)
				}
			}
		}

		if currentBalance+delta < 0 {
			return nil, fmt.Errorf("%w: account %q current balance %d paise, proposed delta %d paise",
				ErrNegativeBalance, acc.Name, currentBalance, delta)
		}
	}

	// 5. Insert journal transaction
	var txID int64
	var createdAt time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO journal_transactions (idempotency_key, description, cause_type, cause_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at
	`, req.IdempotencyKey, req.Description, string(req.CauseType), req.CauseID).Scan(&txID, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("ledger.Post: insert journal_transactions failed: %w", err)
	}

	// 6. Insert journal entries
	for _, line := range req.Lines {
		acc := accountMap[line.AccountName]
		_, err := tx.Exec(ctx, `
			INSERT INTO journal_entries (transaction_id, account_id, direction, amount_paise, created_at)
			VALUES ($1, $2, $3, $4, $5)
		`, txID, acc.ID, string(line.Direction), int64(line.AmountPaise), createdAt)
		if err != nil {
			return nil, fmt.Errorf("ledger.Post: insert journal_entries failed for account %s: %w", line.AccountName, err)
		}
	}

	result := &PostResult{
		TransactionID:  txID,
		IdempotencyKey: req.IdempotencyKey,
		LinesCount:     len(req.Lines),
		TotalPaise:     totalPaise,
		CreatedAt:      createdAt,
	}

	responseBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("ledger.Post: marshal result failed: %w", err)
	}

	// 7. Update idempotency_keys status to 'done' with stored result
	_, err = tx.Exec(ctx, `
		UPDATE idempotency_keys
		SET status = 'done', response = $1
		WHERE key = $2
	`, responseBytes, req.IdempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("ledger.Post: update idempotency_keys failed: %w", err)
	}

	// 8. Commit
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("ledger.Post: transaction commit failed: %w", err)
	}

	return result, nil
}

package ledger

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// MemoryLedger provides a standalone, thread-safe financial double-entry ledger.
// It supports zero-dependency local execution, fast automated property tests, and
// strict mathematical enforcement of Invariants 1, 3, and 4 without requiring an external database.
type MemoryLedger struct {
	mu            sync.RWMutex
	accounts      map[string]Account
	accountByID   map[int64]Account
	nextAccountID int64
	nextTxID      int64
	nextEntryID   int64

	transactions []JournalTransaction
	entries      []JournalEntry
	idempotency  map[string]idempotencyItem
}

type idempotencyItem struct {
	Key         string
	RequestHash string
	Status      string
	Response    []byte
	CreatedAt   time.Time
}

// NewMemoryLedger creates and initializes a new thread-safe in-memory ledger instance.
func NewMemoryLedger() *MemoryLedger {
	return &MemoryLedger{
		accounts:      make(map[string]Account),
		accountByID:   make(map[int64]Account),
		nextAccountID: 1,
		nextTxID:      1,
		nextEntryID:   1,
		idempotency:   make(map[string]idempotencyItem),
	}
}

// RegisterAccount creates or updates an account in the ledger's chart of accounts.
func (m *MemoryLedger) RegisterAccount(name string, accType AccountType, currency string, constrained bool) (Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if name == "" {
		return Account{}, ErrEmptyAccountName
	}

	if existing, ok := m.accounts[name]; ok {
		return existing, nil
	}

	acc := Account{
		ID:            m.nextAccountID,
		Name:          name,
		Type:          accType,
		Currency:      currency,
		Constrained:   constrained,
		NormalBalance: accType.NormalBalance(),
		CreatedAt:     time.Now().UTC(),
	}
	m.nextAccountID++

	m.accounts[name] = acc
	m.accountByID[acc.ID] = acc
	return acc, nil
}

// SeedStandardAccounts initializes the standard ONDC RSF chart of accounts:
// - collector:ondc_settlement (constrained)
// - receiver:ondc_settlement (constrained)
// - receiver:operative (unconstrained)
// - collector:operative (unconstrained)
// - ext:buyer_clearing (unconstrained)
// - ext:nodal_clearing (unconstrained)
func (m *MemoryLedger) SeedStandardAccounts() error {
	standardAccounts := []struct {
		name        string
		accType     AccountType
		constrained bool
	}{
		{"collector:ondc_settlement", AccountLiability, true}, // Fiduciary settlement liability pool
		{"receiver:ondc_settlement", AccountLiability, true},
		{"receiver:operative", AccountAsset, false},
		{"collector:operative", AccountAsset, false},
		{"ext:buyer_clearing", AccountExternal, false},
		{"ext:nodal_clearing", AccountExternal, false},
		{"fees:ondc_network", AccountIncome, false},
		{"fees:withholding_tax", AccountLiability, false},
		{"sa:suspense", AccountLiability, false},
		{"sa:tolerance_adjustment", AccountExpense, false},
	}

	for _, sa := range standardAccounts {
		currency := "INR"
		if _, err := m.RegisterAccount(sa.name, sa.accType, currency, sa.constrained); err != nil {
			return err
		}
	}
	return nil
}

// GetAccount retrieves an account by name.
func (m *MemoryLedger) GetAccount(name string) (Account, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	acc, ok := m.accounts[name]
	return acc, ok
}

// GetBalance calculates the net balance for a given account in integer paise.
func (m *MemoryLedger) GetBalance(accountName string) (AmountPaise, error) {
	return m.Balance(context.Background(), accountName)
}

// Balance implements the unified Engine interface for MemoryLedger.
func (m *MemoryLedger) Balance(ctx context.Context, accountName string) (AmountPaise, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	acc, ok := m.accounts[accountName]
	if !ok {
		return 0, fmt.Errorf("%w: %q", ErrAccountNotFound, accountName)
	}

	var balance AmountPaise
	for _, entry := range m.entries {
		if entry.AccountID == acc.ID {
			balance += acc.CalculateDelta(entry.Direction, entry.AmountPaise)
		}
	}

	return balance, nil
}

// Post atomically records a balanced double-entry transaction into the in-memory journal.
// It verifies Invariant 1 (zero-sum line balance), Invariant 3 (idempotency key and payload hash matching),
// and Invariant 4 (non-negative balance for constrained accounts).
func (m *MemoryLedger) Post(ctx context.Context, req PostRequest) (*PostResult, error) {
	// 1. Line validation
	totalPaise, err := ValidateLines(req.Lines)
	if err != nil {
		return nil, fmt.Errorf("ledger.MemoryLedger.Post: validation failed: %w", err)
	}

	// Compute canonical request hash internally (B5)
	reqHash := ComputeRequestHash(req)
	if req.RequestHash != "" && req.RequestHash != reqHash {
		return nil, fmt.Errorf("%w: caller supplied hash mismatch", ErrDuplicateIdempotency)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 2. Check context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// 3. Idempotency Check
	if item, exists := m.idempotency[req.IdempotencyKey]; exists {
		if item.Status == "done" {
			if item.RequestHash != reqHash {
				return nil, fmt.Errorf("%w: key %q has previous hash %s, new hash %s",
					ErrDuplicateIdempotency, req.IdempotencyKey, item.RequestHash, reqHash)
			}
			var storedResult PostResult
			if err := json.Unmarshal(item.Response, &storedResult); err != nil {
				return nil, fmt.Errorf("ledger.MemoryLedger.Post: unmarshal cached response: %w", err)
			}
			return &storedResult, nil
		}
	}

	// 4. Resolve and verify accounts
	accountMap := make(map[string]Account, len(req.Lines))
	for _, line := range req.Lines {
		acc, ok := m.accounts[line.AccountName]
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrAccountNotFound, line.AccountName)
		}
		accountMap[line.AccountName] = acc
	}

	// 5. Check constrained account balance limits (Invariant 4)
	for _, acc := range accountMap {
		if acc.Constrained {
			var currentBalance AmountPaise
			for _, entry := range m.entries {
				if entry.AccountID == acc.ID {
					currentBalance += acc.CalculateDelta(entry.Direction, entry.AmountPaise)
				}
			}

			// Calculate net delta in this transaction
			var delta AmountPaise
			for _, line := range req.Lines {
				if line.AccountName == acc.Name {
					delta += acc.CalculateDelta(line.Direction, line.AmountPaise)
				}
			}

			if currentBalance+delta < 0 {
				return nil, fmt.Errorf("%w: account %q current balance %d paise, proposed delta %d paise",
					ErrNegativeBalance, acc.Name, currentBalance, delta)
			}
		}
	}

	// 6. Record transaction
	txID := m.nextTxID
	m.nextTxID++

	postedAt := req.PostedAt
	if postedAt.IsZero() {
		postedAt = time.Now().UTC()
	}

	tx := JournalTransaction{
		ID:             txID,
		IdempotencyKey: req.IdempotencyKey,
		Description:    req.Description,
		CauseType:      req.CauseType,
		CauseID:        req.CauseID,
		CreatedAt:      postedAt,
	}
	m.transactions = append(m.transactions, tx)

	// Failpoint hook: before recording journal entry lines (simulating mid-tx interruption)
	if err := TriggerFailpoint(ctx, "after_debit_before_credit"); err != nil {
		return nil, err
	}

	// 7. Record entries
	for _, line := range req.Lines {
		entryID := m.nextEntryID
		m.nextEntryID++

		acc := accountMap[line.AccountName]
		entry := JournalEntry{
			ID:            entryID,
			TransactionID: txID,
			AccountID:     acc.ID,
			AccountName:   acc.Name,
			Direction:     line.Direction,
			AmountPaise:   line.AmountPaise,
			CreatedAt:     postedAt,
		}
		m.entries = append(m.entries, entry)
	}

	// 8. Cache idempotency result
	result := &PostResult{
		TransactionID:  txID,
		IdempotencyKey: req.IdempotencyKey,
		LinesCount:     len(req.Lines),
		TotalPaise:     totalPaise,
		CreatedAt:      postedAt,
	}

	respBytes, err := json.Marshal(result)
	if err == nil {
		m.idempotency[req.IdempotencyKey] = idempotencyItem{
			Key:         req.IdempotencyKey,
			RequestHash: reqHash,
			Status:      "done",
			Response:    respBytes,
			CreatedAt:   postedAt,
		}
	}

	// Failpoint hook: after commit before returning ACK to caller
	if err := TriggerFailpoint(ctx, "after_commit_before_ack"); err != nil {
		return nil, err
	}

	return result, nil
}

// GetTransactions returns a copy of all recorded transactions.
func (m *MemoryLedger) GetTransactions() []JournalTransaction {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]JournalTransaction, len(m.transactions))
	copy(res, m.transactions)
	return res
}

// GetEntries returns a copy of all journal entries.
func (m *MemoryLedger) GetEntries() []JournalEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]JournalEntry, len(m.entries))
	copy(res, m.entries)
	return res
}

// AuditInvariants performs an end-to-end mathematical verification of the in-memory ledger state.
// Invariant 1: Total debits == Total credits across all entries.
// Invariant 4: No constrained account has a negative balance.
func (m *MemoryLedger) AuditInvariants() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var sumDebits AmountPaise
	var sumCredits AmountPaise
	accountBalances := make(map[int64]AmountPaise)

	for _, entry := range m.entries {
		if entry.Direction == Debit {
			sumDebits += entry.AmountPaise
			accountBalances[entry.AccountID] -= entry.AmountPaise
		} else if entry.Direction == Credit {
			sumCredits += entry.AmountPaise
			accountBalances[entry.AccountID] += entry.AmountPaise
		}
	}

	if sumDebits != sumCredits {
		return fmt.Errorf("%w: global debit sum (%d) != global credit sum (%d)",
			ErrUnbalancedTransaction, sumDebits, sumCredits)
	}

	for id, balance := range accountBalances {
		acc := m.accountByID[id]
		if acc.Constrained && balance < 0 {
			return fmt.Errorf("%w: constrained account %q has negative balance %d paise",
				ErrNegativeBalance, acc.Name, balance)
		}
	}

	return nil
}

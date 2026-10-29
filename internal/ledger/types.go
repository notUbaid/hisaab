package ledger

import (
	"errors"
	"time"
)

// AmountPaise represents a monetary value in Indian Paise (1/100th of an Indian Rupee).
// Floats are strictly prohibited for monetary calculations.
type AmountPaise int64

// Direction specifies whether a journal entry is a debit or a credit.
type Direction string

const (
	Debit  Direction = "debit"
	Credit Direction = "credit"
)

// CauseType indicates the operational event triggering a journal transaction.
type CauseType string

const (
	CauseClaim       CauseType = "claim"
	CauseInstruction CauseType = "instruction"
	CauseBankMessage CauseType = "bank_message"
	CauseAdjustment  CauseType = "adjustment"
)

// AccountType classifies the nature of an account in the double-entry chart of accounts.
type AccountType string

const (
	AccountAsset     AccountType = "asset"
	AccountLiability AccountType = "liability"
	AccountEquity    AccountType = "equity"
	AccountIncome    AccountType = "income"
	AccountExpense   AccountType = "expense"
	AccountExternal  AccountType = "external"
)

var (
	ErrUnbalancedTransaction = errors.New("ledger: transaction debits and credits do not balance to zero")
	ErrZeroOrNegativeEntry   = errors.New("ledger: entry amount must be strictly greater than zero paise")
	ErrInsufficientLines     = errors.New("ledger: double-entry posting requires at least two lines")
	ErrNegativeBalance       = errors.New("ledger: transaction would cause negative balance on constrained account")
	ErrDuplicateIdempotency  = errors.New("ledger: idempotency key already processed with different payload")
	ErrAccountNotFound       = errors.New("ledger: specified account name does not exist")
	ErrEmptyAccountName      = errors.New("ledger: account name cannot be empty")
)

// Account represents an entity account record.
type Account struct {
	ID          int64       `json:"id"`
	Name        string      `json:"name"`
	Type        AccountType `json:"type"`
	Currency    string      `json:"currency"`
	Constrained bool        `json:"constrained"`
	CreatedAt   time.Time   `json:"created_at"`
}

// EntryLine represents an individual posting leg in a proposed transaction.
type EntryLine struct {
	AccountName string      `json:"account_name"`
	Direction   Direction   `json:"direction"`
	AmountPaise AmountPaise `json:"amount_paise"`
}

// PostRequest represents an atomic double-entry journal posting request.
type PostRequest struct {
	IdempotencyKey string      `json:"idempotency_key"`
	RequestHash    string      `json:"request_hash"`
	CauseType      CauseType   `json:"cause_type"`
	CauseID        int64       `json:"cause_id"`
	Description    string      `json:"description"`
	Lines          []EntryLine `json:"lines"`
	PostedAt       time.Time   `json:"posted_at"`
}

// PostResult summarizes the persisted transaction details.
type PostResult struct {
	TransactionID  int64       `json:"transaction_id"`
	IdempotencyKey string      `json:"idempotency_key"`
	LinesCount     int         `json:"lines_count"`
	TotalPaise     AmountPaise `json:"total_paise"`
	CreatedAt      time.Time   `json:"created_at"`
}

// JournalTransaction represents a persisted atomic financial posting event.
type JournalTransaction struct {
	ID             int64       `json:"id"`
	IdempotencyKey string      `json:"idempotency_key"`
	Description    string      `json:"description"`
	CauseType      CauseType   `json:"cause_type"`
	CauseID        int64       `json:"cause_id"`
	CreatedAt      time.Time   `json:"created_at"`
}

// JournalEntry represents an individual posted debit or credit leg.
type JournalEntry struct {
	ID            int64       `json:"id"`
	TransactionID int64       `json:"transaction_id"`
	AccountID     int64       `json:"account_id"`
	AccountName   string      `json:"account_name"`
	Direction     Direction   `json:"direction"`
	AmountPaise   AmountPaise `json:"amount_paise"`
	CreatedAt     time.Time   `json:"created_at"`
}

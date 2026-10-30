package ledger

import (
	"context"
	"errors"
	"time"
)

// Direction represents debit or credit double-entry leg.
type Direction string

const (
	Debit  Direction = "debit"
	Credit Direction = "credit"
)

// AccountType models the normal balance convention in financial ledgers.
type AccountType string

const (
	AccountAsset     AccountType = "asset"
	AccountLiability AccountType = "liability"
	AccountEquity    AccountType = "equity"
	AccountIncome    AccountType = "income"
	AccountExpense   AccountType = "expense"
	AccountExternal  AccountType = "external"
)

// NormalBalance returns whether the account is normally Debit or Credit balanced.
func (t AccountType) NormalBalance() Direction {
	switch t {
	case AccountAsset, AccountExpense:
		return Debit
	default:
		return Credit
	}
}

// AmountPaise represents exact currency values in paise (1 INR = 100 paise).
type AmountPaise int64

// MaxLineAmountPaise caps individual entry amounts at 10^13 paise (100 billion INR / 10,000 crore)
// to prevent integer overflow during aggregate additions.
const MaxLineAmountPaise AmountPaise = 10_000_000_000_000

var (
	ErrUnbalancedTransaction = errors.New("ledger: transaction debits and credits do not balance to zero")
	ErrZeroOrNegativeEntry   = errors.New("ledger: entry amount must be strictly greater than zero paise")
	ErrInsufficientLines     = errors.New("ledger: double-entry posting requires at least two lines")
	ErrNegativeBalance       = errors.New("ledger: transaction would cause negative balance on constrained account")
	ErrDuplicateIdempotency  = errors.New("ledger: idempotency key already processed with different payload")
	ErrAccountNotFound       = errors.New("ledger: specified account name does not exist")
	ErrEmptyAccountName      = errors.New("ledger: account name cannot be empty")
	ErrAmountOverflow        = errors.New("ledger: amount exceeds maximum allowed value (10^13 paise)")
)

// CauseType specifies why a ledger transaction occurred.
type CauseType string

const (
	CauseClaim       CauseType = "claim"
	CauseInstruction CauseType = "instruction"
	CauseRefund      CauseType = "refund"
	CauseAdjustment  CauseType = "adjustment"
)

// Account represents a registered chart of accounts node.
type Account struct {
	ID            int64       `json:"id"`
	Name          string      `json:"name"`
	Type          AccountType `json:"type"`
	Currency      string      `json:"currency"`
	Constrained   bool        `json:"constrained"`
	NormalBalance Direction   `json:"normal_balance"`
	CreatedAt     time.Time   `json:"created_at"`
}

// EffectiveNormalBalance returns the normal balance direction (Debit or Credit) for this account.
func (a Account) EffectiveNormalBalance() Direction {
	if a.NormalBalance != "" {
		return a.NormalBalance
	}
	return a.Type.NormalBalance()
}

// CalculateDelta computes the signed balance delta for an entry line based on double-entry accounting rules:
// - Debit-normal accounts (Assets, Expenses): Debit increases balance (+), Credit decreases balance (-).
// - Credit-normal accounts (Liabilities, Equity, Income): Credit increases balance (+), Debit decreases balance (-).
func (a Account) CalculateDelta(dir Direction, amt AmountPaise) AmountPaise {
	if a.EffectiveNormalBalance() == Debit {
		if dir == Debit {
			return amt
		}
		return -amt
	}
	if dir == Credit {
		return amt
	}
	return -amt
}

// EntryLine specifies an individual debit or credit leg for a transaction.
type EntryLine struct {
	AccountName string      `json:"account_name"`
	Direction   Direction   `json:"direction"`
	AmountPaise AmountPaise `json:"amount_paise"`
}

// PostRequest represents an inbound double-entry journal posting request.
type PostRequest struct {
	IdempotencyKey string      `json:"idempotency_key"`
	RequestHash    string      `json:"request_hash,omitempty"`
	CauseType      CauseType   `json:"cause_type"`
	CauseID        int64       `json:"cause_id"`
	Description    string      `json:"description"`
	Lines          []EntryLine `json:"lines"`
	PostedAt       time.Time   `json:"posted_at"`
}

// PostResult models the successful outcome of committing a journal transaction.
type PostResult struct {
	TransactionID  int64       `json:"transaction_id"`
	IdempotencyKey string      `json:"idempotency_key"`
	LinesCount     int         `json:"lines_count"`
	TotalPaise     AmountPaise `json:"total_paise"`
	CreatedAt      time.Time   `json:"created_at"`
}

// Engine unifies the interface between MemoryLedger and PostgreSQL Ledger.
type Engine interface {
	Post(ctx context.Context, req PostRequest) (*PostResult, error)
	Balance(ctx context.Context, accountName string) (AmountPaise, error)
}

// JournalTransaction represents an immutable group of balanced entry legs.
type JournalTransaction struct {
	ID             int64     `json:"id"`
	IdempotencyKey string    `json:"idempotency_key"`
	Description    string    `json:"description"`
	CauseType      CauseType `json:"cause_type"`
	CauseID        int64     `json:"cause_id"`
	CreatedAt      time.Time `json:"created_at"`
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

// FailpointTrigger abstracts fault injection hooks during ledger execution.
type FailpointTrigger interface {
	Trigger(name string) error
}

type failpointKey struct{}

// ContextWithFailpoint attaches a FailpointTrigger to ctx.
func ContextWithFailpoint(ctx context.Context, fp FailpointTrigger) context.Context {
	return context.WithValue(ctx, failpointKey{}, fp)
}

// TriggerFailpoint executes a named failpoint if attached to ctx.
func TriggerFailpoint(ctx context.Context, name string) error {
	if ctx == nil {
		return nil
	}
	if fp, ok := ctx.Value(failpointKey{}).(FailpointTrigger); ok && fp != nil {
		return fp.Trigger(name)
	}
	return nil
}

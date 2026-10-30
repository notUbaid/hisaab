package recon

import (
	"errors"
	"time"

	"github.com/ubaid/hisaab/internal/ledger"
)

// ExceptionClass categorizes reconciliation breaks into an operational taxonomy.
type ExceptionClass string

const (
	ExceptionMissingCounterpart  ExceptionClass = "MISSING_COUNTERPART"
	ExceptionAmountMismatch      ExceptionClass = "AMOUNT_MISMATCH"
	ExceptionComponentBreak      ExceptionClass = "COMPONENT_BREAK"
	ExceptionInvalidSchema       ExceptionClass = "INVALID_SCHEMA"
	ExceptionInsufficientBalance ExceptionClass = "INSUFFICIENT_BALANCE"
	ExceptionBankFailure         ExceptionClass = "BANK_FAILURE"
	ExceptionBankTimeout         ExceptionClass = "BANK_TIMEOUT"
	ExceptionDuplicateCredit     ExceptionClass = "DUPLICATE_CREDIT"
	ExceptionTimingBreak         ExceptionClass = "TIMING_BREAK"
	ExceptionUnclaimedBankCredit ExceptionClass = "UNCLAIMED_BANK_CREDIT"
	ExceptionOrphanOrder         ExceptionClass = "ORPHAN_ORDER"
)

var (
	ErrSelfApproval       = errors.New("recon: maker-checker violation: approver cannot be the same as proposer")
	ErrExceptionNotFound  = errors.New("recon: exception record not found")
	ErrProposalNotPending = errors.New("recon: adjustment proposal is not in pending state")
)

// OrderBookLine is the internal record of an order in an NP's trade ledger.
type OrderBookLine struct {
	NetworkOrderID string             `json:"network_order_id"`
	AmountPaise    ledger.AmountPaise `json:"amount_paise"`
	Commission     ledger.AmountPaise `json:"commission_paise"`
	BFF            ledger.AmountPaise `json:"bff_paise"`
	CreatedAt      time.Time          `json:"created_at"`
}

// SAReportLine is an entry in the Settlement Agency's batch settlement summary.
type SAReportLine struct {
	BatchID        string             `json:"batch_id"`
	NetworkOrderID string             `json:"network_order_id"`
	SettledAmount  ledger.AmountPaise `json:"settled_amount_paise"`
	Status         string             `json:"status"` // "settled", "rejected", "unmatched"
	SettledAt      time.Time          `json:"settled_at"`
}

// BankStatementLine is a verified entry from an NP's bank account statement.
type BankStatementLine struct {
	ReferenceID string             `json:"reference_id"`
	UTR         string             `json:"utr"`
	Direction   ledger.Direction   `json:"direction"`
	AmountPaise ledger.AmountPaise `json:"amount_paise"`
	ValueDate   time.Time          `json:"value_date"`
}

// ReconStatus represents the resolution state of an identified break.
type ReconStatus string

const (
	ReconStatusOpen     ReconStatus = "OPEN"
	ReconStatusPending  ReconStatus = "PENDING_APPROVAL"
	ReconStatusResolved ReconStatus = "RESOLVED"
)

// ReconException represents a single detected break across the three data sources.
type ReconException struct {
	ID             string             `json:"id"`
	Class          ExceptionClass     `json:"class"`
	NetworkOrderID string             `json:"network_order_id,omitempty"`
	ExpectedPaise  ledger.AmountPaise `json:"expected_paise"`
	ActualPaise    ledger.AmountPaise `json:"actual_paise"`
	AgeingDays     int                `json:"ageing_days"`
	Status         ReconStatus        `json:"status"`
	Details        string             `json:"details"`
	DetectedAt     time.Time          `json:"detected_at"`
}

// ThreeWayReconResult aggregates the outcomes of a three-way reconciliation pass.
type ThreeWayReconResult struct {
	TotalChecked      int                    `json:"total_checked"`
	MatchedCount      int                    `json:"matched_count"`
	TotalExceptions   int                    `json:"total_exceptions"`
	Exceptions        []ReconException       `json:"exceptions"`
	ExceptionsByClass map[ExceptionClass]int `json:"exceptions_by_class"`
	ReconciledAt      time.Time              `json:"reconciled_at"`
}

// AdjustmentProposal represents a maker-checker proposed ledger correction.
type AdjustmentProposal struct {
	ProposalID  string             `json:"proposal_id"`
	ExceptionID string             `json:"exception_id"`
	ProposerID  string             `json:"proposer_id"`
	ApproverID  string             `json:"approver_id,omitempty"`
	Status      string             `json:"status"` // "pending", "approved", "rejected"
	Lines       []ledger.EntryLine `json:"lines"`
	Reason      string             `json:"reason"`
	ProposedAt  time.Time          `json:"proposed_at"`
	ApprovedAt  time.Time          `json:"approved_at,omitempty"`
}

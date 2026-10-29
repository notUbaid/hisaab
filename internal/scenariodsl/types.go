package scenariodsl

import (
	"time"

	"github.com/ubaid/hisaab/internal/ledger"
	"github.com/ubaid/hisaab/internal/settlement"
)

// ScenarioDef is the declarative YAML specification for a settlement scenario.
type ScenarioDef struct {
	ID          string   `yaml:"id"`
	Title       string   `yaml:"title"`
	SpecVersion string   `yaml:"spec_version"`
	BRDSection  string   `yaml:"brd_section"`
	Given       GivenDef `yaml:"given"`
	Then        ThenDef  `yaml:"then"`
}

// GivenDef defines initial state, input claims, and policy parameters.
type GivenDef struct {
	CollectorAccount      string            `yaml:"collector_account"`
	CollectorBalancePaise int64             `yaml:"collector_balance_paise"`
	NPPair                string            `yaml:"np_pair"`
	SettlementDate        string            `yaml:"settlement_date"`
	BuyerClaim            *ClaimDef         `yaml:"buyer_claim,omitempty"`
	SellerClaim           *ClaimDef         `yaml:"seller_claim,omitempty"`
	PolicyFlags           map[string]any    `yaml:"policy_flags,omitempty"`
}

// ClaimDef represents a party's /settle claim submission.
type ClaimDef struct {
	MessageID string     `yaml:"message_id"`
	Orders    []OrderDef `yaml:"orders"`
}

// OrderDef represents an individual order in a claim.
type OrderDef struct {
	OrderID         string `yaml:"order_id"`
	AmountPaise     int64  `yaml:"amount_paise"`
	CommissionPaise int64  `yaml:"commission_paise"`
	BFFPaise        int64  `yaml:"bff_paise"`
}

// ThenDef defines expected outcomes and invariant verification assertions.
type ThenDef struct {
	MatchStatus                string   `yaml:"match_status"`
	ErrorCode                  string   `yaml:"error_code,omitempty"`
	MatchedOrdersCount         int      `yaml:"matched_orders_count"`
	UnmatchedOrdersCount       int      `yaml:"unmatched_orders_count"`
	NetPayoutPaise             int64    `yaml:"net_payout_paise"`
	CollectorFinalBalancePaise int64    `yaml:"collector_final_balance_paise"`
	InvariantsChecked          []string `yaml:"invariants_checked"`
}

// ScenarioResult captures the complete execution trace, outcome, and invariant audit.
type ScenarioResult struct {
	ScenarioID         string                            `json:"scenario_id"`
	Title              string                            `json:"title"`
	Passed             bool                              `json:"passed"`
	ExecutedAt         time.Time                         `json:"executed_at"`
	BatchResult        *settlement.BatchSettlementResult `json:"batch_result,omitempty"`
	InitialBalance     ledger.AmountPaise                `json:"initial_balance"`
	FinalBalance       ledger.AmountPaise                `json:"final_balance"`
	NetPayoutPaise     ledger.AmountPaise                `json:"net_payout_paise"`
	JournalPostings    []ledger.JournalEntry             `json:"journal_postings"`
	InvariantsVerified []string                          `json:"invariants_verified"`
	AuditLog           []string                          `json:"audit_log"`
	ErrorMessage       string                            `json:"error_message,omitempty"`
}

package settlement

import (
	"time"

	"github.com/ubaid/hisaab/internal/ledger"
)

// BecknAckStatus represents the synchronous acknowledgement status.
type BecknAckStatus string

const (
	AckStatusACK  BecknAckStatus = "ACK"
	AckStatusNACK BecknAckStatus = "NACK"
)

// BecknContext represents standard Beckn protocol header metadata.
type BecknContext struct {
	Domain        string    `json:"domain"`
	Country       string    `json:"country"`
	City          string    `json:"city"`
	Action        string    `json:"action"` // e.g. "settle", "on_settle"
	CoreVersion   string    `json:"core_version"`
	BapID         string    `json:"bap_id"`
	BapURI        string    `json:"bap_uri"`
	BppID         string    `json:"bpp_id"`
	BppURI        string    `json:"bpp_uri"`
	TransactionID string    `json:"transaction_id"`
	MessageID     string    `json:"message_id"`
	Timestamp     time.Time `json:"timestamp"`
	TTL           string    `json:"ttl"` // ISO 8601 duration e.g. "PT30S"
}

// BecknError represents error details returned in NACK or callback payloads.
type BecknError struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

// AckResponse is the immediate synchronous HTTP response returned by an NP or SA.
type AckResponse struct {
	Context *BecknContext `json:"context"`
	Message struct {
		Ack struct {
			Status BecknAckStatus `json:"status"`
		} `json:"ack"`
	} `json:"message"`
	Error *BecknError `json:"error,omitempty"`
}

// OrderSettlementLine represents an individual order claim submitted within /settle.
type OrderSettlementLine struct {
	NetworkOrderID    string             `json:"network_order_id"`
	OrderAmountPaise  ledger.AmountPaise `json:"order_amount_paise"`
	CommissionPaise   ledger.AmountPaise `json:"commission_paise"`
	BFFPaise          ledger.AmountPaise `json:"bff_paise"`
	SoRDuePaise       ledger.AmountPaise `json:"sor_due_paise"`
	TaxDeductions     ledger.AmountPaise `json:"tax_deductions_paise"`
	ToleranceResidual ledger.AmountPaise `json:"tolerance_residual_paise,omitempty"`
	SettlementDate    time.Time          `json:"settlement_date"`
}

// SettleClaimPayload represents the claim payload submitted by a Buyer App or Seller App.
type SettleClaimPayload struct {
	Context      *BecknContext         `json:"context"`
	SettlementID string                `json:"settlement_id"`
	NPPairID     string                `json:"np_pair_id"`
	SubmittedBy  string                `json:"submitted_by"` // "buyer_app" | "seller_app"
	CollectorAcc string                `json:"collector_account"`
	ReceiverAcc  string                `json:"receiver_account"`
	Orders       []OrderSettlementLine `json:"orders"`
}

// MatchStatus describes the outcome of matching a network order across claims.
type MatchStatus string

const (
	MatchStatusMatched        MatchStatus = "matched"
	MatchStatusUnmatched      MatchStatus = "unmatched"
	MatchStatusAmountMismatch MatchStatus = "amount_mismatch"
	MatchStatusRejected       MatchStatus = "rejected"
)

// MatchedOrder represents an individual order successfully reconciled between both parties.
type MatchedOrder struct {
	NetworkOrderID   string             `json:"network_order_id"`
	OrderAmountPaise ledger.AmountPaise `json:"order_amount_paise"`
	CommissionPaise  ledger.AmountPaise `json:"commission_paise"`
	BFFPaise         ledger.AmountPaise `json:"bff_paise"`
	SoRDuePaise      ledger.AmountPaise `json:"sor_due_paise"`
	Status           MatchStatus        `json:"status"`
	Reason           string             `json:"reason,omitempty"`
}

// NetInstruction represents an aggregated bank transfer instruction for an NP pair.
type NetInstruction struct {
	FromAccount string             `json:"from_account"`
	ToAccount   string             `json:"to_account"`
	AmountPaise ledger.AmountPaise `json:"amount_paise"`
	BatchID     string             `json:"batch_id"`
}

// BatchSettlementResult is the complete outcome of executing matching and balance checking.
type BatchSettlementResult struct {
	BatchID                string             `json:"batch_id"`
	NPPairID               string             `json:"np_pair_id"`
	SettlementDate         time.Time          `json:"settlement_date"`
	TotalMatched           ledger.AmountPaise `json:"total_matched_paise"`
	TotalCommission        ledger.AmountPaise `json:"total_commission_paise"`
	TotalSoRDue            ledger.AmountPaise `json:"total_sor_due_paise"`
	TotalBFF               ledger.AmountPaise `json:"total_bff_paise"`
	ToleranceResidualPaise ledger.AmountPaise `json:"tolerance_residual_paise,omitempty"`
	MatchedOrders          []MatchedOrder     `json:"matched_orders"`
	UnmatchedOrders        []MatchedOrder     `json:"unmatched_orders"`
	Instructions           []NetInstruction   `json:"instructions"`
	BalancePassed          bool               `json:"balance_passed"`
	AvailableBalance       ledger.AmountPaise `json:"available_balance_paise"`
	ErrorCode              string             `json:"error_code,omitempty"`
}

// OnSettleCallbackPayload models the inbound /on_settle Beckn callback structure.
type OnSettleCallbackPayload struct {
	BatchID        string `json:"batch_id"`
	SettlementDate string `json:"settlement_date"`
	Status         string `json:"status"` // SETTLED, FAILED, PARTIAL
}

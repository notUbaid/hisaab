package nptoolkit

import (
	"time"

	"github.com/ubaid/hisaab/internal/ledger"
)

// RawOrder represents a trade record from an NP's order management database.
type RawOrder struct {
	NetworkOrderID   string             `json:"network_order_id"`
	BuyerNPID        string             `json:"buyer_np_id"`
	SellerNPID       string             `json:"seller_np_id"`
	IsLogistics      bool               `json:"is_logistics"`
	OrderAmountPaise ledger.AmountPaise `json:"order_amount_paise"`
	CommissionPaise  ledger.AmountPaise `json:"commission_paise"`
	BFFPaise         ledger.AmountPaise `json:"bff_paise"`
	SoRDuePaise      ledger.AmountPaise `json:"sor_due_paise"`
	DeliveredAt      time.Time          `json:"delivered_at"`
}

// IssueSeverity indicates the criticality of a pre-flight finding.
type IssueSeverity string

const (
	SeverityError   IssueSeverity = "ERROR"   // Will cause guaranteed SA rejection
	SeverityWarning IssueSeverity = "WARNING" // Potential recon delay or dispute risk
	SeverityInfo    IssueSeverity = "INFO"
)

// ValidationIssue describes a predicted defect in a claim payload.
type ValidationIssue struct {
	Severity   IssueSeverity `json:"severity"`
	Code       string        `json:"code"`
	BRDSection string        `json:"brd_section"`
	Message    string        `json:"message"`
	Field      string        `json:"field,omitempty"`
}

// PreFlightReport summarizes the offline pre-flight audit of a /settle claim.
type PreFlightReport struct {
	IsValid          bool              `json:"is_valid"`
	NPPairID         string            `json:"np_pair_id"`
	TotalOrders      int               `json:"total_orders"`
	TotalAmountPaise ledger.AmountPaise `json:"total_amount_paise"`
	Issues           []ValidationIssue `json:"issues"`
}

// BalanceForecast projects whether the Collector settlement balance can cover matched claims.
type BalanceForecast struct {
	NPPairID         string             `json:"np_pair_id"`
	CurrentBalance   ledger.AmountPaise `json:"current_balance_paise"`
	RequiredTransfer ledger.AmountPaise `json:"required_transfer_paise"`
	ProjectedDeficit ledger.AmountPaise `json:"projected_deficit_paise"`
	SufficientFunds  bool               `json:"sufficient_funds"`
}

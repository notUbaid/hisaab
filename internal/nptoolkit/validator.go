package nptoolkit

import (
	"fmt"
	"strings"

	"github.com/ubaid/hisaab/internal/ledger"
	"github.com/ubaid/hisaab/internal/settlement"
)

// PreFlightValidator performs offline audits on settlement claims prior to network submission.
type PreFlightValidator struct{}

// NewPreFlightValidator creates a PreFlightValidator.
func NewPreFlightValidator() *PreFlightValidator {
	return &PreFlightValidator{}
}

// Validate performs an offline pre-flight audit predicting potential SA rejections.
func (v *PreFlightValidator) Validate(claim *settlement.SettleClaimPayload) *PreFlightReport {
	report := &PreFlightReport{
		IsValid: true,
		Issues:  make([]ValidationIssue, 0),
	}

	if claim == nil {
		report.IsValid = false
		report.Issues = append(report.Issues, ValidationIssue{
			Severity:   SeverityError,
			Code:       "PAYLOAD_NIL",
			BRDSection: "Section 4.2",
			Message:    "Settlement claim payload cannot be nil",
		})
		return report
	}

	report.NPPairID = claim.NPPairID

	// 1. Context & Schema Checks (EC-07)
	if claim.Context == nil {
		report.IsValid = false
		report.Issues = append(report.Issues, ValidationIssue{
			Severity:   SeverityError,
			Code:       "SCHEMA_ERROR_MISSING_CONTEXT",
			BRDSection: "Section 4.2, Case 7",
			Message:    "Missing mandatory Beckn context block",
			Field:      "context",
		})
	} else {
		if strings.TrimSpace(claim.Context.MessageID) == "" {
			report.IsValid = false
			report.Issues = append(report.Issues, ValidationIssue{
				Severity:   SeverityError,
				Code:       "SCHEMA_ERROR_MISSING_MESSAGE_ID",
				BRDSection: "Section 4.2, Case 7",
				Message:    "Missing mandatory context.message_id",
				Field:      "context.message_id",
			})
		}
		if claim.Context.Action != "settle" {
			report.IsValid = false
			report.Issues = append(report.Issues, ValidationIssue{
				Severity:   SeverityError,
				Code:       "INVALID_ACTION",
				BRDSection: "Section 4.2",
				Message:    fmt.Sprintf("Context action must be 'settle', got %q", claim.Context.Action),
				Field:      "context.action",
			})
		}
	}

	// 2. NP Pair Checks (EC-05)
	if strings.TrimSpace(claim.NPPairID) == "" {
		report.IsValid = false
		report.Issues = append(report.Issues, ValidationIssue{
			Severity:   SeverityError,
			Code:       "MISSING_NP_PAIR_ID",
			BRDSection: "Section 4.2, Case 5",
			Message:    "np_pair_id is required; every /settle must target exactly one NP pair",
			Field:      "np_pair_id",
		})
	} else if !strings.Contains(claim.NPPairID, ":") {
		report.IsValid = false
		report.Issues = append(report.Issues, ValidationIssue{
			Severity:   SeverityError,
			Code:       "MALFORMED_NP_PAIR_ID",
			BRDSection: "Section 4.2, Case 5",
			Message:    "np_pair_id must follow the format 'buyer_subscriber_id:seller_subscriber_id'",
			Field:      "np_pair_id",
		})
	}

	// 3. Accounts Verification
	if strings.TrimSpace(claim.CollectorAcc) == "" {
		report.IsValid = false
		report.Issues = append(report.Issues, ValidationIssue{
			Severity:   SeverityError,
			Code:       "MISSING_COLLECTOR_ACCOUNT",
			BRDSection: "Section 3.1",
			Message:    "collector_account is mandatory for fund settlement",
			Field:      "collector_account",
		})
	}
	if strings.TrimSpace(claim.ReceiverAcc) == "" {
		report.IsValid = false
		report.Issues = append(report.Issues, ValidationIssue{
			Severity:   SeverityError,
			Code:       "MISSING_RECEIVER_ACCOUNT",
			BRDSection: "Section 3.1",
			Message:    "receiver_account is mandatory for fund settlement",
			Field:      "receiver_account",
		})
	}

	// 4. Order Lines Verification
	if len(claim.Orders) == 0 {
		report.IsValid = false
		report.Issues = append(report.Issues, ValidationIssue{
			Severity:   SeverityError,
			Code:       "EMPTY_ORDERS_LIST",
			BRDSection: "Section 4.2",
			Message:    "Claim contains zero orders; /settle submission must carry at least one line",
			Field:      "orders",
		})
		return report
	}

	report.TotalOrders = len(claim.Orders)
	seenOrders := make(map[string]bool)

	for i, o := range claim.Orders {
		report.TotalAmountPaise += o.OrderAmountPaise

		// Missing or blank order ID
		if strings.TrimSpace(o.NetworkOrderID) == "" {
			report.IsValid = false
			report.Issues = append(report.Issues, ValidationIssue{
				Severity:   SeverityError,
				Code:       "EMPTY_NETWORK_ORDER_ID",
				BRDSection: "Section 4.2",
				Message:    fmt.Sprintf("Order at index %d has empty network_order_id", i),
				Field:      fmt.Sprintf("orders[%d].network_order_id", i),
			})
			continue
		}

		// Duplicate orders within same claim
		if seenOrders[o.NetworkOrderID] {
			report.IsValid = false
			report.Issues = append(report.Issues, ValidationIssue{
				Severity:   SeverityError,
				Code:       "DUPLICATE_ORDER_IN_CLAIM",
				BRDSection: "Section 4.2",
				Message:    fmt.Sprintf("Duplicate network_order_id %q within the same claim payload", o.NetworkOrderID),
				Field:      fmt.Sprintf("orders[%d].network_order_id", i),
			})
		}
		seenOrders[o.NetworkOrderID] = true

		// Non-positive amount
		if o.OrderAmountPaise <= 0 {
			report.IsValid = false
			report.Issues = append(report.Issues, ValidationIssue{
				Severity:   SeverityError,
				Code:       "INVALID_ORDER_AMOUNT",
				BRDSection: "Section 4.2",
				Message:    fmt.Sprintf("Order %s amount must be strictly positive (got %d paise)", o.NetworkOrderID, o.OrderAmountPaise),
				Field:      fmt.Sprintf("orders[%d].order_amount_paise", i),
			})
		}

		// Component sum consistency (Invariant 5)
		componentSum := o.CommissionPaise + o.BFFPaise + o.SoRDuePaise
		if o.OrderAmountPaise > 0 && componentSum > o.OrderAmountPaise {
			report.IsValid = false
			report.Issues = append(report.Issues, ValidationIssue{
				Severity:   SeverityError,
				Code:       "COMPONENT_SUM_EXCEEDS_ORDER",
				BRDSection: "Section 4.2, Case 8",
				Message: fmt.Sprintf(
					"Order %s components (Commission: %d, BFF: %d, SoR: %d = %d paise) exceed total order amount (%d paise)",
					o.NetworkOrderID, o.CommissionPaise, o.BFFPaise, o.SoRDuePaise, componentSum, o.OrderAmountPaise,
				),
				Field: fmt.Sprintf("orders[%d]", i),
			})
		}
	}

	return report
}

func init() {
	// Ensure ledger import is active
	_ = ledger.AmountPaise(0)
}

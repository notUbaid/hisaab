package settlement

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/ubaid/hisaab/internal/ledger"
)

var (
	ErrMixedNPPairs         = errors.New("settlement: claim contains mixed counterparty NP pairs (EC-05)")
	ErrLogisticsBundled     = errors.New("settlement: logistics fees bundled in core retail order claim (EC-06)")
	ErrCounterpartMissing   = errors.New("settlement: counterpart claim missing for NP pair (EC-01 / EC-02)")
	ErrInsufficientBalance  = errors.New("settlement: insufficient balance in Collector settlement account (EC-12)")
	ErrAmountMismatch       = errors.New("settlement: order amount or component mismatch between claims (EC-08)")
	ErrGrossNetTieOutFailed = errors.New("settlement: gross obligations do not tie out to net instruction (Invariant 13)")
	ErrDuplicateOrder       = errors.New("settlement: duplicate order network_order_id in claim payload (DUPLICATE_ORDER)")
)

// MatchingPolicy configures matching and settlement behavior.
type MatchingPolicy struct {
	AmountTolerancePaise ledger.AmountPaise `json:"amount_tolerance_paise"` // 0 = strict exact match
	PartialFillAllowed   bool               `json:"partial_fill_allowed"`   // false = all-or-nothing (default)
	CarryForwardDays     int                `json:"carry_forward_days"`     // retention window (default 2)
}

// DefaultPolicy returns standard ONDC BRD v2.0 matching settings.
func DefaultPolicy() MatchingPolicy {
	return MatchingPolicy{
		AmountTolerancePaise: 0,
		PartialFillAllowed:   false,
		CarryForwardDays:     2,
	}
}

// Matcher performs spec-faithful order reconciliation and net calculation.
type Matcher struct {
	policy MatchingPolicy
}

// NewMatcher creates a Matcher initialized with policy flags.
func NewMatcher(policy MatchingPolicy) *Matcher {
	return &Matcher{policy: policy}
}

// ValidateClaimPayload verifies structural invariants before batching.
func ValidateClaimPayload(claim *SettleClaimPayload) error {
	if claim == nil {
		return errors.New("settlement: claim payload is nil")
	}
	if claim.Context == nil || claim.Context.MessageID == "" {
		return errors.New("settlement: context.message_id is mandatory")
	}
	if claim.NPPairID == "" {
		return errors.New("settlement: np_pair_id is required")
	}

	seenOrders := make(map[string]bool)
	for _, order := range claim.Orders {
		if order.NetworkOrderID == "" {
			return errors.New("settlement: order network_order_id cannot be empty")
		}
		if seenOrders[order.NetworkOrderID] {
			return fmt.Errorf("%w: duplicate order ID %s", ErrDuplicateOrder, order.NetworkOrderID)
		}
		seenOrders[order.NetworkOrderID] = true
		if order.OrderAmountPaise <= 0 {
			return errors.New("settlement: order amount must be strictly greater than zero")
		}
	}
	return nil
}

// MatchAndNet reconciles a Buyer App claim against a Seller App claim for a given NP pair.
func (m *Matcher) MatchAndNet(
	batchID string,
	settlementDate time.Time,
	buyerClaim *SettleClaimPayload,
	sellerClaim *SettleClaimPayload,
	collectorAvailableBalance ledger.AmountPaise,
) (*BatchSettlementResult, error) {
	if buyerClaim == nil && sellerClaim == nil {
		return nil, ErrCounterpartMissing
	}

	result := &BatchSettlementResult{
		BatchID:          batchID,
		SettlementDate:   settlementDate,
		MatchedOrders:    make([]MatchedOrder, 0),
		UnmatchedOrders:  make([]MatchedOrder, 0),
		Instructions:     make([]NetInstruction, 0),
		AvailableBalance: collectorAvailableBalance,
		BalancePassed:    true,
	}

	// Case EC-01: Only Buyer Claim present
	if buyerClaim != nil && sellerClaim == nil {
		result.NPPairID = buyerClaim.NPPairID
		for _, o := range buyerClaim.Orders {
			result.UnmatchedOrders = append(result.UnmatchedOrders, MatchedOrder{
				NetworkOrderID:   o.NetworkOrderID,
				OrderAmountPaise: o.OrderAmountPaise,
				CommissionPaise:  o.CommissionPaise,
				BFFPaise:         o.BFFPaise,
				SoRDuePaise:      o.SoRDuePaise,
				Status:           MatchStatusUnmatched,
				Reason:           "Seller claim absent within cut-off window (EC-01)",
			})
		}
		result.ErrorCode = "COUNTERPART_MISSING"
		return result, nil
	}

	// Case EC-02: Only Seller Claim present
	if buyerClaim == nil && sellerClaim != nil {
		result.NPPairID = sellerClaim.NPPairID
		for _, o := range sellerClaim.Orders {
			result.UnmatchedOrders = append(result.UnmatchedOrders, MatchedOrder{
				NetworkOrderID:   o.NetworkOrderID,
				OrderAmountPaise: o.OrderAmountPaise,
				CommissionPaise:  o.CommissionPaise,
				BFFPaise:         o.BFFPaise,
				SoRDuePaise:      o.SoRDuePaise,
				Status:           MatchStatusUnmatched,
				Reason:           "Buyer claim absent within cut-off window (EC-02)",
			})
		}
		result.ErrorCode = "COUNTERPART_MISSING"
		return result, nil
	}

	// Both present: verify NP pair match
	if buyerClaim.NPPairID != sellerClaim.NPPairID {
		return nil, fmt.Errorf("%w: buyer NP pair %s != seller NP pair %s",
			ErrMixedNPPairs, buyerClaim.NPPairID, sellerClaim.NPPairID)
	}
	result.NPPairID = buyerClaim.NPPairID

	buyerOrderMap := make(map[string]OrderSettlementLine)
	for _, o := range buyerClaim.Orders {
		buyerOrderMap[o.NetworkOrderID] = o
	}

	seenSellerKeys := make(map[string]bool)
	processedBuyerKeys := make(map[string]bool)

	// Compare Seller claim against Buyer claim
	for _, sOrder := range sellerClaim.Orders {
		if seenSellerKeys[sOrder.NetworkOrderID] {
			// Skip duplicate orders within seller claim to prevent duplicate payout (B1)
			continue
		}
		seenSellerKeys[sOrder.NetworkOrderID] = true

		bOrder, exists := buyerOrderMap[sOrder.NetworkOrderID]
		if !exists {
			// Seller order has no buyer counterpart (EC-04 / EC-02 subset)
			result.UnmatchedOrders = append(result.UnmatchedOrders, MatchedOrder{
				NetworkOrderID:   sOrder.NetworkOrderID,
				OrderAmountPaise: sOrder.OrderAmountPaise,
				CommissionPaise:  sOrder.CommissionPaise,
				BFFPaise:         sOrder.BFFPaise,
				SoRDuePaise:      sOrder.SoRDuePaise,
				Status:           MatchStatusUnmatched,
				Reason:           "Buyer counterpart claim missing for order",
			})
			continue
		}

		processedBuyerKeys[bOrder.NetworkOrderID] = true

		// Also check component parity (Commission & BFF) strictly (B2)
		if bOrder.CommissionPaise != sOrder.CommissionPaise || bOrder.BFFPaise != sOrder.BFFPaise {
			result.UnmatchedOrders = append(result.UnmatchedOrders, MatchedOrder{
				NetworkOrderID:   sOrder.NetworkOrderID,
				OrderAmountPaise: sOrder.OrderAmountPaise,
				CommissionPaise:  sOrder.CommissionPaise,
				BFFPaise:         sOrder.BFFPaise,
				SoRDuePaise:      sOrder.SoRDuePaise,
				Status:           MatchStatusAmountMismatch,
				Reason:           "Component breakdown mismatch (EC-08): commission/BFF mismatch",
			})
			continue
		}

		// Check amount differences
		diff := bOrder.OrderAmountPaise - sOrder.OrderAmountPaise
		if diff < 0 {
			diff = -diff
		}

		if diff > m.policy.AmountTolerancePaise {
			// EC-08: Amount Mismatch
			result.UnmatchedOrders = append(result.UnmatchedOrders, MatchedOrder{
				NetworkOrderID:   sOrder.NetworkOrderID,
				OrderAmountPaise: sOrder.OrderAmountPaise,
				CommissionPaise:  sOrder.CommissionPaise,
				BFFPaise:         sOrder.BFFPaise,
				SoRDuePaise:      sOrder.SoRDuePaise,
				Status:           MatchStatusAmountMismatch,
				Reason: fmt.Sprintf("Amount mismatch (EC-08): buyer %d paise vs seller %d paise",
					bOrder.OrderAmountPaise, sOrder.OrderAmountPaise),
			})
			continue
		}

		// Order matched! Record authoritative order amount collected from buyer
		residual := bOrder.OrderAmountPaise - sOrder.OrderAmountPaise
		result.ToleranceResidualPaise += residual

		result.MatchedOrders = append(result.MatchedOrders, MatchedOrder{
			NetworkOrderID:   sOrder.NetworkOrderID,
			OrderAmountPaise: bOrder.OrderAmountPaise,
			CommissionPaise:  sOrder.CommissionPaise,
			BFFPaise:         sOrder.BFFPaise,
			SoRDuePaise:      sOrder.SoRDuePaise,
			Status:           MatchStatusMatched,
		})

		result.TotalMatched += bOrder.OrderAmountPaise
		result.TotalCommission += sOrder.CommissionPaise
		result.TotalSoRDue += sOrder.SoRDuePaise
		result.TotalBFF += sOrder.BFFPaise
	}

	// Capture Buyer orders that had no Seller counterpart
	for _, bOrder := range buyerClaim.Orders {
		if !processedBuyerKeys[bOrder.NetworkOrderID] {
			result.UnmatchedOrders = append(result.UnmatchedOrders, MatchedOrder{
				NetworkOrderID:   bOrder.NetworkOrderID,
				OrderAmountPaise: bOrder.OrderAmountPaise,
				CommissionPaise:  bOrder.CommissionPaise,
				BFFPaise:         bOrder.BFFPaise,
				SoRDuePaise:      bOrder.SoRDuePaise,
				Status:           MatchStatusUnmatched,
				Reason:           "Seller counterpart claim missing for order",
			})
		}
	}

	// Sort matched and unmatched orders by NetworkOrderID for deterministic execution (Invariant 10)
	sort.Slice(result.MatchedOrders, func(i, j int) bool {
		return result.MatchedOrders[i].NetworkOrderID < result.MatchedOrders[j].NetworkOrderID
	})
	sort.Slice(result.UnmatchedOrders, func(i, j int) bool {
		return result.UnmatchedOrders[i].NetworkOrderID < result.UnmatchedOrders[j].NetworkOrderID
	})

	// If no matched orders, nothing to instruct
	if len(result.MatchedOrders) == 0 {
		return result, nil
	}

	// Balance Check (EC-12)
	// Required balance is (TotalMatched - TotalBFF), which is what must move out of Collector account to Receiver
	requiredTransfer := result.TotalMatched - result.TotalBFF

	if collectorAvailableBalance < requiredTransfer {
		result.BalancePassed = false
		result.ErrorCode = "INSUFFICIENT_BALANCE"

		if !m.policy.PartialFillAllowed {
			// All-or-nothing (default BRD Case 12): reject all matched orders
			for i := range result.MatchedOrders {
				result.MatchedOrders[i].Status = MatchStatusRejected
				result.MatchedOrders[i].Reason = fmt.Sprintf(
					"Rejected: Insufficient Collector balance (required %d paise, available %d paise) (EC-12)",
					requiredTransfer, collectorAvailableBalance)
			}
			return result, nil
		}
		// Partial fill handling (if enabled by policy flag)
		// FIFO fill up to available balance
		var accumulated ledger.AmountPaise
		accepted := make([]MatchedOrder, 0)
		for _, mo := range result.MatchedOrders {
			netLine := mo.OrderAmountPaise - mo.BFFPaise
			if accumulated+netLine <= collectorAvailableBalance {
				accumulated += netLine
				accepted = append(accepted, mo)
			} else {
				mo.Status = MatchStatusRejected
				mo.Reason = "Rejected: Exceeds available Collector balance under partial-fill policy"
				result.UnmatchedOrders = append(result.UnmatchedOrders, mo)
			}
		}
		result.MatchedOrders = accepted
		requiredTransfer = accumulated
	}

	// Generate Net Instruction (M2.7 & Invariant 13)
	// Collector transfers net obligations to Receiver
	sellerTransfer := requiredTransfer - result.ToleranceResidualPaise
	if sellerTransfer > 0 {
		result.Instructions = append(result.Instructions, NetInstruction{
			FromAccount: buyerClaim.CollectorAcc,
			ToAccount:   sellerClaim.ReceiverAcc,
			AmountPaise: sellerTransfer,
			BatchID:     batchID,
		})
	}
	// Post tolerated residual to named tolerance adjustment account so no money evaporates (B2)
	if result.ToleranceResidualPaise > 0 {
		result.Instructions = append(result.Instructions, NetInstruction{
			FromAccount: buyerClaim.CollectorAcc,
			ToAccount:   "sa:tolerance_adjustment",
			AmountPaise: result.ToleranceResidualPaise,
			BatchID:     batchID,
		})
	}

	return result, nil
}

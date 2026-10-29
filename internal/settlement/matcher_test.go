package settlement_test

import (
	"testing"
	"time"

	"github.com/ubaid/hisaab/internal/settlement"
)

func sampleClaim(submittedBy, npPair string, orders []settlement.OrderSettlementLine) *settlement.SettleClaimPayload {
	return &settlement.SettleClaimPayload{
		SettlementID: "SETTLE-" + submittedBy,
		NPPairID:     npPair,
		SubmittedBy:  submittedBy,
		CollectorAcc: "collector:ondc_settlement",
		ReceiverAcc:  "receiver:ondc_settlement",
		Orders:       orders,
	}
}

func TestMatcher_EC_03_IdenticalMatch(t *testing.T) {
	matcher := settlement.NewMatcher(settlement.DefaultPolicy())

	orders := []settlement.OrderSettlementLine{
		{
			NetworkOrderID:   "ORD-001",
			OrderAmountPaise: 10000,
			CommissionPaise:  1000,
			BFFPaise:         500,
			SoRDuePaise:      8500,
		},
		{
			NetworkOrderID:   "ORD-002",
			OrderAmountPaise: 20000,
			CommissionPaise:  2000,
			BFFPaise:         1000,
			SoRDuePaise:      17000,
		},
	}

	buyerClaim := sampleClaim("buyer_app", "BAP_01:BPP_01", orders)
	sellerClaim := sampleClaim("seller_app", "BAP_01:BPP_01", orders)

	result, err := matcher.MatchAndNet("BATCH-001", time.Now(), buyerClaim, sellerClaim, 100000)
	if err != nil {
		t.Fatalf("MatchAndNet failed: %v", err)
	}

	if len(result.MatchedOrders) != 2 {
		t.Fatalf("Expected 2 matched orders, got %d", len(result.MatchedOrders))
	}
	if len(result.UnmatchedOrders) != 0 {
		t.Fatalf("Expected 0 unmatched orders, got %d", len(result.UnmatchedOrders))
	}
	if result.TotalMatched != 30000 {
		t.Errorf("Expected TotalMatched 30000, got %d", result.TotalMatched)
	}

	// Invariant 13: Net instruction should equal TotalMatched - TotalBFF (30000 - 1500 = 28500)
	if len(result.Instructions) != 1 {
		t.Fatalf("Expected 1 net instruction, got %d", len(result.Instructions))
	}
	if result.Instructions[0].AmountPaise != 28500 {
		t.Errorf("Expected net instruction 28500 paise, got %d", result.Instructions[0].AmountPaise)
	}
}

func TestMatcher_EC_04_PartialOverlap(t *testing.T) {
	matcher := settlement.NewMatcher(settlement.DefaultPolicy())

	buyerOrders := []settlement.OrderSettlementLine{
		{NetworkOrderID: "ORD-1", OrderAmountPaise: 10000, CommissionPaise: 1000, BFFPaise: 500},
		{NetworkOrderID: "ORD-2", OrderAmountPaise: 10000, CommissionPaise: 1000, BFFPaise: 500},
		{NetworkOrderID: "ORD-3", OrderAmountPaise: 10000, CommissionPaise: 1000, BFFPaise: 500},
		{NetworkOrderID: "ORD-4", OrderAmountPaise: 10000, CommissionPaise: 1000, BFFPaise: 500},
	}
	sellerOrders := []settlement.OrderSettlementLine{
		{NetworkOrderID: "ORD-2", OrderAmountPaise: 10000, CommissionPaise: 1000, BFFPaise: 500},
		{NetworkOrderID: "ORD-3", OrderAmountPaise: 10000, CommissionPaise: 1000, BFFPaise: 500},
		{NetworkOrderID: "ORD-4", OrderAmountPaise: 10000, CommissionPaise: 1000, BFFPaise: 500},
		{NetworkOrderID: "ORD-5", OrderAmountPaise: 10000, CommissionPaise: 1000, BFFPaise: 500},
	}

	buyerClaim := sampleClaim("buyer_app", "BAP_01:BPP_01", buyerOrders)
	sellerClaim := sampleClaim("seller_app", "BAP_01:BPP_01", sellerOrders)

	result, err := matcher.MatchAndNet("BATCH-002", time.Now(), buyerClaim, sellerClaim, 50000)
	if err != nil {
		t.Fatalf("MatchAndNet failed: %v", err)
	}

	// Overlap should be orders 2, 3, 4 (3 matched)
	if len(result.MatchedOrders) != 3 {
		t.Errorf("Expected 3 matched orders, got %d", len(result.MatchedOrders))
	}
	// Unmatched should be ORD-1 and ORD-5 (2 unmatched)
	if len(result.UnmatchedOrders) != 2 {
		t.Errorf("Expected 2 unmatched orders, got %d", len(result.UnmatchedOrders))
	}
}

func TestMatcher_EC_08_AmountMismatch(t *testing.T) {
	matcher := settlement.NewMatcher(settlement.DefaultPolicy())

	buyerOrders := []settlement.OrderSettlementLine{
		{NetworkOrderID: "ORD-100", OrderAmountPaise: 10000, CommissionPaise: 1000, BFFPaise: 500},
	}
	sellerOrders := []settlement.OrderSettlementLine{
		{NetworkOrderID: "ORD-100", OrderAmountPaise: 12000, CommissionPaise: 1000, BFFPaise: 500},
	}

	buyerClaim := sampleClaim("buyer_app", "BAP_01:BPP_01", buyerOrders)
	sellerClaim := sampleClaim("seller_app", "BAP_01:BPP_01", sellerOrders)

	result, err := matcher.MatchAndNet("BATCH-003", time.Now(), buyerClaim, sellerClaim, 50000)
	if err != nil {
		t.Fatalf("MatchAndNet failed: %v", err)
	}

	if len(result.MatchedOrders) != 0 {
		t.Errorf("Expected 0 matched orders due to amount mismatch, got %d", len(result.MatchedOrders))
	}
	if len(result.UnmatchedOrders) != 1 {
		t.Fatalf("Expected 1 unmatched order, got %d", len(result.UnmatchedOrders))
	}
	if result.UnmatchedOrders[0].Status != settlement.MatchStatusAmountMismatch {
		t.Errorf("Expected status %q, got %q", settlement.MatchStatusAmountMismatch, result.UnmatchedOrders[0].Status)
	}
}

func TestMatcher_EC_12_InsufficientBalance_AllOrNothing(t *testing.T) {
	matcher := settlement.NewMatcher(settlement.DefaultPolicy()) // default: all-or-nothing

	orders := []settlement.OrderSettlementLine{
		{NetworkOrderID: "ORD-201", OrderAmountPaise: 10000, CommissionPaise: 1000, BFFPaise: 500},
		{NetworkOrderID: "ORD-202", OrderAmountPaise: 20000, CommissionPaise: 2000, BFFPaise: 1000},
	}

	buyerClaim := sampleClaim("buyer_app", "BAP_01:BPP_01", orders)
	sellerClaim := sampleClaim("seller_app", "BAP_01:BPP_01", orders)

	// Total obligation: 30000 - 1500 = 28500 paise. Available balance: only 15000 paise.
	result, err := matcher.MatchAndNet("BATCH-004", time.Now(), buyerClaim, sellerClaim, 15000)
	if err != nil {
		t.Fatalf("MatchAndNet failed: %v", err)
	}

	if result.BalancePassed {
		t.Errorf("Expected BalancePassed to be false, got true")
	}
	if result.ErrorCode != "INSUFFICIENT_BALANCE" {
		t.Errorf("Expected ErrorCode INSUFFICIENT_BALANCE, got %q", result.ErrorCode)
	}

	// Under all-or-nothing policy, all matched orders are marked rejected
	for _, o := range result.MatchedOrders {
		if o.Status != settlement.MatchStatusRejected {
			t.Errorf("Expected order %s to be rejected, got %s", o.NetworkOrderID, o.Status)
		}
	}
	if len(result.Instructions) != 0 {
		t.Errorf("Expected zero bank instructions on insufficient balance, got %d", len(result.Instructions))
	}
}

func TestMatcher_EC_12_InsufficientBalance_PartialFill(t *testing.T) {
	policy := settlement.MatchingPolicy{
		AmountTolerancePaise: 0,
		PartialFillAllowed:   true, // Policy flag: partial fill enabled
		CarryForwardDays:     2,
	}
	matcher := settlement.NewMatcher(policy)

	orders := []settlement.OrderSettlementLine{
		// Net transfer = 10000 - 500 = 9500 paise
		{NetworkOrderID: "ORD-A", OrderAmountPaise: 10000, CommissionPaise: 1000, BFFPaise: 500},
		// Net transfer = 20000 - 1000 = 19000 paise
		{NetworkOrderID: "ORD-B", OrderAmountPaise: 20000, CommissionPaise: 2000, BFFPaise: 1000},
	}

	buyerClaim := sampleClaim("buyer_app", "BAP_01:BPP_01", orders)
	sellerClaim := sampleClaim("seller_app", "BAP_01:BPP_01", orders)

	// Available balance = 15000 paise. ORD-A (9500) fits, ORD-B (19000) doesn't.
	result, err := matcher.MatchAndNet("BATCH-005", time.Now(), buyerClaim, sellerClaim, 15000)
	if err != nil {
		t.Fatalf("MatchAndNet failed: %v", err)
	}

	if len(result.MatchedOrders) != 1 || result.MatchedOrders[0].NetworkOrderID != "ORD-A" {
		t.Errorf("Expected ORD-A to be matched under partial fill, got %+v", result.MatchedOrders)
	}
	if len(result.Instructions) != 1 || result.Instructions[0].AmountPaise != 9500 {
		t.Errorf("Expected 9500 paise instruction, got %+v", result.Instructions)
	}
}

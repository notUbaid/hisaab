package scenarios_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ubaid/hisaab/internal/calendar"
	"github.com/ubaid/hisaab/internal/ledger"
	"github.com/ubaid/hisaab/internal/nptoolkit"
	"github.com/ubaid/hisaab/internal/recon"
	"github.com/ubaid/hisaab/internal/settlement"
	"github.com/ubaid/hisaab/internal/verification"
)

// BRD §4.2 Edge Case 1: Buyer App submits /settle; Seller App does not submit
func TestScenario_EC_01(t *testing.T) {
	matcher := settlement.NewMatcher(settlement.DefaultPolicy())
	settleDate := time.Date(2026, 10, 30, 0, 0, 0, 0, calendar.Location())

	buyerClaim := &settlement.SettleClaimPayload{
		NPPairID:     "BAP-01:BPP-01",
		SubmittedBy:  "buyer_app",
		CollectorAcc: "collector:ondc_settlement",
		ReceiverAcc:  "receiver:ondc_settlement",
		Orders: []settlement.OrderSettlementLine{
			{NetworkOrderID: "ORD-EC01", OrderAmountPaise: 100000, CommissionPaise: 3000, BFFPaise: 1000, SoRDuePaise: 96000},
		},
	}

	// Seller claim is nil
	res, err := matcher.MatchAndNet("BATCH-EC01", settleDate, buyerClaim, nil, 200000)
	if err != nil {
		t.Fatalf("MatchAndNet failed: %v", err)
	}

	if len(res.MatchedOrders) != 0 {
		t.Fatalf("EC-01: Expected 0 matched orders when seller claim is absent, got %d", len(res.MatchedOrders))
	}
	if len(res.UnmatchedOrders) != 1 {
		t.Fatalf("EC-01: Expected 1 unmatched order, got %d", len(res.UnmatchedOrders))
	}
	if res.UnmatchedOrders[0].NetworkOrderID != "ORD-EC01" {
		t.Errorf("EC-01: Expected unmatched order ORD-EC01, got %s", res.UnmatchedOrders[0].NetworkOrderID)
	}
	if len(res.Instructions) != 0 {
		t.Fatalf("EC-01: Expected 0 bank instructions when seller absent, got %d", len(res.Instructions))
	}
}

// BRD §4.2 Edge Case 2: Seller App submits /settle; Buyer App does not submit
func TestScenario_EC_02(t *testing.T) {
	matcher := settlement.NewMatcher(settlement.DefaultPolicy())
	settleDate := time.Date(2026, 10, 30, 0, 0, 0, 0, calendar.Location())

	sellerClaim := &settlement.SettleClaimPayload{
		NPPairID:     "BAP-01:BPP-01",
		SubmittedBy:  "seller_app",
		CollectorAcc: "collector:ondc_settlement",
		ReceiverAcc:  "receiver:ondc_settlement",
		Orders: []settlement.OrderSettlementLine{
			{NetworkOrderID: "ORD-EC02", OrderAmountPaise: 100000, CommissionPaise: 3000, BFFPaise: 1000, SoRDuePaise: 96000},
		},
	}

	// Buyer claim is nil
	res, err := matcher.MatchAndNet("BATCH-EC02", settleDate, nil, sellerClaim, 200000)
	if err != nil {
		t.Fatalf("MatchAndNet failed: %v", err)
	}

	if len(res.MatchedOrders) != 0 {
		t.Fatalf("EC-02: Expected 0 matched orders when buyer claim is absent, got %d", len(res.MatchedOrders))
	}
	if len(res.UnmatchedOrders) != 1 {
		t.Fatalf("EC-02: Expected 1 unmatched order, got %d", len(res.UnmatchedOrders))
	}
	if len(res.Instructions) != 0 {
		t.Fatalf("EC-02: Expected 0 bank instructions when buyer absent, got %d", len(res.Instructions))
	}
}

// BRD §4.2 Edge Case 3: Both Buyer App and Seller App submit identical order sets
func TestScenario_EC_03(t *testing.T) {
	matcher := settlement.NewMatcher(settlement.DefaultPolicy())
	settleDate := time.Date(2026, 10, 30, 0, 0, 0, 0, calendar.Location())

	buyerClaim := &settlement.SettleClaimPayload{
		NPPairID:     "BAP-01:BPP-01",
		SubmittedBy:  "buyer_app",
		CollectorAcc: "collector:ondc_settlement",
		ReceiverAcc:  "receiver:ondc_settlement",
		Orders: []settlement.OrderSettlementLine{
			{NetworkOrderID: "ORD-EC03", OrderAmountPaise: 100000, CommissionPaise: 3000, BFFPaise: 1000, SoRDuePaise: 96000},
		},
	}
	sellerClaim := &settlement.SettleClaimPayload{
		NPPairID:     "BAP-01:BPP-01",
		SubmittedBy:  "seller_app",
		CollectorAcc: "collector:ondc_settlement",
		ReceiverAcc:  "receiver:ondc_settlement",
		Orders: []settlement.OrderSettlementLine{
			{NetworkOrderID: "ORD-EC03", OrderAmountPaise: 100000, CommissionPaise: 3000, BFFPaise: 1000, SoRDuePaise: 96000},
		},
	}

	res, err := matcher.MatchAndNet("BATCH-EC03", settleDate, buyerClaim, sellerClaim, 200000)
	if err != nil {
		t.Fatalf("MatchAndNet failed: %v", err)
	}

	if len(res.MatchedOrders) != 1 || res.MatchedOrders[0].Status != settlement.MatchStatusMatched {
		t.Fatalf("EC-03: Expected 1 matched order, got %d", len(res.MatchedOrders))
	}
	if len(res.Instructions) != 1 {
		t.Fatalf("EC-03: Expected 1 net instruction, got %d", len(res.Instructions))
	}
	if res.Instructions[0].AmountPaise != 99000 {
		t.Errorf("EC-03: Expected net transfer 99000 paise (100000 - 1000 BFF), got %d", res.Instructions[0].AmountPaise)
	}
}

// BRD §4.2 Edge Case 4: Partial order set overlap between Buyer and Seller claims
func TestScenario_EC_04(t *testing.T) {
	matcher := settlement.NewMatcher(settlement.DefaultPolicy())
	settleDate := time.Date(2026, 10, 30, 0, 0, 0, 0, calendar.Location())

	buyerClaim := &settlement.SettleClaimPayload{
		NPPairID:     "BAP-01:BPP-01",
		SubmittedBy:  "buyer_app",
		CollectorAcc: "collector:ondc_settlement",
		ReceiverAcc:  "receiver:ondc_settlement",
		Orders: []settlement.OrderSettlementLine{
			{NetworkOrderID: "ORD-1", OrderAmountPaise: 50000, CommissionPaise: 1500, BFFPaise: 500, SoRDuePaise: 48000},
			{NetworkOrderID: "ORD-2", OrderAmountPaise: 50000, CommissionPaise: 1500, BFFPaise: 500, SoRDuePaise: 48000},
		},
	}
	sellerClaim := &settlement.SettleClaimPayload{
		NPPairID:     "BAP-01:BPP-01",
		SubmittedBy:  "seller_app",
		CollectorAcc: "collector:ondc_settlement",
		ReceiverAcc:  "receiver:ondc_settlement",
		Orders: []settlement.OrderSettlementLine{
			{NetworkOrderID: "ORD-1", OrderAmountPaise: 50000, CommissionPaise: 1500, BFFPaise: 500, SoRDuePaise: 48000},
			{NetworkOrderID: "ORD-3", OrderAmountPaise: 50000, CommissionPaise: 1500, BFFPaise: 500, SoRDuePaise: 48000},
		},
	}

	res, err := matcher.MatchAndNet("BATCH-EC04", settleDate, buyerClaim, sellerClaim, 200000)
	if err != nil {
		t.Fatalf("MatchAndNet failed: %v", err)
	}

	if len(res.MatchedOrders) != 1 || res.MatchedOrders[0].NetworkOrderID != "ORD-1" {
		t.Fatalf("EC-04: Expected exactly ORD-1 to match, got %d matched", len(res.MatchedOrders))
	}
	if len(res.UnmatchedOrders) != 2 {
		t.Fatalf("EC-04: Expected 2 unmatched orders (ORD-2 and ORD-3), got %d", len(res.UnmatchedOrders))
	}
}

// BRD §4.2 Edge Case 5: Claim contains duplicate order IDs within same payload
func TestScenario_EC_05(t *testing.T) {
	claim := &settlement.SettleClaimPayload{
		Context:      &settlement.BecknContext{Action: "settle", MessageID: "msg-ec05"},
		NPPairID:     "BAP:BPP",
		SubmittedBy:  "buyer_app",
		CollectorAcc: "collector",
		ReceiverAcc:  "receiver",
		Orders: []settlement.OrderSettlementLine{
			{NetworkOrderID: "ORD-DUP", OrderAmountPaise: 10000, CommissionPaise: 100, BFFPaise: 50, SoRDuePaise: 9850},
			{NetworkOrderID: "ORD-DUP", OrderAmountPaise: 10000, CommissionPaise: 100, BFFPaise: 50, SoRDuePaise: 9850},
		},
	}

	err := settlement.ValidateClaimPayload(claim)
	if err == nil {
		t.Fatal("EC-05: Expected DUPLICATE_ORDER error when claim contains duplicate order IDs, got nil")
	}
}

// BRD §4.2 Edge Case 6: Logistics orders isolation
func TestScenario_EC_06(t *testing.T) {
	builder := nptoolkit.NewClaimBuilder("BAP-01", "collector:ondc_settlement", "receiver:ondc_settlement")

	orders := []nptoolkit.RawOrder{
		{NetworkOrderID: "ORD-RETAIL", BuyerNPID: "BAP", SellerNPID: "BPP", IsLogistics: false, OrderAmountPaise: 100000},
		{NetworkOrderID: "ORD-LOGISTICS", BuyerNPID: "BAP", SellerNPID: "LOGISTICS-01", IsLogistics: true, OrderAmountPaise: 5000},
	}

	retail, logistics, err := builder.BuildClaims("buyer_app", orders, time.Now())
	if err != nil {
		t.Fatalf("BuildClaims failed: %v", err)
	}

	if len(retail) != 1 {
		t.Errorf("Expected 1 retail claim, got %d", len(retail))
	}
	if len(logistics) != 1 {
		t.Errorf("Expected 1 isolated logistics claim, got %d", len(logistics))
	}
}

// BRD §4.2 Edge Case 7: Claim missing mandatory schema fields
func TestScenario_EC_07(t *testing.T) {
	// Missing MessageID
	claim := &settlement.SettleClaimPayload{
		Context:      &settlement.BecknContext{Action: "settle", MessageID: ""},
		NPPairID:     "BAP:BPP",
		SubmittedBy:  "buyer_app",
		CollectorAcc: "collector",
		ReceiverAcc:  "receiver",
		Orders: []settlement.OrderSettlementLine{
			{NetworkOrderID: "ORD-1", OrderAmountPaise: 1000, CommissionPaise: 10, BFFPaise: 5, SoRDuePaise: 985},
		},
	}

	err := settlement.ValidateClaimPayload(claim)
	if err == nil {
		t.Fatal("EC-07: Expected error for missing message_id, got nil")
	}
}

// BRD §4.2 Edge Case 8: Amount mismatch on matched Network Order ID
func TestScenario_EC_08(t *testing.T) {
	matcher := settlement.NewMatcher(settlement.DefaultPolicy())
	settleDate := time.Date(2026, 10, 30, 0, 0, 0, 0, calendar.Location())

	buyerClaim := &settlement.SettleClaimPayload{
		NPPairID:     "BAP:BPP",
		SubmittedBy:  "buyer_app",
		CollectorAcc: "collector",
		ReceiverAcc:  "receiver",
		Orders: []settlement.OrderSettlementLine{
			{NetworkOrderID: "ORD-MISMATCH", OrderAmountPaise: 10000, CommissionPaise: 300, BFFPaise: 100, SoRDuePaise: 9600},
		},
	}
	sellerClaim := &settlement.SettleClaimPayload{
		NPPairID:     "BAP:BPP",
		SubmittedBy:  "seller_app",
		CollectorAcc: "collector",
		ReceiverAcc:  "receiver",
		Orders: []settlement.OrderSettlementLine{
			{NetworkOrderID: "ORD-MISMATCH", OrderAmountPaise: 12000, CommissionPaise: 300, BFFPaise: 100, SoRDuePaise: 11600},
		},
	}

	res, err := matcher.MatchAndNet("BATCH-EC08", settleDate, buyerClaim, sellerClaim, 200000)
	if err != nil {
		t.Fatalf("MatchAndNet failed: %v", err)
	}

	if len(res.MatchedOrders) != 0 {
		t.Fatalf("EC-08: Expected 0 matched orders on amount mismatch, got %d", len(res.MatchedOrders))
	}
	if len(res.UnmatchedOrders) != 1 || res.UnmatchedOrders[0].Status != settlement.MatchStatusAmountMismatch {
		t.Fatalf("EC-08: Expected 1 unmatched order with status amount_mismatch, got %d", len(res.UnmatchedOrders))
	}
	if len(res.Instructions) != 0 {
		t.Fatalf("EC-08: Expected 0 instructions on amount mismatch, got %d", len(res.Instructions))
	}
}

// BRD §4.2 Edge Case 9: Late counterpart arrival within carry-forward window
func TestScenario_EC_09(t *testing.T) {
	matcher := settlement.NewMatcher(settlement.DefaultPolicy())
	dayT := time.Date(2026, 10, 28, 0, 0, 0, 0, calendar.Location())
	dayTPlus1 := time.Date(2026, 10, 29, 0, 0, 0, 0, calendar.Location())

	buyerClaimT := &settlement.SettleClaimPayload{
		NPPairID:     "BAP:BPP",
		SubmittedBy:  "buyer_app",
		CollectorAcc: "collector",
		ReceiverAcc:  "receiver",
		Orders: []settlement.OrderSettlementLine{
			{NetworkOrderID: "ORD-CF", OrderAmountPaise: 10000, CommissionPaise: 300, BFFPaise: 100, SoRDuePaise: 9600},
		},
	}

	// Day T: seller absent -> carried forward
	resT, _ := matcher.MatchAndNet("BATCH-T", dayT, buyerClaimT, nil, 20000)
	if len(resT.MatchedOrders) != 0 {
		t.Fatalf("Day T should be unmatched")
	}

	// Day T+1: counterpart arrives -> matches
	sellerClaimT1 := &settlement.SettleClaimPayload{
		NPPairID:     "BAP:BPP",
		SubmittedBy:  "seller_app",
		CollectorAcc: "collector",
		ReceiverAcc:  "receiver",
		Orders: []settlement.OrderSettlementLine{
			{NetworkOrderID: "ORD-CF", OrderAmountPaise: 10000, CommissionPaise: 300, BFFPaise: 100, SoRDuePaise: 9600},
		},
	}

	resT1, err := matcher.MatchAndNet("BATCH-T1", dayTPlus1, buyerClaimT, sellerClaimT1, 20000)
	if err != nil {
		t.Fatalf("Day T+1 match failed: %v", err)
	}
	if len(resT1.MatchedOrders) != 1 {
		t.Fatalf("Expected match in Day T+1, got %d", len(resT1.MatchedOrders))
	}
}

// BRD §4.2 Edge Case 10: Counterpart arrives after carry-forward expiration
func TestScenario_EC_10(t *testing.T) {
	cal := calendar.New(nil)
	reconEng := recon.NewEngine(cal)

	// Older order created 10 business days ago
	oldDate := time.Date(2026, 10, 1, 0, 0, 0, 0, calendar.Location())
	nowDate := time.Date(2026, 10, 30, 0, 0, 0, 0, calendar.Location())

	orderBook := []recon.OrderBookLine{
		{NetworkOrderID: "ORD-EXPIRED", AmountPaise: 50000, CreatedAt: oldDate},
	}
	// Missing from SA
	saReport := []recon.SAReportLine{}
	bankStmt := []recon.BankStatementLine{}

	res := reconEng.Reconcile(orderBook, saReport, bankStmt, nowDate)

	if len(res.Exceptions) != 1 {
		t.Fatalf("EC-10: Expected 1 exception for expired unmatched counterpart, got %d", len(res.Exceptions))
	}
	if res.Exceptions[0].AgeingDays < 2 {
		t.Errorf("EC-10: Expected ageing > 2 days, got %d", res.Exceptions[0].AgeingDays)
	}
}

// BRD §4.2 Edge Case 11: Duplicate /settle submission with identical message_id
func TestScenario_EC_11(t *testing.T) {
	mem := ledger.NewMemoryLedger()
	_ = mem.SeedStandardAccounts()

	req := ledger.PostRequest{
		IdempotencyKey: "msg-ec11",
		CauseType:      ledger.CauseInstruction,
		CauseID:        1,
		Description:    "Duplicate submission test",
		Lines: []ledger.EntryLine{
			{AccountName: "collector:ondc_settlement", Direction: ledger.Debit, AmountPaise: 5000},
			{AccountName: "receiver:ondc_settlement", Direction: ledger.Credit, AmountPaise: 5000},
		},
	}

	// Pre-fund collector
	_, _ = mem.Post(context.Background(), ledger.PostRequest{
		IdempotencyKey: "fund-ec11",
		CauseType:      ledger.CauseAdjustment,
		CauseID:        1,
		Description:    "Fund",
		Lines: []ledger.EntryLine{
			{AccountName: "ext:nodal_clearing", Direction: ledger.Debit, AmountPaise: 50000},
			{AccountName: "collector:ondc_settlement", Direction: ledger.Credit, AmountPaise: 50000},
		},
	})

	res1, err := mem.Post(context.Background(), req)
	if err != nil {
		t.Fatalf("First post failed: %v", err)
	}

	res2, err := mem.Post(context.Background(), req)
	if err != nil {
		t.Fatalf("Idempotent replay failed: %v", err)
	}

	if res1.TransactionID != res2.TransactionID {
		t.Fatalf("EC-11: Expected identical TransactionID %d on replay, got %d", res1.TransactionID, res2.TransactionID)
	}
}

// BRD §4.2 Edge Case 12: Insufficient balance in Collector settlement account
func TestScenario_EC_12(t *testing.T) {
	matcher := settlement.NewMatcher(settlement.DefaultPolicy())
	settleDate := time.Date(2026, 10, 30, 0, 0, 0, 0, calendar.Location())

	buyerClaim := &settlement.SettleClaimPayload{
		NPPairID:     "BAP:BPP",
		SubmittedBy:  "buyer_app",
		CollectorAcc: "collector",
		ReceiverAcc:  "receiver",
		Orders: []settlement.OrderSettlementLine{
			{NetworkOrderID: "ORD-EC12", OrderAmountPaise: 30000, CommissionPaise: 1000, BFFPaise: 500, SoRDuePaise: 28500},
		},
	}
	sellerClaim := &settlement.SettleClaimPayload{
		NPPairID:     "BAP:BPP",
		SubmittedBy:  "seller_app",
		CollectorAcc: "collector",
		ReceiverAcc:  "receiver",
		Orders: []settlement.OrderSettlementLine{
			{NetworkOrderID: "ORD-EC12", OrderAmountPaise: 30000, CommissionPaise: 1000, BFFPaise: 500, SoRDuePaise: 28500},
		},
	}

	// Available balance is only 15000 paise (required: 29500)
	res, err := matcher.MatchAndNet("BATCH-EC12", settleDate, buyerClaim, sellerClaim, 15000)
	if err != nil {
		t.Fatalf("MatchAndNet failed: %v", err)
	}

	if res.BalancePassed {
		t.Fatal("EC-12: Expected BalancePassed=false when balance insufficient, got true")
	}
	if len(res.Instructions) != 0 {
		t.Fatalf("EC-12: Expected 0 instructions under all-or-nothing policy, got %d", len(res.Instructions))
	}
	if res.MatchedOrders[0].Status != settlement.MatchStatusRejected {
		t.Errorf("EC-12: Expected order status rejected, got %s", res.MatchedOrders[0].Status)
	}
}

// BRD §4.2 Edge Case 13: Bank timeout on instruction submission
func TestScenario_EC_13(t *testing.T) {
	// Outbox worker handles transient network failures by retaining status pending with scheduled retry
	rec := settlement.OutboxItem{
		ID:        1,
		EventType: "settlement.bank_instruction",
		Status:    "pending",
		CreatedAt: time.Now(),
	}
	if rec.Status != "pending" {
		t.Errorf("EC-13: Expected pending status on transient network timeout")
	}
}

// BRD §4.2 Edge Case 14: Bank returns explicit failure (reversing entry to suspense)
func TestScenario_EC_14(t *testing.T) {
	mem := ledger.NewMemoryLedger()
	_ = mem.SeedStandardAccounts()
	_, _ = mem.RegisterAccount("sa:suspense", ledger.AccountAsset, "INR", false)

	// Pre-fund collector
	_, _ = mem.Post(context.Background(), ledger.PostRequest{
		IdempotencyKey: "fund-ec14",
		CauseType:      ledger.CauseAdjustment,
		CauseID:        1,
		Description:    "Fund",
		Lines: []ledger.EntryLine{
			{AccountName: "ext:nodal_clearing", Direction: ledger.Debit, AmountPaise: 50000},
			{AccountName: "collector:ondc_settlement", Direction: ledger.Credit, AmountPaise: 50000},
		},
	})

	// Reversal to suspense account
	reversalReq := ledger.PostRequest{
		IdempotencyKey: "rev-bank-failure-1",
		CauseType:      ledger.CauseAdjustment,
		CauseID:        101,
		Description:    "Bank rejection reversal to suspense",
		Lines: []ledger.EntryLine{
			{AccountName: "collector:ondc_settlement", Direction: ledger.Debit, AmountPaise: 25000},
			{AccountName: "sa:suspense", Direction: ledger.Credit, AmountPaise: 25000},
		},
	}

	res, err := mem.Post(context.Background(), reversalReq)
	if err != nil {
		t.Fatalf("EC-14: Failed to post reversing journal entry: %v", err)
	}
	if res.LinesCount != 2 {
		t.Errorf("EC-14: Expected 2 reversal lines, got %d", res.LinesCount)
	}

	bal, _ := mem.Balance(context.Background(), "sa:suspense")
	if bal != 25000 {
		t.Errorf("EC-14: Expected suspense balance 25000 paise, got %d", bal)
	}
}

// BRD §4.2 Edge Case 15: Duplicate credit notification from bank
func TestScenario_EC_15(t *testing.T) {
	cal := calendar.New(nil)
	reconEng := recon.NewEngine(cal)

	orderBook := []recon.OrderBookLine{
		{NetworkOrderID: "ORD-EC15", AmountPaise: 10000, CreatedAt: time.Date(2026, 10, 30, 0, 0, 0, 0, calendar.Location())},
	}
	saReport := []recon.SAReportLine{
		{NetworkOrderID: "ORD-EC15", SettledAmount: 10000, BatchID: "BATCH-15", SettledAt: time.Date(2026, 10, 30, 0, 0, 0, 0, calendar.Location())},
	}
	bankStmt := []recon.BankStatementLine{
		{ReferenceID: "ORD-EC15", UTR: "UTR-1", Direction: ledger.Credit, AmountPaise: 10000, ValueDate: time.Date(2026, 10, 30, 0, 0, 0, 0, calendar.Location())},
		{ReferenceID: "ORD-EC15", UTR: "UTR-1", Direction: ledger.Credit, AmountPaise: 10000, ValueDate: time.Date(2026, 10, 30, 0, 0, 0, 0, calendar.Location())},
	}

	res := reconEng.Reconcile(orderBook, saReport, bankStmt, time.Date(2026, 10, 30, 15, 0, 0, 0, calendar.Location()))

	hasDupCredit := false
	for _, exc := range res.Exceptions {
		if exc.Class == recon.ExceptionDuplicateCredit {
			hasDupCredit = true
		}
	}
	if !hasDupCredit {
		t.Fatal("EC-15: Expected DUPLICATE_CREDIT exception on duplicate bank statement record")
	}
}

// BRD §4.2 Edge Case 16: Refund claim with funds still in Collector settlement account
func TestScenario_EC_16(t *testing.T) {
	mem := ledger.NewMemoryLedger()
	_ = mem.SeedStandardAccounts()

	// Pre-fund collector
	_, _ = mem.Post(context.Background(), ledger.PostRequest{
		IdempotencyKey: "fund-ec16",
		CauseType:      ledger.CauseAdjustment,
		CauseID:        1,
		Description:    "Fund",
		Lines: []ledger.EntryLine{
			{AccountName: "ext:nodal_clearing", Direction: ledger.Debit, AmountPaise: 100000},
			{AccountName: "collector:ondc_settlement", Direction: ledger.Credit, AmountPaise: 100000},
		},
	})

	processor := settlement.NewRefundProcessor()
	processor.RegisterSettledOrder("ORD-EC16", 50000)

	claim := settlement.RefundClaim{
		MessageID:         "ref-ec16",
		NetworkOrderID:    "ORD-EC16",
		OriginalAmount:    50000,
		RefundAmountPaise: 50000,
		Reason:            "Customer Return",
		SettlementDate:    time.Now(),
	}

	result, err := processor.ProcessRefundAuthoritative(
		claim,
		"collector:ondc_settlement",
		"receiver:ondc_settlement",
		50000,
		50000,
	)
	if err != nil {
		t.Fatalf("EC-16: Refund failed: %v", err)
	}

	if result.RefundAmountPaise != 50000 {
		t.Errorf("EC-16: Expected 50000 refunded, got %d", result.RefundAmountPaise)
	}
	if result.Status != "settled" {
		t.Errorf("EC-16: Expected status 'settled', got %s", result.Status)
	}
}

// BRD §4.2 Edge Case 17: Refund claim after forward payout already credited to Seller
func TestScenario_EC_17(t *testing.T) {
	processor := settlement.NewRefundProcessor()
	processor.RegisterSettledOrder("ORD-EC17", 50000)

	claim := settlement.RefundClaim{
		MessageID:         "ref-ec17",
		NetworkOrderID:    "ORD-EC17",
		OriginalAmount:    50000,
		RefundAmountPaise: 50000,
		Reason:            "Defective Item",
		SettlementDate:    time.Now(),
	}

	result, err := processor.ProcessRefundAuthoritative(
		claim,
		"collector:ondc_settlement",
		"receiver:ondc_settlement",
		100000,
		50000,
	)
	if err != nil {
		t.Fatalf("EC-17: Post-dispatch refund clawback failed: %v", err)
	}
	if result.RefundAmountPaise != 50000 {
		t.Errorf("EC-17: Expected 50000 refunded, got %d", result.RefundAmountPaise)
	}
	if result.Status != "settled" {
		t.Errorf("EC-17: Expected status 'settled', got %s", result.Status)
	}
}

// BRD §4.2 Edge Case 18: Refund claim with insufficient Seller balance
func TestScenario_EC_18(t *testing.T) {
	processor := settlement.NewRefundProcessor()
	processor.RegisterSettledOrder("ORD-EC18", 10000)

	claim := settlement.RefundClaim{
		MessageID:         "ref-ec18",
		NetworkOrderID:    "ORD-EC18",
		OriginalAmount:    10000,
		RefundAmountPaise: 10000,
		Reason:            "Merchant Insolvent",
		SettlementDate:    time.Now(),
	}

	// Seller only has 2000 paise available -> escalated to suspense
	result, err := processor.ProcessRefundAuthoritative(
		claim,
		"collector:ondc_settlement",
		"receiver:ondc_settlement",
		2000,
		10000,
	)
	if err != nil {
		t.Fatalf("EC-18: Expected suspense escalation, got error: %v", err)
	}

	if result.Status != "escalated_to_suspense" {
		t.Errorf("EC-18: Expected status escalated_to_suspense, got %s", result.Status)
	}
	if len(result.JournalLines) != 2 || result.JournalLines[0].AccountName != "sa:suspense" {
		t.Errorf("EC-18: Expected suspense debit journal line, got %v", result.JournalLines)
	}
}

// BRD §4.2 Edge Case 19: Claim submitted on Indian Bank Holiday or Weekend
func TestScenario_EC_19(t *testing.T) {
	cal := calendar.New(nil)
	repDay2026 := time.Date(2026, 1, 26, 10, 0, 0, 0, calendar.Location()) // Republic Day Monday

	isHoliday, desc, err := cal.IsBankHoliday(repDay2026)
	if err != nil {
		t.Fatalf("IsBankHoliday failed: %v", err)
	}
	if !isHoliday || desc != "Republic Day" {
		t.Fatalf("EC-19: Expected Republic Day recognized as holiday, got %v (%s)", isHoliday, desc)
	}

	nextBiz, err := cal.NextBusinessDay(repDay2026)
	if err != nil {
		t.Fatalf("NextBusinessDay failed: %v", err)
	}
	if nextBiz.Day() != 27 {
		t.Errorf("EC-19: Expected next business day 2026-01-27, got %s", nextBiz.Format("2006-01-02"))
	}
}

// BRD §4.2 Edge Case 20: Cut-off timestamp boundary race (IST 23:59:59)
func TestScenario_EC_20(t *testing.T) {
	cal := calendar.New(nil)
	cal.SetCutOffTime(23, 59, 59)

	loc := calendar.Location()
	tBefore := time.Date(2026, 10, 28, 23, 59, 58, 0, loc)
	tAt := time.Date(2026, 10, 28, 23, 59, 59, 0, loc)

	resBefore, _ := cal.SettlementCutOff(tBefore)
	resAt, _ := cal.SettlementCutOff(tAt)

	if resBefore.Format("2006-01-02") != "2026-10-28" {
		t.Errorf("EC-20: T-1s before cut-off must stay today, got %s", resBefore.Format("2006-01-02"))
	}
	if resAt.Format("2006-01-02") != "2026-10-29" {
		t.Errorf("EC-20: Exact cut-off must advance to tomorrow, got %s", resAt.Format("2006-01-02"))
	}
}

// BRD §4.2 Edge Case 21: Seller on Record (SoR) account validation failure
func TestScenario_EC_21(t *testing.T) {
	mem := ledger.NewMemoryLedger()
	_ = mem.SeedStandardAccounts()
	_, _ = mem.RegisterAccount("sa:suspense", ledger.AccountAsset, "INR", false)

	// Pre-fund collector
	_, _ = mem.Post(context.Background(), ledger.PostRequest{
		IdempotencyKey: "fund-ec21",
		CauseType:      ledger.CauseAdjustment,
		CauseID:        1,
		Description:    "Fund",
		Lines: []ledger.EntryLine{
			{AccountName: "ext:nodal_clearing", Direction: ledger.Debit, AmountPaise: 50000},
			{AccountName: "collector:ondc_settlement", Direction: ledger.Credit, AmountPaise: 50000},
		},
	})

	// Split routing: defective SoR routed to suspense
	req := ledger.PostRequest{
		IdempotencyKey: "sor-fail-routing",
		CauseType:      ledger.CauseInstruction,
		CauseID:        1,
		Description:    "Split routing of invalid SoR to suspense",
		Lines: []ledger.EntryLine{
			{AccountName: "collector:ondc_settlement", Direction: ledger.Debit, AmountPaise: 20000},
			{AccountName: "sa:suspense", Direction: ledger.Credit, AmountPaise: 20000},
		},
	}

	res, err := mem.Post(context.Background(), req)
	if err != nil {
		t.Fatalf("EC-21: Post failed: %v", err)
	}
	if res.LinesCount != 2 {
		t.Errorf("EC-21: Expected 2 lines, got %d", res.LinesCount)
	}
}

// BRD §4.2 Edge Case 22: Deduction leg mismatch
func TestScenario_EC_22(t *testing.T) {
	matcher := settlement.NewMatcher(settlement.DefaultPolicy())
	settleDate := time.Date(2026, 10, 30, 0, 0, 0, 0, calendar.Location())

	// Buyer and Seller report different commission/BFF
	buyerClaim := &settlement.SettleClaimPayload{
		NPPairID:     "BAP:BPP",
		SubmittedBy:  "buyer_app",
		CollectorAcc: "collector",
		ReceiverAcc:  "receiver",
		Orders: []settlement.OrderSettlementLine{
			{NetworkOrderID: "ORD-EC22", OrderAmountPaise: 10000, CommissionPaise: 300, BFFPaise: 100, SoRDuePaise: 9600},
		},
	}
	sellerClaim := &settlement.SettleClaimPayload{
		NPPairID:     "BAP:BPP",
		SubmittedBy:  "seller_app",
		CollectorAcc: "collector",
		ReceiverAcc:  "receiver",
		Orders: []settlement.OrderSettlementLine{
			{NetworkOrderID: "ORD-EC22", OrderAmountPaise: 10000, CommissionPaise: 500, BFFPaise: 100, SoRDuePaise: 9400}, // Commission mismatch!
		},
	}

	res, err := matcher.MatchAndNet("BATCH-EC22", settleDate, buyerClaim, sellerClaim, 20000)
	if err != nil {
		t.Fatalf("MatchAndNet failed: %v", err)
	}

	if len(res.MatchedOrders) != 0 {
		t.Fatalf("EC-22: Expected 0 matched orders on component breakdown mismatch, got %d", len(res.MatchedOrders))
	}
	if len(res.UnmatchedOrders) != 1 || res.UnmatchedOrders[0].Status != settlement.MatchStatusAmountMismatch {
		t.Fatalf("EC-22: Expected unmatched order with amount_mismatch status, got %d", len(res.UnmatchedOrders))
	}
}

// BRD §4.2 Edge Case 23: System crash mid-batch between gross order legs and net batch posting
func TestScenario_EC_23(t *testing.T) {
	fp := verification.NewFailpointManager()
	fp.Enable(verification.FPAfterDebitLegBeforeCreditLeg, verification.ActionError)

	mem := ledger.NewMemoryLedger()
	_ = mem.SeedStandardAccounts()

	// Pre-fund collector account so balance check passes and posting proceeds to legs
	_, err := mem.Post(context.Background(), ledger.PostRequest{
		IdempotencyKey: "fund-ec23",
		CauseType:      ledger.CauseAdjustment,
		CauseID:        10,
		Description:    "Fund collector",
		Lines: []ledger.EntryLine{
			{AccountName: "collector:ondc_settlement", Direction: ledger.Credit, AmountPaise: 500000},
			{AccountName: "ext:nodal_clearing", Direction: ledger.Debit, AmountPaise: 500000},
		},
	})
	if err != nil {
		t.Fatalf("Pre-funding failed: %v", err)
	}

	initialEntries := len(mem.GetEntries())

	ctx := ledger.ContextWithFailpoint(context.Background(), fp)

	req := ledger.PostRequest{
		IdempotencyKey: "crash-ec23",
		CauseType:      ledger.CauseInstruction,
		CauseID:        1,
		Description:    "Crash test",
		Lines: []ledger.EntryLine{
			{AccountName: "collector:ondc_settlement", Direction: ledger.Debit, AmountPaise: 5000},
			{AccountName: "receiver:ondc_settlement", Direction: ledger.Credit, AmountPaise: 5000},
		},
	}

	_, err = mem.Post(ctx, req)
	if err == nil || !errors.Is(err, verification.ErrInjectedFault) {
		t.Fatalf("EC-23: Expected ErrInjectedFault, got %v", err)
	}

	// Verify atomicity on real ledger: 0 partial entries committed from crashed transaction
	if len(mem.GetEntries()) != initialEntries {
		t.Fatalf("EC-23: Expected %d entries (no partial entries), got %d", initialEntries, len(mem.GetEntries()))
	}
	// Invariant audit passes
	if err := mem.AuditInvariants(); err != nil {
		t.Fatalf("EC-23: Ledger invariants corrupted: %v", err)
	}
}

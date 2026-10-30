package adversarial_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ubaid/hisaab/internal/calendar"
	"github.com/ubaid/hisaab/internal/ledger"
	"github.com/ubaid/hisaab/internal/recon"
	"github.com/ubaid/hisaab/internal/settlement"
)

// A1 (B1): A duplicate order ID inside a claim gets paid twice.
func TestA1_DuplicateOrderIDInClaim_PaysTwice(t *testing.T) {
	matcher := settlement.NewMatcher(settlement.DefaultPolicy())

	buyerOrders := []settlement.OrderSettlementLine{
		{
			NetworkOrderID:   "ORD-DUP-01",
			OrderAmountPaise: 100000, // ₹1,000
			CommissionPaise:  1000,
			BFFPaise:         500,
			SoRDuePaise:      98500,
		},
	}

	// Malicious/buggy seller claim with duplicate order
	sellerOrders := []settlement.OrderSettlementLine{
		{
			NetworkOrderID:   "ORD-DUP-01",
			OrderAmountPaise: 100000,
			CommissionPaise:  1000,
			BFFPaise:         500,
			SoRDuePaise:      98500,
		},
		{
			NetworkOrderID:   "ORD-DUP-01", // duplicate!
			OrderAmountPaise: 100000,
			CommissionPaise:  1000,
			BFFPaise:         500,
			SoRDuePaise:      98500,
		},
	}

	buyerClaim := &settlement.SettleClaimPayload{
		SettlementID: "BAP-01",
		NPPairID:     "BAP_01:BPP_01",
		SubmittedBy:  "buyer_app",
		CollectorAcc: "collector:ondc_settlement",
		ReceiverAcc:  "receiver:ondc_settlement",
		Orders:       buyerOrders,
	}

	sellerClaim := &settlement.SettleClaimPayload{
		SettlementID: "BPP-01",
		NPPairID:     "BAP_01:BPP_01",
		SubmittedBy:  "seller_app",
		CollectorAcc: "collector:ondc_settlement",
		ReceiverAcc:  "receiver:ondc_settlement",
		Orders:       sellerOrders,
	}

	// 1. Validation must reject duplicate order in claim
	if err := settlement.ValidateClaimPayload(sellerClaim); err == nil {
		t.Errorf("ValidateClaimPayload should reject duplicate NetworkOrderID in claim payload")
	}

	// 2. Matcher must never match an order more than once or instruct more than buyer collected
	res, err := matcher.MatchAndNet("BATCH-A1", time.Now(), buyerClaim, sellerClaim, 500000)
	if err != nil {
		t.Fatalf("MatchAndNet unexpected error: %v", err)
	}

	if len(res.MatchedOrders) > 1 {
		t.Fatalf("CRITICAL B1: Duplicate order matched twice: %d orders matched", len(res.MatchedOrders))
	}
	var totalInstructed ledger.AmountPaise
	for _, inst := range res.Instructions {
		totalInstructed += inst.AmountPaise
	}
	if totalInstructed > buyerOrders[0].OrderAmountPaise {
		t.Fatalf("CRITICAL B1: Total instructed (%d paise) > total collected from buyer (%d paise)",
			totalInstructed, buyerOrders[0].OrderAmountPaise)
	}
}

// A2 (B2): Non-zero tolerance disables commission/BFF parity checking.
func TestA2_ToleranceBypassesComponentParity(t *testing.T) {
	policy := settlement.MatchingPolicy{
		AmountTolerancePaise: 500, // 5 paise tolerance
		PartialFillAllowed:   false,
		CarryForwardDays:     2,
	}
	matcher := settlement.NewMatcher(policy)

	buyerOrders := []settlement.OrderSettlementLine{
		{
			NetworkOrderID:   "ORD-TOL-01",
			OrderAmountPaise: 100000,
			CommissionPaise:  1000, // ₹10
			BFFPaise:         500,
			SoRDuePaise:      98500,
		},
	}
	// Seller has totally different commission (₹90)
	sellerOrders := []settlement.OrderSettlementLine{
		{
			NetworkOrderID:   "ORD-TOL-01",
			OrderAmountPaise: 100000,
			CommissionPaise:  9000, // ₹90 mismatch!
			BFFPaise:         500,
			SoRDuePaise:      90500,
		},
	}

	buyerClaim := &settlement.SettleClaimPayload{
		SettlementID: "BAP-02",
		NPPairID:     "BAP_01:BPP_01",
		SubmittedBy:  "buyer_app",
		CollectorAcc: "collector:ondc_settlement",
		ReceiverAcc:  "receiver:ondc_settlement",
		Orders:       buyerOrders,
	}
	sellerClaim := &settlement.SettleClaimPayload{
		SettlementID: "BPP-02",
		NPPairID:     "BAP_01:BPP_01",
		SubmittedBy:  "seller_app",
		CollectorAcc: "collector:ondc_settlement",
		ReceiverAcc:  "receiver:ondc_settlement",
		Orders:       sellerOrders,
	}

	res, err := matcher.MatchAndNet("BATCH-A2", time.Now(), buyerClaim, sellerClaim, 500000)
	if err != nil {
		t.Fatalf("MatchAndNet error: %v", err)
	}

	if len(res.MatchedOrders) > 0 {
		t.Fatalf("CRITICAL B2: Order matched despite ₹80 commission mismatch! Non-zero tolerance disabled component check")
	}
	if len(res.UnmatchedOrders) == 0 {
		t.Fatalf("Expected order to be in UnmatchedOrders due to commission mismatch")
	}
}

// A3 (B2): Within-tolerance residual breaks money conservation without ledger leg.
func TestA3_ToleratedResidualPreservesConservation(t *testing.T) {
	policy := settlement.MatchingPolicy{
		AmountTolerancePaise: 5, // 5 paise
		PartialFillAllowed:   false,
		CarryForwardDays:     2,
	}
	matcher := settlement.NewMatcher(policy)

	buyerOrders := []settlement.OrderSettlementLine{
		{
			NetworkOrderID:   "ORD-TOL-02",
			OrderAmountPaise: 100000, // ₹1,000.00
			CommissionPaise:  1000,
			BFFPaise:         500,
			SoRDuePaise:      98500,
		},
	}
	sellerOrders := []settlement.OrderSettlementLine{
		{
			NetworkOrderID:   "ORD-TOL-02",
			OrderAmountPaise: 99995, // 5 paise difference within tolerance
			CommissionPaise:  1000,
			BFFPaise:         500,
			SoRDuePaise:      98495,
		},
	}

	buyerClaim := &settlement.SettleClaimPayload{
		SettlementID: "BAP-03",
		NPPairID:     "BAP_01:BPP_01",
		SubmittedBy:  "buyer_app",
		CollectorAcc: "collector:ondc_settlement",
		ReceiverAcc:  "receiver:ondc_settlement",
		Orders:       buyerOrders,
	}
	sellerClaim := &settlement.SettleClaimPayload{
		SettlementID: "BPP-03",
		NPPairID:     "BAP_01:BPP_01",
		SubmittedBy:  "seller_app",
		CollectorAcc: "collector:ondc_settlement",
		ReceiverAcc:  "receiver:ondc_settlement",
		Orders:       sellerOrders,
	}

	res, err := matcher.MatchAndNet("BATCH-A3", time.Now(), buyerClaim, sellerClaim, 500000)
	if err != nil {
		t.Fatalf("MatchAndNet error: %v", err)
	}

	if len(res.MatchedOrders) == 1 {
		if res.TotalMatched != buyerOrders[0].OrderAmountPaise {
			t.Fatalf("CRITICAL B2: Money conservation broken! Buyer collected %d paise, but TotalMatched is %d paise. 5 paise vanished with no ledger leg",
				buyerOrders[0].OrderAmountPaise, res.TotalMatched)
		}
	}
}

// A4 (B3a): Refund cap cannot be fabricated by claim payload.
func TestA4_RefundCapCannotBeFabricatedByClaim(t *testing.T) {
	proc := settlement.NewRefundProcessor()

	// Authoritative store recorded this order as settled for ₹1,000 (100,000 paise)
	proc.RegisterSettledOrder("ORD-SETTLED-1000", 100000)

	// Malicious claim claims original amount was ₹10 crore, requests ₹9 crore refund on ₹1000 order
	claim := settlement.RefundClaim{
		MessageID:         "REF-01",
		NetworkOrderID:    "ORD-SETTLED-1000",
		OriginalAmount:    1000000000, // ₹10 crore fabricated!
		RefundAmountPaise: 900000000,  // ₹9 crore!
		SettlementDate:    time.Now(),
	}

	res, err := proc.ProcessRefund(claim, "collector:ondc_settlement", "receiver:ondc_settlement", 1000000000)
	if err == nil && res.Status == "settled" {
		t.Fatalf("CRITICAL B3a: Refund allowed based on claim-supplied OriginalAmount! Fabricated ₹9 crore refund was accepted.")
	}
}

// A5 (B3b): Insolvent seller (suspense) path doesn't update cumulative refunds.
func TestA5_InsolventSellerSuspenseRefundUnlimitedReplay(t *testing.T) {
	proc := settlement.NewRefundProcessor()

	claim1 := settlement.RefundClaim{
		MessageID:         "REF-SUSP-1",
		NetworkOrderID:    "ORD-SUSP-01",
		OriginalAmount:    100000, // ₹1,000
		RefundAmountPaise: 60000,  // ₹600
		SettlementDate:    time.Now(),
	}

	// Seller balance is 0 => routes to suspense
	res1, err := proc.ProcessRefund(claim1, "collector:ondc_settlement", "receiver:ondc_settlement", 0)
	if err != nil {
		t.Fatalf("ProcessRefund 1 failed: %v", err)
	}
	if res1.Status != "escalated_to_suspense" {
		t.Fatalf("Expected escalated_to_suspense, got %s", res1.Status)
	}

	// Second refund of ₹600 on same ₹1000 order => cumulative ₹1200 > ₹1000!
	claim2 := settlement.RefundClaim{
		MessageID:         "REF-SUSP-2",
		NetworkOrderID:    "ORD-SUSP-01",
		OriginalAmount:    100000,
		RefundAmountPaise: 60000,
		SettlementDate:    time.Now(),
	}

	_, err = proc.ProcessRefund(claim2, "collector:ondc_settlement", "receiver:ondc_settlement", 0)
	if err == nil {
		t.Fatalf("CRITICAL B3b: Second suspense refund succeeded! Suspense refunds bypassed cumulative refund tracking (Inv-6)")
	}
}

// A6 (B3c): Suspense account routing fails ErrAccountNotFound without sa:suspense in SeedStandardAccounts.
func TestA6_SuspenseAccountMissingFromStandardAccounts(t *testing.T) {
	ledg := ledger.NewMemoryLedger()
	if err := ledg.SeedStandardAccounts(); err != nil {
		t.Fatalf("SeedStandardAccounts failed: %v", err)
	}

	// Try to post journal lines generated by suspense refund
	postReq := ledger.PostRequest{
		IdempotencyKey: "POST-SUSP-01",
		CauseType:      "refund",
		CauseID:        1,
		Description:    "Suspense routing test",
		Lines: []ledger.EntryLine{
			{AccountName: "sa:suspense", Direction: ledger.Debit, AmountPaise: 50000},
			{AccountName: "collector:ondc_settlement", Direction: ledger.Credit, AmountPaise: 50000},
		},
	}

	_, err := ledg.Post(context.Background(), postReq)
	if err != nil {
		t.Fatalf("CRITICAL B3c: Posting suspense refund failed: %v (sa:suspense not in SeedStandardAccounts)", err)
	}
}

// A7 (B5): Integer overflow in ValidateLines.
func TestA7_IntegerOverflowInValidateLines(t *testing.T) {
	// 4 lines of 2^62 paise debit = 2^64 = 0 mod 2^64
	// 8 lines of 2^62 paise credit = 2^65 = 0 mod 2^64
	huge := ledger.AmountPaise(1) << 62

	lines := []ledger.EntryLine{
		{AccountName: "acc1", Direction: ledger.Debit, AmountPaise: huge},
		{AccountName: "acc1", Direction: ledger.Debit, AmountPaise: huge},
		{AccountName: "acc1", Direction: ledger.Debit, AmountPaise: huge},
		{AccountName: "acc1", Direction: ledger.Debit, AmountPaise: huge},

		{AccountName: "acc2", Direction: ledger.Credit, AmountPaise: huge},
		{AccountName: "acc2", Direction: ledger.Credit, AmountPaise: huge},
		{AccountName: "acc2", Direction: ledger.Credit, AmountPaise: huge},
		{AccountName: "acc2", Direction: ledger.Credit, AmountPaise: huge},
		{AccountName: "acc2", Direction: ledger.Credit, AmountPaise: huge},
		{AccountName: "acc2", Direction: ledger.Credit, AmountPaise: huge},
		{AccountName: "acc2", Direction: ledger.Credit, AmountPaise: huge},
		{AccountName: "acc2", Direction: ledger.Credit, AmountPaise: huge},
	}

	_, err := ledger.ValidateLines(lines)
	if err == nil {
		t.Fatalf("CRITICAL B5: ValidateLines accepted integer overflow where debits and credits differ by 2^64!")
	}
}

// A8 (B5): Request hash concatenation collision without delimiters.
func TestA8_ComputeRequestHashConcatenationCollision(t *testing.T) {
	req1 := ledger.PostRequest{
		IdempotencyKey: "k1",
		CauseType:      "test",
		CauseID:        1,
		Description:    "2x",
		Lines: []ledger.EntryLine{
			{AccountName: "acc1", Direction: ledger.Debit, AmountPaise: 100},
			{AccountName: "acc2", Direction: ledger.Credit, AmountPaise: 100},
		},
	}

	req2 := ledger.PostRequest{
		IdempotencyKey: "k1",
		CauseType:      "test",
		CauseID:        12,
		Description:    "x",
		Lines: []ledger.EntryLine{
			{AccountName: "acc1", Direction: ledger.Debit, AmountPaise: 100},
			{AccountName: "acc2", Direction: ledger.Credit, AmountPaise: 100},
		},
	}

	hash1 := ledger.ComputeRequestHash(req1)
	hash2 := ledger.ComputeRequestHash(req2)

	if hash1 == hash2 {
		t.Fatalf("CRITICAL B5: Request hash collision! Hash1 == Hash2 (%s) due to lack of delimiters", hash1)
	}
}

// A9: Duplicate detection in recon.Reconcile.
func TestA9_ReconDuplicateRecordsNotOverwritten(t *testing.T) {
	cal := calendar.New(nil)
	reconEngine := recon.NewEngine(cal)

	orderBook := []recon.OrderBookLine{
		{
			NetworkOrderID: "ORD-DUP-RECON",
			AmountPaise:    10000,
			Commission:     1000,
			BFF:            500,
			CreatedAt:      time.Now(),
		},
	}
	// Two bank credits with same ReferenceID / UTR (duplicate credit from bank)
	bankStatement := []recon.BankStatementLine{
		{
			ReferenceID: "ORD-DUP-RECON",
			UTR:         "UTR-001",
			Direction:   ledger.Credit,
			AmountPaise: 10000,
			ValueDate:   time.Now(),
		},
		{
			ReferenceID: "ORD-DUP-RECON", // duplicate credit!
			UTR:         "UTR-002",
			Direction:   ledger.Credit,
			AmountPaise: 10000,
			ValueDate:   time.Now(),
		},
	}
	saReport := []recon.SAReportLine{
		{
			BatchID:        "BATCH-01",
			NetworkOrderID: "ORD-DUP-RECON",
			SettledAmount:  10000,
			Status:         "settled",
			SettledAt:      time.Now(),
		},
	}

	result := reconEngine.Reconcile(orderBook, saReport, bankStatement, time.Now())

	hasDupCredit := false
	for _, exc := range result.Exceptions {
		if exc.Class == recon.ExceptionDuplicateCredit {
			hasDupCredit = true
			break
		}
	}
	if !hasDupCredit {
		t.Fatalf("CRITICAL B1/recon: Duplicate bank credit line silently overwritten; ExceptionDuplicateCredit was not raised")
	}
}

// A10 (B3d): Data race on settledRefunds map in RefundProcessor.
func TestA10_RefundProcessorDataRace(t *testing.T) {
	proc := settlement.NewRefundProcessor()

	var wg sync.WaitGroup
	// Concurrently read and write settledRefunds map
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			claim := settlement.RefundClaim{
				MessageID:         fmt.Sprintf("REF-%d", idx),
				NetworkOrderID:    "ORD-CONCURRENT-REFUND",
				OriginalAmount:    10000000,
				RefundAmountPaise: 10,
				SettlementDate:    time.Now(),
			}
			_, _ = proc.ProcessRefund(claim, "collector:ondc_settlement", "receiver:ondc_settlement", 50000000)
		}(i)
	}
	wg.Wait()
}

// A11: Concurrency test on in-memory ledger (The 1 that passes).
func TestA11_MemoryLedgerConcurrencySafe(t *testing.T) {
	ledg := ledger.NewMemoryLedger()
	if err := ledg.SeedStandardAccounts(); err != nil {
		t.Fatalf("SeedStandardAccounts failed: %v", err)
	}

	// Fund collector with 100 paise
	fundReq := ledger.PostRequest{
		IdempotencyKey: "FUND-01",
		CauseType:      "funding",
		CauseID:        1,
		Description:    "Initial funding",
		Lines: []ledger.EntryLine{
			{AccountName: "ext:nodal_clearing", Direction: ledger.Debit, AmountPaise: 100},
			{AccountName: "collector:ondc_settlement", Direction: ledger.Credit, AmountPaise: 100},
		},
	}
	if _, err := ledg.Post(context.Background(), fundReq); err != nil {
		t.Fatalf("Fund failed: %v", err)
	}

	// 50 goroutines attempting to withdraw 80 paise each
	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req := ledger.PostRequest{
				IdempotencyKey: fmt.Sprintf("WITHDRAW-%d", idx),
				CauseType:      "settlement",
				CauseID:        int64(idx + 100),
				Description:    "Concurrent withdrawal",
				Lines: []ledger.EntryLine{
					{AccountName: "collector:ondc_settlement", Direction: ledger.Debit, AmountPaise: 80},
					{AccountName: "receiver:ondc_settlement", Direction: ledger.Credit, AmountPaise: 80},
				},
			}
			_, err := ledg.Post(context.Background(), req)
			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	bal, err := ledg.GetBalance("collector:ondc_settlement")
	if err != nil {
		t.Fatalf("Balance failed: %v", err)
	}
	if bal < 0 {
		t.Fatalf("Overdraft occurred! Balance: %d", bal)
	}
	if successCount > 1 {
		t.Fatalf("More than 1 withdrawal succeeded: %d (balance would be negative)", successCount)
	}
	t.Logf("MemoryLedger passed concurrency overdraft test: successCount=%d, remaining balance=%d", successCount, bal)
}

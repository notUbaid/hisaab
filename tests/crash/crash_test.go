package crash_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ubaid/hisaab/internal/calendar"
	"github.com/ubaid/hisaab/internal/ledger"
	"github.com/ubaid/hisaab/internal/recon"
	"github.com/ubaid/hisaab/internal/settlement"
	"github.com/ubaid/hisaab/internal/verification"
)

// 1. FPAfterDebitLegBeforeCreditLeg: Mid-posting crash rolls back completely (Atomicity)
func TestCrashPoint_FPAfterDebitLegBeforeCreditLeg(t *testing.T) {
	fp := verification.NewFailpointManager()

	mem := ledger.NewMemoryLedger()
	_ = mem.SeedStandardAccounts()
	_, _ = mem.RegisterAccount("sa:suspense", ledger.AccountAsset, "INR", false)

	// Pre-fund collector account
	_, err := mem.Post(context.Background(), ledger.PostRequest{
		IdempotencyKey: "fund-crash-1",
		CauseType:      ledger.CauseAdjustment,
		CauseID:        1,
		Description:    "Fund collector",
		Lines: []ledger.EntryLine{
			{AccountName: "ext:nodal_clearing", Direction: ledger.Debit, AmountPaise: 500000},
			{AccountName: "collector:ondc_settlement", Direction: ledger.Credit, AmountPaise: 500000},
		},
	})
	if err != nil {
		t.Fatalf("Pre-funding failed: %v", err)
	}

	initialEntriesCount := len(mem.GetEntries())

	// Arm failpoint
	fp.Enable(verification.FPAfterDebitLegBeforeCreditLeg, verification.ActionError)
	ctx := ledger.ContextWithFailpoint(context.Background(), fp)

	lines := []ledger.EntryLine{
		{AccountName: "collector:ondc_settlement", Direction: ledger.Debit, AmountPaise: 50000},
		{AccountName: "receiver:ondc_settlement", Direction: ledger.Credit, AmountPaise: 50000},
	}

	req := ledger.PostRequest{
		IdempotencyKey: "crash-idemp-1",
		CauseType:      ledger.CauseInstruction,
		CauseID:        2,
		Description:    "Test crash mid-tx",
		Lines:          lines,
	}

	// 1. First execution should fail due to injected failpoint
	_, err = mem.Post(ctx, req)
	if err == nil || !errors.Is(err, verification.ErrInjectedFault) {
		t.Fatalf("Expected ErrInjectedFault, got: %v", err)
	}

	// Verify atomicity on real ledger: ZERO new journal entries committed
	entries := mem.GetEntries()
	if len(entries) != initialEntriesCount {
		t.Fatalf("Atomicity violated: found %d journal entries in ledger after mid-tx crash", len(entries))
	}

	// Verify invariant audit passes (no corrupted state)
	if err := mem.AuditInvariants(); err != nil {
		t.Fatalf("Ledger invariant audit failed after crash: %v", err)
	}

	// 2. Disarm failpoint and retry identical request (Crash Recovery)
	fp.Disable(verification.FPAfterDebitLegBeforeCreditLeg)
	result, err := mem.Post(ctx, req)
	if err != nil {
		t.Fatalf("Crash recovery retry failed: %v", err)
	}

	if result.LinesCount != 2 {
		t.Errorf("Expected 2 lines committed on recovery, got %d", result.LinesCount)
	}
	if len(mem.GetEntries()) != initialEntriesCount+2 {
		t.Errorf("Expected %d entries in ledger, got %d", initialEntriesCount+2, len(mem.GetEntries()))
	}
}

// 2. FPAfterCommitBeforeACK: Crash between commit and ACK response
func TestCrashPoint_FPAfterCommitBeforeACK(t *testing.T) {
	fp := verification.NewFailpointManager()

	mem := ledger.NewMemoryLedger()
	_ = mem.SeedStandardAccounts()
	_, _ = mem.RegisterAccount("sa:suspense", ledger.AccountAsset, "INR", false)

	// Pre-fund collector account
	_, err := mem.Post(context.Background(), ledger.PostRequest{
		IdempotencyKey: "fund-crash-2",
		CauseType:      ledger.CauseAdjustment,
		CauseID:        1,
		Description:    "Fund collector",
		Lines: []ledger.EntryLine{
			{AccountName: "ext:nodal_clearing", Direction: ledger.Debit, AmountPaise: 500000},
			{AccountName: "collector:ondc_settlement", Direction: ledger.Credit, AmountPaise: 500000},
		},
	})
	if err != nil {
		t.Fatalf("Pre-funding failed: %v", err)
	}

	initialEntriesCount := len(mem.GetEntries())

	fp.Enable(verification.FPAfterCommitBeforeACK, verification.ActionError)
	ctx := ledger.ContextWithFailpoint(context.Background(), fp)

	lines := []ledger.EntryLine{
		{AccountName: "collector:ondc_settlement", Direction: ledger.Debit, AmountPaise: 100000},
		{AccountName: "receiver:ondc_settlement", Direction: ledger.Credit, AmountPaise: 100000},
	}

	req := ledger.PostRequest{
		IdempotencyKey: "crash-idemp-ack",
		CauseType:      ledger.CauseInstruction,
		CauseID:        2,
		Description:    "Test commit success but caller receives network drop",
		Lines:          lines,
	}

	// 1. Post succeeds internally in ledger, but returns error at ACK boundary
	_, err = mem.Post(ctx, req)
	if err == nil || !errors.Is(err, verification.ErrInjectedFault) {
		t.Fatalf("Expected ErrInjectedFault, got: %v", err)
	}

	// Entries were committed to storage
	if len(mem.GetEntries()) != initialEntriesCount+2 {
		t.Fatalf("Expected %d entries committed before ACK drop, got: %d", initialEntriesCount+2, len(mem.GetEntries()))
	}

	// 2. Caller retries with identical idempotency key after clearing network fault
	fp.Disable(verification.FPAfterCommitBeforeACK)
	resRetry, err := mem.Post(ctx, req)
	if err != nil {
		t.Fatalf("Idempotent retry failed: %v", err)
	}

	if resRetry.IdempotencyKey != "crash-idemp-ack" {
		t.Errorf("Expected cached idempotency key, got %s", resRetry.IdempotencyKey)
	}

	// Verify Invariant 3: Exactly-once semantics, NO duplicate lines
	if len(mem.GetEntries()) != initialEntriesCount+2 {
		t.Fatalf("Invariant 3 violated: duplicate entries created on retry: %d entries", len(mem.GetEntries()))
	}
}

// 3. FPAfterSubLedgerBeforeNetBatch: Crash before net instruction batching
func TestCrashPoint_FPAfterSubLedgerBeforeNetBatch(t *testing.T) {
	fp := verification.NewFailpointManager()
	fp.Enable(verification.FPAfterSubLedgerBeforeNetBatch, verification.ActionError)

	mem := ledger.NewMemoryLedger()
	_ = mem.SeedStandardAccounts()
	cal := calendar.New(nil)
	matcher := settlement.NewMatcher(settlement.DefaultPolicy())

	orch := settlement.NewOrchestrator(mem, matcher, cal, nil, fp)

	ctx := context.Background()

	buyerClaim := &settlement.SettleClaimPayload{
		Context:      &settlement.BecknContext{Action: "settle", MessageID: "msg-fp3-buyer"},
		NPPairID:     "BAP:BPP",
		SubmittedBy:  "buyer_app",
		CollectorAcc: "collector:ondc_settlement",
		ReceiverAcc:  "receiver:ondc_settlement",
		Orders: []settlement.OrderSettlementLine{
			{NetworkOrderID: "ORD-FP3", OrderAmountPaise: 50000, CommissionPaise: 1500, BFFPaise: 500, SoRDuePaise: 48000},
		},
	}
	_, _ = orch.SubmitClaim(ctx, buyerClaim)

	sellerClaim := &settlement.SettleClaimPayload{
		Context:      &settlement.BecknContext{Action: "settle", MessageID: "msg-fp3-seller"},
		NPPairID:     "BAP:BPP",
		SubmittedBy:  "seller_app",
		CollectorAcc: "collector:ondc_settlement",
		ReceiverAcc:  "receiver:ondc_settlement",
		Orders: []settlement.OrderSettlementLine{
			{NetworkOrderID: "ORD-FP3", OrderAmountPaise: 50000, CommissionPaise: 1500, BFFPaise: 500, SoRDuePaise: 48000},
		},
	}

	// Fault injected before matching/batching
	_, err := orch.SubmitClaim(ctx, sellerClaim)
	if err == nil {
		t.Fatal("Expected error at FPAfterSubLedgerBeforeNetBatch, got nil")
	}

	// Verify no orphaned batches or outbox items
	if len(orch.ListBatches()) != 0 {
		t.Errorf("Expected 0 batches created after mid-flight crash, got %d", len(orch.ListBatches()))
	}
	if len(orch.GetPendingOutbox()) != 0 {
		t.Errorf("Expected 0 outbox items, got %d", len(orch.GetPendingOutbox()))
	}
}

// 4. FPMidBatchDatabaseSever: Disconnection during active query
func TestCrashPoint_FPMidBatchDatabaseSever(t *testing.T) {
	mem := ledger.NewMemoryLedger()
	_ = mem.SeedStandardAccounts()

	// Cancel context to simulate database connection sever
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Sever immediately

	req := ledger.PostRequest{
		IdempotencyKey: "sever-test",
		CauseType:      ledger.CauseInstruction,
		CauseID:        1,
		Description:    "Severed connection",
		Lines: []ledger.EntryLine{
			{AccountName: "collector:ondc_settlement", Direction: ledger.Debit, AmountPaise: 10000},
			{AccountName: "receiver:ondc_settlement", Direction: ledger.Credit, AmountPaise: 10000},
		},
	}

	_, err := mem.Post(ctx, req)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Expected context.Canceled on connection sever, got: %v", err)
	}

	if len(mem.GetEntries()) != 0 {
		t.Fatalf("Expected 0 entries committed on severed connection, got %d", len(mem.GetEntries()))
	}
}

// 5. FPDuplicateCreditArrival: Duplicate bank credit arrival in recon
func TestCrashPoint_FPDuplicateCreditArrival(t *testing.T) {
	cal := calendar.New(nil)
	reconEng := recon.NewEngine(cal)

	orderBook := []recon.OrderBookLine{
		{NetworkOrderID: "ORD-DUP", AmountPaise: 10000, CreatedAt: time.Date(2026, 10, 30, 0, 0, 0, 0, calendar.Location())},
	}
	saReport := []recon.SAReportLine{
		{NetworkOrderID: "ORD-DUP", SettledAmount: 10000, BatchID: "BATCH-DUP", SettledAt: time.Date(2026, 10, 30, 0, 0, 0, 0, calendar.Location())},
	}

	// Two identical bank credits for the same order (simulating duplicate credit notification crash)
	bankStmt := []recon.BankStatementLine{
		{ReferenceID: "ORD-DUP", UTR: "UTR-1", Direction: ledger.Credit, AmountPaise: 10000, ValueDate: time.Date(2026, 10, 30, 0, 0, 0, 0, calendar.Location())},
		{ReferenceID: "ORD-DUP", UTR: "UTR-1", Direction: ledger.Credit, AmountPaise: 10000, ValueDate: time.Date(2026, 10, 30, 0, 0, 0, 0, calendar.Location())},
	}

	res := reconEng.Reconcile(orderBook, saReport, bankStmt, time.Date(2026, 10, 30, 15, 0, 0, 0, calendar.Location()))

	// Must flag duplicate credit as an exception instead of double-crediting
	if len(res.Exceptions) != 1 {
		t.Fatalf("Expected 1 recon exception for duplicate bank credit, got %d", len(res.Exceptions))
	}
	if res.Exceptions[0].Class != recon.ExceptionDuplicateCredit {
		t.Errorf("Expected exception class DUPLICATE_CREDIT, got %s", res.Exceptions[0].Class)
	}
}

// 6. FPClockSkewMidnight: Deterministic handling across cut-off boundary with timing skew
func TestCrashPoint_FPClockSkewMidnight(t *testing.T) {
	cal := calendar.New(nil)
	cal.SetCutOffTime(23, 59, 59)

	loc := calendar.Location()
	// T - 10ms (just before cut-off): assigned to current day
	tBefore := time.Date(2026, 10, 28, 23, 59, 58, 990000000, loc)
	resBefore, err := cal.SettlementCutOff(tBefore)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if resBefore.Format("2006-01-02") != "2026-10-28" {
		t.Errorf("Expected 2026-10-28, got %s", resBefore.Format("2006-01-02"))
	}

	// T + 10ms (just after cut-off): assigned to next business day
	tAfter := time.Date(2026, 10, 28, 23, 59, 59, 10000000, loc)
	resAfter, err := cal.SettlementCutOff(tAfter)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if resAfter.Format("2006-01-02") != "2026-10-29" {
		t.Errorf("Expected 2026-10-29, got %s", resAfter.Format("2006-01-02"))
	}
}

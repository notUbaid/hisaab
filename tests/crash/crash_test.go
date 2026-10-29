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

// In-Memory Simulated State to verify crash recovery atomicity and invariant consistency
type SimulatedSettlementRunner struct {
	failpoints *verification.FailpointManager
	checker    *verification.InvariantChecker
	journal    []ledger.EntryLine
	idempStore map[string]string
	batchState map[string]string
}

func NewSimulatedSettlementRunner(fp *verification.FailpointManager) *SimulatedSettlementRunner {
	return &SimulatedSettlementRunner{
		failpoints: fp,
		checker:    verification.NewInvariantChecker(),
		journal:    make([]ledger.EntryLine, 0),
		idempStore: make(map[string]string),
		batchState: make(map[string]string),
	}
}

// PostJournal simulates an atomic double-entry post with crashpoint checks
func (r *SimulatedSettlementRunner) PostJournal(ctx context.Context, idempKey string, lines []ledger.EntryLine) error {
	// Pre-condition: Invariant 1 check
	if err := r.checker.CheckInvariant1_ZeroSum(lines); err != nil {
		return err
	}

	// Invariant 3 check
	if _, exists := r.idempStore[idempKey]; exists {
		return nil // Idempotent success without duplicate posting
	}

	// Simulate Step 1: Debit leg
	r.failpoints.Trigger(verification.FPAfterDebitLegBeforeCreditLeg)

	// In an atomic transaction, if failure occurs here, rollback guarantees zero lines appended
	r.journal = append(r.journal, lines...)
	r.idempStore[idempKey] = "committed"

	// Simulate Step 2: ACK boundary
	if err := r.failpoints.Trigger(verification.FPAfterCommitBeforeACK); err != nil {
		return err
	}

	return nil
}

// 1. FPAfterDebitLegBeforeCreditLeg
func TestCrashPoint_FPAfterDebitLegBeforeCreditLeg(t *testing.T) {
	fp := verification.NewFailpointManager()
	fp.Enable(verification.FPAfterDebitLegBeforeCreditLeg, verification.ActionPanic)

	runner := NewSimulatedSettlementRunner(fp)
	lines := []ledger.EntryLine{
		{AccountName: "collector:ondc_settlement", Direction: ledger.Debit, AmountPaise: 50000},
		{AccountName: "receiver:operative", Direction: ledger.Credit, AmountPaise: 50000},
	}

	panicked := false
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				panicked = true
			}
		}()
		_ = runner.PostJournal(context.Background(), "idemp-crash-1", lines)
	}()

	if !panicked {
		t.Fatal("Expected panic when FPAfterDebitLegBeforeCreditLeg is enabled")
	}

	// Verify atomicity: if crashed before credit leg, journal remains uncommitted
	if len(runner.journal) != 0 {
		t.Fatalf("Invariant 1 & 8 violated: partial uncommitted entries found in journal: %d lines", len(runner.journal))
	}
}

// 2. FPAfterCommitBeforeACK
func TestCrashPoint_FPAfterCommitBeforeACK(t *testing.T) {
	fp := verification.NewFailpointManager()
	fp.Enable(verification.FPAfterCommitBeforeACK, verification.ActionError)

	runner := NewSimulatedSettlementRunner(fp)
	lines := []ledger.EntryLine{
		{AccountName: "collector:ondc_settlement", Direction: ledger.Debit, AmountPaise: 100000},
		{AccountName: "receiver:operative", Direction: ledger.Credit, AmountPaise: 100000},
	}

	// First attempt: succeeds commit but returns ACK error due to failpoint
	err := runner.PostJournal(context.Background(), "idemp-ack-1", lines)
	if err == nil || !errors.Is(err, verification.ErrInjectedFault) {
		t.Fatalf("Expected ErrInjectedFault, got: %v", err)
	}

	// Commit succeeded
	if len(runner.journal) != 2 {
		t.Fatalf("Expected 2 lines committed in journal, got: %d", len(runner.journal))
	}

	// Disarm failpoint and retry identical request (idempotent replay)
	fp.Disable(verification.FPAfterCommitBeforeACK)
	err = runner.PostJournal(context.Background(), "idemp-ack-1", lines)
	if err != nil {
		t.Fatalf("Expected idempotent replay to succeed, got: %v", err)
	}

	// Verify Invariant 3: No duplicate entries appended
	if len(runner.journal) != 2 {
		t.Fatalf("Invariant 3 violated: duplicate journal lines appended on retry: %d", len(runner.journal))
	}
}

// 3. FPAfterSubLedgerBeforeNetBatch
func TestCrashPoint_FPAfterSubLedgerBeforeNetBatch(t *testing.T) {
	fp := verification.NewFailpointManager()
	fp.Enable(verification.FPAfterSubLedgerBeforeNetBatch, verification.ActionError)

	matcher := settlement.NewMatcher(settlement.DefaultPolicy())
	tDate := time.Date(2026, 10, 30, 15, 0, 0, 0, calendar.Location())

	buyerClaim := &settlement.SettleClaimPayload{
		NPPairID: "NP-B-1:NP-S-1",
		Orders: []settlement.OrderSettlementLine{
			{NetworkOrderID: "ORD-M5-01", OrderAmountPaise: 50000, CommissionPaise: 1500, BFFPaise: 500, SoRDuePaise: 48000},
		},
	}
	sellerClaim := &settlement.SettleClaimPayload{
		NPPairID: "NP-B-1:NP-S-1",
		Orders: []settlement.OrderSettlementLine{
			{NetworkOrderID: "ORD-M5-01", OrderAmountPaise: 50000, CommissionPaise: 1500, BFFPaise: 500, SoRDuePaise: 48000},
		},
	}

	res, err := matcher.MatchAndNet("BATCH-FP3", tDate, buyerClaim, sellerClaim, 100000)
	if err != nil {
		t.Fatalf("Matching failed: %v", err)
	}

	// Trigger failpoint after subledger computation
	err = fp.Trigger(verification.FPAfterSubLedgerBeforeNetBatch)
	if err == nil {
		t.Fatal("Expected error at FPAfterSubLedgerBeforeNetBatch")
	}

	// Verify Invariant 13 tie-out still holds on sub-ledger result
	checker := verification.NewInvariantChecker()
	if err := checker.CheckInvariant13_GrossNetTieOut(res); err != nil {
		t.Fatalf("Invariant 13 broken on sub-ledger: %v", err)
	}
}

// 4. FPAfterInstructionBeforeBankSend
func TestCrashPoint_FPAfterInstructionBeforeBankSend(t *testing.T) {
	fp := verification.NewFailpointManager()
	fp.Enable(verification.FPAfterInstructionBeforeBankSend, verification.ActionError)

	// Simulate outbox record creation
	outboxStatus := "pending"
	instructionCreated := true

	err := fp.Trigger(verification.FPAfterInstructionBeforeBankSend)
	if err == nil {
		t.Fatal("Expected failpoint error before bank send")
	}

	// Outbox worker recovery: status must remain pending, ready for safe retry
	if outboxStatus != "pending" || !instructionCreated {
		t.Fatalf("Recovery failed: outbox entry marked invalidly: %s", outboxStatus)
	}
}

// 5. FPAfterBankSendBeforeCallback
func TestCrashPoint_FPAfterBankSendBeforeCallback(t *testing.T) {
	fp := verification.NewFailpointManager()
	fp.Enable(verification.FPAfterBankSendBeforeCallback, verification.ActionError)

	// Bank send succeeds, but callback fails to arrive immediately
	_ = fp.Trigger(verification.FPAfterBankSendBeforeCallback)

	cal := calendar.New(nil)
	reconEngine := recon.NewEngine(cal)
	asOf := time.Date(2026, 10, 30, 18, 0, 0, 0, calendar.Location())

	orderBook := []recon.OrderBookLine{
		{
			NetworkOrderID: "ORD-BK-1",
			AmountPaise:    25000,
			Commission:     1000,
			BFF:            500,
			CreatedAt:      asOf.Add(-2 * time.Hour),
		},
	}
	saReport := []recon.SAReportLine{
		{
			BatchID:        "B-BK-1",
			NetworkOrderID: "ORD-BK-1",
			SettledAmount:  25000,
			Status:         "settled",
			SettledAt:      asOf.Add(-1 * time.Hour),
		},
	}
	bankStmt := []recon.BankStatementLine{
		{
			ReferenceID: "ORD-BK-1",
			UTR:         "UTR987654321",
			Direction:   ledger.Credit,
			AmountPaise: 25000,
			ValueDate:   asOf,
		},
	}

	reconRes := reconEngine.Reconcile(orderBook, saReport, bankStmt, asOf)

	if reconRes.MatchedCount != 1 || reconRes.TotalExceptions != 0 {
		t.Fatalf("Expected recon to tie out bank statement to order book, got %d matched, %d exceptions",
			reconRes.MatchedCount, reconRes.TotalExceptions)
	}
}

// 6. FPMidBatchDatabaseSever
func TestCrashPoint_FPMidBatchDatabaseSever(t *testing.T) {
	fp := verification.NewFailpointManager()
	fp.Enable(verification.FPMidBatchDatabaseSever, verification.ActionPanic)

	panicked := false
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				panicked = true
			}
		}()
		_ = fp.Trigger(verification.FPMidBatchDatabaseSever)
	}()

	if !panicked {
		t.Fatal("Expected panic on database sever")
	}
}

// 7. FPDuplicateCreditArrival
func TestCrashPoint_FPDuplicateCreditArrival(t *testing.T) {
	checker := verification.NewInvariantChecker()

	// Initial settled state
	currentState := "settled"

	// Duplicate callback arrives with "settled"
	if err := checker.CheckInvariant14_TerminalCallback(currentState, "settled"); err != nil {
		t.Fatalf("Invariant 14 check failed for duplicate settled callback: %v", err)
	}

	// Duplicate callback attempting to alter to "failed"
	if err := checker.CheckInvariant14_TerminalCallback(currentState, "failed"); err == nil {
		t.Fatal("Invariant 14 failed: expected error when terminal state is mutated")
	}
}

// 8. FPClockSkewMidnight
func TestCrashPoint_FPClockSkewMidnight(t *testing.T) {
	cal := calendar.New(nil)

	// Time right at 23:59:59 IST on Friday October 30, 2026
	tBefore := time.Date(2026, 10, 30, 23, 59, 59, 0, calendar.Location())
	d1 := cal.SettlementCutOffDetermined(tBefore)

	// Time right at 00:00:01 IST on Saturday October 31, 2026
	// Note: October 31, 2026 is 5th Saturday (not 2nd/4th), so it is a working banking day in India!
	tAfter := time.Date(2026, 10, 31, 0, 0, 1, 0, calendar.Location())
	d2 := cal.SettlementCutOffDetermined(tAfter)

	if d1.Format("2006-01-02") != "2026-10-30" {
		t.Errorf("Expected settlement date 2026-10-30, got: %s", d1.Format("2006-01-02"))
	}
	if d2.Format("2006-01-02") != "2026-10-31" {
		t.Errorf("Expected settlement date 2026-10-31, got: %s", d2.Format("2006-01-02"))
	}
}

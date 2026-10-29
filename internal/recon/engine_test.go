package recon_test

import (
	"testing"
	"time"

	"github.com/ubaid/hisaab/internal/calendar"
	"github.com/ubaid/hisaab/internal/ledger"
	"github.com/ubaid/hisaab/internal/recon"
)

func TestRecon_ThreeWay_CleanMatch(t *testing.T) {
	engine := recon.NewEngine(calendar.New(nil))
	tNow := time.Date(2026, 10, 30, 12, 0, 0, 0, calendar.Location())

	orderBook := []recon.OrderBookLine{
		{
			NetworkOrderID: "ORD-101",
			AmountPaise:    10000,
			Commission:     1000,
			BFF:            500,
			CreatedAt:      tNow.Add(-24 * time.Hour),
		},
	}

	saReport := []recon.SAReportLine{
		{
			BatchID:        "BATCH-01",
			NetworkOrderID: "ORD-101",
			SettledAmount:  10000,
			Status:         "settled",
			SettledAt:      tNow.Add(-12 * time.Hour),
		},
	}

	bankStatement := []recon.BankStatementLine{
		{
			ReferenceID: "ORD-101",
			UTR:         "UTR1234567890",
			Direction:   ledger.Credit,
			AmountPaise: 10000,
			ValueDate:   tNow.Add(-6 * time.Hour),
		},
	}

	result := engine.Reconcile(orderBook, saReport, bankStatement, tNow)

	if result.TotalExceptions != 0 {
		t.Fatalf("Expected 0 exceptions for clean match, got %d: %+v", result.TotalExceptions, result.Exceptions)
	}
	if result.MatchedCount != 1 {
		t.Errorf("Expected 1 matched order, got %d", result.MatchedCount)
	}
}

func TestRecon_BreaksDetection(t *testing.T) {
	engine := recon.NewEngine(calendar.New(nil))
	tNow := time.Date(2026, 10, 30, 12, 0, 0, 0, calendar.Location())

	orderBook := []recon.OrderBookLine{
		// 1. Matched cleanly
		{NetworkOrderID: "O-MATCH", AmountPaise: 10000, CreatedAt: tNow.Add(-48 * time.Hour)},
		// 2. Orphan in Order Book
		{NetworkOrderID: "O-ORPHAN", AmountPaise: 20000, CreatedAt: tNow.Add(-48 * time.Hour)},
		// 3. Amount Mismatch
		{NetworkOrderID: "O-MISMATCH", AmountPaise: 30000, CreatedAt: tNow.Add(-48 * time.Hour)},
	}

	saReport := []recon.SAReportLine{
		{NetworkOrderID: "O-MATCH", SettledAmount: 10000, Status: "settled", SettledAt: tNow.Add(-24 * time.Hour)},
		{NetworkOrderID: "O-MISMATCH", SettledAmount: 35000, Status: "settled", SettledAt: tNow.Add(-24 * time.Hour)},
		// 4. Settled by SA but bank failed to credit
		{NetworkOrderID: "O-NOBANK", SettledAmount: 40000, Status: "settled", SettledAt: tNow.Add(-24 * time.Hour)},
	}

	bankStatement := []recon.BankStatementLine{
		{ReferenceID: "O-MATCH", Direction: ledger.Credit, AmountPaise: 10000, ValueDate: tNow},
		{ReferenceID: "O-MISMATCH", Direction: ledger.Credit, AmountPaise: 35000, ValueDate: tNow},
		// 5. Unclaimed bank credit (money in bank with no SA claim)
		{ReferenceID: "UNKNOWN-REF", Direction: ledger.Credit, AmountPaise: 50000, ValueDate: tNow},
	}

	result := engine.Reconcile(orderBook, saReport, bankStatement, tNow)

	if result.TotalExceptions != 4 {
		t.Fatalf("Expected 4 exceptions, got %d: %+v", result.TotalExceptions, result.Exceptions)
	}

	if result.ExceptionsByClass[recon.ExceptionOrphanOrder] != 1 {
		t.Errorf("Expected 1 ExceptionOrphanOrder, got %d", result.ExceptionsByClass[recon.ExceptionOrphanOrder])
	}
	if result.ExceptionsByClass[recon.ExceptionAmountMismatch] != 1 {
		t.Errorf("Expected 1 ExceptionAmountMismatch, got %d", result.ExceptionsByClass[recon.ExceptionAmountMismatch])
	}
	if result.ExceptionsByClass[recon.ExceptionBankFailure] != 1 {
		t.Errorf("Expected 1 ExceptionBankFailure, got %d", result.ExceptionsByClass[recon.ExceptionBankFailure])
	}
	if result.ExceptionsByClass[recon.ExceptionUnclaimedBankCredit] != 1 {
		t.Errorf("Expected 1 ExceptionUnclaimedBankCredit, got %d", result.ExceptionsByClass[recon.ExceptionUnclaimedBankCredit])
	}
}

func TestRecon_MakerChecker_DualControl(t *testing.T) {
	engine := recon.NewEngine(calendar.New(nil))

	lines := []ledger.EntryLine{
		{AccountName: "sa:suspense", Direction: ledger.Debit, AmountPaise: 5000},
		{AccountName: "receiver:operative", Direction: ledger.Credit, AmountPaise: 5000},
	}

	// 1. Maker proposes adjustment
	proposal, err := engine.ProposeAdjustment("EXC-001", "operator_alice", "Reconcile unapplied bank credit", lines)
	if err != nil {
		t.Fatalf("ProposeAdjustment failed: %v", err)
	}

	if proposal.Status != "pending" {
		t.Errorf("Expected pending proposal, got %s", proposal.Status)
	}

	// 2. Checker tries to approve own proposal -> Must fail (ErrSelfApproval)
	_, err = engine.ApproveAdjustment(proposal, "operator_alice")
	if err != recon.ErrSelfApproval {
		t.Fatalf("Expected ErrSelfApproval when proposer == approver, got: %v", err)
	}

	// 3. Different checker approves -> Succeeds
	postReq, err := engine.ApproveAdjustment(proposal, "approver_bob")
	if err != nil {
		t.Fatalf("ApproveAdjustment failed for independent approver: %v", err)
	}

	if postReq.CauseType != ledger.CauseAdjustment {
		t.Errorf("Expected CauseAdjustment, got %s", postReq.CauseType)
	}
	if proposal.Status != "approved" {
		t.Errorf("Expected approved status, got %s", proposal.Status)
	}
}

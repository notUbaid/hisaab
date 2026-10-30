package verification_test

import (
	"errors"
	"testing"
	"time"

	"github.com/ubaid/hisaab/internal/ledger"
	"github.com/ubaid/hisaab/internal/settlement"
	"github.com/ubaid/hisaab/internal/verification"
)

func TestFailpointManager_ErrorInjection(t *testing.T) {
	mgr := verification.NewFailpointManager()

	// Disarmed failpoint -> no error
	if err := mgr.Trigger(string(verification.FPAfterCommitBeforeACK)); err != nil {
		t.Fatalf("Expected nil on disarmed failpoint, got: %v", err)
	}

	// Arm failpoint with error
	mgr.Enable(verification.FPAfterCommitBeforeACK, verification.ActionError)

	err := mgr.Trigger(string(verification.FPAfterCommitBeforeACK))
	if err == nil || !errors.Is(err, verification.ErrInjectedFault) {
		t.Fatalf("Expected ErrInjectedFault, got: %v", err)
	}

	if mgr.HitCount(verification.FPAfterCommitBeforeACK) != 2 {
		t.Errorf("Expected 2 hits, got %d", mgr.HitCount(verification.FPAfterCommitBeforeACK))
	}

	// Disarm and verify hit
	mgr.Disable(verification.FPAfterCommitBeforeACK)
	if err := mgr.Trigger(string(verification.FPAfterCommitBeforeACK)); err != nil {
		t.Fatalf("Expected nil after disable, got: %v", err)
	}
}

func TestFailpointManager_PanicInjection(t *testing.T) {
	mgr := verification.NewFailpointManager()
	mgr.Enable(verification.FPAfterDebitLegBeforeCreditLeg, verification.ActionPanic)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Expected panic from failpoint, got nil")
		}
	}()

	_ = mgr.Trigger(string(verification.FPAfterDebitLegBeforeCreditLeg))
}

func TestInvariantChecker_Invariant1_ZeroSum(t *testing.T) {
	checker := verification.NewInvariantChecker()

	// 1. Balanced lines -> Pass
	balanced := []ledger.EntryLine{
		{AccountName: "acc1", Direction: ledger.Debit, AmountPaise: 10000},
		{AccountName: "acc2", Direction: ledger.Credit, AmountPaise: 10000},
	}
	if err := checker.CheckInvariant1_ZeroSum(balanced); err != nil {
		t.Errorf("Expected nil on balanced lines, got %v", err)
	}

	// 2. Unbalanced lines -> Fail
	unbalanced := []ledger.EntryLine{
		{AccountName: "acc1", Direction: ledger.Debit, AmountPaise: 10000},
		{AccountName: "acc2", Direction: ledger.Credit, AmountPaise: 9999},
	}
	if err := checker.CheckInvariant1_ZeroSum(unbalanced); err == nil {
		t.Error("Expected error on unbalanced lines, got nil")
	}
}

func TestInvariantChecker_Invariant4_NonNegative(t *testing.T) {
	checker := verification.NewInvariantChecker()

	if err := checker.CheckInvariant4_NonNegative("collector:ondc_settlement", 100); err != nil {
		t.Errorf("Expected nil on positive balance, got %v", err)
	}
	if err := checker.CheckInvariant4_NonNegative("collector:ondc_settlement", 0); err != nil {
		t.Errorf("Expected nil on zero balance, got %v", err)
	}
	if err := checker.CheckInvariant4_NonNegative("collector:ondc_settlement", -1); err == nil {
		t.Error("Expected error on negative balance, got nil")
	}
}

func TestInvariantChecker_Invariant5_Conservation(t *testing.T) {
	checker := verification.NewInvariantChecker()

	validOrder := settlement.OrderSettlementLine{
		NetworkOrderID:   "ORD-1",
		OrderAmountPaise: 10000,
		CommissionPaise:  1000,
		BFFPaise:         500,
		SoRDuePaise:      8500,
	}
	if err := checker.CheckInvariant5_Conservation(validOrder); err != nil {
		t.Errorf("Expected nil on valid order conservation, got %v", err)
	}

	leakingOrder := settlement.OrderSettlementLine{
		NetworkOrderID:   "ORD-2",
		OrderAmountPaise: 10000,
		CommissionPaise:  1000,
		BFFPaise:         500,
		SoRDuePaise:      8000, // Missing 500 paise!
	}
	if err := checker.CheckInvariant5_Conservation(leakingOrder); err == nil {
		t.Error("Expected error on leaking order conservation, got nil")
	}
}

func TestInvariantChecker_Invariant13_GrossNetTieOut(t *testing.T) {
	checker := verification.NewInvariantChecker()

	batch := &settlement.BatchSettlementResult{
		TotalMatched: 30000,
		TotalBFF:     1500,
		MatchedOrders: []settlement.MatchedOrder{
			{NetworkOrderID: "O1", OrderAmountPaise: 30000, BFFPaise: 1500},
		},
		Instructions: []settlement.NetInstruction{
			// 30000 - 1500 = 28500
			{AmountPaise: 28500},
		},
	}

	if err := checker.CheckInvariant13_GrossNetTieOut(batch); err != nil {
		t.Errorf("Expected valid tie-out, got: %v", err)
	}

	// Corrupt instruction amount
	batch.Instructions[0].AmountPaise = 28000
	if err := checker.CheckInvariant13_GrossNetTieOut(batch); err == nil {
		t.Error("Expected tie-out error when net instruction doesn't match, got nil")
	}
}

func TestInvariantChecker_Invariant14_TerminalCallback(t *testing.T) {
	checker := verification.NewInvariantChecker()

	// Immutable once settled
	if err := checker.CheckInvariant14_TerminalCallback("settled", "settled"); err != nil {
		t.Errorf("Expected nil on identical settled state, got %v", err)
	}

	// Attempting mutation from settled to failed
	if err := checker.CheckInvariant14_TerminalCallback("settled", "failed"); err == nil {
		t.Error("Expected error on mutating settled state, got nil")
	}
}

func init() {
	_ = time.Now()
}

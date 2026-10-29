package settlement_test

import (
	"testing"
	"time"

	"github.com/ubaid/hisaab/internal/ledger"
	"github.com/ubaid/hisaab/internal/settlement"
)

func TestRefund_CleanFullRefund(t *testing.T) {
	proc := settlement.NewRefundProcessor()

	claim := settlement.RefundClaim{
		MessageID:         "msg-ref-001",
		NetworkOrderID:    "ORD-REF-101",
		OriginalAmount:    150000,
		RefundAmountPaise: 150000,
		Reason:            "Item damaged in transit",
		SettlementDate:    time.Now().UTC(),
	}

	sellerBal := ledger.AmountPaise(300000)
	res, err := proc.ProcessRefund(claim, "collector:ondc_settlement", "receiver:ondc_settlement", sellerBal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Status != "settled" {
		t.Fatalf("expected settled status, got %s", res.Status)
	}
	if len(res.Instructions) != 1 {
		t.Fatalf("expected 1 net instruction, got %d", len(res.Instructions))
	}
	if res.Instructions[0].AmountPaise != 150000 {
		t.Fatalf("expected 150000 paise instruction, got %d", res.Instructions[0].AmountPaise)
	}

	// Validate journal lines balance
	total, err := ledger.ValidateLines(res.JournalLines)
	if err != nil {
		t.Fatalf("journal lines failed validation: %v", err)
	}
	if total != 150000 {
		t.Fatalf("expected total 150000, got %d", total)
	}
}

func TestRefund_PartialRefundsAccumulation(t *testing.T) {
	proc := settlement.NewRefundProcessor()

	claim1 := settlement.RefundClaim{
		MessageID:         "msg-ref-002",
		NetworkOrderID:    "ORD-REF-102",
		OriginalAmount:    200000,
		RefundAmountPaise: 100000,
		Reason:            "Partial return 1",
		SettlementDate:    time.Now().UTC(),
	}

	sellerBal := ledger.AmountPaise(500000)
	res1, err := proc.ProcessRefund(claim1, "collector:ondc_settlement", "receiver:ondc_settlement", sellerBal)
	if err != nil || res1.Status != "settled" {
		t.Fatalf("first partial refund failed: %v", err)
	}

	// Second partial refund for remaining 100000
	claim2 := claim1
	claim2.MessageID = "msg-ref-003"
	res2, err := proc.ProcessRefund(claim2, "collector:ondc_settlement", "receiver:ondc_settlement", sellerBal)
	if err != nil || res2.Status != "settled" {
		t.Fatalf("second partial refund failed: %v", err)
	}

	// Third refund exceeds original total: must fail (Invariant 6)
	claim3 := claim1
	claim3.MessageID = "msg-ref-004"
	claim3.RefundAmountPaise = 1000
	_, err = proc.ProcessRefund(claim3, "collector:ondc_settlement", "receiver:ondc_settlement", sellerBal)
	if err == nil {
		t.Fatalf("expected error exceeding original order amount, got nil")
	}
}

func TestRefund_InsolventSellerEscalatedToSuspense(t *testing.T) {
	proc := settlement.NewRefundProcessor()

	claim := settlement.RefundClaim{
		MessageID:         "msg-ref-005",
		NetworkOrderID:    "ORD-REF-103",
		OriginalAmount:    150000,
		RefundAmountPaise: 150000,
		Reason:            "Customer cancellation",
		SettlementDate:    time.Now().UTC(),
	}

	// Seller only has 20000 paise available (< 150000 required)
	sellerBal := ledger.AmountPaise(20000)
	res, err := proc.ProcessRefund(claim, "collector:ondc_settlement", "receiver:ondc_settlement", sellerBal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Status != "escalated_to_suspense" {
		t.Fatalf("expected escalated_to_suspense status, got %s", res.Status)
	}
	if len(res.Instructions) != 0 {
		t.Fatalf("expected zero net instructions on insolvent seller, got %d", len(res.Instructions))
	}

	// Verify journal lines route debit to sa:suspense
	if res.JournalLines[0].AccountName != "sa:suspense" {
		t.Fatalf("expected debit to sa:suspense, got %s", res.JournalLines[0].AccountName)
	}
	if _, err := ledger.ValidateLines(res.JournalLines); err != nil {
		t.Fatalf("suspense journal lines failed validation: %v", err)
	}
}

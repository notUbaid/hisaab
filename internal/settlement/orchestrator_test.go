package settlement_test

import (
	"context"
	"testing"
	"time"

	"github.com/ubaid/hisaab/internal/calendar"
	"github.com/ubaid/hisaab/internal/ledger"
	"github.com/ubaid/hisaab/internal/settlement"
)

type mockTaxCalculator struct{}

func (m mockTaxCalculator) CalculateOrderTaxPaise(orderAmount, commission, bff ledger.AmountPaise) ledger.AmountPaise {
	// Standard TDS 0.1% + GST TCS 0.5% = 0.6%
	return (orderAmount * 60) / 10000
}

func TestOrchestrator_EndToEnd_TwoSidedSettlement(t *testing.T) {
	ctx := context.Background()

	// 1. Initialize ledger and seed funds
	mem := ledger.NewMemoryLedger()
	_ = mem.SeedStandardAccounts()
	_, _ = mem.RegisterAccount("sa:suspense", ledger.AccountAsset, "INR", false)

	// Pre-fund collector account with 200000 paise (Rs 2000)
	fundReq := ledger.PostRequest{
		IdempotencyKey: "fund-test",
		CauseType:      ledger.CauseAdjustment,
		CauseID:        1,
		Description:    "Initial funding",
		Lines: []ledger.EntryLine{
			{AccountName: "ext:nodal_clearing", Direction: ledger.Debit, AmountPaise: 200000},
			{AccountName: "collector:ondc_settlement", Direction: ledger.Credit, AmountPaise: 200000},
		},
		PostedAt: time.Now().UTC(),
	}
	if _, err := mem.Post(ctx, fundReq); err != nil {
		t.Fatalf("Initial funding failed: %v", err)
	}

	cal := calendar.New(nil)
	matcher := settlement.NewMatcher(settlement.DefaultPolicy())

	orch := settlement.NewOrchestrator(mem, matcher, cal, mockTaxCalculator{}, nil)

	// 2. Buyer submits claim for ORD-101 (amount Rs 1000, comm Rs 30, bff Rs 10 -> sor Rs 960)
	buyerClaim := &settlement.SettleClaimPayload{
		Context: &settlement.BecknContext{
			Action:    "settle",
			MessageID: "msg-buyer-01",
		},
		SettlementID: "SETTLE-OCT-30",
		NPPairID:     "BAP-01:BPP-01",
		SubmittedBy:  "buyer_app",
		CollectorAcc: "collector:ondc_settlement",
		ReceiverAcc:  "receiver:ondc_settlement",
		Orders: []settlement.OrderSettlementLine{
			{
				NetworkOrderID:   "ORD-101",
				OrderAmountPaise: 100000,
				CommissionPaise:  3000,
				BFFPaise:         1000,
				SoRDuePaise:      96000,
			},
		},
	}

	ack1, err := orch.SubmitClaim(ctx, buyerClaim)
	if err != nil {
		t.Fatalf("Buyer claim failed: %v", err)
	}
	if ack1.Message.Ack.Status != settlement.AckStatusACK {
		t.Fatalf("Expected ACK for buyer claim, got %s", ack1.Message.Ack.Status)
	}

	// Batches should not be settled yet (only buyer claim present)
	batches := orch.ListBatches()
	if len(batches) != 0 {
		t.Fatalf("Expected 0 settled batches while counterpart absent, got %d", len(batches))
	}

	// 3. Seller submits counterpart claim for ORD-101
	sellerClaim := &settlement.SettleClaimPayload{
		Context: &settlement.BecknContext{
			Action:    "settle",
			MessageID: "msg-seller-01",
		},
		SettlementID: "SETTLE-OCT-30",
		NPPairID:     "BAP-01:BPP-01",
		SubmittedBy:  "seller_app",
		CollectorAcc: "collector:ondc_settlement",
		ReceiverAcc:  "receiver:ondc_settlement",
		Orders: []settlement.OrderSettlementLine{
			{
				NetworkOrderID:   "ORD-101",
				OrderAmountPaise: 100000,
				CommissionPaise:  3000,
				BFFPaise:         1000,
				SoRDuePaise:      96000,
			},
		},
	}

	ack2, err := orch.SubmitClaim(ctx, sellerClaim)
	if err != nil {
		t.Fatalf("Seller claim failed: %v", err)
	}
	if ack2.Message.Ack.Status != settlement.AckStatusACK {
		t.Fatalf("Expected ACK for seller claim, got %s", ack2.Message.Ack.Status)
	}

	// 4. Verification: Batch should now be settled
	batches = orch.ListBatches()
	if len(batches) != 1 {
		t.Fatalf("Expected exactly 1 settled batch, got %d", len(batches))
	}
	b := batches[0]
	if b.Status != settlement.BatchStatusSettled {
		t.Errorf("Expected status settled, got %s", b.Status)
	}
	if b.MatchedCount != 1 {
		t.Errorf("Expected 1 matched order, got %d", b.MatchedCount)
	}
	if b.NetPayoutPaise != 99000 {
		t.Errorf("Expected net payout 99000 paise, got %d", b.NetPayoutPaise)
	}

	// 5. Verification: Ledger double-entry postings
	// Collector balance was 200000, after 99000 payout should be 101000
	collectorBal, err := mem.Balance(ctx, "collector:ondc_settlement")
	if err != nil {
		t.Fatalf("Balance failed: %v", err)
	}
	if collectorBal != 101000 {
		t.Errorf("Expected collector balance 101000 paise, got %d", collectorBal)
	}

	receiverBal, err := mem.Balance(ctx, "receiver:ondc_settlement")
	if err != nil {
		t.Fatalf("Receiver balance failed: %v", err)
	}
	if receiverBal != 99000 {
		t.Errorf("Expected receiver balance 99000 paise, got %d", receiverBal)
	}

	// 6. Verification: Outbox items staged
	pendingOutbox := orch.GetPendingOutbox()
	if len(pendingOutbox) != 2 {
		t.Fatalf("Expected 2 outbox items (bank instruction + beckn callback), got %d", len(pendingOutbox))
	}
	if pendingOutbox[0].EventType != "settlement.bank_instruction" {
		t.Errorf("Expected outbox item 0 to be bank instruction, got %s", pendingOutbox[0].EventType)
	}
	if pendingOutbox[1].EventType != "beckn.on_settle_callback" {
		t.Errorf("Expected outbox item 1 to be on_settle callback, got %s", pendingOutbox[1].EventType)
	}

	// 7. Verification: Idempotent replay of buyer claim returns ACK without duplicate batch
	ackReplay, err := orch.SubmitClaim(ctx, buyerClaim)
	if err != nil {
		t.Fatalf("Replay failed: %v", err)
	}
	if ackReplay.Message.Ack.Status != settlement.AckStatusACK {
		t.Errorf("Expected ACK on replay, got %s", ackReplay.Message.Ack.Status)
	}
	if len(orch.ListBatches()) != 1 {
		t.Errorf("Replay created duplicate batch")
	}
}

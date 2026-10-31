package settlement_test

import (
	"context"
	"testing"
	"time"

	"github.com/ubaid/hisaab/internal/settlement"
)

func TestMemoryRepository_SettlementLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := settlement.NewMemoryRepository()

	// 1. Save and Get Claim
	claim := &settlement.SettleClaimPayload{
		Context: &settlement.BecknContext{
			MessageID: "MSG-001",
			Timestamp: time.Now(),
		},
		SettlementID: "SETTLE-001",
		NPPairID:     "BAP-BPP-01",
		SubmittedBy:  "buyer_app",
		CollectorAcc: "collector:ondc_settlement",
		ReceiverAcc:  "receiver:ondc_settlement",
		Orders: []settlement.OrderSettlementLine{
			{
				NetworkOrderID:   "ORD-001",
				OrderAmountPaise: 100000,
				CommissionPaise:  3000,
				BFFPaise:         1000,
			},
		},
	}

	claimID, err := repo.SaveClaim(ctx, claim, "hash123", []byte(`{}`))
	if err != nil || claimID <= 0 {
		t.Fatalf("SaveClaim failed: %v", err)
	}

	savedClaim, err := repo.GetClaim(ctx, "MSG-001")
	if err != nil {
		t.Fatalf("GetClaim failed: %v", err)
	}
	if savedClaim.Claim.NPPairID != "BAP-BPP-01" {
		t.Errorf("unexpected NPPairID: %s", savedClaim.Claim.NPPairID)
	}

	// 2. Save and Get Batch
	now := time.Now().UTC()
	batch := &settlement.BatchSummary{
		BatchID:         "BATCH-001",
		NPPairID:        "BAP-BPP-01",
		SettlementDate:  "2026-10-31",
		Status:          settlement.BatchStatusSettled,
		MatchedCount:    1,
		TotalGrossPaise: 100000,
		NetPayoutPaise:  96000,
		Instructions: []settlement.NetInstruction{
			{
				BatchID:     "BATCH-001",
				FromAccount: "collector:ondc_settlement",
				ToAccount:   "receiver:ondc_settlement",
				AmountPaise: 96000,
			},
		},
		CreatedAt: now,
	}

	if err := repo.SaveBatch(ctx, batch); err != nil {
		t.Fatalf("SaveBatch failed: %v", err)
	}

	savedBatch, err := repo.GetBatch(ctx, "BATCH-001")
	if err != nil {
		t.Fatalf("GetBatch failed: %v", err)
	}
	if savedBatch.NetPayoutPaise != 96000 || len(savedBatch.Instructions) != 1 {
		t.Errorf("unexpected batch fields: %+v", savedBatch)
	}

	// 3. Instruction Status Update
	err = repo.UpdateInstructionStatus(ctx, "BATCH-001", "collector:ondc_settlement", "receiver:ondc_settlement", 96000, "sent")
	if err != nil {
		t.Fatalf("UpdateInstructionStatus failed: %v", err)
	}

	// 4. Refund State Recording
	if err := repo.RecordRefundState(ctx, "ORD-001", 50000, "settled"); err != nil {
		t.Fatalf("RecordRefundState failed: %v", err)
	}
	if err := repo.RecordRefundState(ctx, "ORD-001", 20000, "settled"); err != nil {
		t.Fatalf("RecordRefundState failed: %v", err)
	}

	totalRefunded, err := repo.GetRefundState(ctx, "ORD-001")
	if err != nil {
		t.Fatalf("GetRefundState failed: %v", err)
	}
	if totalRefunded != 70000 {
		t.Errorf("expected 70000 paise refunded, got %d", totalRefunded)
	}
}

func TestPostgresRepository_NilPoolGraceful(t *testing.T) {
	ctx := context.Background()
	repo := settlement.NewPostgresRepository(nil)

	id, err := repo.SaveClaim(ctx, nil, "", nil)
	if err != nil || id != 0 {
		t.Errorf("expected 0, nil on nil claim, got %d, %v", id, err)
	}

	_, err = repo.GetClaim(ctx, "MSG-999")
	if err != settlement.ErrClaimNotFound {
		t.Errorf("expected ErrClaimNotFound, got %v", err)
	}

	_, err = repo.GetBatch(ctx, "BATCH-999")
	if err != settlement.ErrBatchNotFound {
		t.Errorf("expected ErrBatchNotFound, got %v", err)
	}

	err = repo.UpdateInstructionStatus(ctx, "BATCH-999", "a", "b", 100, "sent")
	if err != settlement.ErrInstructionNotFound {
		t.Errorf("expected ErrInstructionNotFound on nil pool, got %v", err)
	}
}

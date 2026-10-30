package settlement_test

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ubaid/hisaab/internal/calendar"
	"github.com/ubaid/hisaab/internal/ledger"
	"github.com/ubaid/hisaab/internal/settlement"
)

type mockTaxCalculator struct{}

func (m mockTaxCalculator) CalculateOrderTax(orderAmount, commission, bff ledger.AmountPaise) (*settlement.TaxBreakdown, error) {
	// Standard TDS 0.1% = 100 paise, GST TCS 0.5% = 500 paise
	tds := (orderAmount * 10) / 10000
	tcs := (orderAmount * 50) / 10000
	return &settlement.TaxBreakdown{
		OrderAmountPaise: orderAmount,
		CommissionPaise:  commission,
		BFFPaise:         bff,
		TDSPaise:         tds,
		GSTTCSPaise:      tcs,
		TotalTaxPaise:    tds + tcs,
	}, nil
}

func TestOrchestrator_EndToEnd_TwoSidedSettlement(t *testing.T) {
	ctx := context.Background()

	// 1. Initialize ledger and seed funds
	mem := ledger.NewMemoryLedger()
	_ = mem.SeedStandardAccounts()
	_, _ = mem.RegisterAccount("sa:suspense", ledger.AccountAsset, "INR", false)
	_, _ = mem.RegisterAccount("tax:tds_payable", ledger.AccountLiability, "INR", false)
	_, _ = mem.RegisterAccount("tax:gst_tcs_payable", ledger.AccountLiability, "INR", false)

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
	// Gross seller transfer is 99000 (100000 - 1000 BFF). Tax is 600 (100 TDS + 500 TCS).
	// Net payout must be exactly 98400.
	if b.NetPayoutPaise != 98400 {
		t.Errorf("Expected net payout 98400 paise, got %d", b.NetPayoutPaise)
	}
	if b.TotalTaxPaise != 600 {
		t.Errorf("Expected total tax 600 paise, got %d", b.TotalTaxPaise)
	}
	if b.TDSPaise != 100 {
		t.Errorf("Expected TDS 100 paise, got %d", b.TDSPaise)
	}
	if b.GSTTCSPaise != 500 {
		t.Errorf("Expected GST TCS 500 paise, got %d", b.GSTTCSPaise)
	}
	if b.MerkleRoot == "" {
		t.Errorf("Expected non-empty MerkleRoot on settled batch")
	}

	// 5. Verification: Ledger double-entry postings
	// Collector balance was 200000, after 99000 gross payout should be 101000
	collectorBal, err := mem.Balance(ctx, "collector:ondc_settlement")
	if err != nil {
		t.Fatalf("Balance failed: %v", err)
	}
	if collectorBal != 101000 {
		t.Errorf("Expected collector balance 101000 paise, got %d", collectorBal)
	}

	// Receiver balance must be post-tax net payout: 98400
	receiverBal, err := mem.Balance(ctx, "receiver:ondc_settlement")
	if err != nil {
		t.Fatalf("Receiver balance failed: %v", err)
	}
	if receiverBal != 98400 {
		t.Errorf("Expected receiver balance 98400 paise, got %d", receiverBal)
	}

	// Statutory tax liability balances
	tdsBal, _ := mem.Balance(ctx, "tax:tds_payable")
	if tdsBal != 100 {
		t.Errorf("Expected tax:tds_payable balance 100 paise, got %d", tdsBal)
	}
	tcsBal, _ := mem.Balance(ctx, "tax:gst_tcs_payable")
	if tcsBal != 500 {
		t.Errorf("Expected tax:gst_tcs_payable balance 500 paise, got %d", tcsBal)
	}

	// Invariant 1: Total Debits == Total Credits (101000 + 98400 + 100 + 500 = 200000)
	if err := mem.AuditInvariants(); err != nil {
		t.Fatalf("Invariant audit failed after settlement: %v", err)
	}

	// 6. Verification: Outbox items staged
	pendingOutbox := orch.GetPendingOutbox()
	if len(pendingOutbox) < 2 {
		t.Fatalf("Expected at least 2 outbox items, got %d", len(pendingOutbox))
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

func TestOrchestrator_InsufficientBalance_RejectedStatus(t *testing.T) {
	ctx := context.Background()

	mem := ledger.NewMemoryLedger()
	_ = mem.SeedStandardAccounts()

	// Do NOT fund collector account (balance = 0)
	cal := calendar.New(nil)
	matcher := settlement.NewMatcher(settlement.DefaultPolicy())
	orch := settlement.NewOrchestrator(mem, matcher, cal, mockTaxCalculator{}, nil)

	buyerClaim := &settlement.SettleClaimPayload{
		Context:      &settlement.BecknContext{Action: "settle", MessageID: "msg-rej-buyer"},
		SettlementID: "SETTLE-REJ",
		NPPairID:     "BAP-REJ:BPP-REJ",
		SubmittedBy:  "buyer_app",
		CollectorAcc: "collector:ondc_settlement",
		ReceiverAcc:  "receiver:ondc_settlement",
		Orders: []settlement.OrderSettlementLine{
			{NetworkOrderID: "ORD-REJ-01", OrderAmountPaise: 50000, CommissionPaise: 1000, BFFPaise: 500, SoRDuePaise: 48500},
		},
	}
	sellerClaim := &settlement.SettleClaimPayload{
		Context:      &settlement.BecknContext{Action: "settle", MessageID: "msg-rej-seller"},
		SettlementID: "SETTLE-REJ",
		NPPairID:     "BAP-REJ:BPP-REJ",
		SubmittedBy:  "seller_app",
		CollectorAcc: "collector:ondc_settlement",
		ReceiverAcc:  "receiver:ondc_settlement",
		Orders: []settlement.OrderSettlementLine{
			{NetworkOrderID: "ORD-REJ-01", OrderAmountPaise: 50000, CommissionPaise: 1000, BFFPaise: 500, SoRDuePaise: 48500},
		},
	}

	_, _ = orch.SubmitClaim(ctx, buyerClaim)
	_, _ = orch.SubmitClaim(ctx, sellerClaim)

	batches := orch.ListBatches()
	if len(batches) != 1 {
		t.Fatalf("Expected 1 batch record, got %d", len(batches))
	}
	b := batches[0]
	if b.Status != settlement.BatchStatusRejected {
		t.Errorf("Expected status REJECTED on insufficient balance, got %s", b.Status)
	}
	if b.NetPayoutPaise != 0 {
		t.Errorf("Expected 0 net payout on rejected batch, got %d", b.NetPayoutPaise)
	}

	// Verify callback outbox item has status REJECTED
	outbox := orch.GetPendingOutbox()
	if len(outbox) == 0 {
		t.Fatalf("Expected outbox callback for rejected batch")
	}
	lastItem := outbox[len(outbox)-1]
	if lastItem.EventType != "beckn.on_settle_callback" {
		t.Errorf("Expected on_settle_callback, got %s", lastItem.EventType)
	}
}

func TestOrchestrator_PartialMatch_PartiallySettledStatus(t *testing.T) {
	ctx := context.Background()

	mem := ledger.NewMemoryLedger()
	_ = mem.SeedStandardAccounts()
	_, _ = mem.RegisterAccount("tax:tds_payable", ledger.AccountLiability, "INR", false)
	_, _ = mem.RegisterAccount("tax:gst_tcs_payable", ledger.AccountLiability, "INR", false)

	// Pre-fund collector with enough funds
	_, _ = mem.Post(ctx, ledger.PostRequest{
		IdempotencyKey: "fund-partial",
		CauseType:      ledger.CauseAdjustment,
		CauseID:        1,
		Lines: []ledger.EntryLine{
			{AccountName: "ext:nodal_clearing", Direction: ledger.Debit, AmountPaise: 500000},
			{AccountName: "collector:ondc_settlement", Direction: ledger.Credit, AmountPaise: 500000},
		},
	})

	cal := calendar.New(nil)
	matcher := settlement.NewMatcher(settlement.DefaultPolicy())
	orch := settlement.NewOrchestrator(mem, matcher, cal, nil, nil)

	// Buyer has 2 orders; seller only claims 1
	buyerClaim := &settlement.SettleClaimPayload{
		Context:      &settlement.BecknContext{Action: "settle", MessageID: "msg-part-buyer"},
		SettlementID: "SETTLE-PART",
		NPPairID:     "BAP-PART:BPP-PART",
		SubmittedBy:  "buyer_app",
		CollectorAcc: "collector:ondc_settlement",
		ReceiverAcc:  "receiver:ondc_settlement",
		Orders: []settlement.OrderSettlementLine{
			{NetworkOrderID: "ORD-P1", OrderAmountPaise: 50000, CommissionPaise: 1000, BFFPaise: 500, SoRDuePaise: 48500},
			{NetworkOrderID: "ORD-P2-UNMATCHED", OrderAmountPaise: 30000, CommissionPaise: 1000, BFFPaise: 500, SoRDuePaise: 28500},
		},
	}
	sellerClaim := &settlement.SettleClaimPayload{
		Context:      &settlement.BecknContext{Action: "settle", MessageID: "msg-part-seller"},
		SettlementID: "SETTLE-PART",
		NPPairID:     "BAP-PART:BPP-PART",
		SubmittedBy:  "seller_app",
		CollectorAcc: "collector:ondc_settlement",
		ReceiverAcc:  "receiver:ondc_settlement",
		Orders: []settlement.OrderSettlementLine{
			{NetworkOrderID: "ORD-P1", OrderAmountPaise: 50000, CommissionPaise: 1000, BFFPaise: 500, SoRDuePaise: 48500},
		},
	}

	_, _ = orch.SubmitClaim(ctx, buyerClaim)
	_, _ = orch.SubmitClaim(ctx, sellerClaim)

	batches := orch.ListBatches()
	if len(batches) != 1 {
		t.Fatalf("Expected 1 batch, got %d", len(batches))
	}
	b := batches[0]
	if b.Status != settlement.BatchStatusPartiallySettled {
		t.Errorf("Expected PARTIALLY_SETTLED status, got %s", b.Status)
	}
	if b.MatchedCount != 1 || b.UnmatchedCount != 1 {
		t.Errorf("Expected 1 matched and 1 unmatched order, got %d and %d", b.MatchedCount, b.UnmatchedCount)
	}
}

func findMigrationFileForTest() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for i := 0; i < 5; i++ {
		candidate := filepath.Join(dir, "db", "migrations", "00001_init.sql")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("00001_init.sql not found in parent directories")
}

func loadMigrationUpSQLForTest() (string, error) {
	path, err := findMigrationFileForTest()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	content := string(data)
	if parts := strings.Split(content, "-- goose Down"); len(parts) > 0 {
		return parts[0], nil
	}
	return content, nil
}

func TestOrchestrator_PostgreSQL_PersistenceAndRestore(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping live PostgreSQL orchestrator persistence test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}
	defer pool.Close()

	// Isolate in dedicated test schema
	schema := fmt.Sprintf("test_orch_restore_%d_%d", time.Now().UnixNano(), rand.Intn(10000))
	_, err = pool.Exec(ctx, fmt.Sprintf("CREATE SCHEMA %s; SET search_path TO %s, public;", schema, schema))
	if err != nil {
		t.Fatalf("Failed to create isolated test schema: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, fmt.Sprintf("DROP SCHEMA %s CASCADE;", schema))
	}()

	// Apply migration DDL
	migrationSQL, err := loadMigrationUpSQLForTest()
	if err != nil {
		t.Fatalf("Failed to load migration SQL: %v", err)
	}
	if _, err := pool.Exec(ctx, migrationSQL); err != nil {
		t.Fatalf("Failed to apply migration SQL: %v", err)
	}

	pgLedger := ledger.New(pool)
	if err := pgLedger.SeedStandardAccounts(ctx); err != nil {
		t.Fatalf("Failed to seed standard accounts: %v", err)
	}
	_ = pgLedger.RegisterAccount(ctx, "sa:suspense", ledger.AccountAsset, "INR", false)
	_ = pgLedger.RegisterAccount(ctx, "sa:tolerance_adjustment", ledger.AccountAsset, "INR", false)
	_ = pgLedger.RegisterAccount(ctx, "tax:gst_tcs_payable", ledger.AccountLiability, "INR", false)
	_ = pgLedger.RegisterAccount(ctx, "tax:tds_payable", ledger.AccountLiability, "INR", false)

	// Pre-fund collector
	fundReq := ledger.PostRequest{
		IdempotencyKey: "fund-pg-test",
		CauseType:      ledger.CauseAdjustment,
		CauseID:        1,
		Description:    "Initial funding for PG test",
		Lines: []ledger.EntryLine{
			{AccountName: "ext:nodal_clearing", Direction: ledger.Debit, AmountPaise: 200000},
			{AccountName: "collector:ondc_settlement", Direction: ledger.Credit, AmountPaise: 200000},
		},
		PostedAt: time.Now().UTC(),
	}
	if _, err := pgLedger.Post(ctx, fundReq); err != nil {
		t.Fatalf("Initial funding failed: %v", err)
	}

	cal := calendar.New(nil)
	matcher := settlement.NewMatcher(settlement.DefaultPolicy())

	// 1. First Orchestrator lifecycle: processes claims and settles batch
	orch1 := settlement.NewOrchestrator(pgLedger, matcher, cal, mockTaxCalculator{}, nil)
	orch1.SetPool(pool)

	buyerClaim := &settlement.SettleClaimPayload{
		Context: &settlement.BecknContext{
			Action:    "settle",
			MessageID: "msg-pg-buyer-01",
		},
		SettlementID: "SETTLE-PG-01",
		NPPairID:     "BAP-PG:BPP-PG",
		SubmittedBy:  "buyer_app",
		CollectorAcc: "collector:ondc_settlement",
		ReceiverAcc:  "receiver:ondc_settlement",
		Orders: []settlement.OrderSettlementLine{
			{
				NetworkOrderID:   "ORD-PG-1",
				OrderAmountPaise: 100000,
				CommissionPaise:  3000,
				BFFPaise:         1000,
				SoRDuePaise:      96000,
			},
		},
	}
	ack1, err := orch1.SubmitClaim(ctx, buyerClaim)
	if err != nil || ack1.Message.Ack.Status != settlement.AckStatusACK {
		t.Fatalf("orch1 submit buyer claim failed: %v", err)
	}

	sellerClaim := &settlement.SettleClaimPayload{
		Context: &settlement.BecknContext{
			Action:    "settle",
			MessageID: "msg-pg-seller-01",
		},
		SettlementID: "SETTLE-PG-01",
		NPPairID:     "BAP-PG:BPP-PG",
		SubmittedBy:  "seller_app",
		CollectorAcc: "collector:ondc_settlement",
		ReceiverAcc:  "receiver:ondc_settlement",
		Orders: []settlement.OrderSettlementLine{
			{
				NetworkOrderID:   "ORD-PG-1",
				OrderAmountPaise: 100000,
				CommissionPaise:  3000,
				BFFPaise:         1000,
				SoRDuePaise:      96000,
			},
		},
	}
	ack2, err := orch1.SubmitClaim(ctx, sellerClaim)
	if err != nil || ack2.Message.Ack.Status != settlement.AckStatusACK {
		t.Fatalf("orch1 submit seller claim failed: %v", err)
	}

	batches1 := orch1.ListBatches()
	if len(batches1) != 1 {
		t.Fatalf("Expected 1 batch in orch1, got %d", len(batches1))
	}
	batchID := batches1[0].BatchID

	// 2. Simulate API process crash & restart: Create a fresh Orchestrator instance
	orch2 := settlement.NewOrchestrator(pgLedger, matcher, cal, mockTaxCalculator{}, nil)
	orch2.SetPool(pool) // SetPool triggers RestoreFromPostgres

	// Verify claims restored
	claimRecord, ok := orch2.GetClaim("msg-pg-buyer-01")
	if !ok || claimRecord == nil {
		t.Fatalf("Expected msg-pg-buyer-01 to be restored in orch2")
	}
	if claimRecord.Claim.NPPairID != "BAP-PG:BPP-PG" {
		t.Errorf("Expected NPPairID BAP-PG:BPP-PG, got %s", claimRecord.Claim.NPPairID)
	}

	// Verify batch restored
	restoredBatch, ok := orch2.GetBatch(batchID)
	if !ok || restoredBatch == nil {
		t.Fatalf("Expected batch %s to be restored in orch2", batchID)
	}
	if restoredBatch.Status != settlement.BatchStatusSettled {
		t.Errorf("Expected restored batch status settled, got %s", restoredBatch.Status)
	}
	if restoredBatch.MatchedCount != 1 {
		t.Errorf("Expected matched count 1, got %d", restoredBatch.MatchedCount)
	}
	if restoredBatch.NetPayoutPaise != 98400 {
		t.Errorf("Expected net payout 98400 paise, got %d", restoredBatch.NetPayoutPaise)
	}
	if restoredBatch.TotalTaxPaise != 600 {
		t.Errorf("Expected total tax 600 paise, got %d", restoredBatch.TotalTaxPaise)
	}
	if restoredBatch.MerkleRoot == "" {
		t.Errorf("Expected non-empty MerkleRoot on restored batch")
	}
	if len(restoredBatch.Instructions) == 0 {
		t.Errorf("Expected instructions to be restored for batch %s", batchID)
	}

	// 3. Verify Idempotent Replay on restored instance returns ACK without double-settlement
	ackReplay, err := orch2.SubmitClaim(ctx, buyerClaim)
	if err != nil {
		t.Fatalf("Replay on restored orchestrator failed: %v", err)
	}
	if ackReplay.Message.Ack.Status != settlement.AckStatusACK {
		t.Errorf("Expected ACK on replay, got %s", ackReplay.Message.Ack.Status)
	}
	if len(orch2.ListBatches()) != 1 {
		t.Errorf("Expected still 1 batch after replay, got %d", len(orch2.ListBatches()))
	}
}

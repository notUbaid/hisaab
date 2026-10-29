package ledger_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/ubaid/hisaab/internal/ledger"
)

func TestValidateLines_BalancedTwoLegs(t *testing.T) {
	lines := []ledger.EntryLine{
		{AccountName: "collector:ondc_settlement", Direction: ledger.Debit, AmountPaise: 150000},
		{AccountName: "receiver:ondc_settlement", Direction: ledger.Credit, AmountPaise: 150000},
	}

	total, err := ledger.ValidateLines(lines)
	if err != nil {
		t.Fatalf("expected valid balanced lines, got err: %v", err)
	}
	if total != 150000 {
		t.Fatalf("expected total 150000, got %d", total)
	}
}

func TestValidateLines_BalancedMultiLegs(t *testing.T) {
	lines := []ledger.EntryLine{
		{AccountName: "ext:buyer_clearing", Direction: ledger.Debit, AmountPaise: 100000},
		{AccountName: "receiver:operative", Direction: ledger.Credit, AmountPaise: 85000},
		{AccountName: "collector:operative", Direction: ledger.Credit, AmountPaise: 12000},
		{AccountName: "fees:ondc_network", Direction: ledger.Credit, AmountPaise: 3000},
	}

	total, err := ledger.ValidateLines(lines)
	if err != nil {
		t.Fatalf("expected valid multi-leg lines, got err: %v", err)
	}
	if total != 100000 {
		t.Fatalf("expected total 100000, got %d", total)
	}
}

func TestValidateLines_Unbalanced(t *testing.T) {
	lines := []ledger.EntryLine{
		{AccountName: "collector:ondc_settlement", Direction: ledger.Debit, AmountPaise: 150000},
		{AccountName: "receiver:ondc_settlement", Direction: ledger.Credit, AmountPaise: 149999},
	}

	_, err := ledger.ValidateLines(lines)
	if err == nil {
		t.Fatalf("expected error for unbalanced lines, got nil")
	}
}

func TestValidateLines_ZeroOrNegativeAmount(t *testing.T) {
	linesZero := []ledger.EntryLine{
		{AccountName: "collector:ondc_settlement", Direction: ledger.Debit, AmountPaise: 0},
		{AccountName: "receiver:ondc_settlement", Direction: ledger.Credit, AmountPaise: 0},
	}
	if _, err := ledger.ValidateLines(linesZero); err == nil {
		t.Fatalf("expected error for zero amount line, got nil")
	}

	linesNeg := []ledger.EntryLine{
		{AccountName: "collector:ondc_settlement", Direction: ledger.Debit, AmountPaise: -500},
		{AccountName: "receiver:ondc_settlement", Direction: ledger.Credit, AmountPaise: -500},
	}
	if _, err := ledger.ValidateLines(linesNeg); err == nil {
		t.Fatalf("expected error for negative amount line, got nil")
	}
}

func TestValidateLines_EmptyAccountName(t *testing.T) {
	lines := []ledger.EntryLine{
		{AccountName: "", Direction: ledger.Debit, AmountPaise: 5000},
		{AccountName: "receiver:ondc_settlement", Direction: ledger.Credit, AmountPaise: 5000},
	}
	if _, err := ledger.ValidateLines(lines); err == nil {
		t.Fatalf("expected error for empty account name, got nil")
	}
}

func TestValidateLines_InsufficientLines(t *testing.T) {
	lines := []ledger.EntryLine{
		{AccountName: "collector:ondc_settlement", Direction: ledger.Debit, AmountPaise: 5000},
	}
	if _, err := ledger.ValidateLines(lines); err == nil {
		t.Fatalf("expected error for single line, got nil")
	}
}

func TestComputeRequestHash_DeterministicAndOrderAgnostic(t *testing.T) {
	reqA := ledger.PostRequest{
		IdempotencyKey: "idem-hash-01",
		CauseType:      ledger.CauseClaim,
		CauseID:        101,
		Description:    "Order settlement leg",
		Lines: []ledger.EntryLine{
			{AccountName: "collector:ondc_settlement", Direction: ledger.Debit, AmountPaise: 5000},
			{AccountName: "receiver:ondc_settlement", Direction: ledger.Credit, AmountPaise: 5000},
		},
	}

	// Reverse line order
	reqB := ledger.PostRequest{
		IdempotencyKey: "idem-hash-01",
		CauseType:      ledger.CauseClaim,
		CauseID:        101,
		Description:    "Order settlement leg",
		Lines: []ledger.EntryLine{
			{AccountName: "receiver:ondc_settlement", Direction: ledger.Credit, AmountPaise: 5000},
			{AccountName: "collector:ondc_settlement", Direction: ledger.Debit, AmountPaise: 5000},
		},
	}

	hashA := ledger.ComputeRequestHash(reqA)
	hashB := ledger.ComputeRequestHash(reqB)

	if hashA != hashB {
		t.Fatalf("expected deterministic hash invariant across line order, got %s != %s", hashA, hashB)
	}

	// Changing description or amount must yield distinct hash
	reqC := reqA
	reqC.Description = "Modified description"
	if ledger.ComputeRequestHash(reqC) == hashA {
		t.Fatalf("expected different hash when description changed")
	}
}

func TestMemoryLedger_SuccessfulPost(t *testing.T) {
	mem := ledger.NewMemoryLedger()
	if err := mem.SeedStandardAccounts(); err != nil {
		t.Fatalf("failed to seed accounts: %v", err)
	}

	ctx := context.Background()

	// Initial funding: credit collector:ondc_settlement
	fundReq := ledger.PostRequest{
		IdempotencyKey: "fund-01",
		CauseType:      ledger.CauseAdjustment,
		CauseID:        1,
		Description:    "Initial funding of nodal pool",
		Lines: []ledger.EntryLine{
			{AccountName: "ext:nodal_clearing", Direction: ledger.Debit, AmountPaise: 500000},
			{AccountName: "collector:ondc_settlement", Direction: ledger.Credit, AmountPaise: 500000},
		},
	}
	fundRes, err := mem.Post(ctx, fundReq)
	if err != nil {
		t.Fatalf("fund post failed: %v", err)
	}
	if fundRes.TransactionID != 1 {
		t.Fatalf("expected txID 1, got %d", fundRes.TransactionID)
	}

	bal, err := mem.GetBalance("collector:ondc_settlement")
	if err != nil {
		t.Fatalf("failed to get balance: %v", err)
	}
	if bal != 500000 {
		t.Fatalf("expected balance 500000, got %d", bal)
	}

	// Normal settlement: debit collector, credit receiver
	settleReq := ledger.PostRequest{
		IdempotencyKey: "settle-01",
		CauseType:      ledger.CauseInstruction,
		CauseID:        2,
		Description:    "Settlement order payout",
		Lines: []ledger.EntryLine{
			{AccountName: "collector:ondc_settlement", Direction: ledger.Debit, AmountPaise: 200000},
			{AccountName: "receiver:ondc_settlement", Direction: ledger.Credit, AmountPaise: 200000},
		},
	}
	_, err = mem.Post(ctx, settleReq)
	if err != nil {
		t.Fatalf("settlement post failed: %v", err)
	}

	balCollector, _ := mem.GetBalance("collector:ondc_settlement")
	balReceiver, _ := mem.GetBalance("receiver:ondc_settlement")
	if balCollector != 300000 {
		t.Fatalf("expected collector balance 300000, got %d", balCollector)
	}
	if balReceiver != 200000 {
		t.Fatalf("expected receiver balance 200000, got %d", balReceiver)
	}

	// Audit global invariants
	if err := mem.AuditInvariants(); err != nil {
		t.Fatalf("audit invariants failed: %v", err)
	}
}

func TestMemoryLedger_IdempotencyReplayIdentical(t *testing.T) {
	mem := ledger.NewMemoryLedger()
	_ = mem.SeedStandardAccounts()
	ctx := context.Background()

	req := ledger.PostRequest{
		IdempotencyKey: "idem-test-key-01",
		CauseType:      ledger.CauseAdjustment,
		CauseID:        10,
		Description:    "Idempotency trial",
		Lines: []ledger.EntryLine{
			{AccountName: "ext:buyer_clearing", Direction: ledger.Debit, AmountPaise: 10000},
			{AccountName: "receiver:operative", Direction: ledger.Credit, AmountPaise: 10000},
		},
	}

	res1, err := mem.Post(ctx, req)
	if err != nil {
		t.Fatalf("first post failed: %v", err)
	}

	// Replay exact same request
	res2, err := mem.Post(ctx, req)
	if err != nil {
		t.Fatalf("second post failed: %v", err)
	}

	if res1.TransactionID != res2.TransactionID {
		t.Fatalf("expected identical transaction ID on replay, got %d != %d", res1.TransactionID, res2.TransactionID)
	}
	if len(mem.GetTransactions()) != 1 {
		t.Fatalf("expected only 1 transaction recorded, found %d", len(mem.GetTransactions()))
	}
}

func TestMemoryLedger_IdempotencyReplayMismatchedPayload(t *testing.T) {
	mem := ledger.NewMemoryLedger()
	_ = mem.SeedStandardAccounts()
	ctx := context.Background()

	req1 := ledger.PostRequest{
		IdempotencyKey: "idem-clash-01",
		CauseType:      ledger.CauseAdjustment,
		CauseID:        10,
		Description:    "Payload A",
		Lines: []ledger.EntryLine{
			{AccountName: "ext:buyer_clearing", Direction: ledger.Debit, AmountPaise: 10000},
			{AccountName: "receiver:operative", Direction: ledger.Credit, AmountPaise: 10000},
		},
	}

	if _, err := mem.Post(ctx, req1); err != nil {
		t.Fatalf("first post failed: %v", err)
	}

	// Same key, different amount
	req2 := req1
	req2.Lines = []ledger.EntryLine{
		{AccountName: "ext:buyer_clearing", Direction: ledger.Debit, AmountPaise: 20000},
		{AccountName: "receiver:operative", Direction: ledger.Credit, AmountPaise: 20000},
	}

	_, err := mem.Post(ctx, req2)
	if err == nil {
		t.Fatalf("expected ErrDuplicateIdempotency on payload mismatch, got nil")
	}
}

func TestMemoryLedger_ConstrainedAccountRejectsOverdraft(t *testing.T) {
	mem := ledger.NewMemoryLedger()
	_ = mem.SeedStandardAccounts()
	ctx := context.Background()

	// collector:ondc_settlement starts at 0 balance and is constrained
	// Attempt to debit 10000 without funding
	overdraftReq := ledger.PostRequest{
		IdempotencyKey: "overdraft-01",
		CauseType:      ledger.CauseInstruction,
		CauseID:        50,
		Description:    "Unauthorized debit without funds",
		Lines: []ledger.EntryLine{
			{AccountName: "collector:ondc_settlement", Direction: ledger.Debit, AmountPaise: 10000},
			{AccountName: "receiver:ondc_settlement", Direction: ledger.Credit, AmountPaise: 10000},
		},
	}

	_, err := mem.Post(ctx, overdraftReq)
	if err == nil {
		t.Fatalf("expected overdraft rejection for constrained account, got nil")
	}

	// Verify balance remains 0
	bal, _ := mem.GetBalance("collector:ondc_settlement")
	if bal != 0 {
		t.Fatalf("expected balance 0, got %d", bal)
	}
}

func TestMemoryLedger_UnconstrainedAccountPermitsNegative(t *testing.T) {
	mem := ledger.NewMemoryLedger()
	_ = mem.SeedStandardAccounts()
	ctx := context.Background()

	// ext:buyer_clearing is unconstrained (clearing account can float)
	req := ledger.PostRequest{
		IdempotencyKey: "unconstrained-01",
		CauseType:      ledger.CauseClaim,
		CauseID:        99,
		Description:    "Clearing float debit",
		Lines: []ledger.EntryLine{
			{AccountName: "ext:buyer_clearing", Direction: ledger.Debit, AmountPaise: 50000},
			{AccountName: "receiver:operative", Direction: ledger.Credit, AmountPaise: 50000},
		},
	}

	_, err := mem.Post(ctx, req)
	if err != nil {
		t.Fatalf("expected unconstrained account debit to succeed, got %v", err)
	}

	bal, _ := mem.GetBalance("ext:buyer_clearing")
	if bal != -50000 {
		t.Fatalf("expected negative balance -50000, got %d", bal)
	}
}

func TestMemoryLedger_NonExistentAccountRejection(t *testing.T) {
	mem := ledger.NewMemoryLedger()
	_ = mem.SeedStandardAccounts()
	ctx := context.Background()

	req := ledger.PostRequest{
		IdempotencyKey: "ghost-01",
		CauseType:      ledger.CauseClaim,
		CauseID:        77,
		Description:    "Posting to non existent account",
		Lines: []ledger.EntryLine{
			{AccountName: "ghost:non_existent", Direction: ledger.Debit, AmountPaise: 5000},
			{AccountName: "receiver:operative", Direction: ledger.Credit, AmountPaise: 5000},
		},
	}

	_, err := mem.Post(ctx, req)
	if err == nil {
		t.Fatalf("expected ErrAccountNotFound, got nil")
	}
}

func TestMemoryLedger_ConcurrentPostsThreadSafety(t *testing.T) {
	mem := ledger.NewMemoryLedger()
	_ = mem.SeedStandardAccounts()
	ctx := context.Background()

	numGoroutines := 25
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			req := ledger.PostRequest{
				IdempotencyKey: fmt.Sprintf("conc-req-%d", id),
				CauseType:      ledger.CauseClaim,
				CauseID:        int64(id),
				Description:    fmt.Sprintf("Concurrent line %d", id),
				Lines: []ledger.EntryLine{
					{AccountName: "ext:buyer_clearing", Direction: ledger.Debit, AmountPaise: 1000},
					{AccountName: "receiver:operative", Direction: ledger.Credit, AmountPaise: 1000},
				},
			}
			_, err := mem.Post(ctx, req)
			if err != nil {
				t.Errorf("concurrent post %d failed: %v", id, err)
			}
		}(i)
	}

	wg.Wait()

	if len(mem.GetTransactions()) != numGoroutines {
		t.Fatalf("expected %d transactions recorded, got %d", numGoroutines, len(mem.GetTransactions()))
	}
	if len(mem.GetEntries()) != numGoroutines*2 {
		t.Fatalf("expected %d entries recorded, got %d", numGoroutines*2, len(mem.GetEntries()))
	}

	if err := mem.AuditInvariants(); err != nil {
		t.Fatalf("audit invariants failed after concurrent postings: %v", err)
	}
}

package ledgertest

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ubaid/hisaab/internal/ledger"
)

// Run executes the unified conformance test suite against any ledger.Engine implementation.
// Both MemoryLedger and PostgreSQL Ledger must pass this identical suite to guarantee behavioral parity.
func Run(t *testing.T, factory func(t *testing.T) ledger.Engine) {
	t.Run("IdempotentReplay", func(t *testing.T) {
		eng := factory(t)
		ctx := context.Background()

		req := ledger.PostRequest{
			IdempotencyKey: "test-idem-replay-01",
			CauseType:      ledger.CauseAdjustment,
			CauseID:        1,
			Description:    "Initial funding for replay test",
			Lines: []ledger.EntryLine{
				{AccountName: "ext:nodal_clearing", Direction: ledger.Debit, AmountPaise: 50000},
				{AccountName: "collector:ondc_settlement", Direction: ledger.Credit, AmountPaise: 50000},
			},
			PostedAt: time.Now(),
		}

		res1, err := eng.Post(ctx, req)
		if err != nil {
			t.Fatalf("First post failed: %v", err)
		}
		if res1 == nil || res1.TransactionID == 0 {
			t.Fatalf("Expected valid post result, got %+v", res1)
		}

		// Replay identical request
		res2, err := eng.Post(ctx, req)
		if err != nil {
			t.Fatalf("Replay failed with error: %v", err)
		}
		if res2.TransactionID != res1.TransactionID {
			t.Fatalf("Expected replayed TransactionID %d, got %d", res1.TransactionID, res2.TransactionID)
		}

		bal, err := eng.Balance(ctx, "collector:ondc_settlement")
		if err != nil {
			t.Fatalf("Balance check failed: %v", err)
		}
		if bal != 50000 {
			t.Fatalf("Expected balance 50000 paise after idempotent replay, got %d", bal)
		}
	})

	t.Run("PayloadMismatch", func(t *testing.T) {
		eng := factory(t)
		ctx := context.Background()

		req1 := ledger.PostRequest{
			IdempotencyKey: "test-idem-mismatch-01",
			CauseType:      ledger.CauseAdjustment,
			CauseID:        2,
			Description:    "Original payload",
			Lines: []ledger.EntryLine{
				{AccountName: "ext:nodal_clearing", Direction: ledger.Debit, AmountPaise: 30000},
				{AccountName: "collector:ondc_settlement", Direction: ledger.Credit, AmountPaise: 30000},
			},
			PostedAt: time.Now(),
		}
		_, err := eng.Post(ctx, req1)
		if err != nil {
			t.Fatalf("Initial post failed: %v", err)
		}

		// Same key with different payload
		req2 := req1
		req2.Description = "Altered payload description"
		_, err = eng.Post(ctx, req2)
		if err == nil || !errors.Is(err, ledger.ErrDuplicateIdempotency) {
			t.Fatalf("Expected ErrDuplicateIdempotency on payload mismatch, got: %v", err)
		}
	})

	t.Run("ConcurrentOverdraft", func(t *testing.T) {
		eng := factory(t)
		ctx := context.Background()

		// Initial funding of 10,000 paise on constrained account collector:ondc_settlement
		_, err := eng.Post(ctx, ledger.PostRequest{
			IdempotencyKey: "fund-overdraft-test",
			CauseType:      ledger.CauseAdjustment,
			CauseID:        3,
			Description:    "Fund 10,000 paise",
			Lines: []ledger.EntryLine{
				{AccountName: "ext:nodal_clearing", Direction: ledger.Debit, AmountPaise: 10000},
				{AccountName: "collector:ondc_settlement", Direction: ledger.Credit, AmountPaise: 10000},
			},
			PostedAt: time.Now(),
		})
		if err != nil {
			t.Fatalf("Funding failed: %v", err)
		}

		// 20 concurrent workers attempting to debit 1,000 paise each
		var wg sync.WaitGroup
		var successCount int64
		var failureCount int64
		const workers = 20

		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				_, postErr := eng.Post(ctx, ledger.PostRequest{
					IdempotencyKey: fmt.Sprintf("overdraft-worker-%d", workerID),
					CauseType:      ledger.CauseInstruction,
					CauseID:        int64(100 + workerID),
					Description:    fmt.Sprintf("Debit leg worker %d", workerID),
					Lines: []ledger.EntryLine{
						{AccountName: "collector:ondc_settlement", Direction: ledger.Debit, AmountPaise: 1000},
						{AccountName: "receiver:ondc_settlement", Direction: ledger.Credit, AmountPaise: 1000},
					},
					PostedAt: time.Now(),
				})
				if postErr == nil {
					atomic.AddInt64(&successCount, 1)
				} else if errors.Is(postErr, ledger.ErrNegativeBalance) {
					atomic.AddInt64(&failureCount, 1)
				} else {
					t.Errorf("Unexpected error from worker %d: %v", workerID, postErr)
				}
			}(i)
		}
		wg.Wait()

		if successCount != 10 {
			t.Fatalf("Expected exactly 10 successful debits (10,000 / 1,000), got %d", successCount)
		}
		if failureCount != 10 {
			t.Fatalf("Expected exactly 10 rejected debits with ErrNegativeBalance, got %d", failureCount)
		}

		bal, err := eng.Balance(ctx, "collector:ondc_settlement")
		if err != nil {
			t.Fatalf("Balance failed: %v", err)
		}
		if bal != 0 {
			t.Fatalf("Expected final constrained balance 0, got %d (overdraft occurred!)", bal)
		}
	})

	t.Run("UnbalancedRejected", func(t *testing.T) {
		eng := factory(t)
		ctx := context.Background()

		_, err := eng.Post(ctx, ledger.PostRequest{
			IdempotencyKey: "unbalanced-test-01",
			CauseType:      ledger.CauseAdjustment,
			CauseID:        4,
			Description:    "Unbalanced transaction",
			Lines: []ledger.EntryLine{
				{AccountName: "collector:ondc_settlement", Direction: ledger.Debit, AmountPaise: 5000},
				{AccountName: "receiver:ondc_settlement", Direction: ledger.Credit, AmountPaise: 4999},
			},
			PostedAt: time.Now(),
		})
		if err == nil || !errors.Is(err, ledger.ErrUnbalancedTransaction) {
			t.Fatalf("Expected ErrUnbalancedTransaction, got: %v", err)
		}
	})

	t.Run("ZeroOrNegativeAmount", func(t *testing.T) {
		eng := factory(t)
		ctx := context.Background()

		_, err := eng.Post(ctx, ledger.PostRequest{
			IdempotencyKey: "zero-amt-01",
			CauseType:      ledger.CauseAdjustment,
			CauseID:        5,
			Description:    "Zero amount",
			Lines: []ledger.EntryLine{
				{AccountName: "collector:ondc_settlement", Direction: ledger.Debit, AmountPaise: 0},
				{AccountName: "receiver:ondc_settlement", Direction: ledger.Credit, AmountPaise: 0},
			},
			PostedAt: time.Now(),
		})
		if err == nil || !errors.Is(err, ledger.ErrZeroOrNegativeEntry) {
			t.Fatalf("Expected ErrZeroOrNegativeEntry, got: %v", err)
		}
	})

	t.Run("AmountOverflow", func(t *testing.T) {
		eng := factory(t)
		ctx := context.Background()

		_, err := eng.Post(ctx, ledger.PostRequest{
			IdempotencyKey: "overflow-amt-01",
			CauseType:      ledger.CauseAdjustment,
			CauseID:        6,
			Description:    "Overflow amount",
			Lines: []ledger.EntryLine{
				{AccountName: "collector:ondc_settlement", Direction: ledger.Debit, AmountPaise: ledger.MaxLineAmountPaise + 1},
				{AccountName: "receiver:ondc_settlement", Direction: ledger.Credit, AmountPaise: ledger.MaxLineAmountPaise + 1},
			},
			PostedAt: time.Now(),
		})
		if err == nil || !errors.Is(err, ledger.ErrAmountOverflow) {
			t.Fatalf("Expected ErrAmountOverflow, got: %v", err)
		}
	})

	t.Run("FutureDatedPosting2027", func(t *testing.T) {
		eng := factory(t)
		ctx := context.Background()

		futureDate := time.Date(2027, 2, 15, 10, 0, 0, 0, time.UTC)
		res, err := eng.Post(ctx, ledger.PostRequest{
			IdempotencyKey: "future-2027-posting",
			CauseType:      ledger.CauseAdjustment,
			CauseID:        7,
			Description:    "Post dated in 2027 (verifying partition/boundary safety)",
			Lines: []ledger.EntryLine{
				{AccountName: "ext:nodal_clearing", Direction: ledger.Debit, AmountPaise: 25000},
				{AccountName: "collector:ondc_settlement", Direction: ledger.Credit, AmountPaise: 25000},
			},
			PostedAt: futureDate,
		})
		if err != nil {
			t.Fatalf("Future-dated posting in 2027 failed: %v", err)
		}
		if res == nil || res.TransactionID == 0 {
			t.Fatalf("Expected valid post result for 2027 date, got %+v", res)
		}
	})
}

package bench_test

import (
	"sync"
	"testing"
	"time"

	"github.com/ubaid/hisaab/internal/calendar"
	"github.com/ubaid/hisaab/internal/ledger"
	"github.com/ubaid/hisaab/internal/settlement"
)

// Simulated Account with row lock mutex to model single-row PostgreSQL locking contention
type ContendedAccount struct {
	mu      sync.Mutex
	balance ledger.AmountPaise
}

func (a *ContendedAccount) Debit(amount ledger.AmountPaise) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.balance -= amount
}

func (a *ContendedAccount) Credit(amount ledger.AmountPaise) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.balance += amount
}

// BenchmarkNaivePerOrderLocking benchmarks per-order serialized mutations on a single account
func BenchmarkNaivePerOrderLocking(b *testing.B) {
	collector := &ContendedAccount{balance: 1000000000} // ₹10,000,000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Each order locks collector account directly
		var wg sync.WaitGroup
		orderCount := 100
		wg.Add(orderCount)
		for j := 0; j < orderCount; j++ {
			go func() {
				defer wg.Done()
				collector.Debit(50000) // ₹500
				collector.Credit(50000)
			}()
		}
		wg.Wait()
	}
}

// BenchmarkTwoLevelNetting benchmarks Hisaab's architecture: orders aggregated in memory, exactly one net lock
func BenchmarkTwoLevelNetting(b *testing.B) {
	collector := &ContendedAccount{balance: 1000000000}
	matcher := settlement.NewMatcher(settlement.DefaultPolicy())
	tDate := time.Date(2026, 10, 30, 15, 0, 0, 0, calendar.Location())

	// Prepare 100 orders in batch
	orders := make([]settlement.OrderSettlementLine, 100)
	for i := 0; i < 100; i++ {
		orders[i] = settlement.OrderSettlementLine{
			NetworkOrderID:   "ORD-BENCH",
			OrderAmountPaise: 50000,
			CommissionPaise:  1500,
			BFFPaise:         500,
			SoRDuePaise:      48000,
		}
	}

	buyerClaim := &settlement.SettleClaimPayload{
		NPPairID: "NP-1:NP-2",
		Orders:   orders,
	}
	sellerClaim := &settlement.SettleClaimPayload{
		NPPairID: "NP-1:NP-2",
		Orders:   orders,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 1. In-memory order netting (Two-Level architecture)
		res, err := matcher.MatchAndNet("BATCH-B1", tDate, buyerClaim, sellerClaim, 100000000)
		if err != nil {
			b.Fatalf("Match failed: %v", err)
		}

		// 2. Exactly ONE net posting acquiring the account lock
		totalNet := res.TotalMatched - res.TotalBFF
		collector.Debit(totalNet)
		collector.Credit(totalNet)
	}
}

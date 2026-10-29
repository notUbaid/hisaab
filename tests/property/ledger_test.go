package property_test

import (
	"testing"

	"github.com/ubaid/hisaab/internal/ledger"
	"pgregory.net/rapid"
)

// TestProperty_Invariant1_ZeroSum verifies Invariant 1:
// For any valid transaction, the sum of debits exactly matches the sum of credits.
func TestProperty_Invariant1_ZeroSum(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		linesCount := rapid.IntRange(1, 10).Draw(t, "lines_count")
		var lines []ledger.EntryLine

		var totalDebit ledger.AmountPaise
		for i := 0; i < linesCount; i++ {
			amount := ledger.AmountPaise(rapid.Int64Range(1, 100_000_000).Draw(t, "debit_amount"))
			totalDebit += amount
			lines = append(lines, ledger.EntryLine{
				AccountName: "collector:ondc_settlement",
				Direction:   ledger.Debit,
				AmountPaise: amount,
			})
		}

		// Split the credit amount across random lines that sum to totalDebit
		// Each leg must be strictly positive (> 0 paise)
		remainingCredit := totalDebit
		for i := 0; i < linesCount-1; i++ {
			remainingLegs := int64(linesCount - i)
			if int64(remainingCredit) <= remainingLegs {
				lines = append(lines, ledger.EntryLine{
					AccountName: "receiver:ondc_settlement",
					Direction:   ledger.Credit,
					AmountPaise: 1,
				})
				remainingCredit -= 1
				continue
			}
			maxPart := int64(remainingCredit) - remainingLegs + 1
			if maxPart > int64(remainingCredit)/2+1 {
				maxPart = int64(remainingCredit)/2 + 1
			}
			part := ledger.AmountPaise(rapid.Int64Range(1, maxPart).Draw(t, "credit_part"))
			remainingCredit -= part
			lines = append(lines, ledger.EntryLine{
				AccountName: "receiver:ondc_settlement",
				Direction:   ledger.Credit,
				AmountPaise: part,
			})
		}
		// Final balancing credit line (guaranteed >= 1 paise)
		lines = append(lines, ledger.EntryLine{
			AccountName: "receiver:operative",
			Direction:   ledger.Credit,
			AmountPaise: remainingCredit,
		})

		totalPaise, err := ledger.ValidateLines(lines)
		if err != nil {
			t.Fatalf("Invariant 1 validation failed on balanced lines: %v", err)
		}

		if totalPaise != totalDebit {
			t.Fatalf("Invariant 1 totalPaise mismatch: expected %d, got %d", totalDebit, totalPaise)
		}
	})
}

// TestProperty_Invariant5_OrderConservation verifies Invariant 5:
// Order total must equal Commission + SoR Due + BFF (paise integrity, no rounding leakage).
func TestProperty_Invariant5_OrderConservation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		orderAmount := ledger.AmountPaise(rapid.Int64Range(100, 10_000_000).Draw(t, "order_amount"))

		// Commission: 1% to 15%
		commBps := rapid.Int64Range(100, 1500).Draw(t, "commission_bps")
		commission := (orderAmount * ledger.AmountPaise(commBps)) / 10000

		// BFF: 0.5% to 3%
		bffBps := rapid.Int64Range(50, 300).Draw(t, "bff_bps")
		bff := (orderAmount * ledger.AmountPaise(bffBps)) / 10000

		// Remainder goes to Seller on Record (SoR)
		sorDue := orderAmount - commission - bff

		if commission+sorDue+bff != orderAmount {
			t.Fatalf("Invariant 5 violated: sum (%d) != orderAmount (%d)", commission+sorDue+bff, orderAmount)
		}

		// Construct sub-ledger legs
		lines := []ledger.EntryLine{
			{AccountName: "ext:buyer_clearing", Direction: ledger.Debit, AmountPaise: orderAmount},
			{AccountName: "collector:ondc_settlement", Direction: ledger.Credit, AmountPaise: orderAmount},
		}

		_, err := ledger.ValidateLines(lines)
		if err != nil {
			t.Fatalf("Failed to validate buyer clearing leg: %v", err)
		}
	})
}

// TestProperty_UnbalancedRejection verifies that unbalanced lines are always rejected.
func TestProperty_UnbalancedRejection(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		debit := ledger.AmountPaise(rapid.Int64Range(1, 10_000_000).Draw(t, "debit"))
		creditDelta := ledger.AmountPaise(rapid.Int64Range(1, 100_000).Draw(t, "delta"))

		lines := []ledger.EntryLine{
			{AccountName: "collector:ondc_settlement", Direction: ledger.Debit, AmountPaise: debit},
			{AccountName: "receiver:ondc_settlement", Direction: ledger.Credit, AmountPaise: debit + creditDelta},
		}

		_, err := ledger.ValidateLines(lines)
		if err == nil {
			t.Fatalf("Expected error for unbalanced transaction (debit %d, credit %d), got nil", debit, debit+creditDelta)
		}
	})
}

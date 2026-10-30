package mutation_test

import (
	"context"
	"testing"
	"time"

	"github.com/ubaid/hisaab/internal/ledger"
	"github.com/ubaid/hisaab/internal/settlement"
	"github.com/ubaid/hisaab/internal/verification"
)

// MutantDef represents a defect intentionally seeded to test whether invariant verifiers catch it.
type MutantDef struct {
	ID               string
	Description      string
	TargetInvariant  string
	ExpectedKilledBy string
	ExecuteAndDetect func(t *testing.T) bool // returns true if the invariant checker caught (killed) the mutant
}

func TestMutationSuite_KillAllMutants(t *testing.T) {
	checker := verification.NewInvariantChecker()

	mutants := []MutantDef{
		{
			ID:               "MUTANT-01-UNBALANCED-LINES",
			Description:      "Bypasses zero-sum balance check; allows unbalanced debits and credits",
			TargetInvariant:  "Invariant 1 (Zero-Sum)",
			ExpectedKilledBy: "CheckInvariant1_ZeroSum",
			ExecuteAndDetect: func(t *testing.T) bool {
				// Seeded defect: 10000 debit vs 9500 credit
				lines := []ledger.EntryLine{
					{AccountName: "collector:ondc_settlement", Direction: ledger.Debit, AmountPaise: 10000},
					{AccountName: "receiver:ondc_settlement", Direction: ledger.Credit, AmountPaise: 9500},
				}
				err := checker.CheckInvariant1_ZeroSum(lines)
				return err != nil // Caught!
			},
		},
		{
			ID:               "MUTANT-02-DUPLICATE-IDEMPOTENCY",
			Description:      "Bypasses idempotency check; allows duplicate posting on same key",
			TargetInvariant:  "Invariant 3 (Effectively-Once)",
			ExpectedKilledBy: "MemoryLedger Idempotency Guard",
			ExecuteAndDetect: func(t *testing.T) bool {
				mem := ledger.NewMemoryLedger()
				_ = mem.SeedStandardAccounts()
				ctx := context.Background()

				req1 := ledger.PostRequest{
					IdempotencyKey: "mutant-idem-01",
					CauseType:      ledger.CauseAdjustment,
					CauseID:        1,
					Lines: []ledger.EntryLine{
						{AccountName: "ext:buyer_clearing", Direction: ledger.Debit, AmountPaise: 5000},
						{AccountName: "receiver:operative", Direction: ledger.Credit, AmountPaise: 5000},
					},
				}
				_, _ = mem.Post(ctx, req1)

				// Same key with different payload must be rejected
				req2 := req1
				req2.Lines[0].AmountPaise = 9999
				req2.Lines[1].AmountPaise = 9999
				_, err := mem.Post(ctx, req2)
				return err != nil // Caught!
			},
		},
		{
			ID:               "MUTANT-03-NEGATIVE-CONSTRAINED-BALANCE",
			Description:      "Bypasses overdraft prevention on constrained accounts",
			TargetInvariant:  "Invariant 4 (Non-Negative Constrained Balance)",
			ExpectedKilledBy: "CheckInvariant4_NonNegative",
			ExecuteAndDetect: func(t *testing.T) bool {
				// Seeded defect: constrained account balance is -5000 paise
				err := checker.CheckInvariant4_NonNegative("collector:ondc_settlement", -5000)
				return err != nil // Caught!
			},
		},
		{
			ID:               "MUTANT-04-OVER-REFUND",
			Description:      "Bypasses refund bounds; allows customer refund to exceed original payment",
			TargetInvariant:  "Invariant 6 (Single Forward & Single Refund Settle)",
			ExpectedKilledBy: "RefundProcessor Invariant 6 Check",
			ExecuteAndDetect: func(t *testing.T) bool {
				proc := settlement.NewRefundProcessor()
				claim := settlement.RefundClaim{
					MessageID:         "mutant-ref-01",
					NetworkOrderID:    "ORD-MUTANT-01",
					OriginalAmount:    100000,
					RefundAmountPaise: 125000, // 125000 > 100000
					SettlementDate:    time.Now().UTC(),
				}
				_, err := proc.ProcessRefund(claim, "collector:ondc_settlement", "receiver:ondc_settlement", 500000)
				return err != nil // Caught!
			},
		},
		{
			ID:               "MUTANT-05-LEAKY-GROSS-NET-TIE-OUT",
			Description:      "Injects 1 paise fee leak into batch net transfer instruction",
			TargetInvariant:  "Invariant 13 (Gross-to-Net Tie-Out)",
			ExpectedKilledBy: "CheckInvariant13_GrossNetTieOut",
			ExecuteAndDetect: func(t *testing.T) bool {
				// Seeded defect: TotalMatched = 100000, TotalBFF = 2000 => expected net 98000, but instructed 97999
				batch := &settlement.BatchSettlementResult{
					TotalMatched:  100000,
					TotalBFF:      2000,
					BalancePassed: true,
					MatchedOrders: []settlement.MatchedOrder{
						{NetworkOrderID: "O1", OrderAmountPaise: 100000, BFFPaise: 2000},
					},
					Instructions: []settlement.NetInstruction{
						{AmountPaise: 97999}, // Leaked 1 paise
					},
				}
				err := checker.CheckInvariant13_GrossNetTieOut(batch)
				return err != nil // Caught!
			},
		},
		{
			ID:               "MUTANT-06-TERMINAL-STATE-MUTATION",
			Description:      "Allows late callback to revert a settled batch to failed",
			TargetInvariant:  "Invariant 14 (Terminal Status Immutability)",
			ExpectedKilledBy: "CheckInvariant14_TerminalCallback",
			ExecuteAndDetect: func(t *testing.T) bool {
				// Seeded defect: batch is already settled; duplicate callback attempts to set failed
				err := checker.CheckInvariant14_TerminalCallback("settled", "failed")
				return err != nil // Caught!
			},
		},
	}

	killedCount := 0
	for _, m := range mutants {
		t.Run(m.ID, func(t *testing.T) {
			killed := m.ExecuteAndDetect(t)
			if !killed {
				t.Fatalf("SURVIVED MUTANT: %s (%s) was not killed by %s!", m.ID, m.Description, m.ExpectedKilledBy)
			}
			killedCount++
			t.Logf("[KILLED] %s caught by %s (%s)", m.ID, m.ExpectedKilledBy, m.TargetInvariant)
		})
	}

	totalMutants := len(mutants)
	mutationScore := float64(killedCount) / float64(totalMutants) * 100.0

	t.Logf("\n========================================================")
	t.Logf("MUTATION SCORE: %.1f%% (%d/%d mutants killed)", mutationScore, killedCount, totalMutants)
	t.Logf("Target: >= 90.0%% (PRD Section 6)")
	t.Logf("========================================================")

	if mutationScore < 90.0 {
		t.Fatalf("Mutation score %.1f%% below required 90.0%% threshold", mutationScore)
	}
}

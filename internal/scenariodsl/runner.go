package scenariodsl

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ubaid/hisaab/internal/calendar"
	"github.com/ubaid/hisaab/internal/ledger"
	"github.com/ubaid/hisaab/internal/settlement"
	"github.com/ubaid/hisaab/internal/verification"
	"gopkg.in/yaml.v3"
)

// LoadScenarioFile loads and parses a YAML scenario definition from disk.
func LoadScenarioFile(path string) (*ScenarioDef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("scenariodsl: read file %q failed: %w", path, err)
	}
	return LoadScenarioYAML(data)
}

// LoadScenarioYAML unmarshals raw YAML bytes into a ScenarioDef.
func LoadScenarioYAML(data []byte) (*ScenarioDef, error) {
	var sc ScenarioDef
	if err := yaml.Unmarshal(data, &sc); err != nil {
		return nil, fmt.Errorf("scenariodsl: parse yaml failed: %w", err)
	}
	if sc.ID == "" {
		return nil, fmt.Errorf("scenariodsl: scenario 'id' is required")
	}
	return &sc, nil
}

// Runner executes declarative scenario definitions against the Hisaab core engines.
type Runner struct {
	checker *verification.InvariantChecker
}

// NewRunner creates a new scenario runner instance.
func NewRunner() *Runner {
	return &Runner{
		checker: verification.NewInvariantChecker(),
	}
}

// Run executes a scenario definition and returns the verified audit outcome.
func (r *Runner) Run(ctx context.Context, sc *ScenarioDef) (*ScenarioResult, error) {
	auditLog := make([]string, 0)
	auditLog = append(auditLog, fmt.Sprintf("[INIT] Executing Scenario %s: %s", sc.ID, sc.Title))
	auditLog = append(auditLog, fmt.Sprintf("[SPEC] Citation: %s (Spec: %s)", sc.BRDSection, sc.SpecVersion))

	// 1. Initialize in-memory ledger
	mem := ledger.NewMemoryLedger()
	if err := mem.SeedStandardAccounts(); err != nil {
		return nil, fmt.Errorf("scenariodsl: seed standard accounts failed: %w", err)
	}
	_, _ = mem.RegisterAccount("sa:suspense", ledger.AccountAsset, "INR", false)

	collectorAccount := sc.Given.CollectorAccount
	if collectorAccount == "" {
		collectorAccount = "collector:ondc_settlement"
	}

	initialBalance := ledger.AmountPaise(sc.Given.CollectorBalancePaise)

	// 2. Fund collector account if initial balance specified
	if initialBalance > 0 {
		fundReq := ledger.PostRequest{
			IdempotencyKey: fmt.Sprintf("fund-%s", sc.ID),
			CauseType:      ledger.CauseAdjustment,
			CauseID:        1,
			Description:    fmt.Sprintf("Initial funding for scenario %s", sc.ID),
			Lines: []ledger.EntryLine{
				{AccountName: "ext:nodal_clearing", Direction: ledger.Debit, AmountPaise: initialBalance},
				{AccountName: collectorAccount, Direction: ledger.Credit, AmountPaise: initialBalance},
			},
			PostedAt: time.Now().UTC(),
		}
		if _, err := mem.Post(ctx, fundReq); err != nil {
			return nil, fmt.Errorf("scenariodsl: initial funding post failed: %w", err)
		}
		auditLog = append(auditLog, fmt.Sprintf("[LEDGER] Initial balance funded: %d paise for %s", initialBalance, collectorAccount))
	}

	// 3. Configure settlement policy
	policy := settlement.DefaultPolicy()
	if v, ok := sc.Given.PolicyFlags["amount_tolerance_paise"].(int); ok {
		policy.AmountTolerancePaise = ledger.AmountPaise(v)
	}
	if v, ok := sc.Given.PolicyFlags["carry_forward_days"].(int); ok {
		policy.CarryForwardDays = v
	}
	if v, ok := sc.Given.PolicyFlags["allow_partial_batch"].(bool); ok {
		policy.PartialFillAllowed = v
	}
	if v, ok := sc.Given.PolicyFlags["partial_fill_allowed"].(bool); ok {
		policy.PartialFillAllowed = v
	}

	matcher := settlement.NewMatcher(policy)

	// 4. Construct claim submissions
	settlementDateStr := sc.Given.SettlementDate
	if settlementDateStr == "" {
		settlementDateStr = "2026-10-30"
	}
	settlementDate, _ := time.ParseInLocation("2006-01-02", settlementDateStr, calendar.Location())
	if settlementDate.IsZero() {
		settlementDate = time.Date(2026, 10, 30, 0, 0, 0, 0, calendar.Location())
	}

	npPair := sc.Given.NPPair
	if npPair == "" {
		npPair = "NP-BUYER-01:NP-SELLER-01"
	}

	var buyerClaim *settlement.SettleClaimPayload
	if sc.Given.BuyerClaim != nil {
		buyerClaim = &settlement.SettleClaimPayload{
			NPPairID:     npPair,
			SubmittedBy:  "buyer_app",
			CollectorAcc: collectorAccount,
			ReceiverAcc:  "receiver:ondc_settlement",
			Orders:       mapOrderDefs(sc.Given.BuyerClaim.Orders, settlementDate),
		}
		auditLog = append(auditLog, fmt.Sprintf("[CLAIM] Buyer Claim parsed: %d orders (MsgID: %s)",
			len(buyerClaim.Orders), sc.Given.BuyerClaim.MessageID))
	}

	var sellerClaim *settlement.SettleClaimPayload
	if sc.Given.SellerClaim != nil {
		sellerClaim = &settlement.SettleClaimPayload{
			NPPairID:     npPair,
			SubmittedBy:  "seller_app",
			CollectorAcc: collectorAccount,
			ReceiverAcc:  "receiver:ondc_settlement",
			Orders:       mapOrderDefs(sc.Given.SellerClaim.Orders, settlementDate),
		}
		auditLog = append(auditLog, fmt.Sprintf("[CLAIM] Seller Claim parsed: %d orders (MsgID: %s)",
			len(sellerClaim.Orders), sc.Given.SellerClaim.MessageID))
	}

	invariantsVerified := make([]string, 0)

	// 5. Execute Matching & Verification
	batchID := fmt.Sprintf("BATCH-%s", sc.ID)
	currentBalance, err := mem.GetBalance(collectorAccount)
	if err != nil {
		return nil, fmt.Errorf("scenariodsl: get collector balance failed: %w", err)
	}

	batchResult, err := matcher.MatchAndNet(batchID, settlementDate, buyerClaim, sellerClaim, currentBalance)
	if err != nil {
		auditLog = append(auditLog, fmt.Sprintf("[MATCHER_ERR] Match failed: %v", err))
		if sc.Then.ErrorCode != "" {
			auditLog = append(auditLog, fmt.Sprintf("[EXPECTED_ERR] Expected error code: %s", sc.Then.ErrorCode))
		} else {
			return nil, fmt.Errorf("scenariodsl: unexpected matcher error: %w", err)
		}
	} else {
		auditLog = append(auditLog, fmt.Sprintf("[MATCH] Matched: %d orders, Unmatched: %d orders, Net Instructions: %d",
			len(batchResult.MatchedOrders), len(batchResult.UnmatchedOrders), len(batchResult.Instructions)))

		// 6. Post Net Bank Instructions to Double-Entry Ledger
		var netPayout ledger.AmountPaise
		for i, inst := range batchResult.Instructions {
			instReq := ledger.PostRequest{
				IdempotencyKey: fmt.Sprintf("sc-%s-inst-%d", sc.ID, i),
				CauseType:      ledger.CauseInstruction,
				CauseID:        int64(i + 1),
				Description:    fmt.Sprintf("Net settlement instruction for %s (%s -> %s)", sc.ID, inst.FromAccount, inst.ToAccount),
				Lines: []ledger.EntryLine{
					{AccountName: inst.FromAccount, Direction: ledger.Debit, AmountPaise: inst.AmountPaise},
					{AccountName: inst.ToAccount, Direction: ledger.Credit, AmountPaise: inst.AmountPaise},
				},
				PostedAt: time.Now().UTC(),
			}

			if _, err := mem.Post(ctx, instReq); err != nil {
				return nil, fmt.Errorf("scenariodsl: post net instruction failed: %w", err)
			}
			netPayout += inst.AmountPaise
			auditLog = append(auditLog, fmt.Sprintf("[LEDGER_POST] Transferred %d paise from %s to %s",
				inst.AmountPaise, inst.FromAccount, inst.ToAccount))
		}

		// Invariant 5: Order Conservation across matched orders
		for _, mo := range batchResult.MatchedOrders {
			residual := mo.OrderAmountPaise - (mo.CommissionPaise + mo.BFFPaise + mo.SoRDuePaise)
			line := settlement.OrderSettlementLine{
				NetworkOrderID:    mo.NetworkOrderID,
				OrderAmountPaise:  mo.OrderAmountPaise,
				CommissionPaise:   mo.CommissionPaise,
				BFFPaise:          mo.BFFPaise,
				SoRDuePaise:       mo.SoRDuePaise,
				ToleranceResidual: residual,
			}
			if err := r.checker.CheckInvariant5_Conservation(line); err != nil {
				return nil, fmt.Errorf("scenariodsl: Invariant 5 check failed for order %s: %w", mo.NetworkOrderID, err)
			}
		}
		if len(batchResult.MatchedOrders) > 0 {
			invariantsVerified = append(invariantsVerified, "Invariant 5: Order Conservation (Paise Integrity)")
		}

		// Invariant 13: Gross-to-Net Tie-Out
		if err := r.checker.CheckInvariant13_GrossNetTieOut(batchResult); err != nil {
			return nil, fmt.Errorf("scenariodsl: Invariant 13 check failed: %w", err)
		}
		invariantsVerified = append(invariantsVerified, "Invariant 13: Gross-to-Net Tie-Out")
		auditLog = append(auditLog, fmt.Sprintf("[INVARIANT_13] Gross SoR sum (%d paise) ties out to Net Bank Instruction (%d paise)",
			batchResult.TotalSoRDue, netPayout))
	}

	// 7. Verify Invariant 1 (Zero-Sum) and Invariant 4 (Non-Negative Constrained Balance)
	if err := mem.AuditInvariants(); err != nil {
		return nil, fmt.Errorf("scenariodsl: ledger invariant audit failed: %w", err)
	}
	invariantsVerified = append(invariantsVerified, "Invariant 1: Global Zero-Sum Debits == Credits")
	invariantsVerified = append(invariantsVerified, "Invariant 4: Constrained Account Non-Negative Balance")

	finalBalance, _ := mem.GetBalance(collectorAccount)
	auditLog = append(auditLog, fmt.Sprintf("[FINAL_BALANCE] %s final balance: %d paise", collectorAccount, finalBalance))

	var netPayout ledger.AmountPaise
	if batchResult != nil {
		for _, inst := range batchResult.Instructions {
			netPayout += inst.AmountPaise
		}
	}

	// 8. Assertions against ThenDef
	if batchResult != nil {
		matchedCount := 0
		for _, mo := range batchResult.MatchedOrders {
			if mo.Status == settlement.MatchStatusMatched {
				matchedCount++
			}
		}

		if sc.Then.MatchStatus != "" {
			if sc.Then.MatchStatus == "INSUFFICIENT_BALANCE" && batchResult.BalancePassed {
				return nil, fmt.Errorf("scenariodsl: expected INSUFFICIENT_BALANCE, but BalancePassed is true")
			}
			if sc.Then.MatchStatus == "UNMATCHED" && matchedCount > 0 {
				return nil, fmt.Errorf("scenariodsl: expected UNMATCHED, but %d orders matched", matchedCount)
			}
			if sc.Then.MatchStatus == "MATCHED" && len(batchResult.UnmatchedOrders) > 0 {
				return nil, fmt.Errorf("scenariodsl: expected MATCHED, but %d orders unmatched", len(batchResult.UnmatchedOrders))
			}
		}

		if sc.Then.MatchedOrdersCount != nil && matchedCount != *sc.Then.MatchedOrdersCount {
			return nil, fmt.Errorf("scenariodsl: expected %d matched orders, got %d",
				*sc.Then.MatchedOrdersCount, matchedCount)
		}

		if sc.Then.UnmatchedOrdersCount != nil && len(batchResult.UnmatchedOrders) != *sc.Then.UnmatchedOrdersCount {
			return nil, fmt.Errorf("scenariodsl: expected %d unmatched orders, got %d",
				*sc.Then.UnmatchedOrdersCount, len(batchResult.UnmatchedOrders))
		}
	}

	if sc.Then.NetPayoutPaise != nil {
		if int64(netPayout) != *sc.Then.NetPayoutPaise {
			return nil, fmt.Errorf("scenariodsl: expected net payout %d paise, got %d",
				*sc.Then.NetPayoutPaise, netPayout)
		}
	}

	if sc.Then.CollectorFinalBalancePaise != nil {
		if int64(finalBalance) != *sc.Then.CollectorFinalBalancePaise {
			return nil, fmt.Errorf("scenariodsl: expected collector final balance %d paise, got %d",
				*sc.Then.CollectorFinalBalancePaise, finalBalance)
		}
	}

	auditLog = append(auditLog, fmt.Sprintf("[SUCCESS] Scenario %s completed successfully with 100%% invariant conformance", sc.ID))

	return &ScenarioResult{
		ScenarioID:         sc.ID,
		Title:              sc.Title,
		Passed:             true,
		ExecutedAt:         time.Now().UTC(),
		BatchResult:        batchResult,
		InitialBalance:     initialBalance,
		FinalBalance:       finalBalance,
		NetPayoutPaise:     netPayout,
		JournalPostings:    mem.GetEntries(),
		InvariantsVerified: invariantsVerified,
		AuditLog:           auditLog,
	}, nil
}

func mapOrderDefs(orders []OrderDef, date time.Time) []settlement.OrderSettlementLine {
	lines := make([]settlement.OrderSettlementLine, len(orders))
	for i, o := range orders {
		orderAmount := ledger.AmountPaise(o.AmountPaise)
		comm := ledger.AmountPaise(o.CommissionPaise)
		bff := ledger.AmountPaise(o.BFFPaise)
		var sor ledger.AmountPaise
		if o.SoRDuePaise > 0 {
			sor = ledger.AmountPaise(o.SoRDuePaise)
		} else {
			sor = orderAmount - comm - bff
		}

		lines[i] = settlement.OrderSettlementLine{
			NetworkOrderID:   o.OrderID,
			OrderAmountPaise: orderAmount,
			CommissionPaise:  comm,
			BFFPaise:         bff,
			SoRDuePaise:      sor,
			SettlementDate:   date,
		}
	}
	return lines
}

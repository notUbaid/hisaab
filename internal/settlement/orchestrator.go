package settlement

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ubaid/hisaab/internal/audit"
	"github.com/ubaid/hisaab/internal/calendar"
	"github.com/ubaid/hisaab/internal/ledger"
)

var (
	ErrBatchAlreadySettled = errors.New("orchestrator: settlement batch has already been settled")
)

// FailpointTrigger abstracts crash testing hook invocation.
type FailpointTrigger interface {
	Trigger(name string) error
}

// TaxBreakdown contains calculated paise withholding legs.
type TaxBreakdown struct {
	OrderAmountPaise ledger.AmountPaise `json:"order_amount_paise"`
	CommissionPaise  ledger.AmountPaise `json:"commission_paise"`
	BFFPaise         ledger.AmountPaise `json:"bff_paise"`
	TDSPaise         ledger.AmountPaise `json:"tds_paise"`
	GSTTCSPaise      ledger.AmountPaise `json:"gst_tcs_paise"`
	TotalTaxPaise    ledger.AmountPaise `json:"total_tax_paise"`
}

// TaxCalculator abstracts statutory tax withholding calculations (TDS & GST TCS).
type TaxCalculator interface {
	CalculateOrderTax(orderAmount, commission, bff ledger.AmountPaise) (*TaxBreakdown, error)
}

// LegacyTaxCalculator provides backward compatibility for single-value tax functions.
type LegacyTaxCalculator interface {
	CalculateOrderTaxPaise(orderAmount, commission, bff ledger.AmountPaise) ledger.AmountPaise
}

// BatchStatus represents the execution lifecycle of a settlement batch.
type BatchStatus string

const (
	BatchStatusPending          BatchStatus = "PENDING"
	BatchStatusSettled          BatchStatus = "SETTLED"
	BatchStatusPartiallySettled BatchStatus = "PARTIALLY_SETTLED"
	BatchStatusRejected         BatchStatus = "REJECTED"
	BatchStatusException        BatchStatus = "EXCEPTION"

	// Legacy aliases
	BatchStatusOpen     BatchStatus = "PENDING"
	BatchStatusMatching BatchStatus = "MATCHING"
)

// BatchSummary models the persistent status of a settlement run across an NP pair.
type BatchSummary struct {
	BatchID         string             `json:"batch_id"`
	NPPairID        string             `json:"np_pair_id"`
	SettlementDate  string             `json:"settlement_date"`
	Status          BatchStatus        `json:"status"`
	MatchedCount    int                `json:"matched_count"`
	UnmatchedCount  int                `json:"unmatched_count"`
	TotalGrossPaise ledger.AmountPaise `json:"total_gross_paise"`
	NetPayoutPaise  ledger.AmountPaise `json:"net_payout_paise"`
	TotalTaxPaise   ledger.AmountPaise `json:"total_tax_paise"`
	TDSPaise        ledger.AmountPaise `json:"tds_paise,omitempty"`
	GSTTCSPaise     ledger.AmountPaise `json:"gst_tcs_paise,omitempty"`
	MerkleRoot      string             `json:"merkle_root,omitempty"`
	Instructions    []NetInstruction   `json:"instructions"`
	MatchedOrders   []MatchedOrder     `json:"matched_orders"`
	UnmatchedOrders []MatchedOrder     `json:"unmatched_orders"`
	SettledAt       *time.Time         `json:"settled_at,omitempty"`
	CreatedAt       time.Time          `json:"created_at"`
}

// OutboxItem represents a staged event emitted atomically alongside ledger postings.
type OutboxItem struct {
	ID        int64           `json:"id"`
	EventType string          `json:"event_type"`
	Payload   json.RawMessage `json:"payload"`
	Status    string          `json:"status"` // pending, processing, done, failed
	CreatedAt time.Time       `json:"created_at"`
}

// Orchestrator coordinates the end-to-end settlement lifecycle.
type Orchestrator struct {
	mu         sync.RWMutex
	ledger     ledger.Engine
	matcher    *Matcher
	calendar   *calendar.Calendar
	taxCalc    any
	failpoints FailpointTrigger
	pool       *pgxpool.Pool

	// Persistent/In-memory indices
	claimsByID   map[string]*StoredClaimRecord             // key: message_id
	claimsByPair map[string]map[string]*SettleClaimPayload // pairKey -> ("buyer_app" | "seller_app") -> claim
	batches      map[string]*BatchSummary                  // key: batch_id
	outboxItems  []OutboxItem
	nextOutboxID int64
}

// NewOrchestrator creates a new settlement orchestrator instance.
func NewOrchestrator(
	eng ledger.Engine,
	matcher *Matcher,
	cal *calendar.Calendar,
	taxCalc any,
	fp FailpointTrigger,
) *Orchestrator {
	if matcher == nil {
		matcher = NewMatcher(DefaultPolicy())
	}
	if cal == nil {
		cal = calendar.New(nil)
	}

	return &Orchestrator{
		ledger:       eng,
		matcher:      matcher,
		calendar:     cal,
		taxCalc:      taxCalc,
		failpoints:   fp,
		claimsByID:   make(map[string]*StoredClaimRecord),
		claimsByPair: make(map[string]map[string]*SettleClaimPayload),
		batches:      make(map[string]*BatchSummary),
		outboxItems:  make([]OutboxItem, 0),
		nextOutboxID: 1,
	}
}

// SetPool assigns a PostgreSQL pool for persistent storage of claims, batches, and outbox,
// and immediately restores persistent state from PostgreSQL.
func (o *Orchestrator) SetPool(pool *pgxpool.Pool) {
	o.mu.Lock()
	o.pool = pool
	o.mu.Unlock()

	if pool != nil {
		_ = o.RestoreFromPostgres(context.Background())
	}
}

// SetTaxCalculator configures the statutory tax calculation engine.
func (o *Orchestrator) SetTaxCalculator(taxCalc any) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.taxCalc = taxCalc
}

// SubmitClaim processes an incoming claim from an NP, stores it, and if the counterpart
// claim is present, triggers the complete settlement, ledger posting, and outbox emission atomically.
func (o *Orchestrator) SubmitClaim(ctx context.Context, claim *SettleClaimPayload) (*AckResponse, error) {
	if claim == nil || claim.Context == nil {
		return nil, errors.New("orchestrator: nil claim or context")
	}

	// 1. Validate payload schema and business invariants
	if err := ValidateClaimPayload(claim); err != nil {
		return nil, fmt.Errorf("orchestrator: validation failed: %w", err)
	}

	rawBytes, _ := json.Marshal(claim)
	hasher := sha256.New()
	hasher.Write(rawBytes)
	reqHash := hex.EncodeToString(hasher.Sum(nil))

	msgID := claim.Context.MessageID
	pairKey := fmt.Sprintf("%s|%s", claim.NPPairID, claim.SettlementID)
	if claim.SettlementID == "" {
		pairKey = fmt.Sprintf("%s|default", claim.NPPairID)
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	// 2. Idempotency Check
	if existing, exists := o.claimsByID[msgID]; exists {
		if existing.RequestHash == reqHash {
			return &existing.AckResponse, nil
		}
		return nil, fmt.Errorf("orchestrator: duplicate message_id with conflicting payload: %s", msgID)
	}

	ack := AckResponse{
		Context: claim.Context,
	}
	ack.Message.Ack.Status = AckStatusACK

	record := &StoredClaimRecord{
		MessageID:   msgID,
		RequestHash: reqHash,
		Claim:       *claim,
		AckResponse: ack,
		ReceivedAt:  time.Now().UTC(),
	}
	o.claimsByID[msgID] = record

	// Persist claim to PostgreSQL if pool is available
	if o.pool != nil {
		var totalAmount int64
		for _, ord := range claim.Orders {
			totalAmount += int64(ord.OrderAmountPaise)
		}
		var claimID int64
		settleDateStr := time.Now().UTC().Format("2006-01-02")
		err := o.pool.QueryRow(ctx, `
			INSERT INTO claims (message_id, settlement_id, np_pair_id, settlement_date, submitted_by, collector_acc, receiver_acc, request_hash, order_count, total_amount_paise, raw_payload)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (message_id) DO UPDATE SET raw_payload = EXCLUDED.raw_payload
			RETURNING id
		`, msgID, claim.SettlementID, claim.NPPairID, settleDateStr, claim.SubmittedBy, claim.CollectorAcc, claim.ReceiverAcc, reqHash, len(claim.Orders), totalAmount, rawBytes).Scan(&claimID)
		if err == nil && claimID > 0 {
			for _, ord := range claim.Orders {
				_, _ = o.pool.Exec(ctx, `
					INSERT INTO claim_lines (claim_id, network_order_id, amount_paise, commission_paise, bff_paise, sor_due_paise)
					VALUES ($1, $2, $3, $4, $5, $6)
				`, claimID, ord.NetworkOrderID, int64(ord.OrderAmountPaise), int64(ord.CommissionPaise), int64(ord.BFFPaise), int64(ord.SoRDuePaise))
			}
		}
	}

	// Store by NP pair
	roleMap, ok := o.claimsByPair[pairKey]
	if !ok {
		roleMap = make(map[string]*SettleClaimPayload)
		o.claimsByPair[pairKey] = roleMap
	}
	roleMap[claim.SubmittedBy] = claim

	// 3. Check for Counterpart Presence
	buyerClaim := roleMap["buyer_app"]
	sellerClaim := roleMap["seller_app"]

	if buyerClaim != nil && sellerClaim != nil {
		// Both counterpart claims present: execute settlement batch immediately
		batchID := fmt.Sprintf("BATCH-%s-%d", claim.NPPairID, time.Now().UnixNano())
		settleDate := o.calendar.SettlementCutOffDetermined(time.Now())

		if err := o.executeBatchInternal(ctx, batchID, settleDate, buyerClaim, sellerClaim); err != nil {
			return nil, fmt.Errorf("orchestrator: settlement execution failed: %w", err)
		}
	}

	return &ack, nil
}

// executeBatchInternal matches, computes taxes, posts double-entry journal transactions,
// and stages outbox events in a single atomic sequence. Caller must hold o.mu.
func (o *Orchestrator) executeBatchInternal(
	ctx context.Context,
	batchID string,
	settleDate time.Time,
	buyerClaim *SettleClaimPayload,
	sellerClaim *SettleClaimPayload,
) error {
	collectorAcc := buyerClaim.CollectorAcc
	if collectorAcc == "" {
		collectorAcc = "collector:ondc_settlement"
	}
	receiverAcc := sellerClaim.ReceiverAcc
	if receiverAcc == "" {
		receiverAcc = "receiver:ondc_settlement"
	}

	// Failpoint hook: before matching
	if o.failpoints != nil {
		if err := o.failpoints.Trigger("after_subledger_before_net"); err != nil {
			return err
		}
	}

	// 1. Fetch available collector balance from ledger
	currentBalance, err := o.ledger.Balance(ctx, collectorAcc)
	if err != nil {
		return fmt.Errorf("orchestrator: balance check failed: %w", err)
	}

	// 2. Execute matching and netting
	batchResult, err := o.matcher.MatchAndNet(batchID, settleDate, buyerClaim, sellerClaim, currentBalance)
	if err != nil {
		return fmt.Errorf("orchestrator: matcher error: %w", err)
	}

	matchedCount := len(batchResult.MatchedOrders)
	unmatchedCount := len(batchResult.UnmatchedOrders)

	// 3. State Machine Check: If insufficient balance or 0 matched orders, mark REJECTED
	if !batchResult.BalancePassed || matchedCount == 0 {
		now := time.Now().UTC()
		o.batches[batchID] = &BatchSummary{
			BatchID:         batchID,
			NPPairID:        buyerClaim.NPPairID,
			SettlementDate:  settleDate.Format("2006-01-02"),
			Status:          BatchStatusRejected,
			MatchedCount:    0,
			UnmatchedCount:  unmatchedCount,
			TotalGrossPaise: 0,
			NetPayoutPaise:  0,
			TotalTaxPaise:   0,
			Instructions:    nil,
			MatchedOrders:   batchResult.MatchedOrders,
			UnmatchedOrders: batchResult.UnmatchedOrders,
			CreatedAt:       now,
		}

		callbackPayload, _ := json.Marshal(map[string]any{
			"batch_id":        batchID,
			"settlement_date": settleDate.Format("2006-01-02"),
			"status":          "REJECTED",
			"reason":          batchResult.ErrorCode,
			"matched_orders":  0,
			"net_payout":      0,
		})

		o.outboxItems = append(o.outboxItems, OutboxItem{
			ID:        o.nextOutboxID,
			EventType: "beckn.on_settle_callback",
			Payload:   callbackPayload,
			Status:    "pending",
			CreatedAt: now,
		})
		o.nextOutboxID++

		if o.pool != nil {
			_, _ = o.pool.Exec(ctx, `
				INSERT INTO outbox (event_type, payload, status, next_attempt_at)
				VALUES ('beckn.on_settle_callback', $1, 'pending', NOW())
			`, callbackPayload)
		}

		return nil
	}

	// 4. Compute statutory tax withholding on matched orders with error propagation
	var totalGrossMatched ledger.AmountPaise
	var totalTDS ledger.AmountPaise
	var totalTCS ledger.AmountPaise

	for _, ord := range batchResult.MatchedOrders {
		totalGrossMatched += ord.OrderAmountPaise
		if o.taxCalc != nil {
			if calc, ok := o.taxCalc.(TaxCalculator); ok {
				tb, taxErr := calc.CalculateOrderTax(ord.OrderAmountPaise, ord.CommissionPaise, ord.BFFPaise)
				if taxErr != nil {
					return fmt.Errorf("orchestrator: statutory tax calculation failed on order %s: %w", ord.NetworkOrderID, taxErr)
				}
				if tb != nil {
					totalTDS += tb.TDSPaise
					totalTCS += tb.GSTTCSPaise
				}
			} else if leg, ok := o.taxCalc.(LegacyTaxCalculator); ok {
				tax := leg.CalculateOrderTaxPaise(ord.OrderAmountPaise, ord.CommissionPaise, ord.BFFPaise)
				totalTDS += (tax * 10) / 60
				totalTCS += tax - ((tax * 10) / 60)
			}
		}
	}
	totalTaxDeductions := totalTDS + totalTCS

	// 5. Build Double-Entry Journal Lines with Exact Tax Deductions & Balance Conservation
	// Find base seller payout instruction
	var grossSellerPayout ledger.AmountPaise
	for _, inst := range batchResult.Instructions {
		if inst.ToAccount == receiverAcc {
			grossSellerPayout += inst.AmountPaise
		}
	}

	if totalTaxDeductions > grossSellerPayout {
		return fmt.Errorf("orchestrator: total statutory tax deductions (%d paise) exceed gross seller payout (%d paise)",
			totalTaxDeductions, grossSellerPayout)
	}

	netSellerPayout := grossSellerPayout - totalTaxDeductions

	// Rebuild instructions with net post-tax seller transfer and explicit tax withholding
	finalInstructions := make([]NetInstruction, 0)
	if netSellerPayout > 0 {
		finalInstructions = append(finalInstructions, NetInstruction{
			FromAccount: collectorAcc,
			ToAccount:   receiverAcc,
			AmountPaise: netSellerPayout,
			BatchID:     batchID,
		})
	}
	if totalTDS > 0 {
		finalInstructions = append(finalInstructions, NetInstruction{
			FromAccount: collectorAcc,
			ToAccount:   "tax:tds_payable",
			AmountPaise: totalTDS,
			BatchID:     batchID,
		})
	}
	if totalTCS > 0 {
		finalInstructions = append(finalInstructions, NetInstruction{
			FromAccount: collectorAcc,
			ToAccount:   "tax:gst_tcs_payable",
			AmountPaise: totalTCS,
			BatchID:     batchID,
		})
	}
	if batchResult.ToleranceResidualPaise > 0 {
		finalInstructions = append(finalInstructions, NetInstruction{
			FromAccount: collectorAcc,
			ToAccount:   "sa:tolerance_adjustment",
			AmountPaise: batchResult.ToleranceResidualPaise,
			BatchID:     batchID,
		})
	}

	// Double-entry ledger entries
	journalLines := make([]ledger.EntryLine, 0)

	// Debit collector account for full obligations leaving the nodal account
	journalLines = append(journalLines, ledger.EntryLine{
		AccountName: collectorAcc,
		Direction:   ledger.Debit,
		AmountPaise: grossSellerPayout,
	})

	// Credit receiver operative account for net post-tax payout
	if netSellerPayout > 0 {
		journalLines = append(journalLines, ledger.EntryLine{
			AccountName: receiverAcc,
			Direction:   ledger.Credit,
			AmountPaise: netSellerPayout,
		})
	}

	// Credit statutory tax liability accounts
	if totalTDS > 0 {
		journalLines = append(journalLines, ledger.EntryLine{
			AccountName: "tax:tds_payable",
			Direction:   ledger.Credit,
			AmountPaise: totalTDS,
		})
	}
	if totalTCS > 0 {
		journalLines = append(journalLines, ledger.EntryLine{
			AccountName: "tax:gst_tcs_payable",
			Direction:   ledger.Credit,
			AmountPaise: totalTCS,
		})
	}

	// Credit tolerance residual if present
	if batchResult.ToleranceResidualPaise > 0 {
		journalLines = append(journalLines, ledger.EntryLine{
			AccountName: collectorAcc,
			Direction:   ledger.Debit,
			AmountPaise: batchResult.ToleranceResidualPaise,
		})
		journalLines = append(journalLines, ledger.EntryLine{
			AccountName: "sa:tolerance_adjustment",
			Direction:   ledger.Credit,
			AmountPaise: batchResult.ToleranceResidualPaise,
		})
	}

	// Failpoint hook: before ledger posting
	if o.failpoints != nil {
		if err := o.failpoints.Trigger("after_debit_before_credit"); err != nil {
			return err
		}
	}

	// Determine final batch status
	var finalBatchStatus BatchStatus
	var callbackStatus string
	if unmatchedCount > 0 || batchResult.ErrorCode != "" {
		finalBatchStatus = BatchStatusPartiallySettled
		callbackStatus = "PARTIALLY_SETTLED"
	} else {
		finalBatchStatus = BatchStatusSettled
		callbackStatus = "SETTLED"
	}

	now := time.Now().UTC()
	postReq := ledger.PostRequest{
		IdempotencyKey: fmt.Sprintf("batch-%s", batchID),
		CauseType:      ledger.CauseInstruction,
		CauseID:        1,
		Description:    fmt.Sprintf("ONDC Settlement Batch %s", batchID),
		Lines:          journalLines,
		PostedAt:       now,
	}

	// Compute genuine RFC 6962 Merkle tree over canonical ledger entries
	canonicalBytes := make([][]byte, 0, len(journalLines))
	for _, l := range journalLines {
		canonicalBytes = append(canonicalBytes, audit.CanonicalEntryBytes(1, l))
	}
	var merkleRoot string
	if tree, mErr := audit.BuildMerkleTree(canonicalBytes); mErr == nil && tree != nil {
		merkleRoot = tree.RootHex
	}

	// 6. Atomic Persistence to PostgreSQL or In-Memory
	if o.pool != nil {
		tx, txErr := o.pool.Begin(ctx)
		if txErr != nil {
			return fmt.Errorf("orchestrator: begin postgres tx failed: %w", txErr)
		}
		defer func() {
			_ = tx.Rollback(ctx)
		}()

		var runID int64
		if err := tx.QueryRow(ctx, "INSERT INTO batch_runs DEFAULT VALUES RETURNING run_id").Scan(&runID); err != nil {
			return fmt.Errorf("orchestrator: create batch_run failed: %w", err)
		}

		// Rebuild canonical bytes with actual runID
		canonicalBytesRun := make([][]byte, 0, len(journalLines))
		for _, l := range journalLines {
			canonicalBytesRun = append(canonicalBytesRun, audit.CanonicalEntryBytes(runID, l))
		}
		if tree, mErr := audit.BuildMerkleTree(canonicalBytesRun); mErr == nil && tree != nil {
			merkleRoot = tree.RootHex
			_, _ = tx.Exec(ctx, `
				INSERT INTO batch_roots (batch_id, root_hash, leaf_count, algorithm, np_pair_id)
				VALUES ($1, $2, $3, 'sha256', $4)
				ON CONFLICT (batch_id) DO UPDATE SET root_hash = EXCLUDED.root_hash, leaf_count = EXCLUDED.leaf_count
			`, batchID, merkleRoot, len(canonicalBytesRun), buyerClaim.NPPairID)
		}

		settleDateOnly, _ := time.Parse("2006-01-02", settleDate.Format("2006-01-02"))
		_, err = tx.Exec(ctx, `
			INSERT INTO settlement_batches (batch_id, np_pair_id, settlement_date, status, matched_count, unmatched_count, total_gross_paise, net_payout_paise, total_tax_paise, tds_paise, gst_tcs_paise, merkle_root, run_id, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		`, batchID, buyerClaim.NPPairID, settleDateOnly, string(finalBatchStatus), matchedCount, unmatchedCount,
			int64(totalGrossMatched), int64(netSellerPayout), int64(totalTaxDeductions), int64(totalTDS), int64(totalTCS), merkleRoot, runID, now)
		if err != nil {
			return fmt.Errorf("orchestrator: insert settlement_batches failed: %w", err)
		}

		// Mark counterpart claims as settled in persistent store
		_, _ = tx.Exec(ctx, `UPDATE claims SET status = 'settled' WHERE message_id IN ($1, $2)`, buyerClaim.Context.MessageID, sellerClaim.Context.MessageID)

		// Post double-entry lines atomically inside this transaction
		if pgLedger, ok := o.ledger.(*ledger.Ledger); ok {
			if _, err := pgLedger.PostTx(ctx, tx, postReq); err != nil {
				return fmt.Errorf("orchestrator: postgres PostTx failed: %w", err)
			}
		} else {
			if _, err := o.ledger.Post(ctx, postReq); err != nil {
				return fmt.Errorf("orchestrator: ledger post failed: %w", err)
			}
		}

		// Persist instructions
		for _, inst := range finalInstructions {
			_, err = tx.Exec(ctx, `
				INSERT INTO instructions (batch_id, from_account, to_account, amount_paise, currency, status, created_at)
				VALUES ($1, $2, $3, $4, 'INR', 'pending', $5)
			`, batchID, inst.FromAccount, inst.ToAccount, int64(inst.AmountPaise), now)
			if err != nil {
				return fmt.Errorf("orchestrator: insert instruction failed: %w", err)
			}
		}

		// Persist outbox events
		for _, inst := range finalInstructions {
			instPayload, _ := json.Marshal(inst)
			_, _ = tx.Exec(ctx, `
				INSERT INTO outbox (event_type, payload, status, next_attempt_at)
				VALUES ('settlement.bank_instruction', $1, 'pending', NOW())
			`, instPayload)
		}

		callbackPayload, _ := json.Marshal(map[string]any{
			"batch_id":        batchID,
			"settlement_date": settleDate.Format("2006-01-02"),
			"status":          callbackStatus,
			"matched_orders":  matchedCount,
			"unmatched":       unmatchedCount,
			"net_payout":      netSellerPayout,
			"total_tax":       totalTaxDeductions,
			"merkle_root":     merkleRoot,
		})
		_, _ = tx.Exec(ctx, `
			INSERT INTO outbox (event_type, payload, status, next_attempt_at)
			VALUES ('beckn.on_settle_callback', $1, 'pending', NOW())
		`, callbackPayload)

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("orchestrator: commit settlement transaction failed: %w", err)
		}
	} else {
		// In-Memory Posting
		if len(journalLines) > 0 {
			if _, err := o.ledger.Post(ctx, postReq); err != nil {
				return fmt.Errorf("orchestrator: ledger post failed: %w", err)
			}
		}
	}

	// 7. Update in-memory outbox and summaries
	for _, inst := range finalInstructions {
		instPayload, _ := json.Marshal(inst)
		o.outboxItems = append(o.outboxItems, OutboxItem{
			ID:        o.nextOutboxID,
			EventType: "settlement.bank_instruction",
			Payload:   instPayload,
			Status:    "pending",
			CreatedAt: now,
		})
		o.nextOutboxID++
	}

	callbackPayload, _ := json.Marshal(map[string]any{
		"batch_id":        batchID,
		"settlement_date": settleDate.Format("2006-01-02"),
		"status":          callbackStatus,
		"matched_orders":  matchedCount,
		"unmatched":       unmatchedCount,
		"net_payout":      netSellerPayout,
		"total_tax":       totalTaxDeductions,
		"merkle_root":     merkleRoot,
	})
	o.outboxItems = append(o.outboxItems, OutboxItem{
		ID:        o.nextOutboxID,
		EventType: "beckn.on_settle_callback",
		Payload:   callbackPayload,
		Status:    "pending",
		CreatedAt: now,
	})
	o.nextOutboxID++

	o.batches[batchID] = &BatchSummary{
		BatchID:         batchID,
		NPPairID:        buyerClaim.NPPairID,
		SettlementDate:  settleDate.Format("2006-01-02"),
		Status:          finalBatchStatus,
		MatchedCount:    matchedCount,
		UnmatchedCount:  unmatchedCount,
		TotalGrossPaise: totalGrossMatched,
		NetPayoutPaise:  netSellerPayout,
		TotalTaxPaise:   totalTaxDeductions,
		TDSPaise:        totalTDS,
		GSTTCSPaise:     totalTCS,
		MerkleRoot:      merkleRoot,
		Instructions:    finalInstructions,
		MatchedOrders:   batchResult.MatchedOrders,
		UnmatchedOrders: batchResult.UnmatchedOrders,
		SettledAt:       &now,
		CreatedAt:       now,
	}

	// Failpoint hook: after commit before ACK
	if o.failpoints != nil {
		if err := o.failpoints.Trigger("after_commit_before_ack"); err != nil {
			return err
		}
	}

	return nil
}

// GetBatch returns summary details of an executed settlement batch.
func (o *Orchestrator) GetBatch(batchID string) (*BatchSummary, bool) {
	o.mu.RLock()
	b, ok := o.batches[batchID]
	pool := o.pool
	o.mu.RUnlock()

	if ok {
		return b, true
	}

	if pool == nil {
		return nil, false
	}

	// Fallback to live PostgreSQL lookup if not cached in memory
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var npPairID, status, merkleRoot string
	var settleDate, createdAt time.Time
	var completedAt *time.Time
	var matchedCount, unmatchedCount int
	var totalGross, netPayout, totalTax, tds, gstTcs int64

	err := pool.QueryRow(ctx, `
		SELECT np_pair_id, settlement_date, status, matched_count, unmatched_count,
		       total_gross_paise, net_payout_paise, total_tax_paise, tds_paise, gst_tcs_paise,
		       merkle_root, created_at, completed_at
		FROM settlement_batches
		WHERE batch_id = $1
		LIMIT 1
	`, batchID).Scan(&npPairID, &settleDate, &status, &matchedCount, &unmatchedCount,
		&totalGross, &netPayout, &totalTax, &tds, &gstTcs, &merkleRoot, &createdAt, &completedAt)
	if err != nil {
		return nil, false
	}

	var insts []NetInstruction
	instRows, iErr := pool.Query(ctx, `
		SELECT from_account, to_account, amount_paise
		FROM instructions
		WHERE batch_id = $1
	`, batchID)
	if iErr == nil {
		defer instRows.Close()
		for instRows.Next() {
			var fromAcc, toAcc string
			var amt int64
			if instRows.Scan(&fromAcc, &toAcc, &amt) == nil {
				insts = append(insts, NetInstruction{
					FromAccount: fromAcc,
					ToAccount:   toAcc,
					AmountPaise: ledger.AmountPaise(amt),
					BatchID:     batchID,
				})
			}
		}
	}

	summary := &BatchSummary{
		BatchID:         batchID,
		NPPairID:        npPairID,
		SettlementDate:  settleDate.Format("2006-01-02"),
		Status:          BatchStatus(status),
		MatchedCount:    matchedCount,
		UnmatchedCount:  unmatchedCount,
		TotalGrossPaise: ledger.AmountPaise(totalGross),
		NetPayoutPaise:  ledger.AmountPaise(netPayout),
		TotalTaxPaise:   ledger.AmountPaise(totalTax),
		TDSPaise:        ledger.AmountPaise(tds),
		GSTTCSPaise:     ledger.AmountPaise(gstTcs),
		MerkleRoot:      merkleRoot,
		Instructions:    insts,
		CreatedAt:       createdAt,
		SettledAt:       completedAt,
	}

	o.mu.Lock()
	o.batches[batchID] = summary
	o.mu.Unlock()

	return summary, true
}

// GetClaim retrieves a stored claim by message_id.
func (o *Orchestrator) GetClaim(messageID string) (*StoredClaimRecord, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	c, ok := o.claimsByID[messageID]
	return c, ok
}

// ListClaims returns all stored claim records.
func (o *Orchestrator) ListClaims() []*StoredClaimRecord {
	o.mu.RLock()
	defer o.mu.RUnlock()
	res := make([]*StoredClaimRecord, 0, len(o.claimsByID))
	for _, c := range o.claimsByID {
		res = append(res, c)
	}
	return res
}

// RestoreFromPostgres hydrates claims, pending counterpart pairings, and batch summaries
// from PostgreSQL to ensure crash resilience and continuity across service restarts.
func (o *Orchestrator) RestoreFromPostgres(ctx context.Context) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.pool == nil {
		return nil
	}

	// 1. Restore claims from PostgreSQL
	claimRows, err := o.pool.Query(ctx, `
		SELECT message_id, settlement_id, np_pair_id, submitted_by, collector_acc, receiver_acc,
		       request_hash, raw_payload, received_at, status
		FROM claims
		ORDER BY id ASC
	`)
	if err == nil {
		defer claimRows.Close()
		for claimRows.Next() {
			var msgID, settleID, npPairID, submittedBy, collectorAcc, receiverAcc, reqHash, status string
			var rawPayload []byte
			var receivedAt time.Time

			if err := claimRows.Scan(&msgID, &settleID, &npPairID, &submittedBy, &collectorAcc, &receiverAcc,
				&reqHash, &rawPayload, &receivedAt, &status); err == nil {

				var claim SettleClaimPayload
				if len(rawPayload) > 0 && json.Valid(rawPayload) {
					_ = json.Unmarshal(rawPayload, &claim)
				}
				if claim.Context == nil {
					claim.Context = &BecknContext{
						MessageID: msgID,
						Timestamp: receivedAt,
					}
				}
				claim.SettlementID = settleID
				claim.NPPairID = npPairID
				claim.SubmittedBy = submittedBy
				if claim.CollectorAcc == "" {
					claim.CollectorAcc = collectorAcc
				}
				if claim.ReceiverAcc == "" {
					claim.ReceiverAcc = receiverAcc
				}

				// Reconstruct lines if claim.Orders was empty
				if len(claim.Orders) == 0 {
					lineRows, lErr := o.pool.Query(ctx, `
						SELECT cl.network_order_id, cl.amount_paise, cl.commission_paise, cl.bff_paise, cl.sor_due_paise
						FROM claim_lines cl
						JOIN claims c ON cl.claim_id = c.id
						WHERE c.message_id = $1
					`, msgID)
					if lErr == nil {
						for lineRows.Next() {
							var ordID string
							var amt, comm, bff, sor int64
							if lineRows.Scan(&ordID, &amt, &comm, &bff, &sor) == nil {
								claim.Orders = append(claim.Orders, OrderSettlementLine{
									NetworkOrderID:  ordID,
									OrderAmountPaise: ledger.AmountPaise(amt),
									CommissionPaise:  ledger.AmountPaise(comm),
									BFFPaise:         ledger.AmountPaise(bff),
									SoRDuePaise:      ledger.AmountPaise(sor),
								})
							}
						}
						lineRows.Close()
					}
				}

				ack := AckResponse{Context: claim.Context}
				ack.Message.Ack.Status = AckStatusACK

				record := &StoredClaimRecord{
					MessageID:   msgID,
					RequestHash: reqHash,
					Claim:       claim,
					AckResponse: ack,
					ReceivedAt:  receivedAt,
				}
				o.claimsByID[msgID] = record

				// If not yet settled, keep in claimsByPair for counterpart matching
				if status != "settled" {
					pairKey := fmt.Sprintf("%s|%s", claim.NPPairID, claim.SettlementID)
					if claim.SettlementID == "" {
						pairKey = fmt.Sprintf("%s|default", claim.NPPairID)
					}
					roleMap, ok := o.claimsByPair[pairKey]
					if !ok {
						roleMap = make(map[string]*SettleClaimPayload)
						o.claimsByPair[pairKey] = roleMap
					}
					roleMap[claim.SubmittedBy] = &claim
				}
			}
		}
	}

	// 2. Restore settlement batches from PostgreSQL
	batchRows, err := o.pool.Query(ctx, `
		SELECT batch_id, np_pair_id, settlement_date, status, matched_count, unmatched_count,
		       total_gross_paise, net_payout_paise, total_tax_paise, tds_paise, gst_tcs_paise,
		       merkle_root, created_at, completed_at
		FROM settlement_batches
		ORDER BY id ASC
	`)
	if err == nil {
		defer batchRows.Close()
		for batchRows.Next() {
			var batchID, npPairID, status, merkleRoot string
			var settleDate, createdAt time.Time
			var completedAt *time.Time
			var matchedCount, unmatchedCount int
			var totalGross, netPayout, totalTax, tds, gstTcs int64

			if err := batchRows.Scan(&batchID, &npPairID, &settleDate, &status, &matchedCount, &unmatchedCount,
				&totalGross, &netPayout, &totalTax, &tds, &gstTcs, &merkleRoot, &createdAt, &completedAt); err == nil {

				var insts []NetInstruction
				instRows, iErr := o.pool.Query(ctx, `
					SELECT from_account, to_account, amount_paise
					FROM instructions
					WHERE batch_id = $1
				`, batchID)
				if iErr == nil {
					for instRows.Next() {
						var fromAcc, toAcc string
						var amt int64
						if instRows.Scan(&fromAcc, &toAcc, &amt) == nil {
							insts = append(insts, NetInstruction{
								FromAccount: fromAcc,
								ToAccount:   toAcc,
								AmountPaise: ledger.AmountPaise(amt),
								BatchID:     batchID,
							})
						}
					}
					instRows.Close()
				}

				summary := &BatchSummary{
					BatchID:         batchID,
					NPPairID:        npPairID,
					SettlementDate:  settleDate.Format("2006-01-02"),
					Status:          BatchStatus(status),
					MatchedCount:    matchedCount,
					UnmatchedCount:  unmatchedCount,
					TotalGrossPaise: ledger.AmountPaise(totalGross),
					NetPayoutPaise:  ledger.AmountPaise(netPayout),
					TotalTaxPaise:   ledger.AmountPaise(totalTax),
					TDSPaise:        ledger.AmountPaise(tds),
					GSTTCSPaise:     ledger.AmountPaise(gstTcs),
					MerkleRoot:      merkleRoot,
					Instructions:    insts,
					CreatedAt:       createdAt,
					SettledAt:       completedAt,
				}
				o.batches[batchID] = summary
			}
		}
	}

	return nil
}

// ListBatches returns all known settlement batches.
func (o *Orchestrator) ListBatches() []*BatchSummary {
	o.mu.RLock()
	defer o.mu.RUnlock()
	res := make([]*BatchSummary, 0, len(o.batches))
	for _, b := range o.batches {
		res = append(res, b)
	}
	return res
}

// GetPendingOutbox returns all currently pending outbox items.
func (o *Orchestrator) GetPendingOutbox() []OutboxItem {
	o.mu.RLock()
	defer o.mu.RUnlock()
	res := make([]OutboxItem, 0)
	for _, item := range o.outboxItems {
		if item.Status == "pending" {
			res = append(res, item)
		}
	}
	return res
}

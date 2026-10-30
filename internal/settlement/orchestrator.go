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

// TaxCalculator abstracts statutory tax withholding calculations (TDS & GST TCS).
type TaxCalculator interface {
	CalculateOrderTaxPaise(orderAmount, commission, bff ledger.AmountPaise) ledger.AmountPaise
}

// BatchStatus represents the execution lifecycle of a settlement batch.
type BatchStatus string

const (
	BatchStatusOpen      BatchStatus = "open"
	BatchStatusMatching  BatchStatus = "matching"
	BatchStatusSettled   BatchStatus = "settled"
	BatchStatusException BatchStatus = "exception"
)

// BatchSummary models the persistent status of a settlement run across an NP pair.
type BatchSummary struct {
	BatchID          string             `json:"batch_id"`
	NPPairID         string             `json:"np_pair_id"`
	SettlementDate   string             `json:"settlement_date"`
	Status           BatchStatus        `json:"status"`
	MatchedCount     int                `json:"matched_count"`
	UnmatchedCount   int                `json:"unmatched_count"`
	TotalGrossPaise  ledger.AmountPaise `json:"total_gross_paise"`
	NetPayoutPaise   ledger.AmountPaise `json:"net_payout_paise"`
	TotalTaxPaise    ledger.AmountPaise `json:"total_tax_paise"`
	Instructions     []NetInstruction   `json:"instructions"`
	MatchedOrders    []MatchedOrder     `json:"matched_orders"`
	UnmatchedOrders  []MatchedOrder     `json:"unmatched_orders"`
	SettledAt        *time.Time         `json:"settled_at,omitempty"`
	CreatedAt        time.Time          `json:"created_at"`
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
	taxCalc    TaxCalculator
	failpoints FailpointTrigger

	// Persistent/In-memory indices
	claimsByID    map[string]*StoredClaimRecord        // key: message_id
	claimsByPair  map[string]map[string]*SettleClaimPayload // pairKey -> ("buyer_app" | "seller_app") -> claim
	batches       map[string]*BatchSummary             // key: batch_id
	outboxItems   []OutboxItem
	nextOutboxID  int64
}

// NewOrchestrator creates a new settlement orchestrator instance.
func NewOrchestrator(
	eng ledger.Engine,
	matcher *Matcher,
	cal *calendar.Calendar,
	taxCalc TaxCalculator,
	fp FailpointTrigger,
) *Orchestrator {
	if matcher == nil {
		matcher = NewMatcher(DefaultPolicy())
	}
	if cal == nil {
		cal = calendar.New(nil)
	}

	return &Orchestrator{
		ledger:        eng,
		matcher:       matcher,
		calendar:      cal,
		taxCalc:       taxCalc,
		failpoints:    fp,
		claimsByID:    make(map[string]*StoredClaimRecord),
		claimsByPair:  make(map[string]map[string]*SettleClaimPayload),
		batches:       make(map[string]*BatchSummary),
		outboxItems:   make([]OutboxItem, 0),
		nextOutboxID:  1,
	}
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

	// 3. Compute statutory tax withholding on matched orders
	var totalTaxDeductions ledger.AmountPaise
	var totalGrossMatched ledger.AmountPaise

	for _, ord := range batchResult.MatchedOrders {
		totalGrossMatched += ord.OrderAmountPaise
		if o.taxCalc != nil {
			taxPaise := o.taxCalc.CalculateOrderTaxPaise(ord.OrderAmountPaise, ord.CommissionPaise, ord.BFFPaise)
			totalTaxDeductions += taxPaise
		}
	}

	// 4. Build Double-Entry Journal Lines
	// Debit collector account for total matched
	// Credit receiver account for net payout
	// Credit tax withholding accounts if applicable
	// Credit tolerance adjustment account if residual exists
	journalLines := make([]ledger.EntryLine, 0)
	var totalNetInstruction ledger.AmountPaise
	for _, inst := range batchResult.Instructions {
		totalNetInstruction += inst.AmountPaise
		journalLines = append(journalLines, ledger.EntryLine{
			AccountName: inst.FromAccount,
			Direction:   ledger.Debit,
			AmountPaise: inst.AmountPaise,
		})
		journalLines = append(journalLines, ledger.EntryLine{
			AccountName: inst.ToAccount,
			Direction:   ledger.Credit,
			AmountPaise: inst.AmountPaise,
		})
	}

	// Add tolerance residual line if present
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

	// 5. Post to Ledger
	if len(journalLines) > 0 {
		postReq := ledger.PostRequest{
			IdempotencyKey: fmt.Sprintf("batch-%s", batchID),
			CauseType:      ledger.CauseInstruction,
			CauseID:        1,
			Description:    fmt.Sprintf("ONDC Settlement Batch %s", batchID),
			Lines:          journalLines,
			PostedAt:       time.Now().UTC(),
		}
		if _, err := o.ledger.Post(ctx, postReq); err != nil {
			return fmt.Errorf("orchestrator: ledger post failed: %w", err)
		}
	}

	// 6. Stage Outbox Events
	for _, inst := range batchResult.Instructions {
		instPayload, _ := json.Marshal(inst)
		o.outboxItems = append(o.outboxItems, OutboxItem{
			ID:        o.nextOutboxID,
			EventType: "settlement.bank_instruction",
			Payload:   instPayload,
			Status:    "pending",
			CreatedAt: time.Now().UTC(),
		})
		o.nextOutboxID++
	}

	callbackPayload, _ := json.Marshal(map[string]any{
		"batch_id":        batchID,
		"settlement_date": settleDate.Format("2006-01-02"),
		"status":          "SETTLED",
		"matched_orders":  len(batchResult.MatchedOrders),
		"net_payout":      totalNetInstruction,
	})
	o.outboxItems = append(o.outboxItems, OutboxItem{
		ID:        o.nextOutboxID,
		EventType: "beckn.on_settle_callback",
		Payload:   callbackPayload,
		Status:    "pending",
		CreatedAt: time.Now().UTC(),
	})
	o.nextOutboxID++

	// 7. Store Batch Summary
	now := time.Now().UTC()
	o.batches[batchID] = &BatchSummary{
		BatchID:         batchID,
		NPPairID:        buyerClaim.NPPairID,
		SettlementDate:  settleDate.Format("2006-01-02"),
		Status:          BatchStatusSettled,
		MatchedCount:    len(batchResult.MatchedOrders),
		UnmatchedCount:  len(batchResult.UnmatchedOrders),
		TotalGrossPaise: totalGrossMatched,
		NetPayoutPaise:  totalNetInstruction,
		TotalTaxPaise:   totalTaxDeductions,
		Instructions:    batchResult.Instructions,
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
	defer o.mu.RUnlock()
	b, ok := o.batches[batchID]
	return b, ok
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

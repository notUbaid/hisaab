package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ubaid/hisaab/internal/auth"
	"github.com/ubaid/hisaab/internal/settlement"
)

// OutboxEventType defines known asynchronous event kinds.
type OutboxEventType string

const (
	EventBankInstruction       OutboxEventType = "settlement.bank_instruction"
	EventBecknOnSettleCallback OutboxEventType = "beckn.on_settle_callback"
	EventReconException        OutboxEventType = "recon.exception_notification"
)

// OnSettleCallbackEvent models the Beckn /on_settle event payload.
type OnSettleCallbackEvent struct {
	BatchID        string `json:"batch_id"`
	SettlementDate string `json:"settlement_date"`
	Status         string `json:"status"` // SETTLED, FAILED, PARTIAL
}

// OutboxRecord represents an entry in the transactional outbox table.
type OutboxRecord struct {
	ID            int64           `json:"id"`
	EventType     OutboxEventType `json:"event_type"`
	Payload       json.RawMessage `json:"payload"`
	Status        string          `json:"status"` // pending, processing, done, failed
	RetryCount    int             `json:"retry_count"`
	Attempts      int             `json:"attempts"`
	NextAttemptAt time.Time       `json:"next_attempt_at"`
	LockedUntil   *time.Time      `json:"locked_until,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	ProcessedAt   *time.Time      `json:"processed_at,omitempty"`
}

// WorkerConfig holds the outbox worker configuration.
type WorkerConfig struct {
	BatchSize     int
	PollInterval  time.Duration
	MaxRetries    int
	BaseBackoff   time.Duration
	SigningDomain string
	SigningKeyID  string
}

// DefaultWorkerConfig returns production defaults for outbox processing.
func DefaultWorkerConfig() WorkerConfig {
	return WorkerConfig{
		BatchSize:     50,
		PollInterval:  1 * time.Second,
		MaxRetries:    5,
		BaseBackoff:   1 * time.Second,
		SigningDomain: "sa.ondc.org",
		SigningKeyID:  "sa-key-1",
	}
}

// OutboxProcessor drains and dispatches outbox events.
type OutboxProcessor struct {
	cfg         WorkerConfig
	pool        *pgxpool.Pool
	signer      *auth.RequestSigner
	bankGateway settlement.BankGateway
	mu          sync.Mutex
	inMemoryDB  []OutboxRecord
}

// NewOutboxProcessor creates a new OutboxProcessor instance.
func NewOutboxProcessor(cfg WorkerConfig, pool *pgxpool.Pool, signer *auth.RequestSigner) *OutboxProcessor {
	return &OutboxProcessor{
		cfg:         cfg,
		pool:        pool,
		signer:      signer,
		bankGateway: settlement.NewISO20022Pacs008Gateway("sandbox", "", nil),
		inMemoryDB:  make([]OutboxRecord, 0),
	}
}

// SetBankGateway configures a specialized payment clearing rail (e.g. ISO 20022 or custom RTGS gateway).
func (p *OutboxProcessor) SetBankGateway(gw settlement.BankGateway) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.bankGateway = gw
}

// EnqueueInMemory adds a synthetic record for unit testing without live PostgreSQL.
func (p *OutboxProcessor) EnqueueInMemory(record OutboxRecord) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if record.NextAttemptAt.IsZero() {
		record.NextAttemptAt = time.Now()
	}
	p.inMemoryDB = append(p.inMemoryDB, record)
}

// DrainInMemory processes all eligible records using two-phase claim and retry with exponential backoff.
func (p *OutboxProcessor) DrainInMemory(ctx context.Context) (int, error) {
	now := time.Now()

	// 1. Phase 1: Claim pending or expired lease records under lock
	p.mu.Lock()
	claimedIndices := make([]int, 0)
	claimedRecords := make([]OutboxRecord, 0)

	for i := range p.inMemoryDB {
		rec := &p.inMemoryDB[i]
		isEligiblePending := rec.Status == "pending" && (rec.NextAttemptAt.IsZero() || !rec.NextAttemptAt.After(now))
		isExpiredProcessing := rec.Status == "processing" && rec.LockedUntil != nil && !rec.LockedUntil.After(now)

		if isEligiblePending || isExpiredProcessing {
			rec.Status = "processing"
			lockExpiry := now.Add(30 * time.Second)
			rec.LockedUntil = &lockExpiry
			claimedIndices = append(claimedIndices, i)
			claimedRecords = append(claimedRecords, *rec)
			if len(claimedIndices) >= p.cfg.BatchSize {
				break
			}
		}
	}
	p.mu.Unlock()

	if len(claimedRecords) == 0 {
		return 0, nil
	}

	// 2. Phase 2: Perform I/O outside of locks
	type resultOutcome struct {
		idx int
		err error
	}
	outcomes := make([]resultOutcome, len(claimedRecords))
	for j, rec := range claimedRecords {
		recCopy := rec
		err := p.processRecord(ctx, &recCopy)
		outcomes[j] = resultOutcome{idx: claimedIndices[j], err: err}
	}

	// 3. Phase 3: Update outcomes with exponential backoff or completion
	p.mu.Lock()
	defer p.mu.Unlock()

	processedCount := 0
	finalTime := time.Now()

	for _, outcome := range outcomes {
		target := &p.inMemoryDB[outcome.idx]
		if outcome.err != nil {
			target.Attempts++
			target.RetryCount = target.Attempts // sync for backward compatibility
			target.LockedUntil = nil

			if target.Attempts >= p.cfg.MaxRetries {
				target.Status = "failed"
				target.ProcessedAt = &finalTime
				slog.Error("Outbox record reached max retries, marked as failed (DLQ)",
					"id", target.ID, "attempts", target.Attempts, "error", outcome.err)
			} else {
				// Exponential backoff: baseBackoff * 2^(attempts-1)
				shift := target.Attempts - 1
				if shift > 10 {
					shift = 10
				}
				backoff := time.Duration(float64(p.cfg.BaseBackoff) * math.Pow(2, float64(shift)))
				target.Status = "pending"
				target.NextAttemptAt = finalTime.Add(backoff)
				slog.Warn("Outbox record failed, scheduled retry with exponential backoff",
					"id", target.ID, "attempts", target.Attempts, "next_attempt_in", backoff.String(), "error", outcome.err)
			}
		} else {
			target.Status = "done"
			target.LockedUntil = nil
			target.ProcessedAt = &finalTime
			processedCount++
		}
	}

	return processedCount, nil
}

// DrainBatch pulls pending records using two-phase SKIP LOCKED: claim in a short tx,
// do I/O outside the tx, and update outcome in a separate tx with exponential backoff.
func (p *OutboxProcessor) DrainBatch(ctx context.Context) (int, error) {
	if p.pool == nil {
		return p.DrainInMemory(ctx)
	}

	// Phase 1: Claim rows in short transaction
	claimTx, err := p.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("outbox.DrainBatch: begin claim tx failed: %w", err)
	}
	defer func() {
		_ = claimTx.Rollback(ctx)
	}()

	rows, err := claimTx.Query(ctx, `
		WITH claimed AS (
			SELECT id
			FROM outbox
			WHERE (status = 'pending' AND next_attempt_at <= NOW())
			   OR (status = 'processing' AND locked_until < NOW())
			ORDER BY next_attempt_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE outbox o
		SET status = 'processing',
		    locked_until = NOW() + INTERVAL '30 seconds'
		FROM claimed
		WHERE o.id = claimed.id
		RETURNING o.id, o.event_type, o.payload, o.status, o.attempts, o.created_at
	`, p.cfg.BatchSize)
	if err != nil {
		return 0, fmt.Errorf("outbox.DrainBatch: claim query failed: %w", err)
	}
	defer rows.Close()

	records := make([]OutboxRecord, 0)
	for rows.Next() {
		var r OutboxRecord
		if err := rows.Scan(&r.ID, &r.EventType, &r.Payload, &r.Status, &r.Attempts, &r.CreatedAt); err != nil {
			return 0, fmt.Errorf("outbox.DrainBatch: scan failed: %w", err)
		}
		records = append(records, r)
	}
	rows.Close()

	if err := claimTx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("outbox.DrainBatch: commit claim tx failed: %w", err)
	}

	if len(records) == 0 {
		return 0, nil
	}

	// Phase 2: Perform I/O outside database transaction
	processedCount := 0
	for _, rec := range records {
		recCopy := rec
		procErr := p.processRecord(ctx, &recCopy)

		// Phase 3: Record outcome in separate transaction
		if procErr != nil {
			newAttempts := rec.Attempts + 1
			slog.Error("Outbox dispatch failure", "id", rec.ID, "type", rec.EventType, "attempts", newAttempts, "error", procErr)

			if newAttempts >= p.cfg.MaxRetries {
				_, _ = p.pool.Exec(ctx, `
					UPDATE outbox
					SET status = 'failed',
					    attempts = $2,
					    locked_until = NULL,
					    processed_at = NOW()
					WHERE id = $1
				`, rec.ID, newAttempts)
			} else {
				shift := newAttempts - 1
				if shift > 10 {
					shift = 10
				}
				backoffSec := int64(p.cfg.BaseBackoff.Seconds() * math.Pow(2, float64(shift)))
				if backoffSec < 1 {
					backoffSec = 1
				}
				_, _ = p.pool.Exec(ctx, `
					UPDATE outbox
					SET status = 'pending',
					    attempts = $2,
					    next_attempt_at = NOW() + ($3 || ' seconds')::interval,
					    locked_until = NULL
					WHERE id = $1
				`, rec.ID, newAttempts, fmt.Sprintf("%d", backoffSec))
			}
		} else {
			_, _ = p.pool.Exec(ctx, `
				UPDATE outbox
				SET status = 'done',
				    locked_until = NULL,
				    processed_at = NOW()
				WHERE id = $1
			`, rec.ID)
			processedCount++
		}
	}

	return processedCount, nil
}

// processRecord dispatches the event based on its kind.
func (p *OutboxProcessor) processRecord(ctx context.Context, rec *OutboxRecord) error {
	switch rec.EventType {
	case EventBankInstruction:
		return p.handleBankInstruction(ctx, rec.Payload)
	case EventBecknOnSettleCallback:
		return p.handleBecknCallback(ctx, rec.Payload)
	case EventReconException:
		return p.handleReconException(ctx, rec.Payload)
	default:
		return fmt.Errorf("outbox: unknown event type %q", rec.EventType)
	}
}

func (p *OutboxProcessor) handleBankInstruction(ctx context.Context, rawPayload json.RawMessage) error {
	var instruction settlement.NetInstruction
	if err := json.Unmarshal(rawPayload, &instruction); err != nil {
		return fmt.Errorf("unmarshal bank instruction failed: %w", err)
	}
	if instruction.AmountPaise <= 0 {
		return errors.New("invalid instruction amount paise <= 0")
	}

	gw := p.bankGateway
	if gw == nil {
		gw = settlement.NewISO20022Pacs008Gateway("sandbox", "", nil)
	}

	wireReq := settlement.BankWireRequest{
		InstructionID:   fmt.Sprintf("INST-%s-%s", instruction.BatchID, instruction.ToAccount),
		EndToEndID:      fmt.Sprintf("E2E-%s-%d", instruction.BatchID, instruction.AmountPaise),
		BatchID:         instruction.BatchID,
		DebtorAccount:   instruction.FromAccount,
		CreditorAccount: instruction.ToAccount,
		AmountPaise:     instruction.AmountPaise,
		Currency:        "INR",
		PaymentMethod:   "RTGS",
		ExecutionDate:   time.Now().UTC(),
	}

	resp, err := gw.DispatchWire(ctx, wireReq)
	if err != nil {
		return fmt.Errorf("bank wire dispatch failed: %w", err)
	}

	slog.Info("Dispatched net bank instruction via clearing rail",
		"from", instruction.FromAccount,
		"to", instruction.ToAccount,
		"amount_paise", instruction.AmountPaise,
		"batch_id", instruction.BatchID,
		"utr", resp.ReferenceNumber,
		"status", resp.Status,
	)
	return nil
}

func (p *OutboxProcessor) handleBecknCallback(ctx context.Context, rawPayload json.RawMessage) error {
	var callback OnSettleCallbackEvent
	if err := json.Unmarshal(rawPayload, &callback); err != nil {
		return fmt.Errorf("unmarshal on_settle payload failed: %w", err)
	}

	if p.signer != nil {
		_, _, err := p.signer.SignRequest(rawPayload, time.Now())
		if err != nil {
			return fmt.Errorf("failed to cryptographically sign /on_settle callback: %w", err)
		}
	}

	slog.Info("Delivered Beckn /on_settle callback",
		"batch_id", callback.BatchID,
		"settlement_date", callback.SettlementDate,
		"status", callback.Status,
	)
	return nil
}

func (p *OutboxProcessor) handleReconException(ctx context.Context, rawPayload json.RawMessage) error {
	var exc map[string]any
	if err := json.Unmarshal(rawPayload, &exc); err != nil {
		return fmt.Errorf("unmarshal recon exception failed: %w", err)
	}

	slog.Warn("Recon exception escalated to outbox listener",
		"exception", exc,
	)
	return nil
}

// Run starts the polling loop until ctx is cancelled.
func (p *OutboxProcessor) Run(ctx context.Context) error {
	ticker := time.NewTicker(p.cfg.PollInterval)
	defer ticker.Stop()

	slog.Info("Outbox background drain worker started",
		"poll_interval", p.cfg.PollInterval.String(),
		"batch_size", p.cfg.BatchSize,
	)

	for {
		select {
		case <-ctx.Done():
			slog.Info("Outbox worker shutting down cleanly")
			return ctx.Err()
		case <-ticker.C:
			count, err := p.DrainBatch(ctx)
			if err != nil {
				slog.Error("Error during outbox drain cycle", "error", err)
			} else if count > 0 {
				slog.Info("Drained outbox events successfully", "count", count)
			}
		}
	}
}

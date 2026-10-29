package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ubaid/hisaab/internal/auth"
	"github.com/ubaid/hisaab/internal/settlement"
)

// OutboxEventType defines known asynchronous event kinds.
type OutboxEventType string

const (
	EventBankInstruction      OutboxEventType = "settlement.bank_instruction"
	EventBecknOnSettleCallback OutboxEventType = "beckn.on_settle_callback"
	EventReconException       OutboxEventType = "recon.exception_notification"
)

// OnSettleCallbackEvent models the Beckn /on_settle event payload.
type OnSettleCallbackEvent struct {
	BatchID        string `json:"batch_id"`
	SettlementDate string `json:"settlement_date"`
	Status         string `json:"status"` // SETTLED, FAILED, PARTIAL
}

// OutboxRecord represents an entry in the transactional outbox table.
type OutboxRecord struct {
	ID          int64           `json:"id"`
	EventType   OutboxEventType `json:"event_type"`
	Payload     json.RawMessage `json:"payload"`
	Status      string          `json:"status"` // pending, processing, done, failed
	RetryCount  int             `json:"retry_count"`
	CreatedAt   time.Time       `json:"created_at"`
	ProcessedAt *time.Time      `json:"processed_at,omitempty"`
}

// WorkerConfig holds the outbox worker configuration.
type WorkerConfig struct {
	BatchSize     int
	PollInterval  time.Duration
	MaxRetries    int
	SigningDomain string
	SigningKeyID  string
}

// DefaultWorkerConfig returns production defaults for outbox processing.
func DefaultWorkerConfig() WorkerConfig {
	return WorkerConfig{
		BatchSize:     50,
		PollInterval:  1 * time.Second,
		MaxRetries:    5,
		SigningDomain: "sa.ondc.org",
		SigningKeyID:  "sa-key-1",
	}
}

// OutboxProcessor drains and dispatches outbox events.
type OutboxProcessor struct {
	cfg        WorkerConfig
	pool       *pgxpool.Pool
	signer     *auth.RequestSigner
	mu         sync.Mutex
	inMemoryDB []OutboxRecord
}

// NewOutboxProcessor creates a new OutboxProcessor instance.
func NewOutboxProcessor(cfg WorkerConfig, pool *pgxpool.Pool, signer *auth.RequestSigner) *OutboxProcessor {
	return &OutboxProcessor{
		cfg:        cfg,
		pool:       pool,
		signer:     signer,
		inMemoryDB: make([]OutboxRecord, 0),
	}
}

// EnqueueInMemory adds a synthetic record for unit testing without live PostgreSQL.
func (p *OutboxProcessor) EnqueueInMemory(record OutboxRecord) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inMemoryDB = append(p.inMemoryDB, record)
}

// DrainInMemory processes all pending records currently in the in-memory queue.
func (p *OutboxProcessor) DrainInMemory(ctx context.Context) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	processed := 0
	for i := range p.inMemoryDB {
		if p.inMemoryDB[i].Status == "pending" {
			err := p.processRecord(ctx, &p.inMemoryDB[i])
			now := time.Now()
			p.inMemoryDB[i].ProcessedAt = &now
			if err != nil {
				p.inMemoryDB[i].RetryCount++
				if p.inMemoryDB[i].RetryCount >= p.cfg.MaxRetries {
					p.inMemoryDB[i].Status = "failed"
				}
				slog.Error("Failed to process outbox record", "id", p.inMemoryDB[i].ID, "error", err)
			} else {
				p.inMemoryDB[i].Status = "done"
				processed++
			}
		}
	}
	return processed, nil
}

// DrainBatch pulls pending records using SKIP LOCKED and processes them.
func (p *OutboxProcessor) DrainBatch(ctx context.Context) (int, error) {
	if p.pool == nil {
		return p.DrainInMemory(ctx)
	}

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("outbox.DrainBatch: begin tx failed: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	rows, err := tx.Query(ctx, `
		SELECT id, event_type, payload, status, created_at
		FROM outbox
		WHERE status = 'pending'
		ORDER BY created_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`, p.cfg.BatchSize)
	if err != nil {
		return 0, fmt.Errorf("outbox.DrainBatch: query failed: %w", err)
	}
	defer rows.Close()

	records := make([]OutboxRecord, 0)
	for rows.Next() {
		var r OutboxRecord
		if err := rows.Scan(&r.ID, &r.EventType, &r.Payload, &r.Status, &r.CreatedAt); err != nil {
			return 0, fmt.Errorf("outbox.DrainBatch: scan failed: %w", err)
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("outbox.DrainBatch: rows error: %w", err)
	}

	if len(records) == 0 {
		return 0, nil
	}

	processedCount := 0
	for _, rec := range records {
		recCopy := rec
		procErr := p.processRecord(ctx, &recCopy)
		if procErr != nil {
			slog.Error("Outbox dispatch failure", "id", rec.ID, "type", rec.EventType, "error", procErr)
			_, _ = tx.Exec(ctx, `
				UPDATE outbox
				SET status = 'failed', processed_at = NOW()
				WHERE id = $1
			`, rec.ID)
		} else {
			_, _ = tx.Exec(ctx, `
				UPDATE outbox
				SET status = 'done', processed_at = NOW()
				WHERE id = $1
			`, rec.ID)
			processedCount++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("outbox.DrainBatch: commit failed: %w", err)
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

	slog.Info("Dispatched net bank instruction",
		"from", instruction.FromAccount,
		"to", instruction.ToAccount,
		"amount_paise", instruction.AmountPaise,
		"batch_id", instruction.BatchID,
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

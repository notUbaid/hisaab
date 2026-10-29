package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ubaid/hisaab/internal/auth"
	"github.com/ubaid/hisaab/internal/settlement"
)

func TestOutboxProcessor_DrainInMemory_Success(t *testing.T) {
	keyRegistry := auth.NewMemoryKeyRegistry()
	_, priv, err := auth.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}
	signer := auth.NewRequestSigner("sa.ondc.org", "key-1", priv, keyRegistry)

	cfg := DefaultWorkerConfig()
	processor := NewOutboxProcessor(cfg, nil, signer)

	// 1. Enqueue bank instruction
	instPayload, _ := json.Marshal(settlement.NetInstruction{
		FromAccount: "collector:ondc_settlement",
		ToAccount:   "receiver:operative",
		AmountPaise: 47500,
		BatchID:     "BATCH-01",
	})
	processor.EnqueueInMemory(OutboxRecord{
		ID:        1,
		EventType: EventBankInstruction,
		Payload:   instPayload,
		Status:    "pending",
		CreatedAt: time.Now(),
	})

	// 2. Enqueue Beckn callback
	callbackPayload, _ := json.Marshal(OnSettleCallbackEvent{
		BatchID:        "BATCH-01",
		SettlementDate: "2026-10-30",
		Status:         "SETTLED",
	})
	processor.EnqueueInMemory(OutboxRecord{
		ID:        2,
		EventType: EventBecknOnSettleCallback,
		Payload:   callbackPayload,
		Status:    "pending",
		CreatedAt: time.Now(),
	})

	// 3. Enqueue Recon exception notification
	excPayload, _ := json.Marshal(map[string]any{
		"exception_id": "EXC-101",
		"class":        "AMOUNT_MISMATCH",
	})
	processor.EnqueueInMemory(OutboxRecord{
		ID:        3,
		EventType: EventReconException,
		Payload:   excPayload,
		Status:    "pending",
		CreatedAt: time.Now(),
	})

	// Drain
	count, err := processor.DrainInMemory(context.Background())
	if err != nil {
		t.Fatalf("DrainInMemory failed: %v", err)
	}
	if count != 3 {
		t.Fatalf("Expected 3 records processed, got: %d", count)
	}

	// Verify all records status = done
	for _, rec := range processor.inMemoryDB {
		if rec.Status != "done" {
			t.Errorf("Expected record %d to have status 'done', got %q", rec.ID, rec.Status)
		}
		if rec.ProcessedAt == nil {
			t.Errorf("Expected record %d to have non-nil ProcessedAt", rec.ID)
		}
	}
}

func TestOutboxProcessor_DrainInMemory_FailureAndRetry(t *testing.T) {
	cfg := DefaultWorkerConfig()
	cfg.MaxRetries = 2
	processor := NewOutboxProcessor(cfg, nil, nil)

	// Enqueue malformed bank instruction (amount 0)
	badPayload, _ := json.Marshal(settlement.NetInstruction{
		FromAccount: "collector",
		ToAccount:   "receiver",
		AmountPaise: 0,
		BatchID:     "BATCH-ERR",
	})
	processor.EnqueueInMemory(OutboxRecord{
		ID:        1,
		EventType: EventBankInstruction,
		Payload:   badPayload,
		Status:    "pending",
		CreatedAt: time.Now(),
	})

	// Attempt 1: Should fail and increment retry
	count, err := processor.DrainInMemory(context.Background())
	if err != nil {
		t.Fatalf("DrainInMemory unexpected error: %v", err)
	}
	if count != 0 {
		t.Fatalf("Expected 0 processed, got %d", count)
	}
	if processor.inMemoryDB[0].RetryCount != 1 {
		t.Errorf("Expected retry count 1, got %d", processor.inMemoryDB[0].RetryCount)
	}
	if processor.inMemoryDB[0].Status != "pending" {
		t.Errorf("Expected status 'pending' after 1 retry, got %q", processor.inMemoryDB[0].Status)
	}

	// Attempt 2: Should reach max retries and mark failed
	count, _ = processor.DrainInMemory(context.Background())
	if count != 0 {
		t.Fatalf("Expected 0 processed, got %d", count)
	}
	if processor.inMemoryDB[0].RetryCount != 2 {
		t.Errorf("Expected retry count 2, got %d", processor.inMemoryDB[0].RetryCount)
	}
	if processor.inMemoryDB[0].Status != "failed" {
		t.Errorf("Expected status 'failed' after max retries, got %q", processor.inMemoryDB[0].Status)
	}
}

func TestOutboxProcessor_UnknownEventType(t *testing.T) {
	processor := NewOutboxProcessor(DefaultWorkerConfig(), nil, nil)
	processor.EnqueueInMemory(OutboxRecord{
		ID:        1,
		EventType: "unknown.event",
		Payload:   []byte(`{}`),
		Status:    "pending",
		CreatedAt: time.Now(),
	})

	count, _ := processor.DrainInMemory(context.Background())
	if count != 0 {
		t.Errorf("Expected 0 processed, got %d", count)
	}
	if processor.inMemoryDB[0].RetryCount != 1 {
		t.Errorf("Expected retry count 1, got %d", processor.inMemoryDB[0].RetryCount)
	}
}

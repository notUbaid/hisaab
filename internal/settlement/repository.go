package settlement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ubaid/hisaab/internal/ledger"
)

var (
	ErrClaimNotFound       = errors.New("settlement_repo: claim not found")
	ErrBatchNotFound       = errors.New("settlement_repo: batch not found")
	ErrInstructionNotFound = errors.New("settlement_repo: instruction not found")
)

// Repository defines persistence for claims, batches, instructions, and refund state.
type Repository interface {
	SaveClaim(ctx context.Context, claim *SettleClaimPayload, reqHash string, rawPayload []byte) (int64, error)
	GetClaim(ctx context.Context, messageID string) (*StoredClaimRecord, error)
	ListClaims(ctx context.Context) ([]*StoredClaimRecord, error)
	SaveBatch(ctx context.Context, batch *BatchSummary) error
	GetBatch(ctx context.Context, batchID string) (*BatchSummary, error)
	ListBatches(ctx context.Context, limit int) ([]*BatchSummary, error)
	SaveInstructions(ctx context.Context, batchID string, instructions []NetInstruction) error
	GetInstructions(ctx context.Context, batchID string) ([]NetInstruction, error)
	UpdateInstructionStatus(ctx context.Context, batchID, fromAcc, toAcc string, amount ledger.AmountPaise, status string) error
	RecordRefundState(ctx context.Context, orderID string, amount ledger.AmountPaise, status string) error
	GetRefundState(ctx context.Context, orderID string) (ledger.AmountPaise, error)
}

// PostgresRepository provides PostgreSQL persistence for the settlement package.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository initializes a Postgres-backed settlement repository.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// SaveClaim persists a raw claim and its line items.
func (r *PostgresRepository) SaveClaim(ctx context.Context, claim *SettleClaimPayload, reqHash string, rawPayload []byte) (int64, error) {
	if r.pool == nil || claim == nil || claim.Context == nil {
		return 0, nil
	}

	var totalAmount int64
	for _, ord := range claim.Orders {
		totalAmount += int64(ord.OrderAmountPaise)
	}

	settleDateStr := time.Now().UTC().Format("2006-01-02")
	var claimID int64
	err := r.pool.QueryRow(ctx, `
		INSERT INTO claims (
			message_id, settlement_id, np_pair_id, settlement_date,
			submitted_by, collector_acc, receiver_acc, request_hash,
			order_count, total_amount_paise, raw_payload
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (message_id) DO UPDATE SET raw_payload = EXCLUDED.raw_payload
		RETURNING id
	`, claim.Context.MessageID, claim.SettlementID, claim.NPPairID, settleDateStr,
		claim.SubmittedBy, claim.CollectorAcc, claim.ReceiverAcc, reqHash,
		len(claim.Orders), totalAmount, rawPayload).Scan(&claimID)
	if err != nil {
		return 0, fmt.Errorf("settlement_repo: save claim failed: %w", err)
	}

	for _, ord := range claim.Orders {
		_, _ = r.pool.Exec(ctx, `
			INSERT INTO claim_lines (claim_id, network_order_id, amount_paise, commission_paise, bff_paise, sor_due_paise)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, claimID, ord.NetworkOrderID, int64(ord.OrderAmountPaise), int64(ord.CommissionPaise), int64(ord.BFFPaise), int64(ord.SoRDuePaise))
	}

	return claimID, nil
}

// GetClaim retrieves a stored claim by message_id.
func (r *PostgresRepository) GetClaim(ctx context.Context, messageID string) (*StoredClaimRecord, error) {
	if r.pool == nil {
		return nil, ErrClaimNotFound
	}

	var msgID, settleID, npPairID, submittedBy, collectorAcc, receiverAcc, reqHash string
	var rawPayload []byte
	var receivedAt time.Time

	err := r.pool.QueryRow(ctx, `
		SELECT message_id, settlement_id, np_pair_id, submitted_by, collector_acc, receiver_acc,
		       request_hash, raw_payload, received_at
		FROM claims
		WHERE message_id = $1
	`, messageID).Scan(&msgID, &settleID, &npPairID, &submittedBy, &collectorAcc, &receiverAcc,
		&reqHash, &rawPayload, &receivedAt)
	if err != nil {
		return nil, ErrClaimNotFound
	}

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

	ack := AckResponse{Context: claim.Context}
	ack.Message.Ack.Status = AckStatusACK

	return &StoredClaimRecord{
		MessageID:   msgID,
		RequestHash: reqHash,
		Claim:       claim,
		AckResponse: ack,
		ReceivedAt:  receivedAt,
	}, nil
}

// ListClaims returns all stored claim records from PostgreSQL.
func (r *PostgresRepository) ListClaims(ctx context.Context) ([]*StoredClaimRecord, error) {
	if r.pool == nil {
		return nil, nil
	}

	rows, err := r.pool.Query(ctx, `
		SELECT message_id, settlement_id, np_pair_id, submitted_by, collector_acc, receiver_acc,
		       request_hash, raw_payload, received_at
		FROM claims
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("settlement_repo: list claims failed: %w", err)
	}
	defer rows.Close()

	var records []*StoredClaimRecord
	for rows.Next() {
		var msgID, settleID, npPairID, submittedBy, collectorAcc, receiverAcc, reqHash string
		var rawPayload []byte
		var receivedAt time.Time

		if err := rows.Scan(&msgID, &settleID, &npPairID, &submittedBy, &collectorAcc, &receiverAcc,
			&reqHash, &rawPayload, &receivedAt); err == nil {
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

			ack := AckResponse{Context: claim.Context}
			ack.Message.Ack.Status = AckStatusACK

			records = append(records, &StoredClaimRecord{
				MessageID:   msgID,
				RequestHash: reqHash,
				Claim:       claim,
				AckResponse: ack,
				ReceivedAt:  receivedAt,
			})
		}
	}
	return records, nil
}

// SaveBatch persists a completed or rejected settlement batch record.
func (r *PostgresRepository) SaveBatch(ctx context.Context, batch *BatchSummary) error {
	if r.pool == nil || batch == nil {
		return nil
	}

	settleDateOnly, _ := time.Parse("2006-01-02", batch.SettlementDate)
	now := batch.CreatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}

	var runID int64
	_ = r.pool.QueryRow(ctx, "INSERT INTO batch_runs DEFAULT VALUES RETURNING run_id").Scan(&runID)

	_, err := r.pool.Exec(ctx, `
		INSERT INTO settlement_batches (
			batch_id, np_pair_id, settlement_date, status,
			matched_count, unmatched_count, total_gross_paise,
			net_payout_paise, total_tax_paise, tds_paise, gst_tcs_paise,
			merkle_root, run_id, created_at, completed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		ON CONFLICT (batch_id, settlement_date) DO UPDATE SET
			status = EXCLUDED.status,
			matched_count = EXCLUDED.matched_count,
			unmatched_count = EXCLUDED.unmatched_count,
			total_gross_paise = EXCLUDED.total_gross_paise,
			net_payout_paise = EXCLUDED.net_payout_paise,
			total_tax_paise = EXCLUDED.total_tax_paise,
			completed_at = EXCLUDED.completed_at
	`, batch.BatchID, batch.NPPairID, settleDateOnly, string(batch.Status),
		batch.MatchedCount, batch.UnmatchedCount, int64(batch.TotalGrossPaise),
		int64(batch.NetPayoutPaise), int64(batch.TotalTaxPaise), int64(batch.TDSPaise),
		int64(batch.GSTTCSPaise), batch.MerkleRoot, runID, now, batch.SettledAt)
	if err != nil {
		return fmt.Errorf("settlement_repo: save batch failed: %w", err)
	}

	if len(batch.Instructions) > 0 {
		return r.SaveInstructions(ctx, batch.BatchID, batch.Instructions)
	}
	return nil
}

// GetBatch retrieves a batch summary with associated bank instructions.
func (r *PostgresRepository) GetBatch(ctx context.Context, batchID string) (*BatchSummary, error) {
	if r.pool == nil {
		return nil, ErrBatchNotFound
	}

	var npPairID, status, merkleRoot string
	var settleDate, createdAt time.Time
	var completedAt *time.Time
	var matchedCount, unmatchedCount int
	var totalGross, netPayout, totalTax, tds, gstTcs int64

	err := r.pool.QueryRow(ctx, `
		SELECT np_pair_id, settlement_date, status, matched_count, unmatched_count,
		       total_gross_paise, net_payout_paise, total_tax_paise, tds_paise, gst_tcs_paise,
		       merkle_root, created_at, completed_at
		FROM settlement_batches
		WHERE batch_id = $1
		LIMIT 1
	`, batchID).Scan(&npPairID, &settleDate, &status, &matchedCount, &unmatchedCount,
		&totalGross, &netPayout, &totalTax, &tds, &gstTcs, &merkleRoot, &createdAt, &completedAt)
	if err != nil {
		return nil, ErrBatchNotFound
	}

	insts, _ := r.GetInstructions(ctx, batchID)

	return &BatchSummary{
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
	}, nil
}

// ListBatches returns recent batches.
func (r *PostgresRepository) ListBatches(ctx context.Context, limit int) ([]*BatchSummary, error) {
	if r.pool == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}

	rows, err := r.pool.Query(ctx, `
		SELECT batch_id, np_pair_id, settlement_date, status, matched_count, unmatched_count,
		       total_gross_paise, net_payout_paise, total_tax_paise, tds_paise, gst_tcs_paise,
		       merkle_root, created_at, completed_at
		FROM settlement_batches
		ORDER BY id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("settlement_repo: list batches failed: %w", err)
	}
	defer rows.Close()

	var batches []*BatchSummary
	for rows.Next() {
		var batchID, npPairID, status, merkleRoot string
		var settleDate, createdAt time.Time
		var completedAt *time.Time
		var matchedCount, unmatchedCount int
		var totalGross, netPayout, totalTax, tds, gstTcs int64

		if err := rows.Scan(&batchID, &npPairID, &settleDate, &status, &matchedCount, &unmatchedCount,
			&totalGross, &netPayout, &totalTax, &tds, &gstTcs, &merkleRoot, &createdAt, &completedAt); err == nil {
			batches = append(batches, &BatchSummary{
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
				CreatedAt:       createdAt,
				SettledAt:       completedAt,
			})
		}
	}
	return batches, nil
}

// SaveInstructions persists individual net instructions.
func (r *PostgresRepository) SaveInstructions(ctx context.Context, batchID string, instructions []NetInstruction) error {
	if r.pool == nil || len(instructions) == 0 {
		return nil
	}

	for _, inst := range instructions {
		_, err := r.pool.Exec(ctx, `
			INSERT INTO instructions (batch_id, from_account, to_account, amount_paise, currency, status, created_at)
			VALUES ($1, $2, $3, $4, 'INR', 'pending', NOW())
		`, batchID, inst.FromAccount, inst.ToAccount, int64(inst.AmountPaise))
		if err != nil {
			return fmt.Errorf("settlement_repo: save instruction failed: %w", err)
		}
	}
	return nil
}

// GetInstructions retrieves instructions for a batch.
func (r *PostgresRepository) GetInstructions(ctx context.Context, batchID string) ([]NetInstruction, error) {
	if r.pool == nil {
		return nil, nil
	}

	rows, err := r.pool.Query(ctx, `
		SELECT from_account, to_account, amount_paise
		FROM instructions
		WHERE batch_id = $1
	`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var insts []NetInstruction
	for rows.Next() {
		var fromAcc, toAcc string
		var amt int64
		if rows.Scan(&fromAcc, &toAcc, &amt) == nil {
			insts = append(insts, NetInstruction{
				FromAccount: fromAcc,
				ToAccount:   toAcc,
				AmountPaise: ledger.AmountPaise(amt),
				BatchID:     batchID,
			})
		}
	}
	return insts, nil
}

// UpdateInstructionStatus marks an instruction as sent, credited, or failed.
func (r *PostgresRepository) UpdateInstructionStatus(ctx context.Context, batchID, fromAcc, toAcc string, amount ledger.AmountPaise, status string) error {
	if r.pool == nil {
		return ErrInstructionNotFound
	}

	cmd, err := r.pool.Exec(ctx, `
		UPDATE instructions
		SET status = $5,
		    sent_at = CASE WHEN $5 = 'sent' THEN NOW() ELSE sent_at END,
		    settled_at = CASE WHEN $5 = 'credited' THEN NOW() ELSE settled_at END
		WHERE batch_id = $1 AND from_account = $2 AND to_account = $3 AND amount_paise = $4
	`, batchID, fromAcc, toAcc, int64(amount), status)
	if err != nil {
		return fmt.Errorf("settlement_repo: update instruction status failed: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrInstructionNotFound
	}
	return nil
}

// RecordRefundState records cumulative refunded paise per order.
func (r *PostgresRepository) RecordRefundState(ctx context.Context, orderID string, amount ledger.AmountPaise, status string) error {
	if r.pool == nil {
		return nil
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO audit_log (actor, action, entity_type, entity_id, payload)
		VALUES ('hisaab_settlement', 'refund_processed', 'order', $1, jsonb_build_object('refund_amount_paise', $2::bigint, 'status', $3))
	`, orderID, int64(amount), status)
	return err
}

// GetRefundState retrieves cumulative refunded paise for an order.
func (r *PostgresRepository) GetRefundState(ctx context.Context, orderID string) (ledger.AmountPaise, error) {
	if r.pool == nil {
		return 0, nil
	}
	var totalRefunded int64
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM((payload->>'refund_amount_paise')::bigint), 0)
		FROM audit_log
		WHERE entity_type = 'order' AND entity_id = $1 AND action = 'refund_processed'
	`, orderID).Scan(&totalRefunded)
	if err != nil {
		return 0, err
	}
	return ledger.AmountPaise(totalRefunded), nil
}

// MemoryRepository provides thread-safe in-memory settlement persistence.
type MemoryRepository struct {
	mu           sync.RWMutex
	claims       map[string]*StoredClaimRecord
	batches      map[string]*BatchSummary
	instructions map[string][]NetInstruction
	refunds      map[string]ledger.AmountPaise
}

// NewMemoryRepository initializes an in-memory settlement repository.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		claims:       make(map[string]*StoredClaimRecord),
		batches:      make(map[string]*BatchSummary),
		instructions: make(map[string][]NetInstruction),
		refunds:      make(map[string]ledger.AmountPaise),
	}
}

func (m *MemoryRepository) SaveClaim(ctx context.Context, claim *SettleClaimPayload, reqHash string, rawPayload []byte) (int64, error) {
	if claim == nil || claim.Context == nil {
		return 0, errors.New("nil claim or context")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	ack := AckResponse{Context: claim.Context}
	ack.Message.Ack.Status = AckStatusACK

	record := &StoredClaimRecord{
		MessageID:   claim.Context.MessageID,
		RequestHash: reqHash,
		Claim:       *claim,
		AckResponse: ack,
		ReceivedAt:  time.Now().UTC(),
	}
	m.claims[claim.Context.MessageID] = record
	return int64(len(m.claims)), nil
}

func (m *MemoryRepository) GetClaim(ctx context.Context, messageID string) (*StoredClaimRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.claims[messageID]
	if !ok {
		return nil, ErrClaimNotFound
	}
	return c, nil
}

func (m *MemoryRepository) ListClaims(ctx context.Context) ([]*StoredClaimRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*StoredClaimRecord, 0, len(m.claims))
	for _, c := range m.claims {
		res = append(res, c)
	}
	return res, nil
}

func (m *MemoryRepository) SaveBatch(ctx context.Context, batch *BatchSummary) error {
	if batch == nil {
		return errors.New("nil batch")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.batches[batch.BatchID] = batch
	if len(batch.Instructions) > 0 {
		m.instructions[batch.BatchID] = append([]NetInstruction(nil), batch.Instructions...)
	}
	return nil
}

func (m *MemoryRepository) GetBatch(ctx context.Context, batchID string) (*BatchSummary, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.batches[batchID]
	if !ok {
		return nil, ErrBatchNotFound
	}
	return b, nil
}

func (m *MemoryRepository) ListBatches(ctx context.Context, limit int) ([]*BatchSummary, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*BatchSummary, 0, len(m.batches))
	for _, b := range m.batches {
		res = append(res, b)
		if limit > 0 && len(res) >= limit {
			break
		}
	}
	return res, nil
}

func (m *MemoryRepository) SaveInstructions(ctx context.Context, batchID string, instructions []NetInstruction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.instructions[batchID] = append(m.instructions[batchID], instructions...)
	return nil
}

func (m *MemoryRepository) GetInstructions(ctx context.Context, batchID string) ([]NetInstruction, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	insts, ok := m.instructions[batchID]
	if !ok {
		return nil, nil
	}
	return append([]NetInstruction(nil), insts...), nil
}

func (m *MemoryRepository) UpdateInstructionStatus(ctx context.Context, batchID, fromAcc, toAcc string, amount ledger.AmountPaise, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	insts, ok := m.instructions[batchID]
	if !ok {
		return ErrInstructionNotFound
	}
	found := false
	for i := range insts {
		if insts[i].FromAccount == fromAcc && insts[i].ToAccount == toAcc && insts[i].AmountPaise == amount {
			found = true
			break
		}
	}
	if !found {
		return ErrInstructionNotFound
	}
	return nil
}

func (m *MemoryRepository) RecordRefundState(ctx context.Context, orderID string, amount ledger.AmountPaise, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refunds[orderID] += amount
	return nil
}

func (m *MemoryRepository) GetRefundState(ctx context.Context, orderID string) (ledger.AmountPaise, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.refunds[orderID], nil
}

package settlement

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ubaid/hisaab/internal/ledger"
)

var (
	ErrInsufficientSellerBalance = errors.New("settlement: insufficient balance in Seller settlement account for refund (EC-16)")
	ErrRefundExceedsOrderAmount  = errors.New("settlement: refund amount strictly exceeds original order amount (Invariant 6)")
	ErrDuplicateRefundOrder      = errors.New("settlement: order already refunded; duplicate refund rejected (Invariant 6)")
)

// RefundClaim represents a customer return or order cancellation refund request.
type RefundClaim struct {
	MessageID         string             `json:"message_id"`
	NetworkOrderID    string             `json:"network_order_id"`
	OriginalAmount    ledger.AmountPaise `json:"original_amount_paise"`
	RefundAmountPaise ledger.AmountPaise `json:"refund_amount_paise"`
	Reason            string             `json:"reason"`
	SettlementDate    time.Time          `json:"settlement_date"`
}

// RefundResult contains the outcome of processing a refund settlement leg.
type RefundResult struct {
	NetworkOrderID    string             `json:"network_order_id"`
	RefundAmountPaise ledger.AmountPaise `json:"refund_amount_paise"`
	Status            string             `json:"status"` // "settled", "escalated_to_suspense", "rejected"
	Instructions      []NetInstruction   `json:"instructions"`
	JournalLines      []ledger.EntryLine `json:"journal_lines"`
	Reason            string             `json:"reason,omitempty"`
}

// RefundProcessor handles reverse settlement flows per RSF 2.0 BRD Case 16.
// It is thread-safe and enforces Invariant 6 using authoritative state.
type RefundProcessor struct {
	mu             sync.RWMutex
	settledRefunds map[string]ledger.AmountPaise // tracks cumulative refunded paise per order
	authoritative  map[string]ledger.AmountPaise // tracks authoritative forward settled amounts
}

// NewRefundProcessor creates an initialized RefundProcessor.
func NewRefundProcessor() *RefundProcessor {
	return &RefundProcessor{
		settledRefunds: make(map[string]ledger.AmountPaise),
		authoritative:  make(map[string]ledger.AmountPaise),
	}
}

// RegisterSettledOrder records the authoritative forward settled amount for an order.
func (p *RefundProcessor) RegisterSettledOrder(orderID string, amount ledger.AmountPaise) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.authoritative[orderID] = amount
}

// ProcessRefundAuthoritative processes a refund using an explicit authoritative settled amount.
func (p *RefundProcessor) ProcessRefundAuthoritative(
	claim RefundClaim,
	collectorAcc string,
	receiverAcc string,
	sellerAvailableBalance ledger.AmountPaise,
	authoritativeSettledAmount ledger.AmountPaise,
) (*RefundResult, error) {
	p.RegisterSettledOrder(claim.NetworkOrderID, authoritativeSettledAmount)
	return p.ProcessRefund(claim, collectorAcc, receiverAcc, sellerAvailableBalance)
}

// ProcessRefund executes the refund settlement flow.
// Invariant 4: Checks Seller App balance. If insolvent, routes obligation to sa:suspense.
// Invariant 6: Asserts cumulative refund <= authoritative order amount.
func (p *RefundProcessor) ProcessRefund(
	claim RefundClaim,
	collectorAcc string,
	receiverAcc string,
	sellerAvailableBalance ledger.AmountPaise,
) (*RefundResult, error) {
	if claim.RefundAmountPaise <= 0 {
		return nil, errors.New("settlement: refund amount must be strictly greater than zero paise")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Invariant 6: Bound check against authoritative settled amount if registered, else claim.OriginalAmount
	bound := claim.OriginalAmount
	if authAmount, exists := p.authoritative[claim.NetworkOrderID]; exists {
		bound = authAmount
	}

	alreadyRefunded := p.settledRefunds[claim.NetworkOrderID]
	if alreadyRefunded+claim.RefundAmountPaise > bound {
		return nil, fmt.Errorf("%w: cumulative %d + proposed %d > bound %d paise",
			ErrRefundExceedsOrderAmount, alreadyRefunded, claim.RefundAmountPaise, bound)
	}

	// Always increment cumulative refunds so suspense routing cannot be replayed infinitely (B3b)
	p.settledRefunds[claim.NetworkOrderID] += claim.RefundAmountPaise

	// Case EC-16: Seller-side balance verification
	if sellerAvailableBalance < claim.RefundAmountPaise {
		// Insolvent Seller App: route to suspense account to avoid nodal deficit
		journalLines := []ledger.EntryLine{
			{AccountName: "sa:suspense", Direction: ledger.Debit, AmountPaise: claim.RefundAmountPaise},
			{AccountName: collectorAcc, Direction: ledger.Credit, AmountPaise: claim.RefundAmountPaise},
		}

		return &RefundResult{
			NetworkOrderID:    claim.NetworkOrderID,
			RefundAmountPaise: claim.RefundAmountPaise,
			Status:            "escalated_to_suspense",
			JournalLines:      journalLines,
			Reason: fmt.Sprintf("Seller balance (%d paise) < refund (%d paise); routed to suspense break",
				sellerAvailableBalance, claim.RefundAmountPaise),
		}, nil
	}

	// Normal Refund Settlement: Seller App pays Collector
	instruction := NetInstruction{
		FromAccount: receiverAcc,
		ToAccount:   collectorAcc,
		AmountPaise: claim.RefundAmountPaise,
		BatchID:     fmt.Sprintf("REFUND-%s", claim.NetworkOrderID),
	}

	journalLines := []ledger.EntryLine{
		{AccountName: receiverAcc, Direction: ledger.Debit, AmountPaise: claim.RefundAmountPaise},
		{AccountName: collectorAcc, Direction: ledger.Credit, AmountPaise: claim.RefundAmountPaise},
	}

	return &RefundResult{
		NetworkOrderID:    claim.NetworkOrderID,
		RefundAmountPaise: claim.RefundAmountPaise,
		Status:            "settled",
		Instructions:      []NetInstruction{instruction},
		JournalLines:      journalLines,
	}, nil
}

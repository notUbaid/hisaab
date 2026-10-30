package recon

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/ubaid/hisaab/internal/calendar"
	"github.com/ubaid/hisaab/internal/ledger"
)

// Engine performs three-way reconciliation between Order Book, SA Report, and Bank Statement.
type Engine struct {
	cal *calendar.Calendar
}

// NewEngine creates a new Recon Engine instance.
func NewEngine(cal *calendar.Calendar) *Engine {
	if cal == nil {
		cal = calendar.New(nil)
	}
	return &Engine{cal: cal}
}

func generateID(prefix string) string {
	bytes := make([]byte, 8)
	_, _ = rand.Read(bytes)
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(bytes))
}

// calculateBusinessDaysBetween calculates the number of Indian business days between t1 and t2.
func (e *Engine) calculateBusinessDaysBetween(from, to time.Time) int {
	if to.Before(from) {
		return 0
	}
	days := 0
	curr := from.In(calendar.Location())
	target := to.In(calendar.Location())

	for curr.Before(target) {
		curr = curr.AddDate(0, 0, 1)
		isHoliday, _, _ := e.cal.IsBankHoliday(curr)
		if !isHoliday {
			days++
		}
	}
	return days
}

// Reconcile performs three-way reconciliation across all three data sources.
func (e *Engine) Reconcile(
	orderBook []OrderBookLine,
	saReport []SAReportLine,
	bankStatement []BankStatementLine,
	asOf time.Time,
) *ThreeWayReconResult {
	result := &ThreeWayReconResult{
		TotalChecked:      len(orderBook),
		Exceptions:        make([]ReconException, 0),
		ExceptionsByClass: make(map[ExceptionClass]int),
		ReconciledAt:      asOf,
	}

	saMap := make(map[string]SAReportLine)
	for _, sa := range saReport {
		if _, exists := saMap[sa.NetworkOrderID]; exists {
			exc := ReconException{
				ID:             generateID("EXC-DUP-SA"),
				Class:          ExceptionDuplicateCredit,
				NetworkOrderID: sa.NetworkOrderID,
				ExpectedPaise:  0,
				ActualPaise:    sa.SettledAmount,
				AgeingDays:     0,
				Status:         ReconStatusOpen,
				Details:        fmt.Sprintf("Duplicate settlement advice line observed for order %s", sa.NetworkOrderID),
				DetectedAt:     asOf,
			}
			result.Exceptions = append(result.Exceptions, exc)
			result.ExceptionsByClass[ExceptionDuplicateCredit]++
		}
		saMap[sa.NetworkOrderID] = sa
	}

	bankCreditMap := make(map[string]BankStatementLine)
	for _, b := range bankStatement {
		if b.Direction == ledger.Credit {
			if _, exists := bankCreditMap[b.ReferenceID]; exists {
				exc := ReconException{
					ID:             generateID("EXC-DUP-CREDIT"),
					Class:          ExceptionDuplicateCredit,
					NetworkOrderID: b.ReferenceID,
					ExpectedPaise:  0,
					ActualPaise:    b.AmountPaise,
					AgeingDays:     0,
					Status:         ReconStatusOpen,
					Details:        fmt.Sprintf("Duplicate bank credit line observed for reference %s (UTR %s)", b.ReferenceID, b.UTR),
					DetectedAt:     asOf,
				}
				result.Exceptions = append(result.Exceptions, exc)
				result.ExceptionsByClass[ExceptionDuplicateCredit]++
			}
			bankCreditMap[b.ReferenceID] = b
		}
	}

	matchedOrders := make(map[string]bool)

	// 1. Compare Order Book vs SA Report
	for _, ob := range orderBook {
		sa, foundInSA := saMap[ob.NetworkOrderID]
		ageing := e.calculateBusinessDaysBetween(ob.CreatedAt, asOf)

		if !foundInSA {
			// Order in order book but never settled or submitted to SA
			exc := ReconException{
				ID:             generateID("EXC-ORPHAN"),
				Class:          ExceptionOrphanOrder,
				NetworkOrderID: ob.NetworkOrderID,
				ExpectedPaise:  ob.AmountPaise,
				ActualPaise:    0,
				AgeingDays:     ageing,
				Status:         ReconStatusOpen,
				Details:        fmt.Sprintf("Order %s in order book has no corresponding entry in SA report", ob.NetworkOrderID),
				DetectedAt:     asOf,
			}
			result.Exceptions = append(result.Exceptions, exc)
			result.ExceptionsByClass[ExceptionOrphanOrder]++
			continue
		}

		if sa.Status == "rejected" {
			exc := ReconException{
				ID:             generateID("EXC-REJ"),
				Class:          ExceptionInsufficientBalance,
				NetworkOrderID: ob.NetworkOrderID,
				ExpectedPaise:  ob.AmountPaise,
				ActualPaise:    0,
				AgeingDays:     ageing,
				Status:         ReconStatusOpen,
				Details:        fmt.Sprintf("Order %s was rejected by SA: insufficient balance or validation failure", ob.NetworkOrderID),
				DetectedAt:     asOf,
			}
			result.Exceptions = append(result.Exceptions, exc)
			result.ExceptionsByClass[ExceptionInsufficientBalance]++
			continue
		}

		if sa.Status == "unmatched" {
			exc := ReconException{
				ID:             generateID("EXC-UNMATCHED"),
				Class:          ExceptionMissingCounterpart,
				NetworkOrderID: ob.NetworkOrderID,
				ExpectedPaise:  ob.AmountPaise,
				ActualPaise:    0,
				AgeingDays:     ageing,
				Status:         ReconStatusOpen,
				Details:        fmt.Sprintf("Order %s counterpart claim is missing or unverified", ob.NetworkOrderID),
				DetectedAt:     asOf,
			}
			result.Exceptions = append(result.Exceptions, exc)
			result.ExceptionsByClass[ExceptionMissingCounterpart]++
			continue
		}

		if sa.SettledAmount != ob.AmountPaise {
			exc := ReconException{
				ID:             generateID("EXC-MISMATCH"),
				Class:          ExceptionAmountMismatch,
				NetworkOrderID: ob.NetworkOrderID,
				ExpectedPaise:  ob.AmountPaise,
				ActualPaise:    sa.SettledAmount,
				AgeingDays:     ageing,
				Status:         ReconStatusOpen,
				Details: fmt.Sprintf("Order %s amount mismatch: OrderBook %d paise vs SA Report %d paise",
					ob.NetworkOrderID, ob.AmountPaise, sa.SettledAmount),
				DetectedAt: asOf,
			}
			result.Exceptions = append(result.Exceptions, exc)
			result.ExceptionsByClass[ExceptionAmountMismatch]++
			continue
		}

		matchedOrders[ob.NetworkOrderID] = true
	}

	// 2. Compare Settled SA Batches against Bank Statement Credits
	// For each SA line that settled, verify there is an associated bank credit
	for _, sa := range saReport {
		if sa.Status != "settled" {
			continue
		}
		// In bank statement, reference is typically batch_id or network_order_id
		bLine, foundInBank := bankCreditMap[sa.NetworkOrderID]
		if !foundInBank {
			bLine, foundInBank = bankCreditMap[sa.BatchID]
		}

		if !foundInBank {
			ageing := e.calculateBusinessDaysBetween(sa.SettledAt, asOf)
			exc := ReconException{
				ID:             generateID("EXC-BANK"),
				Class:          ExceptionBankFailure,
				NetworkOrderID: sa.NetworkOrderID,
				ExpectedPaise:  sa.SettledAmount,
				ActualPaise:    0,
				AgeingDays:     ageing,
				Status:         ReconStatusOpen,
				Details:        fmt.Sprintf("SA settled order %s but no matching credit found in bank statement", sa.NetworkOrderID),
				DetectedAt:     asOf,
			}
			result.Exceptions = append(result.Exceptions, exc)
			result.ExceptionsByClass[ExceptionBankFailure]++
		} else {
			_ = bLine
		}
	}

	// 3. Detect Unclaimed Bank Credits (Money in bank without SA / Order Book cause)
	for refID, bLine := range bankCreditMap {
		if _, exists := saMap[refID]; !exists {
			exc := ReconException{
				ID:             generateID("EXC-UNCLAIMED"),
				Class:          ExceptionUnclaimedBankCredit,
				NetworkOrderID: refID,
				ExpectedPaise:  0,
				ActualPaise:    bLine.AmountPaise,
				AgeingDays:     e.calculateBusinessDaysBetween(bLine.ValueDate, asOf),
				Status:         ReconStatusOpen,
				Details:        fmt.Sprintf("Bank credit of %d paise with reference %s has no corresponding SA claim", bLine.AmountPaise, refID),
				DetectedAt:     asOf,
			}
			result.Exceptions = append(result.Exceptions, exc)
			result.ExceptionsByClass[ExceptionUnclaimedBankCredit]++
		}
	}

	result.TotalExceptions = len(result.Exceptions)
	result.MatchedCount = len(matchedOrders)

	return result
}

// ProposeAdjustment creates a proposed adjustment entry under maker-checker protocol.
func (e *Engine) ProposeAdjustment(
	exceptionID string,
	proposerID string,
	reason string,
	lines []ledger.EntryLine,
) (*AdjustmentProposal, error) {
	if _, err := ledger.ValidateLines(lines); err != nil {
		return nil, fmt.Errorf("recon: proposed lines invalid: %w", err)
	}

	return &AdjustmentProposal{
		ProposalID:  generateID("PROP"),
		ExceptionID: exceptionID,
		ProposerID:  proposerID,
		Status:      "pending",
		Lines:       lines,
		Reason:      reason,
		ProposedAt:  time.Now(),
	}, nil
}

// ApproveAdjustment enforces dual control (maker-checker) and produces an executable ledger posting.
func (e *Engine) ApproveAdjustment(
	proposal *AdjustmentProposal,
	approverID string,
) (*ledger.PostRequest, error) {
	if proposal == nil {
		return nil, errors.New("recon: proposal cannot be nil")
	}

	if proposal.Status != "pending" {
		return nil, ErrProposalNotPending
	}

	// Invariant: Approver cannot be the same person as Proposer
	if proposal.ProposerID == approverID {
		return nil, ErrSelfApproval
	}

	// Re-verify lines balance
	if _, err := ledger.ValidateLines(proposal.Lines); err != nil {
		return nil, fmt.Errorf("recon: lines do not balance: %w", err)
	}

	proposal.Status = "approved"
	proposal.ApproverID = approverID
	proposal.ApprovedAt = time.Now()

	postReq := &ledger.PostRequest{
		IdempotencyKey: fmt.Sprintf("adj-%s", proposal.ProposalID),
		CauseType:      ledger.CauseAdjustment,
		CauseID:        time.Now().UnixNano(),
		Description:    fmt.Sprintf("Maker-checker adjustment: %s (approved by %s)", proposal.Reason, approverID),
		Lines:          proposal.Lines,
		PostedAt:       time.Now(),
	}

	return postReq, nil
}

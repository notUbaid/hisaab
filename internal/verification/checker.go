package verification

import (
	"errors"
	"fmt"

	"github.com/ubaid/hisaab/internal/ledger"
	"github.com/ubaid/hisaab/internal/settlement"
)

var (
	ErrInvariant1ZeroSumBroken        = errors.New("invariant 1 broken: transaction debits and credits do not balance")
	ErrInvariant4NegativeBalance      = errors.New("invariant 4 broken: constrained account has negative balance")
	ErrInvariant5ConservationBroken   = errors.New("invariant 5 broken: order components do not sum to total amount")
	ErrInvariant13TieOutBroken        = errors.New("invariant 13 broken: gross sub-ledger obligations do not tie out to net instruction")
	ErrInvariant14CallbackStateBroken = errors.New("invariant 14 broken: terminal callback mutated after completion")
)

// InvariantChecker validates system state against the 15 formal ledger invariants.
type InvariantChecker struct{}

// NewInvariantChecker creates a new InvariantChecker.
func NewInvariantChecker() *InvariantChecker {
	return &InvariantChecker{}
}

// CheckInvariant1_ZeroSum asserts that all entry lines sum to zero in paise.
func (c *InvariantChecker) CheckInvariant1_ZeroSum(lines []ledger.EntryLine) error {
	var debitSum ledger.AmountPaise
	var creditSum ledger.AmountPaise

	for _, l := range lines {
		if l.Direction == ledger.Debit {
			debitSum += l.AmountPaise
		} else if l.Direction == ledger.Credit {
			creditSum += l.AmountPaise
		}
	}

	if debitSum != creditSum {
		return fmt.Errorf("%w: debits (%d paise) != credits (%d paise)", ErrInvariant1ZeroSumBroken, debitSum, creditSum)
	}
	return nil
}

// CheckInvariant4_NonNegative asserts that a constrained account balance is never negative.
func (c *InvariantChecker) CheckInvariant4_NonNegative(accountName string, balance ledger.AmountPaise) error {
	if balance < 0 {
		return fmt.Errorf("%w: account %s has negative balance %d paise", ErrInvariant4NegativeBalance, accountName, balance)
	}
	return nil
}

// CheckInvariant5_Conservation asserts that order components exactly match total order amount.
func (c *InvariantChecker) CheckInvariant5_Conservation(order settlement.OrderSettlementLine) error {
	sum := order.CommissionPaise + order.BFFPaise + order.SoRDuePaise + order.TaxDeductions
	if sum != order.OrderAmountPaise {
		return fmt.Errorf("%w: order %s sum (%d paise) != total amount (%d paise)",
			ErrInvariant5ConservationBroken, order.NetworkOrderID, sum, order.OrderAmountPaise)
	}
	return nil
}

// CheckInvariant13_GrossNetTieOut asserts that net instructions tie out to gross matched order totals.
func (c *InvariantChecker) CheckInvariant13_GrossNetTieOut(batch *settlement.BatchSettlementResult) error {
	if batch == nil {
		return errors.New("verification: batch cannot be nil")
	}

	// If balance check failed, no net bank instructions should be generated
	if !batch.BalancePassed && batch.ErrorCode == "INSUFFICIENT_BALANCE" {
		if len(batch.Instructions) > 0 {
			return fmt.Errorf("%w: failed balance check but %d bank instructions generated",
				ErrInvariant13TieOutBroken, len(batch.Instructions))
		}
		return nil
	}

	if len(batch.MatchedOrders) == 0 {
		if len(batch.Instructions) > 0 {
			return fmt.Errorf("%w: zero matched orders but %d bank instructions generated",
				ErrInvariant13TieOutBroken, len(batch.Instructions))
		}
		return nil
	}

	expectedNet := batch.TotalMatched - batch.TotalBFF

	var actualInstructed ledger.AmountPaise
	for _, inst := range batch.Instructions {
		actualInstructed += inst.AmountPaise
	}

	if expectedNet != actualInstructed {
		return fmt.Errorf("%w: expected net %d paise (Total %d - BFF %d), got instructed %d paise",
			ErrInvariant13TieOutBroken, expectedNet, batch.TotalMatched, batch.TotalBFF, actualInstructed)
	}

	return nil
}

// CheckInvariant14_TerminalCallback asserts that a terminal state is immutable against duplicate callbacks.
func (c *InvariantChecker) CheckInvariant14_TerminalCallback(currentState string, incomingState string) error {
	// Terminal states: "settled", "failed"
	if currentState == "settled" || currentState == "failed" {
		if incomingState != currentState {
			return fmt.Errorf("%w: attempted state mutation from terminal %q to %q",
				ErrInvariant14CallbackStateBroken, currentState, incomingState)
		}
	}
	return nil
}

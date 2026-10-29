package verification

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// FailpointName identifies an enumerated crash point along the settlement execution path.
type FailpointName string

const (
	FPAfterDebitLegBeforeCreditLeg     FailpointName = "after_debit_before_credit"
	FPAfterCommitBeforeACK             FailpointName = "after_commit_before_ack"
	FPAfterSubLedgerBeforeNetBatch     FailpointName = "after_subledger_before_net"
	FPAfterInstructionBeforeBankSend   FailpointName = "after_instruction_before_bank_send"
	FPAfterBankSendBeforeCallback      FailpointName = "after_bank_send_before_callback"
	FPMidBatchDatabaseSever            FailpointName = "mid_batch_database_sever"
	FPDuplicateCreditArrival           FailpointName = "duplicate_credit_arrival"
	FPClockSkewMidnight                FailpointName = "clock_skew_midnight"
)

// FailAction specifies the failure behavior to inject.
type FailAction string

const (
	ActionPanic   FailAction = "panic"
	ActionError   FailAction = "error"
	ActionDelay   FailAction = "delay"
	ActionNothing FailAction = "none"
)

var ErrInjectedFault = errors.New("verification: intentional fault injected by crash test harness")

// FailpointManager coordinates deterministic fault injection for crash-point testing.
type FailpointManager struct {
	mu         sync.RWMutex
	failpoints map[FailpointName]FailAction
	delays     map[FailpointName]time.Duration
	hitCounts  map[FailpointName]int
}

// NewFailpointManager creates an active failpoint manager.
func NewFailpointManager() *FailpointManager {
	return &FailpointManager{
		failpoints: make(map[FailpointName]FailAction),
		delays:     make(map[FailpointName]time.Duration),
		hitCounts:  make(map[FailpointName]int),
	}
}

// Enable arms a failpoint with a specified action.
func (m *FailpointManager) Enable(name FailpointName, action FailAction) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failpoints[name] = action
}

// EnableWithDelay arms a failpoint that injects a timing latency.
func (m *FailpointManager) EnableWithDelay(name FailpointName, delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failpoints[name] = ActionDelay
	m.delays[name] = delay
}

// Disable disarms a failpoint.
func (m *FailpointManager) Disable(name FailpointName) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.failpoints, name)
	delete(m.delays, name)
}

// Reset clears all active failpoints and counters.
func (m *FailpointManager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failpoints = make(map[FailpointName]FailAction)
	m.delays = make(map[FailpointName]time.Duration)
	m.hitCounts = make(map[FailpointName]int)
}

// HitCount returns the number of times a failpoint was evaluated.
func (m *FailpointManager) HitCount(name FailpointName) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.hitCounts[name]
}

// Trigger evaluates whether an active failpoint should inject a fault at this execution point.
func (m *FailpointManager) Trigger(name FailpointName) error {
	m.mu.Lock()
	m.hitCounts[name]++
	action, armed := m.failpoints[name]
	delay := m.delays[name]
	m.mu.Unlock()

	if !armed {
		return nil
	}

	switch action {
	case ActionPanic:
		panic(fmt.Sprintf("CRASH: Failpoint %s triggered simulated fatal panic", name))
	case ActionError:
		return fmt.Errorf("%w at failpoint %s", ErrInjectedFault, name)
	case ActionDelay:
		if delay > 0 {
			time.Sleep(delay)
		}
		return nil
	default:
		return nil
	}
}

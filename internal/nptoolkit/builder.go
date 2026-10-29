package nptoolkit

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/ubaid/hisaab/internal/ledger"
	"github.com/ubaid/hisaab/internal/settlement"
)

// ClaimBuilder transforms an NP's order book into spec-compliant /settle payloads.
type ClaimBuilder struct {
	bapID        string
	collectorAcc string
	receiverAcc  string
}

// NewClaimBuilder creates a new ClaimBuilder instance.
func NewClaimBuilder(bapID, collectorAcc, receiverAcc string) *ClaimBuilder {
	return &ClaimBuilder{
		bapID:        bapID,
		collectorAcc: collectorAcc,
		receiverAcc:  receiverAcc,
	}
}

func generateMessageID() string {
	bytes := make([]byte, 16)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// BuildClaims groups raw orders by NP pair and isolates logistics orders into separate /settle messages.
// Returns a map of retail claims keyed by NPPairID, and a slice of logistics claims.
func (b *ClaimBuilder) BuildClaims(
	submittedBy string,
	orders []RawOrder,
	now time.Time,
) (retailClaims map[string]*settlement.SettleClaimPayload, logisticsClaims []*settlement.SettleClaimPayload, err error) {
	retailClaims = make(map[string]*settlement.SettleClaimPayload)
	logisticsClaims = make([]*settlement.SettleClaimPayload, 0)

	for _, o := range orders {
		npPairID := fmt.Sprintf("%s:%s", o.BuyerNPID, o.SellerNPID)

		// Calculate SoR Due if missing: OrderAmount - Commission - BFF
		sorDue := o.SoRDuePaise
		if sorDue == 0 && o.OrderAmountPaise > 0 {
			sorDue = o.OrderAmountPaise - o.CommissionPaise - o.BFFPaise
		}

		line := settlement.OrderSettlementLine{
			NetworkOrderID:   o.NetworkOrderID,
			OrderAmountPaise: o.OrderAmountPaise,
			CommissionPaise:  o.CommissionPaise,
			BFFPaise:         o.BFFPaise,
			SoRDuePaise:      sorDue,
			SettlementDate:   now,
		}

		if o.IsLogistics {
			// EC-06: Logistics orders must be transmitted in dedicated claims
			logisticsClaim := &settlement.SettleClaimPayload{
				Context: &settlement.BecknContext{
					Domain:    "ONDC:LOG10",
					Action:    "settle",
					BapID:     b.bapID,
					MessageID: generateMessageID(),
					Timestamp: now,
					TTL:       "PT30S",
				},
				SettlementID: fmt.Sprintf("SETTLE-LOG-%s-%d", npPairID, now.Unix()),
				NPPairID:     npPairID,
				SubmittedBy:  submittedBy,
				CollectorAcc: b.collectorAcc,
				ReceiverAcc:  b.receiverAcc,
				Orders:       []settlement.OrderSettlementLine{line},
			}
			logisticsClaims = append(logisticsClaims, logisticsClaim)
			continue
		}

		// Retail orders grouped by NP pair
		claim, exists := retailClaims[npPairID]
		if !exists {
			claim = &settlement.SettleClaimPayload{
				Context: &settlement.BecknContext{
					Domain:    "ONDC:RET10",
					Action:    "settle",
					BapID:     b.bapID,
					MessageID: generateMessageID(),
					Timestamp: now,
					TTL:       "PT30S",
				},
				SettlementID: fmt.Sprintf("SETTLE-RET-%s-%d", npPairID, now.Unix()),
				NPPairID:     npPairID,
				SubmittedBy:  submittedBy,
				CollectorAcc: b.collectorAcc,
				ReceiverAcc:  b.receiverAcc,
				Orders:       make([]settlement.OrderSettlementLine, 0),
			}
			retailClaims[npPairID] = claim
		}

		claim.Orders = append(claim.Orders, line)
	}

	return retailClaims, logisticsClaims, nil
}

// ForecastBalance projects whether the Collector settlement account has adequate funds.
func ForecastBalance(
	npPairID string,
	currentBalance ledger.AmountPaise,
	claim *settlement.SettleClaimPayload,
) BalanceForecast {
	var totalMatched ledger.AmountPaise
	var totalBFF ledger.AmountPaise

	if claim != nil {
		for _, o := range claim.Orders {
			totalMatched += o.OrderAmountPaise
			totalBFF += o.BFFPaise
		}
	}

	// Obligation leaving Collector settlement account is (TotalMatched - TotalBFF)
	requiredTransfer := totalMatched - totalBFF

	forecast := BalanceForecast{
		NPPairID:         npPairID,
		CurrentBalance:   currentBalance,
		RequiredTransfer: requiredTransfer,
		SufficientFunds:  currentBalance >= requiredTransfer,
	}

	if currentBalance < requiredTransfer {
		forecast.ProjectedDeficit = requiredTransfer - currentBalance
	}

	return forecast
}

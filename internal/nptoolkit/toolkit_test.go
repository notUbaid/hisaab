package nptoolkit_test

import (
	"testing"
	"time"

	"github.com/ubaid/hisaab/internal/nptoolkit"
	"github.com/ubaid/hisaab/internal/settlement"
)

func TestClaimBuilder_PartitioningAndLogisticsSeparation(t *testing.T) {
	builder := nptoolkit.NewClaimBuilder("np.bap.io", "collector:ondc_settlement", "receiver:ondc_settlement")

	orders := []nptoolkit.RawOrder{
		// Pair 1: Retail
		{
			NetworkOrderID:   "ORD-001",
			BuyerNPID:        "bap.retail.com",
			SellerNPID:       "bpp.grocery.com",
			IsLogistics:      false,
			OrderAmountPaise: 10000,
			CommissionPaise:  1000,
			BFFPaise:         500,
		},
		// Pair 1: Another Retail order
		{
			NetworkOrderID:   "ORD-002",
			BuyerNPID:        "bap.retail.com",
			SellerNPID:       "bpp.grocery.com",
			IsLogistics:      false,
			OrderAmountPaise: 20000,
			CommissionPaise:  2000,
			BFFPaise:         1000,
		},
		// Pair 2: Different Seller NP
		{
			NetworkOrderID:   "ORD-003",
			BuyerNPID:        "bap.retail.com",
			SellerNPID:       "bpp.electronics.com",
			IsLogistics:      false,
			OrderAmountPaise: 50000,
			CommissionPaise:  5000,
			BFFPaise:         1500,
		},
		// Pair 3: Logistics Order (Must be isolated per EC-06)
		{
			NetworkOrderID:   "ORD-LOG-001",
			BuyerNPID:        "bap.retail.com",
			SellerNPID:       "lsp.delhivery.com",
			IsLogistics:      true,
			OrderAmountPaise: 5000,
			CommissionPaise:  0,
			BFFPaise:         0,
		},
	}

	retailClaims, logisticsClaims, err := builder.BuildClaims("buyer_app", orders, time.Now())
	if err != nil {
		t.Fatalf("BuildClaims failed: %v", err)
	}

	// Should have partitioned into 2 distinct retail NP pairs
	if len(retailClaims) != 2 {
		t.Fatalf("Expected 2 retail NP pairs, got %d", len(retailClaims))
	}

	groceryPair := "bap.retail.com:bpp.grocery.com"
	claim1, ok := retailClaims[groceryPair]
	if !ok || len(claim1.Orders) != 2 {
		t.Fatalf("Expected 2 orders for grocery pair, got %v", claim1)
	}

	// Check automated SoR Due calculation: 10000 - 1000 - 500 = 8500
	if claim1.Orders[0].SoRDuePaise != 8500 {
		t.Errorf("Expected SoRDue 8500, got %d", claim1.Orders[0].SoRDuePaise)
	}

	// Logistics must be in separate slice
	if len(logisticsClaims) != 1 {
		t.Fatalf("Expected 1 logistics claim, got %d", len(logisticsClaims))
	}
	if logisticsClaims[0].Context.Domain != "ONDC:LOG10" {
		t.Errorf("Expected ONDC:LOG10 domain, got %s", logisticsClaims[0].Context.Domain)
	}
}

func TestPreFlightValidator_DetectionOfRejections(t *testing.T) {
	validator := nptoolkit.NewPreFlightValidator()

	// 1. Missing context
	report1 := validator.Validate(&settlement.SettleClaimPayload{})
	if report1.IsValid {
		t.Error("Expected report to be invalid for missing context")
	}

	// 2. Malformed NP pair
	claim2 := &settlement.SettleClaimPayload{
		Context: &settlement.BecknContext{
			MessageID: "msg-1",
			Action:    "settle",
		},
		NPPairID:     "bap_only_no_colon",
		CollectorAcc: "acc1",
		ReceiverAcc:  "acc2",
		Orders: []settlement.OrderSettlementLine{
			{NetworkOrderID: "O1", OrderAmountPaise: 1000},
		},
	}
	report2 := validator.Validate(claim2)
	if report2.IsValid {
		t.Error("Expected report to be invalid for malformed NP pair")
	}

	// 3. Duplicate order in claim
	claim3 := &settlement.SettleClaimPayload{
		Context: &settlement.BecknContext{
			MessageID: "msg-2",
			Action:    "settle",
		},
		NPPairID:     "bap:bpp",
		CollectorAcc: "acc1",
		ReceiverAcc:  "acc2",
		Orders: []settlement.OrderSettlementLine{
			{NetworkOrderID: "DUP-1", OrderAmountPaise: 1000},
			{NetworkOrderID: "DUP-1", OrderAmountPaise: 2000},
		},
	}
	report3 := validator.Validate(claim3)
	if report3.IsValid {
		t.Error("Expected report to fail on duplicate order ID")
	}

	// 4. Clean claim
	claimClean := &settlement.SettleClaimPayload{
		Context: &settlement.BecknContext{
			MessageID: "msg-clean",
			Action:    "settle",
		},
		NPPairID:     "bap.app.io:bpp.app.io",
		CollectorAcc: "collector:ondc_settlement",
		ReceiverAcc:  "receiver:ondc_settlement",
		Orders: []settlement.OrderSettlementLine{
			{
				NetworkOrderID:   "CLEAN-1",
				OrderAmountPaise: 10000,
				CommissionPaise:  1000,
				BFFPaise:         500,
				SoRDuePaise:      8500,
			},
		},
	}
	reportClean := validator.Validate(claimClean)
	if !reportClean.IsValid {
		t.Errorf("Expected clean claim to be valid, got issues: %+v", reportClean.Issues)
	}
}

func TestForecastBalance(t *testing.T) {
	claim := &settlement.SettleClaimPayload{
		Orders: []settlement.OrderSettlementLine{
			// Net required: 10000 - 500 = 9500 paise
			{NetworkOrderID: "O1", OrderAmountPaise: 10000, BFFPaise: 500},
		},
	}

	// Case 1: Sufficient balance
	f1 := nptoolkit.ForecastBalance("bap:bpp", 10000, claim)
	if !f1.SufficientFunds || f1.ProjectedDeficit != 0 {
		t.Errorf("Expected sufficient funds for 10000 balance against 9500 required, got %+v", f1)
	}

	// Case 2: Insufficient balance
	f2 := nptoolkit.ForecastBalance("bap:bpp", 5000, claim)
	if f2.SufficientFunds || f2.ProjectedDeficit != 4500 {
		t.Errorf("Expected deficit of 4500 paise, got %+v", f2)
	}
}

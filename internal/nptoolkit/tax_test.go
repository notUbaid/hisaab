package nptoolkit_test

import (
	"testing"

	"github.com/ubaid/hisaab/internal/ledger"
	"github.com/ubaid/hisaab/internal/nptoolkit"
)

func TestCalculateTaxWithholding_StandardRates(t *testing.T) {
	cfg := nptoolkit.DefaultIndiaTaxConfig()

	orderAmount := ledger.AmountPaise(150000) // ₹1,500.00
	commission := ledger.AmountPaise(4500)    // ₹45.00 (3%)
	bff := ledger.AmountPaise(1500)           // ₹15.00 (1%)

	tb, err := nptoolkit.CalculateTaxWithholding(orderAmount, commission, bff, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Gross SoR = 150000 - 4500 - 1500 = 144000
	if tb.GrossSoRDuePaise != 144000 {
		t.Fatalf("expected gross SoR 144000, got %d", tb.GrossSoRDuePaise)
	}

	// GST TCS: 0.5% (50 bps) of 144000 = 720 paise (₹7.20) per CGST Notification 15/2024
	if tb.GSTTCSPaise != 720 {
		t.Fatalf("expected GST TCS 720, got %d", tb.GSTTCSPaise)
	}

	// TDS (Section 393(1)): 0.1% (10 bps) of 150000 = 150 paise (₹1.50)
	if tb.TDSPaise != 150 {
		t.Fatalf("expected TDS 150, got %d", tb.TDSPaise)
	}

	// Total Tax = 720 + 150 = 870 paise
	if tb.TotalTaxPaise != 870 {
		t.Fatalf("expected total tax 870, got %d", tb.TotalTaxPaise)
	}

	// Net Merchant Due = 144000 - 870 = 143130 paise
	if tb.NetMerchantDue != 143130 {
		t.Fatalf("expected net merchant due 143130, got %d", tb.NetMerchantDue)
	}

	// Conservation check
	sum := tb.CommissionPaise + tb.BFFPaise + tb.TotalTaxPaise + tb.NetMerchantDue
	if sum != orderAmount {
		t.Fatalf("conservation violated: sum %d != orderAmount %d", sum, orderAmount)
	}
}

func TestCalculateTaxWithholding_PANExemption(t *testing.T) {
	cfg := nptoolkit.DefaultIndiaTaxConfig()
	cfg.PanExempt = true

	orderAmount := ledger.AmountPaise(100000)
	commission := ledger.AmountPaise(3000)
	bff := ledger.AmountPaise(1000)

	tb, err := nptoolkit.CalculateTaxWithholding(orderAmount, commission, bff, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tb.TDSPaise != 0 {
		t.Fatalf("expected 0 TDS on PAN exempt seller, got %d", tb.TDSPaise)
	}
}

func TestCalculateTaxWithholding_HigherRateNoPAN(t *testing.T) {
	cfg := nptoolkit.DefaultIndiaTaxConfig()
	cfg.HasNoPAN = true // 5% without PAN under statutory withholding rules

	orderAmount := ledger.AmountPaise(100000)
	commission := ledger.AmountPaise(3000)
	bff := ledger.AmountPaise(1000)

	tb, err := nptoolkit.CalculateTaxWithholding(orderAmount, commission, bff, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 5% of 100000 = 5000 paise
	if tb.TDSPaise != 5000 {
		t.Fatalf("expected 5000 paise TDS without PAN, got %d", tb.TDSPaise)
	}
}

func TestGenerateTaxJournalLines_Balanced(t *testing.T) {
	cfg := nptoolkit.DefaultIndiaTaxConfig()
	tb, _ := nptoolkit.CalculateTaxWithholding(200000, 6000, 2000, cfg)

	lines := nptoolkit.GenerateTaxJournalLines(tb, "receiver:operative", "fees:withholding_tax")
	if len(lines) != 2 {
		t.Fatalf("expected 2 journal lines, got %d", len(lines))
	}

	total, err := ledger.ValidateLines(lines)
	if err != nil {
		t.Fatalf("tax journal lines failed double-entry validation: %v", err)
	}
	if total != tb.TotalTaxPaise {
		t.Fatalf("expected total %d, got %d", tb.TotalTaxPaise, total)
	}
}

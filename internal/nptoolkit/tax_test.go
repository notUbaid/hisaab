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

	// GST TCS: 1% of 144000 = 1440 paise (₹14.40)
	if tb.GSTTCSPaise != 1440 {
		t.Fatalf("expected GST TCS 1440, got %d", tb.GSTTCSPaise)
	}

	// TDS (194-O): 0.1% of 150000 = 150 paise (₹1.50)
	if tb.TDSPaise != 150 {
		t.Fatalf("expected TDS 150, got %d", tb.TDSPaise)
	}

	// Total Tax = 1440 + 150 = 1590 paise
	if tb.TotalTaxPaise != 1590 {
		t.Fatalf("expected total tax 1590, got %d", tb.TotalTaxPaise)
	}

	// Net Merchant Due = 144000 - 1590 = 142410 paise
	if tb.NetMerchantDue != 142410 {
		t.Fatalf("expected net merchant due 142410, got %d", tb.NetMerchantDue)
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
	cfg.TDSBps = 100 // 1% without PAN under Section 206AA

	orderAmount := ledger.AmountPaise(100000)
	commission := ledger.AmountPaise(3000)
	bff := ledger.AmountPaise(1000)

	tb, err := nptoolkit.CalculateTaxWithholding(orderAmount, commission, bff, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 1% of 100000 = 1000 paise
	if tb.TDSPaise != 1000 {
		t.Fatalf("expected 1000 paise TDS, got %d", tb.TDSPaise)
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

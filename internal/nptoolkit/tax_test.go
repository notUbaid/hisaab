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

func TestCalculateStatutoryTaxWithholding_DirectMerchantCollection(t *testing.T) {
	cfg := nptoolkit.DefaultIndiaTaxConfig()

	req := nptoolkit.StatutoryTaxRequest{
		OrderAmountPaise:      100000,
		CommissionPaise:       3000,
		BFFPaise:              1000,
		CollectionArrangement: nptoolkit.CollectionDirectByMerchant, // COD or Direct Merchant
		Config:                cfg,
	}

	tb, err := nptoolkit.CalculateStatutoryTaxWithholding(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Under Section 52(1), operator collects TCS ONLY when consideration is collected by operator
	if tb.GSTTCSPaise != 0 {
		t.Fatalf("expected 0 GST TCS for direct merchant collection, got %d", tb.GSTTCSPaise)
	}

	// TDS under Section 393(1) still applies on gross consideration
	if tb.TDSPaise != 100 { // 0.1% of 100000
		t.Fatalf("expected 100 paise TDS, got %d", tb.TDSPaise)
	}

	// Conservation
	if tb.CommissionPaise+tb.BFFPaise+tb.TotalTaxPaise+tb.NetMerchantDue != req.OrderAmountPaise {
		t.Fatalf("conservation violated")
	}
}

func TestCalculateStatutoryTaxWithholding_ExemptAndMixedBasket(t *testing.T) {
	cfg := nptoolkit.DefaultIndiaTaxConfig()

	// Basket of ₹2,000 total: ₹1,000 taxable goods, ₹1,000 exempt food grains
	req := nptoolkit.StatutoryTaxRequest{
		OrderAmountPaise:      200000,
		CommissionPaise:       6000, // ₹60
		BFFPaise:              2000, // ₹20
		CollectionArrangement: nptoolkit.CollectionByOperator,
		LineItems: []nptoolkit.TaxLineItem{
			{
				LineID:             "item-1-taxable",
				SupplyType:         nptoolkit.SupplyTaxable,
				GrossAmountPaise:   100000,
				TaxableAmountPaise: 100000,
			},
			{
				LineID:            "item-2-exempt",
				SupplyType:        nptoolkit.SupplyExempt,
				GrossAmountPaise:  100000,
				ExemptAmountPaise: 100000,
			},
		},
		Config: cfg,
	}

	tb, err := nptoolkit.CalculateStatutoryTaxWithholding(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Total Gross SoR = 200000 - 6000 - 2000 = 192000
	// Taxable portion is 50% = 96000 paise
	if tb.TaxableSupplyPaise != 96000 {
		t.Fatalf("expected taxable base 96000 paise, got %d", tb.TaxableSupplyPaise)
	}

	// GST TCS: 0.5% of 96000 = 480 paise (NOT 0.5% of 192000 = 960 paise)
	if tb.GSTTCSPaise != 480 {
		t.Fatalf("expected GST TCS 480 paise on taxable portion, got %d", tb.GSTTCSPaise)
	}

	// TDS: 0.1% on total order = 200 paise
	if tb.TDSPaise != 200 {
		t.Fatalf("expected TDS 200 paise, got %d", tb.TDSPaise)
	}

	// Total Tax = 480 + 200 = 680 paise
	if tb.TotalTaxPaise != 680 {
		t.Fatalf("expected total tax 680 paise, got %d", tb.TotalTaxPaise)
	}

	// Conservation
	sum := tb.CommissionPaise + tb.BFFPaise + tb.TotalTaxPaise + tb.NetMerchantDue
	if sum != req.OrderAmountPaise {
		t.Fatalf("conservation violated: sum %d != %d", sum, req.OrderAmountPaise)
	}
}

func TestCalculateMonthlyGSTR8Return(t *testing.T) {
	cfg := nptoolkit.DefaultIndiaTaxConfig()

	records := []nptoolkit.SupplyRecord{
		// Supplier A: Intra-state sales ₹50,000, sales return ₹10,000 -> Net taxable ₹40,000
		{
			RecordID:              "rec-1",
			SupplierGSTIN:         "29AABCU9603R1ZM",
			IsInterState:          false,
			SupplyType:            nptoolkit.SupplyTaxable,
			CollectionArrangement: nptoolkit.CollectionByOperator,
			GrossSalesPaise:       5000000,
			TaxableAmountPaise:    5000000,
			IsReturn:              false,
		},
		{
			RecordID:              "rec-2",
			SupplierGSTIN:         "29AABCU9603R1ZM",
			IsInterState:          false,
			SupplyType:            nptoolkit.SupplyTaxable,
			CollectionArrangement: nptoolkit.CollectionByOperator,
			GrossSalesPaise:       1000000,
			TaxableAmountPaise:    1000000,
			IsReturn:              true, // Return / credit note
		},
		// Supplier B: Inter-state sales ₹100,000
		{
			RecordID:              "rec-3",
			SupplierGSTIN:         "27AABCT2345K1ZK",
			IsInterState:          true,
			SupplyType:            nptoolkit.SupplyTaxable,
			CollectionArrangement: nptoolkit.CollectionByOperator,
			GrossSalesPaise:       10000000,
			TaxableAmountPaise:    10000000,
			IsReturn:              false,
		},
		// Direct merchant record - should be excluded from GSTR-8 Section 52
		{
			RecordID:              "rec-4",
			SupplierGSTIN:         "27AABCT2345K1ZK",
			IsInterState:          true,
			SupplyType:            nptoolkit.SupplyTaxable,
			CollectionArrangement: nptoolkit.CollectionDirectByMerchant,
			GrossSalesPaise:       2000000,
			TaxableAmountPaise:    2000000,
			IsReturn:              false,
		},
	}

	summary := nptoolkit.CalculateMonthlyGSTR8Return("29AABCH0000A1Z5", "2026-10", records, cfg)

	if summary.TotalGrossSuppliesPaise != 15000000 { // 50,000 + 100,000 = ₹1,50,000 (excluding direct merchant)
		t.Fatalf("expected total gross supplies 15000000, got %d", summary.TotalGrossSuppliesPaise)
	}

	if summary.TotalReturnsPaise != 1000000 { // ₹10,000 return
		t.Fatalf("expected total returns 1000000, got %d", summary.TotalReturnsPaise)
	}

	if summary.TotalNetTaxablePaise != 14000000 { // ₹1,40,000 net taxable
		t.Fatalf("expected total net taxable 14000000, got %d", summary.TotalNetTaxablePaise)
	}

	// Supplier A (intra-state): Net ₹40,000 (4,000,000 paise).
	// Total TCS = 0.5% = 20,000 paise. Split: CGST = 10,000 paise (0.25%), SGST = 10,000 paise (0.25%).
	var supplierA *nptoolkit.SupplierGSTR8Entry
	var supplierB *nptoolkit.SupplierGSTR8Entry
	for i := range summary.SupplierEntries {
		if summary.SupplierEntries[i].SupplierGSTIN == "29AABCU9603R1ZM" {
			supplierA = &summary.SupplierEntries[i]
		} else if summary.SupplierEntries[i].SupplierGSTIN == "27AABCT2345K1ZK" {
			supplierB = &summary.SupplierEntries[i]
		}
	}

	if supplierA == nil || supplierB == nil {
		t.Fatalf("missing expected suppliers in summary")
	}

	if supplierA.NetTaxableSuppliesPaise != 4000000 {
		t.Fatalf("supplier A expected net taxable 4000000, got %d", supplierA.NetTaxableSuppliesPaise)
	}
	if supplierA.CGSTTCSPaise != 10000 || supplierA.SGSTTCSPaise != 10000 || supplierA.IGSTTCSPaise != 0 {
		t.Fatalf("supplier A intra-state split error: CGST=%d, SGST=%d, IGST=%d",
			supplierA.CGSTTCSPaise, supplierA.SGSTTCSPaise, supplierA.IGSTTCSPaise)
	}

	// Supplier B (inter-state): Net ₹1,00,000 (10,000,000 paise).
	// Total TCS = 0.5% = 50,000 paise IGST.
	if supplierB.IGSTTCSPaise != 50000 || supplierB.CGSTTCSPaise != 0 || supplierB.SGSTTCSPaise != 0 {
		t.Fatalf("supplier B inter-state error: CGST=%d, SGST=%d, IGST=%d",
			supplierB.CGSTTCSPaise, supplierB.SGSTTCSPaise, supplierB.IGSTTCSPaise)
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

func TestRoundHalfEven(t *testing.T) {
	// Half rounds to even (Banker's rounding)
	if nptoolkit.RoundHalfEven(75000, 10000) != 8 { // 7.5 -> 8 (odd to even)
		t.Errorf("Expected 75000/10000 to round to 8, got %d", nptoolkit.RoundHalfEven(75000, 10000))
	}
	if nptoolkit.RoundHalfEven(65000, 10000) != 6 { // 6.5 -> 6 (even stays even)
		t.Errorf("Expected 65000/10000 to round to 6, got %d", nptoolkit.RoundHalfEven(65000, 10000))
	}
	if nptoolkit.RoundHalfEven(74000, 10000) != 7 { // 7.4 -> 7
		t.Errorf("Expected 74000/10000 to round to 7, got %d", nptoolkit.RoundHalfEven(74000, 10000))
	}
	if nptoolkit.RoundHalfEven(76000, 10000) != 8 { // 7.6 -> 8
		t.Errorf("Expected 76000/10000 to round to 8, got %d", nptoolkit.RoundHalfEven(76000, 10000))
	}
}

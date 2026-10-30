package nptoolkit

import (
	"errors"
	"fmt"

	"github.com/ubaid/hisaab/internal/ledger"
)

// SupplyType models the statutory classification of supplies under Indian GST legislation.
type SupplyType string

const (
	SupplyTaxable    SupplyType = "taxable"     // Standard GST taxable supplies (Section 9 CGST Act)
	SupplyExempt     SupplyType = "exempt"      // Exempt supplies under Section 11 CGST Act (TCS does not apply)
	SupplyNilRated   SupplyType = "nil_rated"   // 0% rate supplies (TCS does not apply)
	SupplyNonTaxable SupplyType = "non_taxable" // Alcoholic liquor, specified petroleum (outside GST purview)
)

// CollectionArrangement indicates whether the e-commerce operator collects consideration from the buyer.
// Under Section 52(1) of the CGST Act 2017, TCS is only to be collected by the operator
// "where the consideration with respect to such supplies is to be collected by the operator".
type CollectionArrangement string

const (
	CollectionByOperator        CollectionArrangement = "operator_collects" // Digital / prepaid via collector RSP
	CollectionDirectByMerchant  CollectionArrangement = "direct_merchant"   // COD or direct merchant gateway (0 TCS)
)

// TaxConfig holds configurable statutory withholding parameters in basis points (1 bps = 0.01%).
type TaxConfig struct {
	GSTTCSBps int64 // 50 bps = 0.5% under Section 52 of CGST Act 2017 (CGST Notification 15/2024 dated 10 July 2024)
	TDSBps    int64 // 10 bps = 0.1% under Section 393(1) Table Sl. 8(v) of Income-tax Act 2025 / code 1035
	HasNoPAN  bool  // true if seller has no PAN/Aadhaar; rate defaults to 5.0% (500 bps)
	PanExempt bool  // true if seller holds valid lower-deduction certificate under Section 197
}

// DefaultIndiaTaxConfig returns the standard Indian statutory tax settings for e-commerce.
func DefaultIndiaTaxConfig() TaxConfig {
	return TaxConfig{
		GSTTCSBps: 50, // 0.5% GST TCS per CGST Notification 15/2024
		TDSBps:    10, // 0.1% TDS under Section 393(1)
		HasNoPAN:  false,
		PanExempt: false,
	}
}

// TaxLineItem represents an individual line item within an order basket.
type TaxLineItem struct {
	LineID             string             `json:"line_id"`
	SupplyType         SupplyType         `json:"supply_type"`
	GrossAmountPaise   ledger.AmountPaise `json:"gross_amount_paise"`
	TaxableAmountPaise ledger.AmountPaise `json:"taxable_amount_paise"`
	ExemptAmountPaise  ledger.AmountPaise `json:"exempt_amount_paise"`
}

// StatutoryTaxRequest contains all parameters required for exact statutory withholding.
type StatutoryTaxRequest struct {
	OrderAmountPaise      ledger.AmountPaise    `json:"order_amount_paise"`
	CommissionPaise       ledger.AmountPaise    `json:"commission_paise"`
	BFFPaise              ledger.AmountPaise    `json:"bff_paise"`
	CollectionArrangement CollectionArrangement `json:"collection_arrangement"`
	LineItems             []TaxLineItem         `json:"line_items,omitempty"`
	Config                TaxConfig             `json:"config"`
}

// TaxBreakdown contains calculated paise withholding legs.
type TaxBreakdown struct {
	OrderAmountPaise   ledger.AmountPaise    `json:"order_amount_paise"`
	CommissionPaise    ledger.AmountPaise    `json:"commission_paise"`
	BFFPaise           ledger.AmountPaise    `json:"bff_paise"`
	GrossSoRDuePaise   ledger.AmountPaise    `json:"gross_sor_due_paise"`
	TaxableSupplyPaise ledger.AmountPaise    `json:"taxable_supply_paise"`
	GSTTCSPaise        ledger.AmountPaise    `json:"gst_tcs_paise"`
	TDSPaise           ledger.AmountPaise    `json:"tds_paise"`
	TotalTaxPaise      ledger.AmountPaise    `json:"total_tax_paise"`
	NetMerchantDue     ledger.AmountPaise    `json:"net_merchant_due_paise"`
	Arrangement        CollectionArrangement `json:"collection_arrangement"`
}

// RoundHalfEven performs deterministic Banker's rounding on integer division (numerator / denominator).
// It eliminates sub-paise truncation bias while adhering to a strict zero-float policy.
func RoundHalfEven(num, denom int64) int64 {
	if denom <= 0 {
		return 0
	}
	quot := num / denom
	rem := num % denom

	half := denom / 2
	isExactHalf := (denom%2 == 0) && (rem == half)

	if rem > half {
		return quot + 1
	} else if isExactHalf {
		if quot%2 != 0 {
			return quot + 1
		}
		return quot
	}
	return quot
}

// CalculateStatutoryTaxWithholding calculates exact statutory withholding in integer paise,
// strictly distinguishing between taxable and non-taxable supplies, evaluating the consideration
// collection arrangement under Section 52(1) CGST Act, and applying Banker's rounding.
func CalculateStatutoryTaxWithholding(req StatutoryTaxRequest) (TaxBreakdown, error) {
	if req.OrderAmountPaise <= 0 {
		return TaxBreakdown{}, errors.New("tax: order amount must be strictly greater than zero paise")
	}
	if req.CommissionPaise+req.BFFPaise >= req.OrderAmountPaise {
		return TaxBreakdown{}, fmt.Errorf("tax: commission (%d) + bff (%d) exceeds order amount (%d)",
			req.CommissionPaise, req.BFFPaise, req.OrderAmountPaise)
	}

	grossSoR := req.OrderAmountPaise - req.CommissionPaise - req.BFFPaise

	// Default collection arrangement to CollectionByOperator if unspecified
	arrangement := req.CollectionArrangement
	if arrangement == "" {
		arrangement = CollectionByOperator
	}

	// 1. Determine net value of taxable supplies under Section 52 of CGST Act 2017
	var taxableSupplyBase ledger.AmountPaise
	if len(req.LineItems) > 0 {
		var sumTaxable ledger.AmountPaise
		var sumTotalLines ledger.AmountPaise
		for _, item := range req.LineItems {
			sumTotalLines += item.GrossAmountPaise
			if item.SupplyType == SupplyTaxable {
				if item.TaxableAmountPaise > 0 {
					sumTaxable += item.TaxableAmountPaise
				} else {
					sumTaxable += item.GrossAmountPaise
				}
			}
		}

		if sumTotalLines > req.OrderAmountPaise {
			return TaxBreakdown{}, fmt.Errorf("tax: line items total (%d) exceeds order amount (%d)",
				sumTotalLines, req.OrderAmountPaise)
		}

		// Proportionally reduce platform fee deductions from the taxable base
		if sumTotalLines > 0 && sumTaxable > 0 {
			taxableRatio := int64(sumTaxable)
			taxableSoR := int64(grossSoR) * taxableRatio / int64(sumTotalLines)
			taxableSupplyBase = ledger.AmountPaise(taxableSoR)
		} else {
			taxableSupplyBase = 0
		}
	} else {
		// Single-item default: Entire gross SoR is treated as taxable
		taxableSupplyBase = grossSoR
	}

	// 2. GST TCS under Section 52
	// Applies ONLY if consideration is collected by the operator AND supply is taxable
	var gstTCS ledger.AmountPaise
	if arrangement == CollectionByOperator && req.Config.GSTTCSBps > 0 && taxableSupplyBase > 0 {
		microTCS := int64(taxableSupplyBase) * req.Config.GSTTCSBps
		gstTCS = ledger.AmountPaise(RoundHalfEven(microTCS, 10000))
	}

	// 3. TDS under Section 393(1) (formerly Section 194-O)
	// Calculated on gross consideration for the sale of goods/services facilitated through the marketplace
	var tds ledger.AmountPaise
	if !req.Config.PanExempt {
		tdsBps := req.Config.TDSBps
		if req.Config.HasNoPAN {
			tdsBps = 500 // 5.0% without PAN under statutory withholding rules
		}
		if tdsBps > 0 {
			microTDS := int64(req.OrderAmountPaise) * tdsBps
			tds = ledger.AmountPaise(RoundHalfEven(microTDS, 10000))
		}
	}

	totalTax := gstTCS + tds
	if totalTax > grossSoR {
		return TaxBreakdown{}, fmt.Errorf("tax: total statutory withholding (%d paise) exceeds merchant gross due (%d paise)",
			totalTax, grossSoR)
	}

	netMerchant := grossSoR - totalTax

	// Conservation check: Commission + BFF + NetMerchant + TotalTax must strictly equal OrderAmount
	if req.CommissionPaise+req.BFFPaise+netMerchant+totalTax != req.OrderAmountPaise {
		return TaxBreakdown{}, fmt.Errorf("tax: conservation violated: %d != %d paise",
			req.CommissionPaise+req.BFFPaise+netMerchant+totalTax, req.OrderAmountPaise)
	}

	return TaxBreakdown{
		OrderAmountPaise:   req.OrderAmountPaise,
		CommissionPaise:    req.CommissionPaise,
		BFFPaise:           req.BFFPaise,
		GrossSoRDuePaise:   grossSoR,
		TaxableSupplyPaise: taxableSupplyBase,
		GSTTCSPaise:        gstTCS,
		TDSPaise:           tds,
		TotalTaxPaise:      totalTax,
		NetMerchantDue:     netMerchant,
		Arrangement:        arrangement,
	}, nil
}

// CalculateTaxWithholding provides backward compatibility for standard per-order tax withholding.
func CalculateTaxWithholding(
	orderAmount ledger.AmountPaise,
	commission ledger.AmountPaise,
	bff ledger.AmountPaise,
	cfg TaxConfig,
) (TaxBreakdown, error) {
	return CalculateStatutoryTaxWithholding(StatutoryTaxRequest{
		OrderAmountPaise:      orderAmount,
		CommissionPaise:       commission,
		BFFPaise:              bff,
		CollectionArrangement: CollectionByOperator,
		Config:                cfg,
	})
}

// GenerateTaxJournalLines produces balanced double-entry sub-ledger lines for tax remittance escrow.
func GenerateTaxJournalLines(
	breakdown TaxBreakdown,
	receiverOperativeAcc string,
	withholdingTaxAcc string,
) []ledger.EntryLine {
	if breakdown.TotalTaxPaise == 0 {
		return nil
	}

	return []ledger.EntryLine{
		{AccountName: receiverOperativeAcc, Direction: ledger.Debit, AmountPaise: breakdown.TotalTaxPaise},
		{AccountName: withholdingTaxAcc, Direction: ledger.Credit, AmountPaise: breakdown.TotalTaxPaise},
	}
}

// SupplyRecord models an order or credit note event for monthly statutory reporting.
type SupplyRecord struct {
	RecordID              string                `json:"record_id"`
	SupplierGSTIN         string                `json:"supplier_gstin"`
	SupplierPAN           string                `json:"supplier_pan"`
	IsInterState          bool                  `json:"is_inter_state"`
	SupplyType            SupplyType            `json:"supply_type"`
	CollectionArrangement CollectionArrangement `json:"collection_arrangement"`
	GrossSalesPaise       ledger.AmountPaise    `json:"gross_sales_paise"`
	TaxableAmountPaise    ledger.AmountPaise    `json:"taxable_amount_paise"`
	IsReturn              bool                  `json:"is_return"` // true if return / credit note
}

// SupplierGSTR8Entry details Table 3 of GSTR-8 for an individual supplier.
type SupplierGSTR8Entry struct {
	SupplierGSTIN               string             `json:"supplier_gstin"`
	GrossTaxableSuppliesPaise   ledger.AmountPaise `json:"gross_taxable_supplies_paise"`
	ReturnedTaxableSuppliesPaise ledger.AmountPaise `json:"returned_taxable_supplies_paise"`
	NetTaxableSuppliesPaise     ledger.AmountPaise `json:"net_taxable_supplies_paise"`
	CGSTTCSPaise                ledger.AmountPaise `json:"cgst_tcs_paise"`
	SGSTTCSPaise                ledger.AmountPaise `json:"sgst_tcs_paise"`
	IGSTTCSPaise                ledger.AmountPaise `json:"igst_tcs_paise"`
	TotalTCSPaise               ledger.AmountPaise `json:"total_tcs_paise"`
}

// MonthlyGSTR8Summary aggregates monthly Section 52 tax collection at source across all suppliers.
type MonthlyGSTR8Summary struct {
	ReturnPeriod            string               `json:"return_period"` // e.g. "2026-10"
	OperatorGSTIN           string               `json:"operator_gstin"`
	TotalGrossSuppliesPaise ledger.AmountPaise   `json:"total_gross_supplies_paise"`
	TotalReturnsPaise       ledger.AmountPaise   `json:"total_returns_paise"`
	TotalNetTaxablePaise    ledger.AmountPaise   `json:"total_net_taxable_paise"`
	TotalCGSTTCSPaise       ledger.AmountPaise   `json:"total_cgst_tcs_paise"`
	TotalSGSTTCSPaise       ledger.AmountPaise   `json:"total_sgst_tcs_paise"`
	TotalIGSTTCSPaise       ledger.AmountPaise   `json:"total_igst_tcs_paise"`
	TotalTCSPaise           ledger.AmountPaise   `json:"total_tcs_paise"`
	SupplierEntries         []SupplierGSTR8Entry `json:"supplier_entries"`
}

// CalculateMonthlyGSTR8Return compiles the official monthly statement under Section 52(4) of the CGST Act 2017.
// It computes net taxable supplies by deducting returned taxable supplies from gross taxable supplies,
// respects collection arrangements, and breaks down TCS into CGST, SGST, and IGST components.
func CalculateMonthlyGSTR8Return(
	operatorGSTIN string,
	period string,
	records []SupplyRecord,
	cfg TaxConfig,
) MonthlyGSTR8Summary {
	type supplierAgg struct {
		grossTaxable    ledger.AmountPaise
		returnsTaxable  ledger.AmountPaise
		interstateNet   ledger.AmountPaise
		intrastateNet   ledger.AmountPaise
	}

	suppliers := make(map[string]*supplierAgg)

	for _, rec := range records {
		// Only operator-collected consideration qualifies for Section 52 TCS
		if rec.CollectionArrangement != CollectionByOperator {
			continue
		}
		// Only taxable supplies qualify
		if rec.SupplyType != SupplyTaxable {
			continue
		}

		agg, exists := suppliers[rec.SupplierGSTIN]
		if !exists {
			agg = &supplierAgg{}
			suppliers[rec.SupplierGSTIN] = agg
		}

		taxableVal := rec.TaxableAmountPaise
		if taxableVal == 0 {
			taxableVal = rec.GrossSalesPaise
		}

		if rec.IsReturn {
			agg.returnsTaxable += taxableVal
			if rec.IsInterState {
				agg.interstateNet -= taxableVal
			} else {
				agg.intrastateNet -= taxableVal
			}
		} else {
			agg.grossTaxable += taxableVal
			if rec.IsInterState {
				agg.interstateNet += taxableVal
			} else {
				agg.intrastateNet += taxableVal
			}
		}
	}

	summary := MonthlyGSTR8Summary{
		ReturnPeriod:    period,
		OperatorGSTIN:   operatorGSTIN,
		SupplierEntries: make([]SupplierGSTR8Entry, 0, len(suppliers)),
	}

	for gstin, agg := range suppliers {
		netTaxable := agg.grossTaxable - agg.returnsTaxable
		if netTaxable < 0 {
			netTaxable = 0 // Negative carry-forward or nil TCS for the period
		}

		var igstTCS, cgstTCS, sgstTCS ledger.AmountPaise

		// If interstate: total rate (0.5% = 50 bps) is IGST
		if agg.interstateNet > 0 && cfg.GSTTCSBps > 0 {
			microIGST := int64(agg.interstateNet) * cfg.GSTTCSBps
			igstTCS = ledger.AmountPaise(RoundHalfEven(microIGST, 10000))
		}

		// If intrastate: split equally between CGST (25 bps = 0.25%) and SGST (25 bps = 0.25%)
		if agg.intrastateNet > 0 && cfg.GSTTCSBps > 0 {
			halfBps := cfg.GSTTCSBps / 2 // 25 bps
			microCGST := int64(agg.intrastateNet) * halfBps
			cgstTCS = ledger.AmountPaise(RoundHalfEven(microCGST, 10000))

			microSGST := int64(agg.intrastateNet) * (cfg.GSTTCSBps - halfBps)
			sgstTCS = ledger.AmountPaise(RoundHalfEven(microSGST, 10000))
		}

		totalTCS := igstTCS + cgstTCS + sgstTCS

		entry := SupplierGSTR8Entry{
			SupplierGSTIN:               gstin,
			GrossTaxableSuppliesPaise:   agg.grossTaxable,
			ReturnedTaxableSuppliesPaise: agg.returnsTaxable,
			NetTaxableSuppliesPaise:     netTaxable,
			CGSTTCSPaise:                cgstTCS,
			SGSTTCSPaise:                sgstTCS,
			IGSTTCSPaise:                igstTCS,
			TotalTCSPaise:               totalTCS,
		}

		summary.SupplierEntries = append(summary.SupplierEntries, entry)
		summary.TotalGrossSuppliesPaise += agg.grossTaxable
		summary.TotalReturnsPaise += agg.returnsTaxable
		summary.TotalNetTaxablePaise += netTaxable
		summary.TotalCGSTTCSPaise += cgstTCS
		summary.TotalSGSTTCSPaise += sgstTCS
		summary.TotalIGSTTCSPaise += igstTCS
		summary.TotalTCSPaise += totalTCS
	}

	return summary
}

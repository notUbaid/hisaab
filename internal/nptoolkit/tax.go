package nptoolkit

import (
	"errors"
	"fmt"

	"github.com/ubaid/hisaab/internal/ledger"
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

// TaxBreakdown contains calculated paise withholding legs.
type TaxBreakdown struct {
	OrderAmountPaise ledger.AmountPaise `json:"order_amount_paise"`
	CommissionPaise  ledger.AmountPaise `json:"commission_paise"`
	BFFPaise         ledger.AmountPaise `json:"bff_paise"`
	GrossSoRDuePaise ledger.AmountPaise `json:"gross_sor_due_paise"`
	GSTTCSPaise      ledger.AmountPaise `json:"gst_tcs_paise"`
	TDSPaise         ledger.AmountPaise `json:"tds_paise"`
	TotalTaxPaise    ledger.AmountPaise `json:"total_tax_paise"`
	NetMerchantDue   ledger.AmountPaise `json:"net_merchant_due_paise"`
}

// CalculateTaxWithholding calculates exact statutory withholding in integer paise.
// It guarantees paise integrity (zero rounding leakage) and preserves Invariant 5.
func CalculateTaxWithholding(
	orderAmount ledger.AmountPaise,
	commission ledger.AmountPaise,
	bff ledger.AmountPaise,
	cfg TaxConfig,
) (TaxBreakdown, error) {
	if orderAmount <= 0 {
		return TaxBreakdown{}, errors.New("tax: order amount must be strictly greater than zero paise")
	}
	if commission+bff >= orderAmount {
		return TaxBreakdown{}, fmt.Errorf("tax: commission (%d) + bff (%d) exceeds order amount (%d)",
			commission, bff, orderAmount)
	}

	grossSoR := orderAmount - commission - bff

	// GST TCS is calculated on net value of taxable supplies (excluding platform fees)
	var gstTCS ledger.AmountPaise
	if cfg.GSTTCSBps > 0 {
		gstTCS = (grossSoR * ledger.AmountPaise(cfg.GSTTCSBps)) / 10000
	}

	// TDS under Section 393(1) (formerly Section 194-O) is calculated on gross sales value
	var tds ledger.AmountPaise
	if !cfg.PanExempt {
		tdsBps := cfg.TDSBps
		if cfg.HasNoPAN {
			tdsBps = 500 // 5.0% without PAN under statutory withholding rules
		}
		if tdsBps > 0 {
			tds = (orderAmount * ledger.AmountPaise(tdsBps)) / 10000
		}
	}

	totalTax := gstTCS + tds
	if totalTax > grossSoR {
		return TaxBreakdown{}, fmt.Errorf("tax: total statutory withholding (%d paise) exceeds merchant gross due (%d paise)",
			totalTax, grossSoR)
	}

	netMerchant := grossSoR - totalTax

	// Conservation check: Commission + BFF + NetMerchant + TotalTax must strictly equal OrderAmount
	if commission+bff+netMerchant+totalTax != orderAmount {
		return TaxBreakdown{}, fmt.Errorf("tax: conservation violated: %d != %d paise",
			commission+bff+netMerchant+totalTax, orderAmount)
	}

	return TaxBreakdown{
		OrderAmountPaise: orderAmount,
		CommissionPaise:  commission,
		BFFPaise:         bff,
		GrossSoRDuePaise: grossSoR,
		GSTTCSPaise:      gstTCS,
		TDSPaise:         tds,
		TotalTaxPaise:    totalTax,
		NetMerchantDue:   netMerchant,
	}, nil
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

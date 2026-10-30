package nptoolkit

import (
	"time"

	"github.com/ubaid/hisaab/internal/calendar"
	"github.com/ubaid/hisaab/internal/ledger"
)

// StatutoryRule describes an effective-dated tax or fee withholding directive.
type StatutoryRule struct {
	ID        string    `json:"id"`
	Category  string    `json:"category"` // "GST_TCS" or "TDS"
	RateBps   int64     `json:"rate_bps"`
	LegalRef  string    `json:"legal_ref"`
	ValidFrom time.Time `json:"valid_from"`
	ValidTo   time.Time `json:"valid_to"` // Zero value time means currently active / no end date
	SourceURL string    `json:"source_url"`
}

// RulesEngine evaluates statutory rates as a function of the effective transaction date.
type RulesEngine struct {
	rules []StatutoryRule
}

// NewRulesEngine loads the official timeline of Indian e-commerce statutory withholding rules.
func NewRulesEngine() *RulesEngine {
	loc := calendar.Location()

	return &RulesEngine{
		rules: []StatutoryRule{
			// GST TCS History:
			// Pre-10 July 2024: 1.0% (100 bps)
			{
				ID:        "GST-TCS-2017",
				Category:  "GST_TCS",
				RateBps:   100,
				LegalRef:  "CGST Act 2017 Section 52",
				ValidFrom: time.Date(2017, 7, 1, 0, 0, 0, 0, loc),
				ValidTo:   time.Date(2024, 7, 9, 23, 59, 59, 999999999, loc),
				SourceURL: "https://cbic-gst.gov.in/central-tax.html",
			},
			// Effective 10 July 2024 onwards: 0.5% (50 bps)
			{
				ID:        "GST-TCS-2024",
				Category:  "GST_TCS",
				RateBps:   50,
				LegalRef:  "CGST Notification 15/2024 dated 10 July 2024",
				ValidFrom: time.Date(2024, 7, 10, 0, 0, 0, 0, loc),
				ValidTo:   time.Time{}, // Ongoing
				SourceURL: "https://cbic-gst.gov.in/pdf/central-tax-notif-15-2024.pdf",
			},

			// E-commerce TDS History:
			// Pre-1 Oct 2024: 1.0% (100 bps)
			{
				ID:        "TDS-194O-2020",
				Category:  "TDS",
				RateBps:   100,
				LegalRef:  "Income-tax Act 1961 Section 194-O",
				ValidFrom: time.Date(2020, 10, 1, 0, 0, 0, 0, loc),
				ValidTo:   time.Date(2024, 9, 30, 23, 59, 59, 999999999, loc),
				SourceURL: "https://incometaxindia.gov.in/pages/acts/income-tax-act.aspx",
			},
			// 1 Oct 2024 to 31 March 2026: 0.1% (10 bps) under amended Section 194-O
			{
				ID:        "TDS-194O-2024",
				Category:  "TDS",
				RateBps:   10,
				LegalRef:  "Finance (No. 2) Act 2024 amendment to Section 194-O",
				ValidFrom: time.Date(2024, 10, 1, 0, 0, 0, 0, loc),
				ValidTo:   time.Date(2026, 3, 31, 23, 59, 59, 999999999, loc),
				SourceURL: "https://incometaxindia.gov.in/communications/circular/circular-20-2024.pdf",
			},
			// Effective 1 April 2026 onwards: 0.1% (10 bps) under Section 393(1) Table Sl. 8(v), Code 1035
			{
				ID:        "TDS-393-2026",
				Category:  "TDS",
				RateBps:   10,
				LegalRef:  "Income-tax Act 2025 Section 393(1), Table Sl. 8(v), Payment Code 1035",
				ValidFrom: time.Date(2026, 4, 1, 0, 0, 0, 0, loc),
				ValidTo:   time.Time{}, // Ongoing
				SourceURL: "https://incometaxindia.gov.in/",
			},
		},
	}
}

// GetConfigAsOf resolves statutory rates active on a specific date.
func (e *RulesEngine) GetConfigAsOf(asOf time.Time, hasNoPAN bool, panExempt bool) TaxConfig {
	var gstTCSBps int64 = 50 // default fallback
	var tdsBps int64 = 10    // default fallback

	for _, rule := range e.rules {
		if asOf.Before(rule.ValidFrom) {
			continue
		}
		if !rule.ValidTo.IsZero() && asOf.After(rule.ValidTo) {
			continue
		}

		switch rule.Category {
		case "GST_TCS":
			gstTCSBps = rule.RateBps
		case "TDS":
			tdsBps = rule.RateBps
		}
	}

	return TaxConfig{
		GSTTCSBps: gstTCSBps,
		TDSBps:    tdsBps,
		HasNoPAN:  hasNoPAN,
		PanExempt: panExempt,
	}
}

// ComputeWithEffectiveDate resolves rates for the given date and calculates withholding.
func (e *RulesEngine) ComputeWithEffectiveDate(
	asOf time.Time,
	orderAmount ledger.AmountPaise,
	commission ledger.AmountPaise,
	bff ledger.AmountPaise,
	hasNoPAN bool,
	panExempt bool,
) (TaxBreakdown, error) {
	cfg := e.GetConfigAsOf(asOf, hasNoPAN, panExempt)
	return CalculateTaxWithholding(orderAmount, commission, bff, cfg)
}

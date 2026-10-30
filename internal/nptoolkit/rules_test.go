package nptoolkit_test

import (
	"testing"
	"time"

	"github.com/ubaid/hisaab/internal/calendar"
	"github.com/ubaid/hisaab/internal/ledger"
	"github.com/ubaid/hisaab/internal/nptoolkit"
)

func TestRulesEngine_HistoricalAndGoldenDates(t *testing.T) {
	engine := nptoolkit.NewRulesEngine()
	loc := calendar.Location()

	tests := []struct {
		name          string
		asOf          time.Time
		hasNoPAN      bool
		panExempt     bool
		expectedTCS   int64
		expectedTDS   int64
		orderAmount   ledger.AmountPaise
		comm          ledger.AmountPaise
		bff           ledger.AmountPaise
		wantTCSAmount ledger.AmountPaise
		wantTDSAmount ledger.AmountPaise
	}{
		{
			name:          "Pre-10 July 2024 (GST TCS 1.0%, TDS 1.0%)",
			asOf:          time.Date(2024, 6, 1, 12, 0, 0, 0, loc),
			hasNoPAN:      false,
			panExempt:     false,
			expectedTCS:   100,
			expectedTDS:   100,
			orderAmount:   100000,
			comm:          3000,
			bff:           1000,
			wantTCSAmount: 960,  // (100000 - 4000) * 1.0% = 960 paise
			wantTDSAmount: 1000, // 100000 * 1.0% = 1000 paise
		},
		{
			name:          "Post-10 July 2024 / Pre-1 Oct 2024 (GST TCS 0.5%, TDS 1.0%)",
			asOf:          time.Date(2024, 8, 15, 12, 0, 0, 0, loc),
			hasNoPAN:      false,
			panExempt:     false,
			expectedTCS:   50,
			expectedTDS:   100,
			orderAmount:   100000,
			comm:          3000,
			bff:           1000,
			wantTCSAmount: 480,  // (100000 - 4000) * 0.5% = 480 paise
			wantTDSAmount: 1000, // 100000 * 1.0% = 1000 paise
		},
		{
			name:          "Post-1 Oct 2024 (GST TCS 0.5%, TDS 0.1%)",
			asOf:          time.Date(2024, 10, 15, 12, 0, 0, 0, loc),
			hasNoPAN:      false,
			panExempt:     false,
			expectedTCS:   50,
			expectedTDS:   10,
			orderAmount:   100000,
			comm:          3000,
			bff:           1000,
			wantTCSAmount: 480, // (100000 - 4000) * 0.5% = 480 paise
			wantTDSAmount: 100, // 100000 * 0.1% = 100 paise
		},
		{
			name:          "Post-1 April 2026 Section 393(1) (GST TCS 0.5%, TDS 0.1%)",
			asOf:          time.Date(2026, 4, 15, 12, 0, 0, 0, loc),
			hasNoPAN:      false,
			panExempt:     false,
			expectedTCS:   50,
			expectedTDS:   10,
			orderAmount:   100000,
			comm:          3000,
			bff:           1000,
			wantTCSAmount: 480,
			wantTDSAmount: 100,
		},
		{
			name:          "Post-1 April 2026 Seller Has No PAN (TDS 5.0%)",
			asOf:          time.Date(2026, 10, 30, 12, 0, 0, 0, loc),
			hasNoPAN:      true,
			panExempt:     false,
			expectedTCS:   50,
			expectedTDS:   10,
			orderAmount:   100000,
			comm:          3000,
			bff:           1000,
			wantTCSAmount: 480,
			wantTDSAmount: 5000, // 100000 * 5.0% = 5000 paise
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := engine.GetConfigAsOf(tc.asOf, tc.hasNoPAN, tc.panExempt)
			if cfg.GSTTCSBps != tc.expectedTCS {
				t.Errorf("Expected GST TCS %d bps, got %d", tc.expectedTCS, cfg.GSTTCSBps)
			}
			if cfg.TDSBps != tc.expectedTDS {
				t.Errorf("Expected TDS %d bps, got %d", tc.expectedTDS, cfg.TDSBps)
			}

			bd, err := engine.ComputeWithEffectiveDate(
				tc.asOf,
				tc.orderAmount,
				tc.comm,
				tc.bff,
				tc.hasNoPAN,
				tc.panExempt,
			)
			if err != nil {
				t.Fatalf("ComputeWithEffectiveDate failed: %v", err)
			}

			if bd.GSTTCSPaise != tc.wantTCSAmount {
				t.Errorf("Expected GST TCS %d paise, got %d", tc.wantTCSAmount, bd.GSTTCSPaise)
			}
			if bd.TDSPaise != tc.wantTDSAmount {
				t.Errorf("Expected TDS %d paise, got %d", tc.wantTDSAmount, bd.TDSPaise)
			}
		})
	}
}

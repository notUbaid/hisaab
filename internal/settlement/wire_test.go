package settlement_test

import (
	"testing"

	"github.com/ubaid/hisaab/internal/ledger"
	"github.com/ubaid/hisaab/internal/settlement"
)

func TestParseDecimalAmountToPaise_Valid(t *testing.T) {
	cases := []struct {
		input    string
		expected ledger.AmountPaise
	}{
		{"100.50", 10050},
		{"100", 10000},
		{"100.5", 10050},
		{"0.25", 25},
		{"0.05", 5},
		{"0.00", 0},
		{"0", 0},
		{"123456.78", 12345678},
		{"  50.00  ", 5000},
	}

	for _, tc := range cases {
		got, err := settlement.ParseDecimalAmountToPaise(tc.input)
		if err != nil {
			t.Fatalf("ParseDecimalAmountToPaise(%q) unexpected error: %v", tc.input, err)
		}
		if got != tc.expected {
			t.Errorf("ParseDecimalAmountToPaise(%q) = %d, expected %d", tc.input, got, tc.expected)
		}
	}
}

func TestParseDecimalAmountToPaise_Invalid(t *testing.T) {
	invalids := []string{
		"",
		"-100.00",
		"+100.00",
		"100.555", // 3 decimals (exceeds paise precision)
		"100.50.25",
		"100a.50",
		"0100.00", // leading zeroes
		"abc",
		"1e5",
	}

	for _, inv := range invalids {
		_, err := settlement.ParseDecimalAmountToPaise(inv)
		if err == nil {
			t.Errorf("Expected error for invalid decimal %q, got nil", inv)
		}
	}
}

func TestFormatPaiseToDecimal(t *testing.T) {
	cases := []struct {
		input    ledger.AmountPaise
		expected string
	}{
		{10050, "100.50"},
		{10000, "100.00"},
		{25, "0.25"},
		{5, "0.05"},
		{0, "0.00"},
		{-5000, "-50.00"},
	}

	for _, tc := range cases {
		got := settlement.FormatPaiseToDecimal(tc.input)
		if got != tc.expected {
			t.Errorf("FormatPaiseToDecimal(%d) = %q, expected %q", tc.input, got, tc.expected)
		}
	}
}

func TestWireClaimPayload_RoundTrip(t *testing.T) {
	wireJSON := []byte(`{
		"context": {
			"domain": "ONDC:RET10",
			"action": "settle",
			"message_id": "msg-wire-1"
		},
		"settlement_id": "SETTLE-001",
		"np_pair_id": "BAP-01:BPP-01",
		"submitted_by": "buyer_app",
		"collector_account": "collector:ondc_settlement",
		"receiver_account": "receiver:ondc_settlement",
		"orders": [
			{
				"network_order_id": "ORD-WIRE-01",
				"order_amount": {"value": "1000.00", "currency": "INR"},
				"commission": {"value": "30.00", "currency": "INR"},
				"withholding_amount": {"value": "10.00", "currency": "INR"},
				"sor_due_amount": {"value": "960.00", "currency": "INR"},
				"settlement_date": "2026-10-30"
			}
		]
	}`)

	claim, err := settlement.ParseClaimFromJSON(wireJSON)
	if err != nil {
		t.Fatalf("ParseClaimFromJSON failed: %v", err)
	}

	if len(claim.Orders) != 1 {
		t.Fatalf("Expected 1 order, got %d", len(claim.Orders))
	}

	ord := claim.Orders[0]
	if ord.OrderAmountPaise != 100000 {
		t.Errorf("Expected OrderAmountPaise=100000, got %d", ord.OrderAmountPaise)
	}
	if ord.CommissionPaise != 3000 {
		t.Errorf("Expected CommissionPaise=3000, got %d", ord.CommissionPaise)
	}
	if ord.BFFPaise != 1000 {
		t.Errorf("Expected BFFPaise=1000, got %d", ord.BFFPaise)
	}
	if ord.SoRDuePaise != 96000 {
		t.Errorf("Expected SoRDuePaise=96000, got %d", ord.SoRDuePaise)
	}
}

package settlement

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ubaid/hisaab/internal/ledger"
)

var (
	ErrInvalidAmountString     = errors.New("wire: invalid decimal amount string")
	ErrNegativeAmount          = errors.New("wire: amount cannot be negative")
	ErrInvalidDecimalPrecision = errors.New("wire: decimal precision exceeds 2 fractional places (paise)")
	ErrAmountOverflow          = errors.New("wire: amount exceeds maximum allowed value (10^13 paise)")
)

// ParseDecimalAmountToPaise converts a decimal currency string (e.g. "100.50", "100", "0.25")
// into exact integer AmountPaise using integer arithmetic. Floats are strictly prohibited.
func ParseDecimalAmountToPaise(s string) (ledger.AmountPaise, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("%w: empty string", ErrInvalidAmountString)
	}

	if s[0] == '-' {
		return 0, ErrNegativeAmount
	}
	if s[0] == '+' {
		return 0, fmt.Errorf("%w: explicit plus sign not permitted", ErrInvalidAmountString)
	}

	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		return 0, fmt.Errorf("%w: multiple decimal separators in %q", ErrInvalidAmountString, s)
	}

	intPartStr := parts[0]
	if intPartStr == "" {
		intPartStr = "0"
	}

	// Validate integer digits
	for _, ch := range intPartStr {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("%w: non-numeric character in integer part %q", ErrInvalidAmountString, intPartStr)
		}
	}

	// Disallow leading zeroes (e.g. "0100" is rejected, "0" is accepted)
	if len(intPartStr) > 1 && intPartStr[0] == '0' {
		return 0, fmt.Errorf("%w: leading zeroes not permitted in %q", ErrInvalidAmountString, intPartStr)
	}

	intVal, err := strconv.ParseInt(intPartStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: integer parse failed: %v", ErrInvalidAmountString, err)
	}

	var fracVal int64
	if len(parts) == 2 {
		fracPartStr := parts[1]
		if len(fracPartStr) > 2 {
			return 0, fmt.Errorf("%w: fractional part %q has %d decimals", ErrInvalidDecimalPrecision, fracPartStr, len(fracPartStr))
		}
		for _, ch := range fracPartStr {
			if ch < '0' || ch > '9' {
				return 0, fmt.Errorf("%w: non-numeric character in fractional part %q", ErrInvalidAmountString, fracPartStr)
			}
		}

		if len(fracPartStr) == 1 {
			fracVal = int64(fracPartStr[0]-'0') * 10
		} else if len(fracPartStr) == 2 {
			fracVal = int64(fracPartStr[0]-'0')*10 + int64(fracPartStr[1]-'0')
		}
	}

	// Checked arithmetic: intVal * 100 + fracVal
	if ledger.AmountPaise(intVal) > ledger.MaxLineAmountPaise/100 {
		return 0, ErrAmountOverflow
	}

	totalPaise := intVal*100 + fracVal
	if ledger.AmountPaise(totalPaise) > ledger.MaxLineAmountPaise {
		return 0, ErrAmountOverflow
	}

	return ledger.AmountPaise(totalPaise), nil
}

// FormatPaiseToDecimal formats integer AmountPaise into exact standard decimal string representation (e.g. "100.50").
func FormatPaiseToDecimal(paise ledger.AmountPaise) string {
	if paise < 0 {
		absVal := -int64(paise)
		return fmt.Sprintf("-%d.%02d", absVal/100, absVal%100)
	}
	val := int64(paise)
	return fmt.Sprintf("%d.%02d", val/100, val%100)
}

// WireAmountObj models Beckn amount object { "value": "100.00", "currency": "INR" }
type WireAmountObj struct {
	Value    string `json:"value"`
	Currency string `json:"currency,omitempty"`
}

// WireOrderSettlementLine represents an individual order in standard Beckn / RSF 2.0 wire format.
type WireOrderSettlementLine struct {
	NetworkOrderID string         `json:"network_order_id"`
	OrderAmount    WireAmountObj  `json:"order_amount"`
	Commission     WireAmountObj  `json:"commission"`
	BFF            WireAmountObj  `json:"withholding_amount"` // Buyer Finder Fee / withholding
	SoRDue         *WireAmountObj `json:"sor_due_amount,omitempty"`
	TaxDeductions  *WireAmountObj `json:"tax_deductions,omitempty"`
	SettlementDate string         `json:"settlement_date"`
}

// WireRSFClaimPayload represents the outer Beckn /settle message payload with decimal strings.
type WireRSFClaimPayload struct {
	Context          *BecknContext             `json:"context"`
	SettlementID     string                    `json:"settlement_id"`
	NPPairID         string                    `json:"np_pair_id"`
	SubmittedBy      string                    `json:"submitted_by"`
	CollectorAccount string                    `json:"collector_account"`
	ReceiverAccount  string                    `json:"receiver_account"`
	Orders           []WireOrderSettlementLine `json:"orders"`
}

// AdaptWireClaimToInternal converts wire format decimal claims into canonical integer paise SettleClaimPayload.
func AdaptWireClaimToInternal(wire *WireRSFClaimPayload) (*SettleClaimPayload, error) {
	if wire == nil {
		return nil, errors.New("wire: claim payload is nil")
	}

	orders := make([]OrderSettlementLine, len(wire.Orders))
	for i, wo := range wire.Orders {
		orderPaise, err := ParseDecimalAmountToPaise(wo.OrderAmount.Value)
		if err != nil {
			return nil, fmt.Errorf("order %q order_amount invalid: %w", wo.NetworkOrderID, err)
		}

		commPaise, err := ParseDecimalAmountToPaise(wo.Commission.Value)
		if err != nil {
			return nil, fmt.Errorf("order %q commission invalid: %w", wo.NetworkOrderID, err)
		}

		bffPaise, err := ParseDecimalAmountToPaise(wo.BFF.Value)
		if err != nil {
			return nil, fmt.Errorf("order %q bff/withholding invalid: %w", wo.NetworkOrderID, err)
		}

		var sorPaise ledger.AmountPaise
		if wo.SoRDue != nil && wo.SoRDue.Value != "" {
			sorPaise, err = ParseDecimalAmountToPaise(wo.SoRDue.Value)
			if err != nil {
				return nil, fmt.Errorf("order %q sor_due invalid: %w", wo.NetworkOrderID, err)
			}
		} else {
			sorPaise = orderPaise - commPaise - bffPaise
		}

		var taxPaise ledger.AmountPaise
		if wo.TaxDeductions != nil && wo.TaxDeductions.Value != "" {
			taxPaise, err = ParseDecimalAmountToPaise(wo.TaxDeductions.Value)
			if err != nil {
				return nil, fmt.Errorf("order %q tax_deductions invalid: %w", wo.NetworkOrderID, err)
			}
		}

		var settleDate time.Time
		if wo.SettlementDate != "" {
			settleDate, _ = time.Parse("2006-01-02", wo.SettlementDate)
		}

		orders[i] = OrderSettlementLine{
			NetworkOrderID:   wo.NetworkOrderID,
			OrderAmountPaise: orderPaise,
			CommissionPaise:  commPaise,
			BFFPaise:         bffPaise,
			SoRDuePaise:      sorPaise,
			TaxDeductions:    taxPaise,
			SettlementDate:   settleDate,
		}
	}

	return &SettleClaimPayload{
		Context:      wire.Context,
		SettlementID: wire.SettlementID,
		NPPairID:     wire.NPPairID,
		SubmittedBy:  wire.SubmittedBy,
		CollectorAcc: wire.CollectorAccount,
		ReceiverAcc:  wire.ReceiverAccount,
		Orders:       orders,
	}, nil
}

// AdaptInternalToWireClaim converts internal integer paise payload to protocol wire representation.
func AdaptInternalToWireClaim(claim *SettleClaimPayload) *WireRSFClaimPayload {
	if claim == nil {
		return nil
	}

	wireOrders := make([]WireOrderSettlementLine, len(claim.Orders))
	for i, o := range claim.Orders {
		wireOrders[i] = WireOrderSettlementLine{
			NetworkOrderID: o.NetworkOrderID,
			OrderAmount:    WireAmountObj{Value: FormatPaiseToDecimal(o.OrderAmountPaise), Currency: "INR"},
			Commission:     WireAmountObj{Value: FormatPaiseToDecimal(o.CommissionPaise), Currency: "INR"},
			BFF:            WireAmountObj{Value: FormatPaiseToDecimal(o.BFFPaise), Currency: "INR"},
			SoRDue:         &WireAmountObj{Value: FormatPaiseToDecimal(o.SoRDuePaise), Currency: "INR"},
			TaxDeductions:  &WireAmountObj{Value: FormatPaiseToDecimal(o.TaxDeductions), Currency: "INR"},
			SettlementDate: o.SettlementDate.Format("2006-01-02"),
		}
	}

	return &WireRSFClaimPayload{
		Context:          claim.Context,
		SettlementID:     claim.SettlementID,
		NPPairID:         claim.NPPairID,
		SubmittedBy:      claim.SubmittedBy,
		CollectorAccount: claim.CollectorAcc,
		ReceiverAccount:  claim.ReceiverAcc,
		Orders:           wireOrders,
	}
}

// ParseClaimFromJSON detects whether JSON payload is in wire format (string amounts) or legacy format (paise ints)
// and normalizes it to SettleClaimPayload.
func ParseClaimFromJSON(data []byte) (*SettleClaimPayload, error) {
	// First attempt unmarshaling as WireRSFClaimPayload
	var wire WireRSFClaimPayload
	if err := json.Unmarshal(data, &wire); err == nil && len(wire.Orders) > 0 && wire.Orders[0].OrderAmount.Value != "" {
		return AdaptWireClaimToInternal(&wire)
	}

	// Fallback to direct integer paise payload
	var direct SettleClaimPayload
	if err := json.Unmarshal(data, &direct); err != nil {
		return nil, fmt.Errorf("wire: failed to parse claim payload: %w", err)
	}
	return &direct, nil
}

// BankWireRequest represents the standardized interbank credit transfer payload.
type BankWireRequest struct {
	InstructionID   string             `json:"instruction_id"`
	EndToEndID      string             `json:"end_to_end_id"`
	BatchID         string             `json:"batch_id"`
	DebtorAccount   string             `json:"debtor_account"`
	CreditorAccount string             `json:"creditor_account"`
	AmountPaise     ledger.AmountPaise `json:"amount_paise"`
	Currency        string             `json:"currency"`
	PaymentMethod   string             `json:"payment_method"` // "RTGS", "NEFT", "IMPS"
	ExecutionDate   time.Time          `json:"execution_date"`
	ISO20022XML     string             `json:"iso20022_xml,omitempty"`
}

// BankWireResponse represents the institutional gateway response containing UTR confirmation.
type BankWireResponse struct {
	ReferenceNumber string    `json:"reference_number"` // RBI Unique Transaction Reference (UTR)
	Status          string    `json:"status"`           // "ACCEPTED", "SETTLED", "REJECTED"
	Timestamp       time.Time `json:"timestamp"`
	ErrorCode       string    `json:"error_code,omitempty"`
	ErrorMessage    string    `json:"error_message,omitempty"`
}

// BankGateway defines the pluggable financial clearing rail interface for nodal settlement dispatches.
type BankGateway interface {
	DispatchWire(ctx context.Context, req BankWireRequest) (*BankWireResponse, error)
	GetStatus(ctx context.Context, referenceNumber string) (*BankWireResponse, error)
}

// ISO20022Pacs008Gateway formats and dispatches ISO 20022 pacs.008.001.08 credit transfer messages.
type ISO20022Pacs008Gateway struct {
	mode         string // "sandbox" or "live"
	endpointURL  string
	httpClient   *http.Client
	mu           sync.RWMutex
	settledWires map[string]*BankWireResponse
}

// NewISO20022Pacs008Gateway initializes a payment gateway conforming to ISO 20022 interbank standards.
func NewISO20022Pacs008Gateway(mode, endpointURL string, client *http.Client) *ISO20022Pacs008Gateway {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if mode == "" {
		mode = "sandbox"
	}
	return &ISO20022Pacs008Gateway{
		mode:         mode,
		endpointURL:  endpointURL,
		httpClient:   client,
		settledWires: make(map[string]*BankWireResponse),
	}
}

// GeneratePacs008XML formats a BankWireRequest into standard ISO 20022 pacs.008.001.08 XML document.
func (g *ISO20022Pacs008Gateway) GeneratePacs008XML(req BankWireRequest) string {
	ccy := req.Currency
	if ccy == "" {
		ccy = "INR"
	}
	method := req.PaymentMethod
	if method == "" {
		method = "RTGS"
	}
	decAmt := FormatPaiseToDecimal(req.AmountPaise)
	nowStr := req.ExecutionDate.UTC().Format(time.RFC3339)
	if req.ExecutionDate.IsZero() {
		nowStr = time.Now().UTC().Format(time.RFC3339)
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.08">
  <FIToFICstmrCdtTrf>
    <GrpHdr>
      <MsgId>MSG-%s</MsgId>
      <CreDtTm>%s</CreDtTm>
      <NbOfTxs>1</NbOfTxs>
      <SttlmInf>
        <SttlmMtd>CLRG</SttlmMtd>
        <ClrSys>
          <Prtry>%s</Prtry>
        </ClrSys>
      </SttlmInf>
    </GrpHdr>
    <CdtTrfTxInf>
      <PmtId>
        <EndToEndId>%s</EndToEndId>
      </PmtId>
      <IntrBkSttlmAmt Ccy="%s">%s</IntrBkSttlmAmt>
      <Dbtr>
        <Nm>ONDC Nodal Escrow Account</Nm>
        <Id><OrgId><Othr><Id>%s</Id></Othr></OrgId></Id>
      </Dbtr>
      <Cdtr>
        <Nm>Settlement Beneficiary Participant</Nm>
        <Id><OrgId><Othr><Id>%s</Id></Othr></OrgId></Id>
      </Cdtr>
      <RmtInf>
        <Ustrd>Hisaab Settlement Batch %s</Ustrd>
      </RmtInf>
    </CdtTrfTxInf>
  </FIToFICstmrCdtTrf>
</Document>`,
		req.InstructionID, nowStr, method, req.EndToEndID, ccy, decAmt,
		req.DebtorAccount, req.CreditorAccount, req.BatchID)
}

// DispatchWire submits an interbank payment instruction via the configured clearing rail.
func (g *ISO20022Pacs008Gateway) DispatchWire(ctx context.Context, req BankWireRequest) (*BankWireResponse, error) {
	if req.AmountPaise <= 0 {
		return nil, errors.New("gateway: wire transfer amount must be strictly positive")
	}
	if req.DebtorAccount == "" || req.CreditorAccount == "" {
		return nil, errors.New("gateway: debtor and creditor accounts must be non-empty")
	}

	xmlDoc := g.GeneratePacs008XML(req)
	req.ISO20022XML = xmlDoc

	// In sandbox mode, generate deterministic RBI UTR reference and record settlement
	if g.mode == "sandbox" {
		h := sha256.New()
		h.Write([]byte(fmt.Sprintf("%s|%s|%d|%s", req.InstructionID, req.BatchID, req.AmountPaise, req.CreditorAccount)))
		utrHash := hex.EncodeToString(h.Sum(nil))[:16]
		utr := fmt.Sprintf("UTR-ONDC-%s-%s", time.Now().Format("20060102"), strings.ToUpper(utrHash))

		resp := &BankWireResponse{
			ReferenceNumber: utr,
			Status:          "SETTLED",
			Timestamp:       time.Now().UTC(),
		}

		g.mu.Lock()
		g.settledWires[utr] = resp
		g.mu.Unlock()

		return resp, nil
	}

	// Live mTLS or clearing rail endpoint dispatch
	if g.endpointURL == "" {
		return nil, errors.New("gateway: live endpoint URL is unconfigured")
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", g.endpointURL, strings.NewReader(xmlDoc))
	if err != nil {
		return nil, fmt.Errorf("gateway: failed to create clearing request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/xml")
	httpReq.Header.Set("X-Idempotency-Key", req.InstructionID)

	httpResp, err := g.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gateway: clearing rail network dispatch error: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode >= 400 {
		return nil, fmt.Errorf("gateway: clearing rail returned HTTP error status %d", httpResp.StatusCode)
	}

	var parsedResp BankWireResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&parsedResp); err != nil {
		return nil, fmt.Errorf("gateway: failed to decode clearing response: %w", err)
	}

	g.mu.Lock()
	g.settledWires[parsedResp.ReferenceNumber] = &parsedResp
	g.mu.Unlock()

	return &parsedResp, nil
}

// GetStatus retrieves the clearing confirmation status of a previously dispatched wire.
func (g *ISO20022Pacs008Gateway) GetStatus(ctx context.Context, referenceNumber string) (*BankWireResponse, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	res, ok := g.settledWires[referenceNumber]
	if !ok {
		return nil, fmt.Errorf("gateway: reference number %q not found", referenceNumber)
	}
	return res, nil
}

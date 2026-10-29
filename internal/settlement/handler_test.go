package settlement_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ubaid/hisaab/internal/auth"
	"github.com/ubaid/hisaab/internal/ledger"
	"github.com/ubaid/hisaab/internal/settlement"
)

func setupTestSigner(t *testing.T) (*auth.RequestSigner, *auth.MemoryKeyRegistry) {
	pub, priv, err := auth.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	registry := auth.NewMemoryKeyRegistry()
	registry.RegisterKey("np.bap.io", "key-1", pub)

	signer := auth.NewRequestSigner("np.bap.io", "key-1", priv, registry)
	return signer, registry
}

func sampleClaimJSON(messageID, npPair string, amount int64) []byte {
	claim := settlement.SettleClaimPayload{
		Context: &settlement.BecknContext{
			Domain:    "ONDC:RET10",
			Action:    "settle",
			BapID:     "np.bap.io",
			MessageID: messageID,
			Timestamp: time.Now(),
		},
		SettlementID: "SETTLE-001",
		NPPairID:     npPair,
		SubmittedBy:  "buyer_app",
		CollectorAcc: "collector:ondc_settlement",
		ReceiverAcc:  "receiver:ondc_settlement",
		Orders: []settlement.OrderSettlementLine{
			{
				NetworkOrderID:   "ORD-999",
				OrderAmountPaise: ledger.AmountPaise(amount),
				CommissionPaise:  1000,
				BFFPaise:         500,
				SoRDuePaise:      ledger.AmountPaise(amount - 1500),
			},
		},
	}
	b, _ := json.Marshal(claim)
	return b
}

func TestHandler_HandleSettle_Signed_Success(t *testing.T) {
	signer, _ := setupTestSigner(t)
	handler := settlement.NewHandler(
		settlement.HandlerConfig{VerifySignatures: true},
		signer,
		nil,
	)

	payload := sampleClaimJSON("msg-valid-1", "BAP_01:BPP_01", 10000)
	digest, authHeader, err := signer.SignRequest(payload, time.Now())
	if err != nil {
		t.Fatalf("SignRequest failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/settle", bytes.NewReader(payload))
	req.Header.Set("Digest", digest)
	req.Header.Set("Authorization", authHeader)
	w := httptest.NewRecorder()

	handler.HandleSettle(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp settlement.AckResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse AckResponse: %v", err)
	}
	if resp.Message.Ack.Status != settlement.AckStatusACK {
		t.Errorf("Expected ACK status, got %s", resp.Message.Ack.Status)
	}

	// Verify claim is stored
	rec, found := handler.GetStoredClaim("msg-valid-1")
	if !found || rec.Claim.NPPairID != "BAP_01:BPP_01" {
		t.Errorf("Expected stored claim for msg-valid-1, found=%v", found)
	}
}

func TestHandler_HandleSettle_TamperedSignature_Fails(t *testing.T) {
	signer, _ := setupTestSigner(t)
	handler := settlement.NewHandler(
		settlement.HandlerConfig{VerifySignatures: true},
		signer,
		nil,
	)

	payloadOriginal := sampleClaimJSON("msg-tamper-1", "BAP_01:BPP_01", 10000)
	digest, authHeader, _ := signer.SignRequest(payloadOriginal, time.Now())

	payloadTampered := sampleClaimJSON("msg-tamper-1", "BAP_01:BPP_01", 99999)

	req := httptest.NewRequest(http.MethodPost, "/settle", bytes.NewReader(payloadTampered))
	req.Header.Set("Digest", digest)
	req.Header.Set("Authorization", authHeader)
	w := httptest.NewRecorder()

	handler.HandleSettle(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("Expected status 401 Unauthorized, got %d", w.Code)
	}

	var resp settlement.AckResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Message.Ack.Status != settlement.AckStatusNACK {
		t.Errorf("Expected NACK status, got %s", resp.Message.Ack.Status)
	}
}

func TestHandler_HandleSettle_IdempotentReplay(t *testing.T) {
	handler := settlement.NewHandler(
		settlement.HandlerConfig{VerifySignatures: false},
		nil,
		nil,
	)

	payload := sampleClaimJSON("msg-idemp-1", "BAP_01:BPP_01", 10000)

	// First request
	req1 := httptest.NewRequest(http.MethodPost, "/settle", bytes.NewReader(payload))
	w1 := httptest.NewRecorder()
	handler.HandleSettle(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("First request failed: %d", w1.Code)
	}

	// Replay exact same request
	req2 := httptest.NewRequest(http.MethodPost, "/settle", bytes.NewReader(payload))
	w2 := httptest.NewRecorder()
	handler.HandleSettle(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("Replay request failed: %d", w2.Code)
	}

	var resp settlement.AckResponse
	_ = json.Unmarshal(w2.Body.Bytes(), &resp)
	if resp.Message.Ack.Status != settlement.AckStatusACK {
		t.Errorf("Expected cached ACK, got %s", resp.Message.Ack.Status)
	}
}

func TestHandler_HandleSettle_ConflictingIdempotency(t *testing.T) {
	handler := settlement.NewHandler(
		settlement.HandlerConfig{VerifySignatures: false},
		nil,
		nil,
	)

	payload1 := sampleClaimJSON("msg-conflict-1", "BAP_01:BPP_01", 10000)
	payload2 := sampleClaimJSON("msg-conflict-1", "BAP_02:BPP_02", 20000)

	// First request
	req1 := httptest.NewRequest(http.MethodPost, "/settle", bytes.NewReader(payload1))
	w1 := httptest.NewRecorder()
	handler.HandleSettle(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("First request failed: %d", w1.Code)
	}

	// Second request with SAME message_id but conflicting payload
	req2 := httptest.NewRequest(http.MethodPost, "/settle", bytes.NewReader(payload2))
	w2 := httptest.NewRecorder()
	handler.HandleSettle(w2, req2)

	if w2.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400 for conflicting idempotency, got %d", w2.Code)
	}
	var resp settlement.AckResponse
	_ = json.Unmarshal(w2.Body.Bytes(), &resp)
	if resp.Message.Ack.Status != settlement.AckStatusNACK {
		t.Errorf("Expected NACK status for conflicting replay, got %s", resp.Message.Ack.Status)
	}
}

func TestHandler_HandleOnSettle(t *testing.T) {
	handler := settlement.NewHandler(
		settlement.HandlerConfig{VerifySignatures: false},
		nil,
		nil,
	)

	body := []byte(`{"context":{"action":"on_settle","message_id":"msg-cb-1"}}`)
	req := httptest.NewRequest(http.MethodPost, "/on_settle", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.HandleOnSettle(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200 on /on_settle, got %d", w.Code)
	}
}

package auth_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ubaid/hisaab/internal/auth"
)

func TestRequestSigner_SignAndVerify_Success(t *testing.T) {
	pub, priv, err := auth.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	registry := auth.NewMemoryKeyRegistry()
	registry.RegisterKey("np.buyer.com", "key-001", pub)

	signer := auth.NewRequestSigner("np.buyer.com", "key-001", priv, registry)

	body := []byte(`{"context":{"action":"settle","message_id":"msg-123"}}`)
	digest, authHeader, err := signer.SignRequest(body, time.Now())
	if err != nil {
		t.Fatalf("SignRequest failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/settle", bytes.NewReader(body))
	req.Header.Set("Digest", digest)
	req.Header.Set("Authorization", authHeader)

	if err := signer.VerifyRequest(req, body); err != nil {
		t.Fatalf("VerifyRequest failed on valid signature: %v", err)
	}
}

func TestRequestSigner_TamperedBody_Fails(t *testing.T) {
	pub, priv, err := auth.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	registry := auth.NewMemoryKeyRegistry()
	registry.RegisterKey("np.buyer.com", "key-001", pub)

	signer := auth.NewRequestSigner("np.buyer.com", "key-001", priv, registry)

	bodyOriginal := []byte(`{"amount": 10000}`)
	bodyTampered := []byte(`{"amount": 99999}`)

	digest, authHeader, err := signer.SignRequest(bodyOriginal, time.Now())
	if err != nil {
		t.Fatalf("SignRequest failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/settle", bytes.NewReader(bodyTampered))
	req.Header.Set("Digest", digest)
	req.Header.Set("Authorization", authHeader)

	err = signer.VerifyRequest(req, bodyTampered)
	if err == nil {
		t.Fatal("Expected verification error on tampered body, got nil")
	}
}

func TestRequestSigner_ExpiredSignature_Fails(t *testing.T) {
	pub, priv, err := auth.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	registry := auth.NewMemoryKeyRegistry()
	registry.RegisterKey("np.buyer.com", "key-001", pub)

	signer := auth.NewRequestSigner("np.buyer.com", "key-001", priv, registry)

	body := []byte(`{"context":{"action":"settle"}}`)
	// Signed 1 hour ago
	digest, authHeader, err := signer.SignRequest(body, time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("SignRequest failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/settle", bytes.NewReader(body))
	req.Header.Set("Digest", digest)
	req.Header.Set("Authorization", authHeader)

	err = signer.VerifyRequest(req, body)
	if err == nil {
		t.Fatal("Expected error for expired signature, got nil")
	}
}

func TestRequestSigner_UnknownSubscriber_Fails(t *testing.T) {
	_, priv, err := auth.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	registry := auth.NewMemoryKeyRegistry()
	// Do not register the key

	signer := auth.NewRequestSigner("np.unknown.com", "key-001", priv, registry)

	body := []byte(`{"context":{"action":"settle"}}`)
	digest, authHeader, err := signer.SignRequest(body, time.Now())
	if err != nil {
		t.Fatalf("SignRequest failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/settle", bytes.NewReader(body))
	req.Header.Set("Digest", digest)
	req.Header.Set("Authorization", authHeader)

	err = signer.VerifyRequest(req, body)
	if err == nil {
		t.Fatal("Expected error for unknown subscriber key, got nil")
	}
}

func TestRequestSigner_Blake512_NoDigestHeader_Success(t *testing.T) {
	pub, priv, err := auth.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	registry := auth.NewMemoryKeyRegistry()
	registry.RegisterKey("np.buyer.com", "key-001", pub)

	signer := auth.NewRequestSigner("np.buyer.com", "key-001", priv, registry)

	body := []byte(`{"context":{"action":"settle","message_id":"msg-blake512"}}`)
	_, authHeader, err := signer.SignRequest(body, time.Now())
	if err != nil {
		t.Fatalf("SignRequest failed: %v", err)
	}

	// ONDC requests often carry digest only inside Authorization header
	req := httptest.NewRequest(http.MethodPost, "/settle", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader)

	if err := signer.VerifyRequest(req, body); err != nil {
		t.Fatalf("VerifyRequest failed with ONDC Blake512 without Digest header: %v", err)
	}
}

func TestRequestSigner_ReplayDetection_Fails(t *testing.T) {
	pub, priv, err := auth.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	registry := auth.NewMemoryKeyRegistry()
	registry.RegisterKey("np.buyer.com", "key-001", pub)

	signer := auth.NewRequestSigner("np.buyer.com", "key-001", priv, registry)

	body := []byte(`{"context":{"action":"settle","message_id":"msg-replay"}}`)
	_, authHeader, err := signer.SignRequest(body, time.Now())
	if err != nil {
		t.Fatalf("SignRequest failed: %v", err)
	}

	req1 := httptest.NewRequest(http.MethodPost, "/settle", bytes.NewReader(body))
	req1.Header.Set("Authorization", authHeader)

	if err := signer.VerifyRequest(req1, body); err != nil {
		t.Fatalf("First request should succeed, got: %v", err)
	}

	// Replay identical request
	req2 := httptest.NewRequest(http.MethodPost, "/settle", bytes.NewReader(body))
	req2.Header.Set("Authorization", authHeader)

	err = signer.VerifyRequest(req2, body)
	if err == nil {
		t.Fatal("Expected ErrSignatureReplayed for duplicate replayed signature, got nil")
	}
}

func TestRequestSigner_PrematureAndTTLBounds(t *testing.T) {
	pub, priv, err := auth.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	registry := auth.NewMemoryKeyRegistry()
	registry.RegisterKey("np.buyer.com", "key-001", pub)

	signer := auth.NewRequestSigner("np.buyer.com", "key-001", priv, registry)
	body := []byte(`{"context":{"action":"settle"}}`)

	// 1. Premature (created 10 minutes in the future)
	_, authHeaderPremature, err := signer.SignRequest(body, time.Now().Add(10*time.Minute))
	if err != nil {
		t.Fatalf("SignRequest failed: %v", err)
	}
	reqPremature := httptest.NewRequest(http.MethodPost, "/settle", bytes.NewReader(body))
	reqPremature.Header.Set("Authorization", authHeaderPremature)
	if err := signer.VerifyRequest(reqPremature, body); err == nil {
		t.Fatal("Expected error for premature signature created in future, got nil")
	}
}


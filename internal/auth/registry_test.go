package auth_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ubaid/hisaab/internal/auth"
)

func TestONDCRegistryClient_LocalFallback(t *testing.T) {
	pub, _, err := auth.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	local := auth.NewMemoryKeyRegistry()
	local.RegisterKey("np.buyer.com", "key-1", pub)

	client := auth.NewONDCRegistryClient("", nil, local)

	resolvedPub, err := client.LookupPublicKey("np.buyer.com", "key-1")
	if err != nil {
		t.Fatalf("LookupPublicKey failed: %v", err)
	}

	if !resolvedPub.Equal(pub) {
		t.Fatalf("resolved public key does not match local key")
	}
}

func TestONDCRegistryClient_RemoteLookupAndCaching(t *testing.T) {
	pub, _, err := auth.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	pubB64 := base64.StdEncoding.EncodeToString(pub)

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		entries := []auth.ONDCSubscriberEntry{
			{
				SubscriberID:  "seller.ondc.org",
				UKId:          "seller-key-1",
				SigningPubKey: pubB64,
				Status:        "SUBSCRIBED",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(entries)
	}))
	defer server.Close()

	client := auth.NewONDCRegistryClient(server.URL, nil, nil)

	// First call: Should hit remote HTTP mock
	resolvedPub, err := client.LookupPublicKey("seller.ondc.org", "seller-key-1")
	if err != nil {
		t.Fatalf("LookupPublicKey failed: %v", err)
	}
	if !resolvedPub.Equal(pub) {
		t.Fatalf("resolved public key does not match remote key")
	}
	if callCount != 1 {
		t.Fatalf("expected 1 remote call, got %d", callCount)
	}

	// Second call: Should hit TTL cache (callCount remains 1)
	cachedPub, err := client.LookupPublicKey("seller.ondc.org", "seller-key-1")
	if err != nil {
		t.Fatalf("second LookupPublicKey failed: %v", err)
	}
	if !cachedPub.Equal(pub) {
		t.Fatalf("cached key does not match")
	}
	if callCount != 1 {
		t.Fatalf("expected cache hit with callCount 1, got %d", callCount)
	}
}

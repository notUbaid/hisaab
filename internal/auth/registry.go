package auth

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// ONDCSubscriberEntry models an NP record returned from the ONDC Registry lookup API.
type ONDCSubscriberEntry struct {
	SubscriberID    string `json:"subscriber_id"`
	UKId            string `json:"ukId"`
	SigningPubKey   string `json:"signing_public_key"`
	ValidFrom       string `json:"valid_from"`
	ValidUntil      string `json:"valid_until"`
	Type            string `json:"type"`
	Status          string `json:"status"` // SUBSCRIBED, SUSPENDED, REVOKED
}

// ONDCRegistryClient resolves subscriber public keys via the ONDC Registry /lookup endpoint
// with thread-safe caching and local fallback for offline resilience.
type ONDCRegistryClient struct {
	registryURL string
	httpClient  *http.Client
	cacheTTL    time.Duration
	local       *MemoryKeyRegistry
	mu          sync.RWMutex
	cache       map[string]cachedKey
}

type cachedKey struct {
	pub       ed25519.PublicKey
	expiresAt time.Time
}

// NewONDCRegistryClient initializes a client for the ONDC subscriber registry.
func NewONDCRegistryClient(registryURL string, client *http.Client, localFallback *MemoryKeyRegistry) *ONDCRegistryClient {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	if localFallback == nil {
		localFallback = NewMemoryKeyRegistry()
	}
	return &ONDCRegistryClient{
		registryURL: registryURL,
		httpClient:  client,
		cacheTTL:    15 * time.Minute,
		local:       localFallback,
		cache:       make(map[string]cachedKey),
	}
}

// LookupPublicKey resolves a subscriber's signing public key by checking cache, local fallback, and remote registry.
func (c *ONDCRegistryClient) LookupPublicKey(subscriberID, keyID string) (ed25519.PublicKey, error) {
	cacheKey := fmt.Sprintf("%s|%s", subscriberID, keyID)

	// 1. Check in-memory TTL cache
	c.mu.RLock()
	entry, ok := c.cache[cacheKey]
	c.mu.RUnlock()
	if ok && time.Now().Before(entry.expiresAt) {
		return entry.pub, nil
	}

	// 2. Check local fallback registry if present
	if c.local != nil {
		if pub, err := c.local.LookupPublicKey(subscriberID, keyID); err == nil && len(pub) == ed25519.PublicKeySize {
			return pub, nil
		}
	}

	// 3. Query remote ONDC registry if configured
	if c.registryURL == "" {
		return nil, fmt.Errorf("%w: subscriber %s with key %s not found (remote registry URL unconfigured)",
			ErrPublicKeyNotFound, subscriberID, keyID)
	}

	pub, err := c.fetchFromRemote(subscriberID, keyID)
	if err != nil {
		return nil, fmt.Errorf("%w: remote lookup failed for %s: %v", ErrPublicKeyNotFound, cacheKey, err)
	}

	// Cache valid key
	c.mu.Lock()
	c.cache[cacheKey] = cachedKey{
		pub:       pub,
		expiresAt: time.Now().Add(c.cacheTTL),
	}
	c.mu.Unlock()

	return pub, nil
}

// fetchFromRemote calls the ONDC registry /lookup endpoint.
func (c *ONDCRegistryClient) fetchFromRemote(subscriberID, keyID string) (ed25519.PublicKey, error) {
	lookupReq := map[string]string{
		"subscriber_id": subscriberID,
	}
	if keyID != "" {
		lookupReq["ukId"] = keyID
	}

	reqBytes, err := json.Marshal(lookupReq)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.registryURL, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned HTTP status %d", resp.StatusCode)
	}

	var entries []ONDCSubscriberEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("failed to decode registry response: %w", err)
	}

	for _, e := range entries {
		if e.SubscriberID == subscriberID && (keyID == "" || e.UKId == keyID) {
			if e.Status != "" && e.Status != "SUBSCRIBED" {
				return nil, fmt.Errorf("subscriber status is %s", e.Status)
			}
			rawKey, err := base64.StdEncoding.DecodeString(e.SigningPubKey)
			if err != nil {
				return nil, fmt.Errorf("invalid base64 signing public key: %w", err)
			}
			if len(rawKey) != ed25519.PublicKeySize {
				return nil, errors.New("invalid public key length: expected 32 bytes for Ed25519")
			}
			return ed25519.PublicKey(rawKey), nil
		}
	}

	return nil, fmt.Errorf("no matching active key found for subscriber %s", subscriberID)
}

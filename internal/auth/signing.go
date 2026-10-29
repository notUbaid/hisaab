package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrMissingAuthHeader     = errors.New("auth: missing Authorization header")
	ErrInvalidAuthFormat     = errors.New("auth: invalid Authorization header format")
	ErrSignatureExpired      = errors.New("auth: request signature expired")
	ErrPublicKeyNotFound     = errors.New("auth: public key not found for subscriber")
	ErrSignatureVerification = errors.New("auth: ed25519 signature verification failed")
	ErrMissingDigest         = errors.New("auth: missing Digest header")
	ErrDigestMismatch        = errors.New("auth: request body digest mismatch")
)

// KeyLookup abstracts resolving subscriber public keys from the ONDC Registry.
type KeyLookup interface {
	LookupPublicKey(subscriberID, keyID string) (ed25519.PublicKey, error)
}

// MemoryKeyRegistry provides an in-memory key registry for testing and local simulator operation.
type MemoryKeyRegistry struct {
	mu   sync.RWMutex
	keys map[string]ed25519.PublicKey
}

// NewMemoryKeyRegistry creates an empty in-memory key registry.
func NewMemoryKeyRegistry() *MemoryKeyRegistry {
	return &MemoryKeyRegistry{
		keys: make(map[string]ed25519.PublicKey),
	}
}

// RegisterKey registers a subscriber's public key with a specific key identifier.
func (r *MemoryKeyRegistry) RegisterKey(subscriberID, keyID string, pub ed25519.PublicKey) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := fmt.Sprintf("%s|%s", subscriberID, keyID)
	r.keys[key] = pub
}

// LookupPublicKey resolves a subscriber's public key.
func (r *MemoryKeyRegistry) LookupPublicKey(subscriberID, keyID string) (ed25519.PublicKey, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key := fmt.Sprintf("%s|%s", subscriberID, keyID)
	pub, ok := r.keys[key]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrPublicKeyNotFound, key)
	}
	return pub, nil
}

// GenerateKeyPair generates a new Ed25519 key pair for an NP or SA.
func GenerateKeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

// ComputeDigest generates the standard SHA-256 digest string for a request body: "SHA-256=<base64>".
func ComputeDigest(body []byte) string {
	hash := sha256.Sum256(body)
	return "SHA-256=" + base64.StdEncoding.EncodeToString(hash[:])
}

// VerifyDigest ensures the HTTP Digest header matches the SHA-256 hash of the payload body.
func VerifyDigest(body []byte, digestHeader string) error {
	if digestHeader == "" {
		return ErrMissingDigest
	}
	expected := ComputeDigest(body)
	if digestHeader != expected {
		return fmt.Errorf("%w: expected %s, got %s", ErrDigestMismatch, expected, digestHeader)
	}
	return nil
}

var (
	keyIDRegex     = regexp.MustCompile(`keyId="([^"]+)"`)
	signatureRegex = regexp.MustCompile(`signature="([^"]+)"`)
	createdRegex   = regexp.MustCompile(`created="([^"]+)"`)
	expiresRegex   = regexp.MustCompile(`expires="([^"]+)"`)
)

// RequestSigner handles signing outbound callbacks and verifying inbound requests.
type RequestSigner struct {
	subscriberID string
	keyID        string
	privateKey   ed25519.PrivateKey
	registry     KeyLookup
	clockSkew    time.Duration
}

// NewRequestSigner creates a RequestSigner instance.
func NewRequestSigner(subscriberID, keyID string, privateKey ed25519.PrivateKey, registry KeyLookup) *RequestSigner {
	return &RequestSigner{
		subscriberID: subscriberID,
		keyID:        keyID,
		privateKey:   privateKey,
		registry:     registry,
		clockSkew:    300 * time.Second, // 5 minutes standard grace
	}
}

// SignRequest computes the Ed25519 signature headers for an outbound request.
func (s *RequestSigner) SignRequest(body []byte, now time.Time) (digestHeader string, authHeader string, err error) {
	digestHeader = ComputeDigest(body)
	created := now.Unix()
	expires := now.Add(300 * time.Second).Unix()

	// Signing string format: (created): <created>\n(expires): <expires>\ndigest: <digest>
	signingString := fmt.Sprintf("(created): %d\n(expires): %d\ndigest: %s", created, expires, digestHeader)
	signature := ed25519.Sign(s.privateKey, []byte(signingString))
	sigB64 := base64.StdEncoding.EncodeToString(signature)

	fullKeyID := fmt.Sprintf("%s|%s|ed25519", s.subscriberID, s.keyID)
	authHeader = fmt.Sprintf(
		`Signature keyId="%s",algorithm="ed25519",created="%d",expires="%d",headers="(created) (expires) digest",signature="%s"`,
		fullKeyID, created, expires, sigB64,
	)

	return digestHeader, authHeader, nil
}

// VerifyRequest verifies the signature and digest of an incoming HTTP request.
func (s *RequestSigner) VerifyRequest(r *http.Request, body []byte) error {
	// 1. Verify Digest header
	digestHeader := r.Header.Get("Digest")
	if err := VerifyDigest(body, digestHeader); err != nil {
		return err
	}

	// 2. Parse Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return ErrMissingAuthHeader
	}

	keyIDMatches := keyIDRegex.FindStringSubmatch(authHeader)
	sigMatches := signatureRegex.FindStringSubmatch(authHeader)
	createdMatches := createdRegex.FindStringSubmatch(authHeader)
	expiresMatches := expiresRegex.FindStringSubmatch(authHeader)

	if len(keyIDMatches) < 2 || len(sigMatches) < 2 || len(createdMatches) < 2 || len(expiresMatches) < 2 {
		return ErrInvalidAuthFormat
	}

	fullKeyID := keyIDMatches[1]
	sigB64 := sigMatches[1]
	createdStr := createdMatches[1]
	expiresStr := expiresMatches[1]

	created, err := strconv.ParseInt(createdStr, 10, 64)
	if err != nil {
		return fmt.Errorf("%w: invalid created timestamp: %v", ErrInvalidAuthFormat, err)
	}
	expires, err := strconv.ParseInt(expiresStr, 10, 64)
	if err != nil {
		return fmt.Errorf("%w: invalid expires timestamp: %v", ErrInvalidAuthFormat, err)
	}

	// 3. Verify TTL
	nowUnix := time.Now().Unix()
	if nowUnix > expires+int64(s.clockSkew.Seconds()) {
		return fmt.Errorf("%w: signature expired at %d, current time %d", ErrSignatureExpired, expires, nowUnix)
	}

	// 4. Resolve Public Key from KeyID: "<subscriber_id>|<unique_key_id>|ed25519"
	parts := strings.Split(fullKeyID, "|")
	if len(parts) < 2 {
		return fmt.Errorf("%w: malformed keyId %q", ErrInvalidAuthFormat, fullKeyID)
	}
	subscriberID := parts[0]
	keyID := parts[1]

	pubKey, err := s.registry.LookupPublicKey(subscriberID, keyID)
	if err != nil {
		return err
	}

	// 5. Reconstruct signing string
	signingString := fmt.Sprintf("(created): %d\n(expires): %d\ndigest: %s", created, expires, digestHeader)

	sigBytes, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return fmt.Errorf("%w: base64 signature decode: %v", ErrInvalidAuthFormat, err)
	}

	// 6. Verify cryptographic signature
	if !ed25519.Verify(pubKey, []byte(signingString), sigBytes) {
		return ErrSignatureVerification
	}

	return nil
}

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

	"golang.org/x/crypto/blake2b"
)

var (
	ErrMissingAuthHeader     = errors.New("auth: missing Authorization header")
	ErrInvalidAuthFormat     = errors.New("auth: invalid Authorization header format")
	ErrSignatureExpired      = errors.New("auth: request signature expired")
	ErrSignaturePremature    = errors.New("auth: request signature created in the future")
	ErrSignatureTTLTooLong   = errors.New("auth: request signature TTL exceeds maximum allowed (600s)")
	ErrSignatureReplayed     = errors.New("auth: request signature has already been used (replay detected)")
	ErrPublicKeyNotFound     = errors.New("auth: public key not found for subscriber")
	ErrSignatureVerification = errors.New("auth: ed25519 signature verification failed")
	ErrMissingDigest         = errors.New("auth: missing Digest header")
	ErrDigestMismatch        = errors.New("auth: request body digest mismatch")
)

// DigestAlgorithm specifies whether BLAKE2b-512 (ONDC standard) or SHA-256 is used.
type DigestAlgorithm string

const (
	DigestBlake512 DigestAlgorithm = "BLAKE-512"
	DigestSHA256   DigestAlgorithm = "SHA-256"
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

// ComputeBlake2bDigest computes the official ONDC BLAKE2b-512 digest: "BLAKE-512=<base64>".
func ComputeBlake2bDigest(body []byte) string {
	hash := blake2b.Sum512(body)
	return "BLAKE-512=" + base64.StdEncoding.EncodeToString(hash[:])
}

// ComputeSHA256Digest computes SHA-256 digest: "SHA-256=<base64>".
func ComputeSHA256Digest(body []byte) string {
	hash := sha256.Sum256(body)
	return "SHA-256=" + base64.StdEncoding.EncodeToString(hash[:])
}

// ComputeDigest generates the standard ONDC BLAKE-512 digest string for a request body.
func ComputeDigest(body []byte) string {
	return ComputeBlake2bDigest(body)
}

// VerifyDigest ensures the HTTP Digest header matches the expected hash of the payload body.
func VerifyDigest(body []byte, digestHeader string) error {
	if digestHeader == "" {
		return ErrMissingDigest
	}
	if strings.HasPrefix(digestHeader, "BLAKE-512=") {
		expected := ComputeBlake2bDigest(body)
		if digestHeader != expected {
			return fmt.Errorf("%w: expected %s, got %s", ErrDigestMismatch, expected, digestHeader)
		}
		return nil
	}
	if strings.HasPrefix(digestHeader, "SHA-256=") {
		expected := ComputeSHA256Digest(body)
		if digestHeader != expected {
			return fmt.Errorf("%w: expected %s, got %s", ErrDigestMismatch, expected, digestHeader)
		}
		return nil
	}
	return fmt.Errorf("%w: unsupported algorithm prefix in %q", ErrDigestMismatch, digestHeader)
}

var (
	keyIDRegex     = regexp.MustCompile(`keyId="([^"]+)"`)
	signatureRegex = regexp.MustCompile(`signature="([^"]+)"`)
	createdRegex   = regexp.MustCompile(`created="([^"]+)"`)
	expiresRegex   = regexp.MustCompile(`expires="([^"]+)"`)
)

// RequestSigner handles signing outbound callbacks and verifying inbound requests according to ONDC specs.
type RequestSigner struct {
	subscriberID string
	keyID        string
	privateKey   ed25519.PrivateKey
	registry     KeyLookup
	clockSkew    time.Duration
	maxTTL       time.Duration
	algorithm    DigestAlgorithm

	replayMu    sync.Mutex
	replayCache map[string]int64 // key: signature, value: expires timestamp
}

// NewRequestSigner creates a RequestSigner instance defaulting to ONDC BLAKE-512.
func NewRequestSigner(subscriberID, keyID string, privateKey ed25519.PrivateKey, registry KeyLookup) *RequestSigner {
	return &RequestSigner{
		subscriberID: subscriberID,
		keyID:        keyID,
		privateKey:   privateKey,
		registry:     registry,
		clockSkew:    300 * time.Second, // 5 minutes standard grace
		maxTTL:       600 * time.Second, // 10 minutes maximum TTL window
		algorithm:    DigestBlake512,
		replayCache:  make(map[string]int64),
	}
}

// SetAlgorithm sets the digest algorithm (BLAKE-512 or SHA-256).
func (s *RequestSigner) SetAlgorithm(algo DigestAlgorithm) {
	s.algorithm = algo
}

// SignRequest computes the Ed25519 signature headers for an outbound request.
func (s *RequestSigner) SignRequest(body []byte, now time.Time) (digestHeader string, authHeader string, err error) {
	if s.algorithm == DigestSHA256 {
		digestHeader = ComputeSHA256Digest(body)
	} else {
		digestHeader = ComputeBlake2bDigest(body)
	}

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
	// 1. Parse Authorization header
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

	// 2. Verify Timestamps & TTL bounds
	nowUnix := time.Now().Unix()
	clockSkewSec := int64(s.clockSkew.Seconds())

	// Created cannot be too far in the future
	if created > nowUnix+clockSkewSec {
		return fmt.Errorf("%w: created at %d, current time %d", ErrSignaturePremature, created, nowUnix)
	}

	// Check expiration
	if nowUnix > expires+clockSkewSec {
		return fmt.Errorf("%w: signature expired at %d, current time %d", ErrSignatureExpired, expires, nowUnix)
	}

	// Signature TTL must not exceed maxTTL
	if (expires - created) > int64(s.maxTTL.Seconds()) {
		return fmt.Errorf("%w: TTL is %ds, max allowed %ds", ErrSignatureTTLTooLong, expires-created, int64(s.maxTTL.Seconds()))
	}

	// 3. Replay Protection
	s.replayMu.Lock()
	// Evict expired signatures periodically
	for sig, exp := range s.replayCache {
		if nowUnix > exp+clockSkewSec {
			delete(s.replayCache, sig)
		}
	}
	if _, replayed := s.replayCache[sigB64]; replayed {
		s.replayMu.Unlock()
		return ErrSignatureReplayed
	}
	s.replayCache[sigB64] = expires
	s.replayMu.Unlock()

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

	// 5. Determine digest: check Digest header if provided, or derive from body
	var digestHeader string
	if headerDigest := r.Header.Get("Digest"); headerDigest != "" {
		if err := VerifyDigest(body, headerDigest); err != nil {
			return err
		}
		digestHeader = headerDigest
	} else {
		// Default to ONDC BLAKE-512
		digestHeader = ComputeBlake2bDigest(body)
	}

	// 6. Reconstruct signing string
	// Try the selected digest first, or check both BLAKE-512 and SHA-256 if no explicit Digest header was set
	sigBytes, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return fmt.Errorf("%w: base64 signature decode: %v", ErrInvalidAuthFormat, err)
	}

	signingString := fmt.Sprintf("(created): %d\n(expires): %d\ndigest: %s", created, expires, digestHeader)
	if ed25519.Verify(pubKey, []byte(signingString), sigBytes) {
		return nil
	}

	// If no Digest header was provided and BLAKE-512 failed, test SHA-256 fallback
	if r.Header.Get("Digest") == "" {
		fallbackDigest := ComputeSHA256Digest(body)
		fallbackString := fmt.Sprintf("(created): %d\n(expires): %d\ndigest: %s", created, expires, fallbackDigest)
		if ed25519.Verify(pubKey, []byte(fallbackString), sigBytes) {
			return nil
		}
	}

	return ErrSignatureVerification
}

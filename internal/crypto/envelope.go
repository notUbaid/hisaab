package crypto

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"sync"
)

var (
	ErrInvalidCiphertext = errors.New("crypto: invalid ciphertext length or structure")
	ErrInvalidDEK        = errors.New("crypto: invalid encrypted data encryption key")
	ErrInvalidKEKSize    = errors.New("crypto: KEK must be exactly 32 bytes for AES-256")
	ErrDecryptionFailed  = errors.New("crypto: authentication tag mismatch or corrupted ciphertext")
)

// FieldEncryptor defines the envelope encryption contract specified in ADR-002.
type FieldEncryptor interface {
	Encrypt(ctx context.Context, plaintext []byte) (ciphertext, encryptedDEK []byte, err error)
	Decrypt(ctx context.Context, ciphertext, encryptedDEK []byte) ([]byte, error)
	RotateKEK(ctx context.Context, newKEK []byte) error
}

// EnvelopeEncryptor implements FieldEncryptor using AES-256-GCM envelope encryption.
// Each record is protected with an ephemeral 32-byte Data Encryption Key (DEK),
// which is itself encrypted under the primary Key Encryption Key (KEK).
type EnvelopeEncryptor struct {
	mu  sync.RWMutex
	kek []byte
}

// NewEnvelopeEncryptor creates an EnvelopeEncryptor with a 32-byte master KEK.
func NewEnvelopeEncryptor(kek []byte) (*EnvelopeEncryptor, error) {
	if len(kek) != 32 {
		return nil, ErrInvalidKEKSize
	}
	kekCopy := make([]byte, 32)
	copy(kekCopy, kek)
	return &EnvelopeEncryptor{kek: kekCopy}, nil
}

// GenerateRandomKEK generates a cryptographically secure 256-bit Key Encryption Key.
func GenerateRandomKEK() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("crypto: failed to generate random KEK: %w", err)
	}
	return key, nil
}

// Encrypt generates an ephemeral DEK, encrypts the plaintext with the DEK, and wraps the DEK under KEK.
// Returns nonce-prepended ciphertext and nonce-prepended encryptedDEK.
func (e *EnvelopeEncryptor) Encrypt(ctx context.Context, plaintext []byte) ([]byte, []byte, error) {
	e.mu.RLock()
	kek := e.kek
	e.mu.RUnlock()

	// 1. Generate ephemeral 256-bit Data Encryption Key (DEK)
	dek := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, nil, fmt.Errorf("crypto: failed to generate DEK: %w", err)
	}

	// 2. Encrypt plaintext with DEK using AES-GCM
	dekBlock, err := aes.NewCipher(dek)
	if err != nil {
		return nil, nil, fmt.Errorf("crypto: dek cipher init failed: %w", err)
	}
	dekGCM, err := cipher.NewGCM(dekBlock)
	if err != nil {
		return nil, nil, fmt.Errorf("crypto: dek gcm init failed: %w", err)
	}

	nonceData := make([]byte, dekGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonceData); err != nil {
		return nil, nil, fmt.Errorf("crypto: failed to generate data nonce: %w", err)
	}
	ciphertext := dekGCM.Seal(nonceData, nonceData, plaintext, nil)

	// 3. Encrypt DEK with KEK using AES-GCM
	kekBlock, err := aes.NewCipher(kek)
	if err != nil {
		return nil, nil, fmt.Errorf("crypto: kek cipher init failed: %w", err)
	}
	kekGCM, err := cipher.NewGCM(kekBlock)
	if err != nil {
		return nil, nil, fmt.Errorf("crypto: kek gcm init failed: %w", err)
	}

	nonceDEK := make([]byte, kekGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonceDEK); err != nil {
		return nil, nil, fmt.Errorf("crypto: failed to generate dek nonce: %w", err)
	}
	encryptedDEK := kekGCM.Seal(nonceDEK, nonceDEK, dek, nil)

	return ciphertext, encryptedDEK, nil
}

// Decrypt un-wraps the DEK using KEK, then decrypts the ciphertext using the un-wrapped DEK.
func (e *EnvelopeEncryptor) Decrypt(ctx context.Context, ciphertext, encryptedDEK []byte) ([]byte, error) {
	e.mu.RLock()
	kek := e.kek
	e.mu.RUnlock()

	// 1. Decrypt DEK using KEK
	kekBlock, err := aes.NewCipher(kek)
	if err != nil {
		return nil, fmt.Errorf("crypto: kek cipher init failed: %w", err)
	}
	kekGCM, err := cipher.NewGCM(kekBlock)
	if err != nil {
		return nil, fmt.Errorf("crypto: kek gcm init failed: %w", err)
	}

	nonceSizeDEK := kekGCM.NonceSize()
	if len(encryptedDEK) < nonceSizeDEK {
		return nil, ErrInvalidDEK
	}

	nonceDEK := encryptedDEK[:nonceSizeDEK]
	rawEncryptedDEK := encryptedDEK[nonceSizeDEK:]

	dek, err := kekGCM.Open(nil, nonceDEK, rawEncryptedDEK, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to unwrap DEK", ErrDecryptionFailed)
	}

	// 2. Decrypt data ciphertext using un-wrapped DEK
	dekBlock, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("crypto: dek cipher init failed: %w", err)
	}
	dekGCM, err := cipher.NewGCM(dekBlock)
	if err != nil {
		return nil, fmt.Errorf("crypto: dek gcm init failed: %w", err)
	}

	nonceSizeData := dekGCM.NonceSize()
	if len(ciphertext) < nonceSizeData {
		return nil, ErrInvalidCiphertext
	}

	nonceData := ciphertext[:nonceSizeData]
	rawCiphertext := ciphertext[nonceSizeData:]

	plaintext, err := dekGCM.Open(nil, nonceData, rawCiphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to decrypt payload", ErrDecryptionFailed)
	}

	return plaintext, nil
}

// RotateKEK replaces the master KEK with a new 32-byte key.
func (e *EnvelopeEncryptor) RotateKEK(ctx context.Context, newKEK []byte) error {
	if len(newKEK) != 32 {
		return ErrInvalidKEKSize
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	copy(e.kek, newKEK)
	return nil
}

// EncryptString encodes output as standard base64 strings for storage or API transfer.
func (e *EnvelopeEncryptor) EncryptString(ctx context.Context, plaintext string) (string, string, error) {
	cipherBytes, dekBytes, err := e.Encrypt(ctx, []byte(plaintext))
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(cipherBytes), base64.StdEncoding.EncodeToString(dekBytes), nil
}

// DecryptString decrypts base64 encoded ciphertext and encrypted DEK strings.
func (e *EnvelopeEncryptor) DecryptString(ctx context.Context, cipherB64, dekB64 string) (string, error) {
	cipherBytes, err := base64.StdEncoding.DecodeString(cipherB64)
	if err != nil {
		return "", fmt.Errorf("crypto: invalid base64 ciphertext: %w", err)
	}
	dekBytes, err := base64.StdEncoding.DecodeString(dekB64)
	if err != nil {
		return "", fmt.Errorf("crypto: invalid base64 encrypted DEK: %w", err)
	}
	plainBytes, err := e.Decrypt(ctx, cipherBytes, dekBytes)
	if err != nil {
		return "", err
	}
	return string(plainBytes), nil
}

// MaskAccountNumber masks sensitive bank account digits, displaying only the last 4 characters.
func MaskAccountNumber(acc string) string {
	if len(acc) <= 4 {
		return "••••"
	}
	return "•••• •••• " + acc[len(acc)-4:]
}

// MaskIFSC masks intermediate bank branch identifiers.
func MaskIFSC(ifsc string) string {
	if len(ifsc) <= 4 {
		return "••••"
	}
	return ifsc[:4] + "•••••••"
}

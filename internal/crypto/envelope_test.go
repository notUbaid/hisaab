package crypto_test

import (
	"context"
	"testing"

	"github.com/ubaid/hisaab/internal/crypto"
)

func TestEnvelopeEncryptor_RoundTrip(t *testing.T) {
	kek, err := crypto.GenerateRandomKEK()
	if err != nil {
		t.Fatalf("GenerateRandomKEK failed: %v", err)
	}

	enc, err := crypto.NewEnvelopeEncryptor(kek)
	if err != nil {
		t.Fatalf("NewEnvelopeEncryptor failed: %v", err)
	}

	ctx := context.Background()
	plaintext := []byte("ACCOUNT:91823746192837|IFSC:HDFC0001234|BENEFICIARY:ACME_RETAIL")

	cipher, dek, err := enc.Encrypt(ctx, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	decrypted, err := enc.Decrypt(ctx, cipher, dek)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Fatalf("decrypted text %q != plaintext %q", string(decrypted), string(plaintext))
	}
}

func TestEnvelopeEncryptor_TamperedCiphertext_Fails(t *testing.T) {
	kek, _ := crypto.GenerateRandomKEK()
	enc, _ := crypto.NewEnvelopeEncryptor(kek)
	ctx := context.Background()

	plaintext := []byte("SECRET_FINANCIAL_PAYLOAD")
	cipher, dek, err := enc.Encrypt(ctx, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Tamper one byte of ciphertext
	cipher[len(cipher)-1] ^= 0xFF

	_, err = enc.Decrypt(ctx, cipher, dek)
	if err == nil {
		t.Fatalf("expected decryption error on tampered ciphertext, got nil")
	}
}

func TestEnvelopeEncryptor_TamperedDEK_Fails(t *testing.T) {
	kek, _ := crypto.GenerateRandomKEK()
	enc, _ := crypto.NewEnvelopeEncryptor(kek)
	ctx := context.Background()

	plaintext := []byte("SECRET_FINANCIAL_PAYLOAD")
	cipher, dek, _ := enc.Encrypt(ctx, plaintext)

	// Tamper one byte of wrapped DEK
	dek[len(dek)-1] ^= 0xFF

	_, decErr := enc.Decrypt(ctx, cipher, dek)
	if decErr == nil {
		t.Fatalf("expected decryption error on tampered DEK, got nil")
	}
}

func TestEnvelopeEncryptor_RotateKEK(t *testing.T) {
	kek1, _ := crypto.GenerateRandomKEK()
	kek2, _ := crypto.GenerateRandomKEK()

	enc, _ := crypto.NewEnvelopeEncryptor(kek1)
	ctx := context.Background()

	plaintext := []byte("TRANSFER_INSTRUCTION_ACC_99182")
	cipher, dek, err := enc.Encrypt(ctx, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Decrypt under KEK1 succeeds
	dec1, err := enc.Decrypt(ctx, cipher, dek)
	if err != nil || string(dec1) != string(plaintext) {
		t.Fatalf("Decrypt under KEK1 failed: %v", err)
	}

	// Rotate to KEK2
	if err := enc.RotateKEK(ctx, kek2); err != nil {
		t.Fatalf("RotateKEK failed: %v", err)
	}

	// Decrypt with old DEK unwrapped under KEK2 should fail because DEK was wrapped with KEK1
	_, err = enc.Decrypt(ctx, cipher, dek)
	if err == nil {
		t.Fatalf("expected decryption with mismatched KEK to fail")
	}
}

func TestEnvelopeEncryptor_StringHelpersAndMasking(t *testing.T) {
	kek, _ := crypto.GenerateRandomKEK()
	enc, _ := crypto.NewEnvelopeEncryptor(kek)
	ctx := context.Background()

	rawAcc := "987654321098"
	cipherB64, dekB64, err := enc.EncryptString(ctx, rawAcc)
	if err != nil {
		t.Fatalf("EncryptString failed: %v", err)
	}

	decrypted, err := enc.DecryptString(ctx, cipherB64, dekB64)
	if err != nil {
		t.Fatalf("DecryptString failed: %v", err)
	}
	if decrypted != rawAcc {
		t.Fatalf("decrypted %s != original %s", decrypted, rawAcc)
	}

	// Masking checks
	maskedAcc := crypto.MaskAccountNumber(rawAcc)
	if maskedAcc != "•••• •••• 1098" {
		t.Errorf("unexpected masked account: %s", maskedAcc)
	}

	maskedIFSC := crypto.MaskIFSC("HDFC0001234")
	if maskedIFSC != "HDFC•••••••" {
		t.Errorf("unexpected masked IFSC: %s", maskedIFSC)
	}
}

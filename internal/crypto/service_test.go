package crypto

import (
	"encoding/base64"
	"testing"
)

func testKey() string {
	// Exactly 32-byte key base64-encoded
	return base64.StdEncoding.EncodeToString([]byte("test-encryption-key-32-bytes!xxx"))
}

func TestNewService(t *testing.T) {
	_, err := NewService(testKey())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
}

func TestNewService_InvalidBase64(t *testing.T) {
	_, err := NewService("not-valid-base64!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestNewService_WrongKeyLength(t *testing.T) {
	shortKey := base64.StdEncoding.EncodeToString([]byte("short"))
	_, err := NewService(shortKey)
	if err == nil {
		t.Fatal("expected error for short key")
	}
}

func TestEncryptDecrypt(t *testing.T) {
	svc, err := NewService(testKey())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	tests := []string{
		"hello world",
		"ghp_abc123secrettoken",
		"sk-ant-api-key-very-long-string-here",
		"", // empty string
	}

	for _, plaintext := range tests {
		encrypted, err := svc.Encrypt(plaintext)
		if err != nil {
			t.Fatalf("Encrypt(%q): %v", plaintext, err)
		}

		if plaintext == "" {
			if encrypted != "" {
				t.Fatalf("expected empty ciphertext for empty plaintext, got %q", encrypted)
			}
			continue
		}

		if encrypted == plaintext {
			t.Fatalf("ciphertext should differ from plaintext")
		}

		decrypted, err := svc.Decrypt(encrypted)
		if err != nil {
			t.Fatalf("Decrypt: %v", err)
		}

		if decrypted != plaintext {
			t.Fatalf("expected %q, got %q", plaintext, decrypted)
		}
	}
}

func TestEncrypt_DifferentNonce(t *testing.T) {
	svc, err := NewService(testKey())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	c1, _ := svc.Encrypt("same input")
	c2, _ := svc.Encrypt("same input")

	if c1 == c2 {
		t.Fatal("two encryptions of same plaintext should produce different ciphertexts")
	}
}

func TestDecrypt_InvalidCiphertext(t *testing.T) {
	svc, err := NewService(testKey())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = svc.Decrypt("dGhpcyBpcyBub3QgdmFsaWQ=") // valid base64, invalid ciphertext
	if err == nil {
		t.Fatal("expected error for invalid ciphertext")
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	svc1, _ := NewService(testKey())
	key2 := base64.StdEncoding.EncodeToString([]byte("different-key-also-32-bytes!!xxx"))
	svc2, _ := NewService(key2)

	encrypted, _ := svc1.Encrypt("secret data")
	_, err := svc2.Decrypt(encrypted)
	if err == nil {
		t.Fatal("expected error when decrypting with wrong key")
	}
}

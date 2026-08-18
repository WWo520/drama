package cryptox

import "testing"

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := DeriveKey("dev-secret-for-local-test")
	plain := "sk-test-abcdef1234567890abcdef"

	enc, err := Encrypt(plain, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if enc == plain {
		t.Fatal("密文不应等于明文")
	}

	dec, err := Decrypt(enc, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if dec != plain {
		t.Fatalf("往返不一致: got %q want %q", dec, plain)
	}

	// 错误密钥解密必须失败
	if _, err := Decrypt(enc, DeriveKey("wrong-secret")); err == nil {
		t.Fatal("错误密钥应解密失败")
	}
}

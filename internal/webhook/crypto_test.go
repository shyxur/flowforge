package webhook

import "testing"

func TestHMACSignatureDeterministicAndPayloadBound(t *testing.T) {
	signer := HMACSigner{}
	first := signer.Sign("secret", "1722326400", []byte(`{"task":"one"}`))
	second := signer.Sign("secret", "1722326400", []byte(`{"task":"one"}`))
	changed := signer.Sign("secret", "1722326400", []byte(`{"task":"two"}`))

	if first != second {
		t.Fatalf("same input produced different signatures: %q != %q", first, second)
	}
	if first == changed {
		t.Fatal("payload change did not change signature")
	}
	if !signer.Verify("secret", "1722326400", []byte(`{"task":"one"}`), first) {
		t.Fatal("valid signature was rejected")
	}
	if signer.Verify("wrong-secret", "1722326400", []byte(`{"task":"one"}`), first) {
		t.Fatal("wrong secret was accepted")
	}
}

func TestSecretCipherRoundTripAndWrongKey(t *testing.T) {
	cipher := NewSecretCipher("test-key")
	encrypted, err := cipher.Encrypt("signing-secret")
	if err != nil {
		t.Fatal(err)
	}
	if encrypted == "signing-secret" {
		t.Fatal("secret was stored as plaintext")
	}
	decrypted, err := cipher.Decrypt(encrypted)
	if err != nil || decrypted != "signing-secret" {
		t.Fatalf("decrypt = %q, err=%v", decrypted, err)
	}
	if _, err := NewSecretCipher("wrong-key").Decrypt(encrypted); err == nil {
		t.Fatal("wrong encryption key decrypted secret")
	}
}

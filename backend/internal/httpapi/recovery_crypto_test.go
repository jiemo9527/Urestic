package httpapi

import (
	"encoding/json"
	"testing"
)

func TestEncryptDecryptRecoveryPack(t *testing.T) {
	original := configExport{
		FormatVersion:    2,
		ExportedAt:       "2026-07-05T00:00:00Z",
		DefaultVariables: map[string]string{"RESTIC_COMPRESSION": "auto"},
		Client:           json.RawMessage(`{"locale":"zh-CN"}`),
	}
	pack, err := encryptRecoveryPack(original, "strong-password")
	if err != nil {
		t.Fatalf("encryptRecoveryPack failed: %v", err)
	}
	if pack.Kind != recoveryPackKind || pack.Payload == "" {
		t.Fatalf("unexpected encrypted pack: %+v", pack)
	}
	raw, err := json.Marshal(pack)
	if err != nil {
		t.Fatalf("marshal encrypted pack failed: %v", err)
	}
	decrypted, err := decryptRecoveryPack(raw, "strong-password")
	if err != nil {
		t.Fatalf("decryptRecoveryPack failed: %v", err)
	}
	if decrypted.FormatVersion != original.FormatVersion || decrypted.DefaultVariables["RESTIC_COMPRESSION"] != "auto" || string(decrypted.Client) != string(original.Client) {
		t.Fatalf("unexpected decrypted payload: %+v", decrypted)
	}
	if _, err := decryptRecoveryPack(raw, "wrong-password"); err == nil {
		t.Fatal("expected wrong password to fail")
	}
}

func TestRecoveryPackPasswordValidation(t *testing.T) {
	for _, password := range []string{"", "   ", "short"} {
		if err := validateRecoveryPackPassword(password); err == nil {
			t.Fatalf("expected password %q to be rejected", password)
		}
	}
	if err := validateRecoveryPackPassword("12345678"); err != nil {
		t.Fatalf("expected valid password: %v", err)
	}
}

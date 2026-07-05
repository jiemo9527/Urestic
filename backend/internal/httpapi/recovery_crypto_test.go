package httpapi

import (
	"encoding/json"
	"testing"
)

func TestEncryptDecryptRecoveryPack(t *testing.T) {
	original := configExport{
		FormatVersion: 2,
		ExportedAt:    "2026-07-05T00:00:00Z",
		Repositories: []repositoryExport{{
			Name:        "main-r2",
			Backend:     "r2",
			RepoURL:     "s3:https://example.r2.cloudflarestorage.com/bucket/prefix",
			Password:    "restic-secret",
			Variables:   map[string]string{"r2_access_key_id": "key", "r2_secret_access_key": "secret"},
			Description: "primary repo",
		}},
		Notifications: []notificationExport{{
			Name:     "telegram",
			Type:     "telegram",
			Settings: map[string]string{"bot_token": "token", "chat_id": "1000"},
		}},
		DefaultVariables: map[string]string{"RESTIC_COMPRESSION": "auto"},
		RcloneConfig:     rcloneConfigExport{Included: true, Path: "/app/data/rclone/rclone.conf", Content: "[remote]\ntype = s3\n"},
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
	if decrypted.FormatVersion != original.FormatVersion || decrypted.Repositories[0].Password != "restic-secret" || decrypted.Notifications[0].Settings["bot_token"] != "token" || decrypted.DefaultVariables["RESTIC_COMPRESSION"] != "auto" || decrypted.RcloneConfig.Content == "" || string(decrypted.Client) != string(original.Client) {
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

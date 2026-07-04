package scriptgen

import (
	"strings"
	"testing"

	"github.com/urestic/urestic/backend/internal/repositories"
)

func TestGenerateRestoreShellScript(t *testing.T) {
	repository := repositories.Repository{
		ID:      "repo-1",
		Name:    "Main Repo",
		Backend: "s3",
		RepoURL: "s3:https://example.com/bucket/prefix",
	}
	result := Generate(repository, Request{
		ScriptType: "sh",
		SecretMode: "placeholder",
		Mode:       "restore",
		Tags:       []string{"daily"},
		Options: BackupOptions{
			Host: "server-a",
		},
		Restore: RestoreOptions{
			SnapshotID:   "latest",
			TargetDir:    "/restore",
			IncludePaths: []string{"/var/www", ""},
		},
	})

	if len(result.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(result.Files))
	}
	if result.Files[0].Name != "main-repo-restore.sh" {
		t.Fatalf("unexpected script file name: %s", result.Files[0].Name)
	}
	if result.Files[1].Name != "main-repo-restore-config.json" {
		t.Fatalf("unexpected config file name: %s", result.Files[1].Name)
	}

	script := result.Files[0].Content
	for _, want := range []string{
		"restic restore 'latest' --target '/restore' --include '/var/www' --host 'server-a' --tag 'daily'",
		"restic backup",
		"restic forget",
	} {
		if strings.HasPrefix(want, "restic restore") && !strings.Contains(script, want) {
			t.Fatalf("restore command missing from script:\n%s", script)
		}
		if !strings.HasPrefix(want, "restic restore") && strings.Contains(script, want) {
			t.Fatalf("restore script should not contain %q:\n%s", want, script)
		}
	}

	config := result.Files[1].Content
	for _, want := range []string{`"mode": "restore"`, `"snapshotId": "latest"`, `"targetDir": "/restore"`, `"/var/www"`} {
		if !strings.Contains(config, want) {
			t.Fatalf("restore config missing %s:\n%s", want, config)
		}
	}
}

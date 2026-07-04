package auth

import (
	"bytes"
	"context"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/urestic/urestic/backend/internal/config"
)

func TestOpenGeneratesInitialPassword(t *testing.T) {
	dataDir := t.TempDir()
	cfg := config.Config{
		AuthEnabled:     true,
		AdminUsername:   "admin",
		DataDir:         dataDir,
		SessionTTLHours: 12,
	}

	var logs bytes.Buffer
	originalWriter := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(originalWriter) })

	manager, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if _, err := os.Stat(passwordHashPath(dataDir)); err != nil {
		t.Fatalf("generated password hash was not written: %v", err)
	}

	password := passwordFromLog(t, logs.String())
	if _, ok := manager.Login(context.Background(), "admin", password); !ok {
		t.Fatal("generated password did not authenticate")
	}

	logs.Reset()
	if _, err := Open(cfg); err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	if strings.Contains(logs.String(), "password=") {
		t.Fatalf("initial password was printed again: %s", logs.String())
	}
}

func passwordFromLog(t *testing.T, value string) string {
	t.Helper()
	const marker = "password="
	start := strings.Index(value, marker)
	if start < 0 {
		t.Fatalf("initial password was not printed: %s", value)
	}
	fields := strings.Fields(value[start+len(marker):])
	if len(fields) == 0 {
		t.Fatalf("initial password log line was malformed: %s", value)
	}
	return fields[0]
}

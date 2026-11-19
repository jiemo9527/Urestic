package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/urestic/urestic/backend/internal/auth"
	"github.com/urestic/urestic/backend/internal/config"
	"github.com/urestic/urestic/backend/internal/db"
	"github.com/urestic/urestic/backend/internal/notifications"
	"github.com/urestic/urestic/backend/internal/repositories"
	"github.com/urestic/urestic/backend/internal/secrets"
	"github.com/urestic/urestic/backend/internal/settings"
)

// Uses a disposable local restic repository; never touches cloud credentials or production data.
func TestSnapshotDeletionHTTPWithRealRestic(t *testing.T) {
	binary := os.Getenv("URESTIC_TEST_RESTIC")
	if binary == "" {
		t.Skip("set URESTIC_TEST_RESTIC to run real repository integration test")
	}
	t.Setenv("PATH", filepath.Dir(binary)+string(os.PathListSeparator)+os.Getenv("PATH"))
	root := t.TempDir()
	repoPath := filepath.Join(root, "repository")
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	password := "disposable-test-repository"
	env := append(os.Environ(), "RESTIC_PASSWORD="+password)
	run := func(args ...string) []byte {
		t.Helper()
		command := exec.Command(binary, append([]string{"-r", repoPath}, args...)...)
		command.Env = env
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("restic %q failed: %v %s", args, err, output)
		}
		return output
	}
	run("init")
	for i := 0; i < 3; i++ {
		if err := os.WriteFile(filepath.Join(source, "fixture.txt"), []byte{byte('a' + i)}, 0600); err != nil {
			t.Fatal(err)
		}
		run("backup", source, "--tag", "fixture")
	}
	var initial []resticSnapshot
	if err := json.Unmarshal(run("snapshots", "--json"), &initial); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{DataDir: filepath.Join(root, "data"), DatabasePath: filepath.Join(root, "data", "test.db"), AdminUsername: "fixture"}
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	secretManager, err := secrets.Open(cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := secretManager.Encrypt(password)
	if err != nil {
		t.Fatal(err)
	}
	store := repositories.NewStore(database)
	repository, err := store.Create(context.Background(), repositories.Params{Name: "fixture", Backend: "b2", RepoURL: repoPath, PasswordCiphertext: encrypted})
	if err != nil {
		t.Fatal(err)
	}
	manager := newSnapshotDeletions()
	s := server{cfg: cfg, repos: store, secrets: secretManager, deletions: manager}
	router := gin.New()
	router.POST("/delete", s.deleteSnapshots)
	router.GET("/jobs/:id", s.snapshotDeletionStatus)
	submit := func(body deleteSnapshotsRequest, status int) snapshotDeletion {
		t.Helper()
		payload, _ := json.Marshal(body)
		request := httptest.NewRequest(http.MethodPost, "/delete", bytes.NewReader(payload))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != status {
			t.Fatalf("status=%d want=%d body=%s", response.Code, status, response.Body.String())
		}
		var result struct {
			Data snapshotDeletion `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result.Data
	}
	all := []string{initial[0].ID, initial[1].ID, initial[2].ID}
	submit(deleteSnapshotsRequest{RepositoryID: repository.ID, SnapshotIDs: all, ClearAll: true, Confirmation: "wrong"}, 400)
	job := submit(deleteSnapshotsRequest{RepositoryID: repository.ID, SnapshotIDs: all[:2]}, 202)
	if result := waitSnapshotDeletion(t, manager, job.ID); result.Status != "succeeded" {
		t.Fatalf("batch failed: %+v", result)
	}
	var remaining []resticSnapshot
	json.Unmarshal(run("snapshots", "--json"), &remaining)
	if len(remaining) != 1 || remaining[0].ID != initial[2].ID {
		t.Fatalf("wrong remaining snapshots: %+v", remaining)
	}
	// A stale clear-all view must be rejected before touching any snapshot.
	stale := submit(deleteSnapshotsRequest{RepositoryID: repository.ID, SnapshotIDs: all, ClearAll: true, Confirmation: "fixture"}, 202)
	if result := waitSnapshotDeletion(t, manager, stale.ID); result.Status != "failed" || result.Forgotten {
		t.Fatalf("stale clear accepted: %+v", result)
	}
	job = submit(deleteSnapshotsRequest{RepositoryID: repository.ID, SnapshotIDs: []string{remaining[0].ID}, ClearAll: true, Confirmation: "fixture"}, 202)
	if result := waitSnapshotDeletion(t, manager, job.ID); result.Status != "succeeded" {
		t.Fatalf("clear failed: %+v", result)
	}
	json.Unmarshal(run("snapshots", "--json"), &remaining)
	if len(remaining) != 0 {
		t.Fatalf("clear left snapshots: %+v", remaining)
	}
	if locks := run("list", "locks"); len(bytes.TrimSpace(locks)) != 0 {
		t.Fatalf("locks remain: %s", locks)
	}
	run("check")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/jobs/"+job.ID, nil))
	if response.Code != 200 || !bytes.Contains(response.Body.Bytes(), []byte(`"succeeded"`)) {
		t.Fatalf("status endpoint: %s", response.Body.String())
	}
	cfg.AuthEnabled = true
	cfg.AdminPassword = "fixture-password-not-used"
	authManager, err := auth.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	protected := NewRouter(cfg, store, notifications.NewStore(database), settings.NewStore(database), secretManager, authManager)
	for _, path := range []string{"/api/v1/snapshots/delete", "/api/v1/snapshot-deletions/" + job.ID} {
		method := "GET"
		if path == "/api/v1/snapshots/delete" {
			method = "POST"
		}
		response := httptest.NewRecorder()
		protected.ServeHTTP(response, httptest.NewRequest(method, path, nil))
		if response.Code != 401 {
			t.Fatalf("unprotected route %s: %d", path, response.Code)
		}
	}
}

package httpapi

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBatchDeletionRunsOneForgetAndOnePruneAfterRequestCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var mu sync.Mutex
	var calls [][]string
	manager := newSnapshotDeletions()
	manager.run = func(ctx context.Context, repo string, env, args []string) error {
		mu.Lock()
		calls = append(calls, append([]string(nil), args...))
		mu.Unlock()
		if args[0] == "forget" {
			close(started)
			<-release
			return ctx.Err()
		}
		return nil
	}
	requestCtx, cancel := context.WithCancel(context.Background())
	job, err := manager.start(requestCtx, "repo-id", "repo-url", nil, []string{strings.Repeat("a", 64), strings.Repeat("b", 64)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	<-started
	cancel()
	if _, err := manager.start(context.Background(), "alias", "repo-url", nil, []string{strings.Repeat("c", 64)}, nil); !errors.Is(err, errSnapshotDeletionBusy) {
		t.Fatalf("expected repository-wide conflict, got %v", err)
	}
	close(release)
	result := waitSnapshotDeletion(t, manager, job.ID)
	if result.Status != "succeeded" || !result.Forgotten || result.Count != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	mu.Lock()
	defer mu.Unlock()
	want := [][]string{{"forget", strings.Repeat("a", 64), strings.Repeat("b", 64)}, {"prune"}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls=%q, want=%q", calls, want)
	}
}

func TestAllSelectedSnapshotsRequireClearConfirmation(t *testing.T) {
	current := []snapshot{{ID: strings.Repeat("a", 64)}, {ID: strings.Repeat("b", 64)}}
	request := deleteSnapshotsRequest{SnapshotIDs: []string{current[0].ID, current[1].ID}}
	if verifySnapshotSelection(request, current) == nil {
		t.Fatal("all-selected bypassed clear confirmation")
	}
	request.ClearAll = true
	if err := verifySnapshotSelection(request, current); err != nil {
		t.Fatal(err)
	}
	request.SnapshotIDs = request.SnapshotIDs[:1]
	if verifySnapshotSelection(request, current) == nil {
		t.Fatal("stale clear accepted")
	}
}

func TestDeletionRejectsEmptyInvalidDuplicateAndUnconfirmedClear(t *testing.T) {
	id := strings.Repeat("a", 64)
	tests := []deleteSnapshotsRequest{
		{},
		{SnapshotIDs: []string{"--all"}},
		{SnapshotIDs: []string{"deadbeef"}},
		{SnapshotIDs: []string{id, id}},
		{SnapshotIDs: []string{id}, ClearAll: true},
		{SnapshotIDs: []string{id}, ClearAll: true, Confirmation: "other"},
	}
	for _, request := range tests {
		if validateSnapshotDeletion(request, "Jellyfin") {
			t.Fatalf("accepted unsafe selection: %+v", request)
		}
	}
	if !validateSnapshotDeletion(deleteSnapshotsRequest{SnapshotIDs: []string{id}, ClearAll: true, Confirmation: "Jellyfin"}, "Jellyfin") {
		t.Fatal("valid clear rejected")
	}
}

func TestPostDeletionVerificationFailureIsNotReportedAsSuccess(t *testing.T) {
	manager := newSnapshotDeletions()
	manager.run = func(context.Context, string, []string, []string) error { return nil }
	job, err := manager.start(context.Background(), "id", "repo", nil, []string{strings.Repeat("a", 64)}, nil, func(context.Context) error { return errors.New("new backup appeared") })
	if err != nil {
		t.Fatal(err)
	}
	result := waitSnapshotDeletion(t, manager, job.ID)
	if result.Status != "failed" || !result.Forgotten {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestPruneFailureDistinguishesAlreadyForgottenSnapshots(t *testing.T) {
	manager := newSnapshotDeletions()
	manager.run = func(ctx context.Context, repo string, env, args []string) error {
		if args[0] == "prune" {
			return errors.New("secret must not be exposed")
		}
		return nil
	}
	job, err := manager.start(context.Background(), "id", "repo", nil, []string{strings.Repeat("a", 64)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := waitSnapshotDeletion(t, manager, job.ID)
	if result.Status != "failed" || !result.Forgotten || strings.Contains(result.Message, "secret") {
		t.Fatalf("unexpected failure result: %+v", result)
	}
}

func TestChangedClearSelectionDoesNotExecuteAnyCommands(t *testing.T) {
	manager := newSnapshotDeletions()
	manager.run = func(context.Context, string, []string, []string) error {
		t.Error("command must not execute")
		return nil
	}
	job, err := manager.start(context.Background(), "id", "repo", nil, []string{strings.Repeat("a", 64)}, func(context.Context) error { return errors.New("snapshot list changed") })
	if err != nil {
		t.Fatal(err)
	}
	result := waitSnapshotDeletion(t, manager, job.ID)
	if result.Status != "failed" || result.Forgotten {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func waitSnapshotDeletion(t *testing.T, manager *snapshotDeletions, id string) snapshotDeletion {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		job, ok := manager.get(id)
		if !ok {
			t.Fatal("job missing")
		}
		if job.Status == "succeeded" || job.Status == "failed" {
			return job
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("job did not finish")
	return snapshotDeletion{}
}

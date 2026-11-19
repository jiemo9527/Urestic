package httpapi

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/urestic/urestic/backend/internal/applog"
	"github.com/urestic/urestic/backend/internal/ids"
)

var errSnapshotDeletionBusy = errors.New("repository deletion already running")

type snapshotDeletion struct {
	ID           string    `json:"id"`
	RepositoryID string    `json:"repositoryId"`
	Status       string    `json:"status"`
	Count        int       `json:"count"`
	Forgotten    bool      `json:"forgotten"`
	Message      string    `json:"message,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

type snapshotDeletions struct {
	mu     sync.Mutex
	jobs   map[string]snapshotDeletion
	active map[string]string
	run    func(context.Context, string, []string, []string) error
}

func newSnapshotDeletions() *snapshotDeletions {
	return &snapshotDeletions{jobs: map[string]snapshotDeletion{}, active: map[string]string{}, run: runSnapshotDeletionCommand}
}

func runSnapshotDeletionCommand(ctx context.Context, repo string, env, args []string) error {
	command := exec.CommandContext(ctx, "restic", append([]string{"-r", repo}, args...)...)
	command.Env = env
	// Give restic time to remove its lock before falling back to a hard kill.
	command.Cancel = func() error {
		if err := command.Process.Signal(os.Interrupt); err != nil {
			return command.Process.Kill()
		}
		return nil
	}
	command.WaitDelay = 30 * time.Second
	// Do not return raw restic output: backend URLs and errors can contain secrets.
	return command.Run()
}

func (m *snapshotDeletions) start(parent context.Context, repositoryID, repo string, env, snapshotIDs []string, prepare func(context.Context) error, verify ...func(context.Context) error) (snapshotDeletion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, busy := m.active[repo]; busy {
		return snapshotDeletion{}, errSnapshotDeletionBusy
	}
	if len(m.jobs) >= 100 {
		for id, job := range m.jobs {
			if job.Status == "succeeded" || job.Status == "failed" {
				delete(m.jobs, id)
			}
		}
		if len(m.jobs) >= 100 {
			return snapshotDeletion{}, errSnapshotDeletionBusy
		}
	}
	id, err := ids.New()
	if err != nil {
		return snapshotDeletion{}, err
	}
	job := snapshotDeletion{ID: id, RepositoryID: repositoryID, Status: "verifying", Count: len(snapshotIDs), CreatedAt: time.Now().UTC()}
	m.jobs[id] = job
	m.active[repo] = id
	snapshotIDs = append([]string(nil), snapshotIDs...)
	env = append([]string(nil), env...)
	// A browser, reverse proxy or CDN disconnect must not cancel repository maintenance.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), time.Hour)
	go func() {
		defer cancel()
		update := func(status string, forgotten bool, message string) {
			m.mu.Lock()
			current := m.jobs[id]
			current.Status, current.Forgotten, current.Message = status, forgotten, message
			m.jobs[id] = current
			m.mu.Unlock()
		}
		defer func() { m.mu.Lock(); delete(m.active, repo); m.mu.Unlock() }()
		if prepare != nil {
			if err := prepare(ctx); err != nil {
				update("failed", false, "快照列表已变化或无法读取，请刷新后重试；未执行删除。")
				return
			}
		}
		update("forgetting", false, "")
		if err := m.run(ctx, repo, env, append([]string{"forget"}, snapshotIDs...)); err != nil {
			update("failed", false, "删除未完成，可能已删除部分快照；请刷新列表并检查仓库锁。")
			return
		}
		update("pruning", true, "")
		if err := m.run(ctx, repo, env, []string{"prune"}); err != nil {
			update("failed", true, "快照已删除，但空间清理失败；请检查仓库锁并补跑 prune。")
			return
		}
		for _, check := range verify {
			if err := check(ctx); err != nil {
				update("failed", true, "已删除所选快照并清理空间，但仓库出现了新快照或无法核验；未确认清空，请刷新列表。")
				return
			}
		}
		update("succeeded", true, "")
		applog.Operation("delete_snapshots", "repository_id", repositoryID, "count", strconv.Itoa(len(snapshotIDs)))
	}()
	return job, nil
}

func (m *snapshotDeletions) get(id string) (snapshotDeletion, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[id]
	return job, ok
}

type deleteSnapshotsRequest struct {
	RepositoryID string   `json:"repositoryId"`
	SnapshotIDs  []string `json:"snapshotIds"`
	ClearAll     bool     `json:"clearAll"`
	Confirmation string   `json:"confirmation"`
}

func validateSnapshotDeletion(request deleteSnapshotsRequest, repositoryName string) bool {
	if len(request.SnapshotIDs) == 0 || len(request.SnapshotIDs) > 10000 || runtime.GOOS == "windows" && len(request.SnapshotIDs) > 400 {
		return false
	}
	if request.ClearAll && request.Confirmation != repositoryName {
		return false
	}
	seen := map[string]bool{}
	for _, id := range request.SnapshotIDs {
		if len(id) != 64 || !validSnapshotID(id) || seen[strings.ToLower(id)] {
			return false
		}
		seen[strings.ToLower(id)] = true
	}
	return true
}

func verifySnapshotSelection(request deleteSnapshotsRequest, current []snapshot) error {
	available := map[string]bool{}
	for _, item := range current {
		available[strings.ToLower(item.ID)] = true
	}
	if len(available) == len(request.SnapshotIDs) && !request.ClearAll {
		return errors.New("all snapshots require clear confirmation")
	}
	if request.ClearAll && len(available) != len(request.SnapshotIDs) {
		return errors.New("snapshot list changed")
	}
	for _, id := range request.SnapshotIDs {
		if !available[strings.ToLower(id)] {
			return errors.New("snapshot missing")
		}
	}
	return nil
}

func (s server) deleteSnapshots(c *gin.Context) {
	var request deleteSnapshotsRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024*1024)
	if err := c.ShouldBindJSON(&request); err != nil {
		fail(c, http.StatusBadRequest, "INVALID_JSON", "删除请求格式不正确。")
		return
	}
	request.RepositoryID = strings.TrimSpace(request.RepositoryID)
	if request.RepositoryID == "" {
		fail(c, http.StatusBadRequest, "REPOSITORY_REQUIRED", "请选择仓库。")
		return
	}
	repository, err := s.repos.GetPrivate(c.Request.Context(), request.RepositoryID)
	if err != nil {
		s.handleRepositoryError(c, err)
		return
	}
	if !validateSnapshotDeletion(request, repository.Name) {
		fail(c, http.StatusBadRequest, "INVALID_SNAPSHOT_SELECTION", "请选择有效快照；清空仓库必须输入完整仓库名确认。")
		return
	}
	repo, env, err := s.resticAccess(c.Request.Context(), repository)
	if err != nil {
		fail(c, http.StatusBadRequest, "SNAPSHOT_DELETE_FAILED", "仓库凭据读取失败。")
		return
	}
	prepare := func(ctx context.Context) error {
		current, err := s.querySnapshots(ctx, repository)
		if err != nil {
			return err
		}
		return verifySnapshotSelection(request, current)
	}
	verify := func(ctx context.Context) error {
		current, err := s.querySnapshots(ctx, repository)
		if err != nil {
			return err
		}
		deleted := map[string]bool{}
		for _, id := range request.SnapshotIDs {
			deleted[strings.ToLower(id)] = true
		}
		for _, item := range current {
			if request.ClearAll || deleted[strings.ToLower(item.ID)] {
				return errors.New("snapshot still present")
			}
		}
		return nil
	}
	job, err := s.deletions.start(c.Request.Context(), repository.ID, repo, env, request.SnapshotIDs, prepare, verify)
	if errors.Is(err, errSnapshotDeletionBusy) {
		fail(c, http.StatusConflict, "SNAPSHOT_DELETE_BUSY", "该仓库已有删除或清理任务，请等待完成。")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "SNAPSHOT_DELETE_FAILED", "无法创建删除任务。")
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": job})
}

func (s server) latestSnapshotDeletion(c *gin.Context) {
	repositoryID := c.Query("repositoryId")
	s.deletions.mu.Lock()
	var latest *snapshotDeletion
	for _, job := range s.deletions.jobs {
		if job.RepositoryID == repositoryID && (latest == nil || job.CreatedAt.After(latest.CreatedAt)) {
			copy := job
			latest = &copy
		}
	}
	s.deletions.mu.Unlock()
	ok(c, gin.H{"job": latest})
}

func (s server) snapshotDeletionStatus(c *gin.Context) {
	job, exists := s.deletions.get(c.Param("id"))
	if !exists {
		fail(c, http.StatusNotFound, "SNAPSHOT_JOB_NOT_FOUND", "任务不存在或服务已重启，请刷新快照并检查仓库状态。")
		return
	}
	ok(c, job)
}

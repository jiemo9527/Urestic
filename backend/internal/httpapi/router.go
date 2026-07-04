package httpapi

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/urestic/urestic/backend/internal/auth"
	"github.com/urestic/urestic/backend/internal/config"
	"github.com/urestic/urestic/backend/internal/notifications"
	"github.com/urestic/urestic/backend/internal/repositories"
	"github.com/urestic/urestic/backend/internal/scriptgen"
	"github.com/urestic/urestic/backend/internal/secrets"
	"github.com/urestic/urestic/backend/internal/settings"
)

type server struct {
	cfg      config.Config
	repos    *repositories.Store
	notify   *notifications.Store
	settings *settings.Store
	secrets  *secrets.Manager
	auth     *auth.Manager
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type repositoryRequest struct {
	Name        string            `json:"name"`
	Backend     string            `json:"backend"`
	RepoURL     string            `json:"repoUrl"`
	Password    string            `json:"password"`
	Variables   map[string]string `json:"variables"`
	Description string            `json:"description"`
}

type notificationRequest struct {
	Name     string            `json:"name"`
	Type     string            `json:"type"`
	Settings map[string]string `json:"settings"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

type defaultVariablesRequest struct {
	Variables map[string]string `json:"variables"`
}

type rcloneRemote struct {
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	Settings     map[string]string `json:"settings"`
	SecretFields []string          `json:"secretFields"`
}

type configExport struct {
	FormatVersion    int                  `json:"formatVersion"`
	ExportedAt       string               `json:"exportedAt"`
	Repositories     []repositoryExport   `json:"repositories"`
	Notifications    []notificationExport `json:"notifications"`
	DefaultVariables map[string]string    `json:"defaultVariables"`
	RcloneConfig     rcloneConfigExport   `json:"rcloneConfig"`
}

type repositoryExport struct {
	Name        string            `json:"name"`
	Backend     string            `json:"backend"`
	RepoURL     string            `json:"repoUrl"`
	Password    string            `json:"password"`
	Variables   map[string]string `json:"variables"`
	Description string            `json:"description"`
}

type notificationExport struct {
	Name     string            `json:"name"`
	Type     string            `json:"type"`
	Settings map[string]string `json:"settings"`
}

type rcloneConfigExport struct {
	Included bool   `json:"included"`
	Path     string `json:"path"`
	Content  string `json:"content"`
}

const maxRcloneConfigBytes = 1024 * 1024

type generateScriptRequest struct {
	RepositoryID string `json:"repositoryId"`
	scriptgen.Request
}

type snapshot struct {
	ID             string   `json:"id"`
	ShortID        string   `json:"shortId,omitempty"`
	Time           string   `json:"time"`
	Tree           string   `json:"tree,omitempty"`
	Paths          []string `json:"paths"`
	Hostname       string   `json:"hostname,omitempty"`
	Username       string   `json:"username,omitempty"`
	UID            int      `json:"uid"`
	GID            int      `json:"gid"`
	Tags           []string `json:"tags,omitempty"`
	ProgramVersion string   `json:"programVersion,omitempty"`
	Parent         string   `json:"parent,omitempty"`
}

type resticSnapshot struct {
	ID             string   `json:"id"`
	ShortID        string   `json:"short_id"`
	Time           string   `json:"time"`
	Tree           string   `json:"tree"`
	Paths          []string `json:"paths"`
	Hostname       string   `json:"hostname"`
	Username       string   `json:"username"`
	UID            int      `json:"uid"`
	GID            int      `json:"gid"`
	Tags           []string `json:"tags"`
	ProgramVersion string   `json:"program_version"`
	Parent         string   `json:"parent"`
}

func NewRouter(cfg config.Config, repoStore *repositories.Store, notificationStore *notifications.Store, settingsStore *settings.Store, secretManager *secrets.Manager, authManager *auth.Manager) *gin.Engine {
	s := server{cfg: cfg, repos: repoStore, notify: notificationStore, settings: settingsStore, secrets: secretManager, auth: authManager}
	r := gin.Default()
	if err := r.SetTrustedProxies(nil); err != nil {
		panic(err)
	}
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173", "http://127.0.0.1:5173"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	r.GET("/healthz", func(c *gin.Context) {
		ok(c, gin.H{"status": "ok"})
	})

	api := r.Group("/api/v1")
	{
		api.POST("/auth/login", s.login)
		api.POST("/auth/logout", s.logout)

		protected := api.Group("")
		protected.Use(s.authRequired())
		protected.GET("/auth/me", s.currentUser)
		protected.GET("/system/info", s.systemInfo)
		protected.GET("/backends", s.backendTemplates)
		protected.GET("/repositories", s.listRepositories)
		protected.POST("/repositories", s.createRepository)
		protected.PUT("/repositories/:id", s.updateRepository)
		protected.DELETE("/repositories/:id", s.deleteRepository)
		protected.POST("/scripts/generate", s.generateScript)
		protected.GET("/snapshots", s.listSnapshots)
		protected.DELETE("/snapshots/:id", s.deleteSnapshot)
		protected.GET("/insights", s.insights)
		protected.GET("/notifications/templates", s.notificationTemplates)
		protected.GET("/notifications", s.listNotifications)
		protected.POST("/notifications", s.createNotification)
		protected.PUT("/notifications/:id", s.updateNotification)
		protected.POST("/notifications/:id/test", s.testNotification)
		protected.DELETE("/notifications/:id", s.deleteNotification)
		protected.GET("/settings/default-variables", s.getDefaultVariables)
		protected.PUT("/settings/default-variables", s.setDefaultVariables)
		protected.GET("/settings/export", s.exportConfig)
		protected.POST("/settings/import", s.importConfig)
		protected.POST("/settings/password", s.changePassword)
		protected.GET("/settings/rclone", s.rcloneStatus)
		protected.POST("/settings/rclone/update", s.updateRclone)
		protected.POST("/settings/rclone/import-config", s.importRcloneConfig)
		protected.GET("/settings/rclone/remotes", s.listRcloneRemotes)
	}

	serveWeb(r, cfg.WebDir)
	return r
}

func (s server) authRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !s.auth.Enabled() {
			c.Next()
			return
		}
		user, ok := s.auth.VerifyToken(extractToken(c))
		if !ok {
			fail(c, http.StatusUnauthorized, "UNAUTHENTICATED", "请先登录管理员账号。")
			c.Abort()
			return
		}
		c.Set("user", user)
		c.Next()
	}
}

func (s server) login(c *gin.Context) {
	var request loginRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		fail(c, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON request body.")
		return
	}
	result, authenticated := s.auth.Login(c.Request.Context(), strings.TrimSpace(request.Username), request.Password)
	if !authenticated {
		fail(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "管理员账号或密码错误。")
		return
	}
	setSessionCookie(c, result.Token, int(s.auth.TTL().Seconds()))
	ok(c, result)
}

func (s server) logout(c *gin.Context) {
	setSessionCookie(c, "", -1)
	ok(c, gin.H{"loggedOut": true})
}

func (s server) currentUser(c *gin.Context) {
	if value, exists := c.Get("user"); exists {
		ok(c, value)
		return
	}
	ok(c, gin.H{"username": s.auth.Username()})
}

func (s server) systemInfo(c *gin.Context) {
	ok(c, gin.H{
		"name":          "Urestic",
		"version":       "0.2.0-dev",
		"mode":          "easy-use-for-restic",
		"language":      s.cfg.DefaultLang,
		"dataDir":       s.cfg.DataDir,
		"databasePath":  s.cfg.DatabasePath,
		"authEnabled":   s.auth.Enabled(),
		"adminUsername": s.auth.Username(),
	})
}

func (s server) rcloneStatus(c *gin.Context) {
	ok(c, s.rcloneStatusData(c.Request.Context()))
}

func (s server) updateRclone(c *gin.Context) {
	commandCtx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, "rclone", "selfupdate")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message == "" {
			message = err.Error()
		}
		fail(c, http.StatusBadRequest, "RCLONE_UPDATE_FAILED", limitText(message, 2000))
		return
	}
	ok(c, gin.H{"updated": true, "output": limitText(strings.TrimSpace(stdout.String()+"\n"+stderr.String()), 4000), "status": s.rcloneStatusData(c.Request.Context())})
}

func (s server) importRcloneConfig(c *gin.Context) {
	source := filepath.Clean(s.cfg.RcloneImportPath)
	target := filepath.Clean(s.cfg.RcloneConfig)
	if source == "" || target == "" || source == target {
		fail(c, http.StatusBadRequest, "INVALID_RCLONE_CONFIG_PATH", "rclone 配置导入路径无效。")
		return
	}
	content := []byte{}
	info, err := os.Stat(source)
	if err == nil && !info.IsDir() {
		content, err = os.ReadFile(source)
		if err != nil {
			fail(c, http.StatusInternalServerError, "RCLONE_IMPORT_READ_FAILED", "rclone.conf 读取失败。")
			return
		}
	} else if err != nil && !os.IsNotExist(err) {
		fail(c, http.StatusInternalServerError, "RCLONE_IMPORT_SOURCE_STAT_FAILED", "rclone.conf 导入源检查失败。")
		return
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		fail(c, http.StatusInternalServerError, "RCLONE_CONFIG_DIR_FAILED", "rclone 配置目录创建失败。")
		return
	}
	if err := os.WriteFile(target, content, 0o600); err != nil {
		fail(c, http.StatusInternalServerError, "RCLONE_IMPORT_WRITE_FAILED", "rclone.conf 写入失败。")
		return
	}
	ok(c, gin.H{"imported": true, "createdEmpty": len(content) == 0, "status": s.rcloneStatusData(c.Request.Context())})
}

func (s server) listRcloneRemotes(c *gin.Context) {
	items, err := readRcloneRemotes(s.cfg.RcloneConfig)
	if err != nil {
		fail(c, http.StatusInternalServerError, "RCLONE_CONFIG_READ_FAILED", "rclone 配置读取失败。")
		return
	}
	ok(c, gin.H{"items": publicRcloneRemotes(items)})
}

func (s server) rcloneStatusData(ctx context.Context) gin.H {
	version, installed, message := rcloneVersion(ctx)
	return gin.H{
		"installed":        installed,
		"version":          version,
		"message":          message,
		"configPath":       s.cfg.RcloneConfig,
		"configExists":     fileExists(s.cfg.RcloneConfig),
		"importPath":       s.cfg.RcloneImportPath,
		"importPathExists": fileExists(s.cfg.RcloneImportPath),
		"cacheDir":         s.cfg.RcloneCacheDir,
	}
}

func rcloneVersion(ctx context.Context) (string, bool, string) {
	commandCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, "rclone", "version")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", false, limitText(message, 1000)
	}
	for _, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line, true, ""
		}
	}
	return "", true, ""
}

func readRcloneRemotes(path string) ([]rcloneRemote, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []rcloneRemote{}, nil
		}
		return nil, err
	}
	items := []rcloneRemote{}
	current := -1
	for _, rawLine := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(rawLine, "\r"))
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			if name == "" {
				current = -1
				continue
			}
			items = append(items, rcloneRemote{Name: name, Settings: map[string]string{}})
			current = len(items) - 1
			continue
		}
		if current < 0 {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		items[current].Settings[key] = strings.TrimSpace(value)
	}
	for index := range items {
		items[index].Type = strings.TrimSpace(items[index].Settings["type"])
		items[index].SecretFields = rcloneSecretFields(items[index].Settings)
	}
	return items, nil
}

func publicRcloneRemotes(items []rcloneRemote) []rcloneRemote {
	result := make([]rcloneRemote, 0, len(items))
	for _, item := range items {
		result = append(result, publicRcloneRemote(item))
	}
	return result
}

func publicRcloneRemote(item rcloneRemote) rcloneRemote {
	public := rcloneRemote{Name: item.Name, Type: item.Type, Settings: map[string]string{}, SecretFields: rcloneSecretFields(item.Settings)}
	for key, value := range item.Settings {
		if key == "type" {
			continue
		}
		if rcloneSecretKey(key) && value != "" {
			public.Settings[key] = "********"
			continue
		}
		public.Settings[key] = value
	}
	return public
}

func rcloneSecretFields(settings map[string]string) []string {
	fields := []string{}
	for key, value := range settings {
		if key != "type" && value != "" && rcloneSecretKey(key) {
			fields = append(fields, key)
		}
	}
	sort.Strings(fields)
	return fields
}

func rcloneSecretKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	secretParts := []string{"pass", "secret", "token", "key", "credential", "client_secret", "refresh_token", "access_token"}
	for _, part := range secretParts {
		if strings.Contains(key, part) {
			return true
		}
	}
	return false
}

func (s server) backendTemplates(c *gin.Context) {
	ok(c, gin.H{"items": []gin.H{
		{"id": "r2", "name": "Cloudflare R2", "repoExample": "s3:<r2_s3_api>/<bucket>/<prefix>", "fields": []string{"r2_s3_api", "r2_account_id", "r2_bucket", "r2_prefix", "r2_access_key_id", "r2_secret_access_key"}},
		{"id": "s3", "name": "S3 Compatible", "repoExample": "s3:<s3_endpoint>/<bucket>/<prefix>", "fields": []string{"s3_endpoint", "s3_region", "s3_bucket", "s3_prefix", "s3_access_key_id", "s3_secret_access_key"}},
		{"id": "b2", "name": "Backblaze B2", "repoExample": "b2:<bucket>:<prefix>", "fields": []string{"b2_bucket", "b2_prefix", "b2_account_id", "b2_account_key"}},
		{"id": "rclone", "name": "rclone", "repoExample": "rclone:<remote>:<path>", "fields": []string{}},
	}})
}

func (s server) listRepositories(c *gin.Context) {
	items, err := s.repos.List(c.Request.Context())
	if err != nil {
		fail(c, http.StatusInternalServerError, "REPOSITORY_LIST_FAILED", "仓库列表读取失败。")
		return
	}
	ok(c, gin.H{"items": items})
}

func (s server) createRepository(c *gin.Context) {
	var request repositoryRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		fail(c, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON request body.")
		return
	}
	params, code, message := s.repositoryParams(request)
	if code != "" {
		fail(c, http.StatusBadRequest, code, message)
		return
	}
	item, err := s.repos.Create(c.Request.Context(), params)
	if err != nil {
		s.handleRepositoryError(c, err)
		return
	}
	okStatus(c, http.StatusCreated, item)
}

func (s server) updateRepository(c *gin.Context) {
	var request repositoryRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		fail(c, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON request body.")
		return
	}
	params, code, message := s.repositoryParams(request)
	if code != "" {
		fail(c, http.StatusBadRequest, code, message)
		return
	}
	item, err := s.repos.Update(c.Request.Context(), c.Param("id"), params)
	if err != nil {
		s.handleRepositoryError(c, err)
		return
	}
	ok(c, item)
}

func (s server) deleteRepository(c *gin.Context) {
	if err := s.repos.Delete(c.Request.Context(), c.Param("id")); err != nil {
		s.handleRepositoryError(c, err)
		return
	}
	ok(c, gin.H{"deleted": true})
}

func (s server) generateScript(c *gin.Context) {
	var request generateScriptRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		fail(c, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON request body.")
		return
	}
	repository, err := s.repos.GetPrivate(c.Request.Context(), strings.TrimSpace(request.RepositoryID))
	if err != nil {
		s.handleRepositoryError(c, err)
		return
	}
	inline := strings.EqualFold(strings.TrimSpace(request.SecretMode), "inline")
	variables := map[string]string{}
	if inline {
		variables, err = s.decryptVariables(repository)
		if err != nil {
			fail(c, http.StatusInternalServerError, "SECRET_DECRYPT_FAILED", err.Error())
			return
		}
		password, err := s.secrets.Decrypt(repository.PasswordCiphertext)
		if err != nil {
			fail(c, http.StatusInternalServerError, "SECRET_DECRYPT_FAILED", "仓库密码解密失败。")
			return
		}
		request.ResticPassword = password
	} else {
		for key, value := range repository.Variables {
			variables[key] = value
		}
	}
	repository.Variables = variables
	if request.Notify.Enabled {
		channels, err := s.scriptNotificationChannels(c.Request.Context(), inline, request.Notify.ChannelIDs)
		if err != nil {
			fail(c, http.StatusInternalServerError, "NOTIFICATION_LIST_FAILED", "通知渠道读取失败。")
			return
		}
		request.Notify.Channels = channels
	}
	result := scriptgen.Generate(repository, request.Request)
	ok(c, result)
}

func (s server) listSnapshots(c *gin.Context) {
	repositoryID := strings.TrimSpace(c.Query("repositoryId"))
	if repositoryID == "" {
		fail(c, http.StatusBadRequest, "REPOSITORY_REQUIRED", "请选择仓库。")
		return
	}
	repository, err := s.repos.GetPrivate(c.Request.Context(), repositoryID)
	if err != nil {
		s.handleRepositoryError(c, err)
		return
	}
	items, err := s.querySnapshots(c.Request.Context(), repository)
	if err != nil {
		fail(c, http.StatusBadRequest, "SNAPSHOT_QUERY_FAILED", err.Error())
		return
	}
	ok(c, gin.H{"items": items})
}

func (s server) deleteSnapshot(c *gin.Context) {
	repositoryID := strings.TrimSpace(c.Query("repositoryId"))
	if repositoryID == "" {
		fail(c, http.StatusBadRequest, "REPOSITORY_REQUIRED", "请选择仓库。")
		return
	}
	snapshotID := strings.TrimSpace(c.Param("id"))
	if !validSnapshotID(snapshotID) {
		fail(c, http.StatusBadRequest, "INVALID_SNAPSHOT_ID", "快照 ID 格式不正确。")
		return
	}
	repository, err := s.repos.GetPrivate(c.Request.Context(), repositoryID)
	if err != nil {
		s.handleRepositoryError(c, err)
		return
	}
	repoURL, env, err := s.resticAccess(c.Request.Context(), repository)
	if err != nil {
		fail(c, http.StatusBadRequest, "SNAPSHOT_DELETE_FAILED", err.Error())
		return
	}
	commandCtx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, "restic", "-r", repoURL, "forget", snapshotID)
	cmd.Env = env
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message == "" {
			message = "restic forget 执行失败"
		}
		fail(c, http.StatusBadRequest, "SNAPSHOT_DELETE_FAILED", message)
		return
	}
	ok(c, gin.H{"deleted": true})
}

func (s server) insights(c *gin.Context) {
	repositories, err := s.repos.List(c.Request.Context())
	if err != nil {
		fail(c, http.StatusInternalServerError, "INSIGHTS_FAILED", "分析数据读取失败。")
		return
	}
	thresholdHours := 48.0
	now := time.Now().UTC()
	hosts := map[string]time.Time{}
	tags := map[string]time.Time{}
	paths := map[string]time.Time{}
	failures := []gin.H{}
	snapshotCount := 0
	for _, item := range repositories {
		repository, err := s.repos.GetPrivate(c.Request.Context(), item.ID)
		if err != nil {
			continue
		}
		snapshots, err := s.querySnapshots(c.Request.Context(), repository)
		if err != nil {
			failures = append(failures, gin.H{"repository": repository.Name, "error": err.Error()})
			continue
		}
		snapshotCount += len(snapshots)
		for _, snapshot := range snapshots {
			parsed, err := time.Parse(time.RFC3339Nano, snapshot.Time)
			if err != nil {
				parsed, err = time.Parse(time.RFC3339, snapshot.Time)
			}
			if err != nil {
				continue
			}
			rememberLatest(hosts, snapshot.Hostname, parsed)
			for _, tag := range snapshot.Tags {
				rememberLatest(tags, tag, parsed)
			}
			for _, path := range snapshot.Paths {
				rememberLatest(paths, path, parsed)
			}
		}
	}
	ok(c, gin.H{
		"repositoryCount": len(repositories),
		"snapshotCount":   snapshotCount,
		"staleHours":      thresholdHours,
		"hosts":           insightItems(hosts, now, thresholdHours),
		"tags":            insightItems(tags, now, thresholdHours),
		"paths":           insightItems(paths, now, thresholdHours),
		"failures":        failures,
		"message":         "已按 snapshots 聚合 host/tag/path；超过 48 小时未更新会标记为过期。",
	})
}

func (s server) notificationTemplates(c *gin.Context) {
	ok(c, gin.H{"items": []gin.H{
		{"type": "telegram", "name": "Telegram Bot", "secretFields": []string{"bot_token"}, "fields": []string{"bot_token", "chat_id"}},
		{"type": "email", "name": "Email SMTP", "secretFields": []string{"password"}, "fields": []string{"host", "port", "username", "password", "from", "to"}},
		{"type": "webhook", "name": "Webhook", "secretFields": []string{"token"}, "fields": []string{"url", "token"}},
	}})
}

func (s server) listNotifications(c *gin.Context) {
	items, err := s.notify.List(c.Request.Context())
	if err != nil {
		fail(c, http.StatusInternalServerError, "NOTIFICATION_LIST_FAILED", "通知渠道读取失败。")
		return
	}
	ok(c, gin.H{"items": items})
}

func (s server) createNotification(c *gin.Context) {
	var request notificationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		fail(c, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON request body.")
		return
	}
	params, code, message := s.notificationParams(request)
	if code != "" {
		fail(c, http.StatusBadRequest, code, message)
		return
	}
	item, err := s.notify.Create(c.Request.Context(), params)
	if err != nil {
		s.handleNotificationError(c, err)
		return
	}
	okStatus(c, http.StatusCreated, item)
}

func (s server) updateNotification(c *gin.Context) {
	var request notificationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		fail(c, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON request body.")
		return
	}
	existing, err := s.notify.GetPrivate(c.Request.Context(), c.Param("id"))
	if err != nil {
		s.handleNotificationError(c, err)
		return
	}
	params, code, message := s.notificationParams(request)
	if code != "" {
		fail(c, http.StatusBadRequest, code, message)
		return
	}
	for _, field := range params.SecretFields {
		if params.Settings[field] == "" || params.Settings[field] == "********" {
			params.Settings[field] = existing.Settings[field]
		}
	}
	item, err := s.notify.Update(c.Request.Context(), c.Param("id"), params)
	if err != nil {
		s.handleNotificationError(c, err)
		return
	}
	ok(c, item)
}

func (s server) testNotification(c *gin.Context) {
	item, err := s.notify.GetPrivate(c.Request.Context(), c.Param("id"))
	if err != nil {
		s.handleNotificationError(c, err)
		return
	}
	settings, err := s.decryptNotificationSettings(item)
	if err != nil {
		fail(c, http.StatusInternalServerError, "NOTIFICATION_TEST_FAILED", err.Error())
		return
	}
	if err := testNotificationChannel(c.Request.Context(), item.Type, settings); err != nil {
		fail(c, http.StatusBadRequest, "NOTIFICATION_TEST_FAILED", err.Error())
		return
	}
	ok(c, gin.H{"tested": true})
}

func (s server) deleteNotification(c *gin.Context) {
	if err := s.notify.Delete(c.Request.Context(), c.Param("id")); err != nil {
		s.handleNotificationError(c, err)
		return
	}
	ok(c, gin.H{"deleted": true})
}

func (s server) getDefaultVariables(c *gin.Context) {
	variables, err := s.defaultVariables(c.Request.Context(), true)
	if err != nil {
		fail(c, http.StatusInternalServerError, "SETTINGS_READ_FAILED", "默认变量读取失败。")
		return
	}
	ok(c, gin.H{"variables": variables})
}

func (s server) setDefaultVariables(c *gin.Context) {
	var request defaultVariablesRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		fail(c, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON request body.")
		return
	}
	values := normalizeVariables(request.Variables)
	for key, value := range values {
		if !defaultVariableAllowed(key) {
			fail(c, http.StatusBadRequest, "INVALID_SETTING_KEY", "不支持的默认变量: "+key)
			return
		}
		if value == "********" || value == "" {
			continue
		}
		secret := defaultVariableSecret(key)
		stored := value
		if secret {
			encrypted, err := s.secrets.Encrypt(value)
			if err != nil {
				fail(c, http.StatusInternalServerError, "SECRET_ENCRYPT_FAILED", "默认变量加密失败。")
				return
			}
			stored = encrypted
		}
		if err := s.settings.Upsert(c.Request.Context(), key, stored, secret); err != nil {
			fail(c, http.StatusInternalServerError, "SETTINGS_SAVE_FAILED", "默认变量保存失败。")
			return
		}
	}
	s.getDefaultVariables(c)
}

func (s server) exportConfig(c *gin.Context) {
	items, err := s.repos.List(c.Request.Context())
	if err != nil {
		fail(c, http.StatusInternalServerError, "CONFIG_EXPORT_FAILED", "仓库配置读取失败。")
		return
	}
	repositoriesExport := []repositoryExport{}
	for _, item := range items {
		repository, err := s.repos.GetPrivate(c.Request.Context(), item.ID)
		if err != nil {
			fail(c, http.StatusInternalServerError, "CONFIG_EXPORT_FAILED", "仓库配置读取失败。")
			return
		}
		password, err := s.secrets.Decrypt(repository.PasswordCiphertext)
		if err != nil {
			fail(c, http.StatusInternalServerError, "CONFIG_EXPORT_FAILED", "仓库密码解密失败。")
			return
		}
		variables, err := s.decryptVariables(repository)
		if err != nil {
			fail(c, http.StatusInternalServerError, "CONFIG_EXPORT_FAILED", err.Error())
			return
		}
		repositoriesExport = append(repositoriesExport, repositoryExport{
			Name:        repository.Name,
			Backend:     repository.Backend,
			RepoURL:     repository.RepoURL,
			Password:    password,
			Variables:   variables,
			Description: repository.Description,
		})
	}

	notificationItems, err := s.notify.ListPrivate(c.Request.Context())
	if err != nil {
		fail(c, http.StatusInternalServerError, "CONFIG_EXPORT_FAILED", "通知配置读取失败。")
		return
	}
	notificationsExport := []notificationExport{}
	for _, item := range notificationItems {
		settings, err := s.decryptNotificationSettings(item)
		if err != nil {
			fail(c, http.StatusInternalServerError, "CONFIG_EXPORT_FAILED", err.Error())
			return
		}
		notificationsExport = append(notificationsExport, notificationExport{
			Name:     item.Name,
			Type:     item.Type,
			Settings: settings,
		})
	}
	defaultVariables, err := s.defaultVariables(c.Request.Context(), false)
	if err != nil {
		fail(c, http.StatusInternalServerError, "CONFIG_EXPORT_FAILED", "默认变量读取失败。")
		return
	}
	rcloneConfig, err := exportRcloneConfig(s.cfg.RcloneConfig)
	if err != nil {
		fail(c, http.StatusInternalServerError, "CONFIG_EXPORT_FAILED", "rclone.conf 读取失败。")
		return
	}

	ok(c, configExport{
		FormatVersion:    2,
		ExportedAt:       time.Now().UTC().Format(time.RFC3339Nano),
		Repositories:     repositoriesExport,
		Notifications:    notificationsExport,
		DefaultVariables: defaultVariables,
		RcloneConfig:     rcloneConfig,
	})
}

func (s server) importConfig(c *gin.Context) {
	var request configExport
	if err := c.ShouldBindJSON(&request); err != nil {
		fail(c, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON request body.")
		return
	}
	if request.FormatVersion != 1 && request.FormatVersion != 2 {
		fail(c, http.StatusBadRequest, "UNSUPPORTED_CONFIG_VERSION", "只支持导入 formatVersion=1 或 formatVersion=2 的配置文件。")
		return
	}
	restoreMode := request.FormatVersion >= 2
	repositoriesCreated := 0
	repositoriesUpdated := 0
	repositoriesDeleted := 0
	repositoriesSkipped := 0
	notificationsCreated := 0
	notificationsUpdated := 0
	notificationsDeleted := 0
	notificationsSkipped := 0
	defaultVariablesRestored := 0
	defaultVariablesDeleted := 0
	rcloneConfigRestored := false
	rcloneConfigRemoved := false

	existingRepositories := map[string]repositories.Repository{}
	if restoreMode {
		existing, err := s.repos.List(c.Request.Context())
		if err != nil {
			fail(c, http.StatusInternalServerError, "CONFIG_IMPORT_FAILED", "仓库配置读取失败。")
			return
		}
		for _, item := range existing {
			existingRepositories[item.Name] = item
		}
	}
	desiredRepositories := map[string]struct{}{}

	for _, item := range request.Repositories {
		params, code, message := s.repositoryParams(repositoryRequest{
			Name:        item.Name,
			Backend:     item.Backend,
			RepoURL:     item.RepoURL,
			Password:    item.Password,
			Variables:   item.Variables,
			Description: item.Description,
		})
		if code != "" {
			fail(c, http.StatusBadRequest, code, message)
			return
		}
		desiredRepositories[params.Name] = struct{}{}
		if restoreMode {
			if existing, ok := existingRepositories[params.Name]; ok {
				if _, err := s.repos.Update(c.Request.Context(), existing.ID, params); err != nil {
					s.handleRepositoryError(c, err)
					return
				}
				repositoriesUpdated++
				continue
			}
		}
		if _, err := s.repos.Create(c.Request.Context(), params); err != nil {
			if errors.Is(err, repositories.ErrDuplicateName) {
				repositoriesSkipped++
				continue
			}
			s.handleRepositoryError(c, err)
			return
		}
		repositoriesCreated++
	}
	if restoreMode {
		for name, item := range existingRepositories {
			if _, ok := desiredRepositories[name]; ok {
				continue
			}
			if err := s.repos.Delete(c.Request.Context(), item.ID); err != nil && !errors.Is(err, repositories.ErrNotFound) {
				s.handleRepositoryError(c, err)
				return
			}
			repositoriesDeleted++
		}
	}

	existingNotifications := map[string]notifications.Channel{}
	if restoreMode {
		existing, err := s.notify.List(c.Request.Context())
		if err != nil {
			fail(c, http.StatusInternalServerError, "CONFIG_IMPORT_FAILED", "通知配置读取失败。")
			return
		}
		for _, item := range existing {
			existingNotifications[item.Name] = item
		}
	}
	desiredNotifications := map[string]struct{}{}

	for _, item := range request.Notifications {
		params, code, message := s.notificationParams(notificationRequest{
			Name:     item.Name,
			Type:     item.Type,
			Settings: item.Settings,
		})
		if code != "" {
			fail(c, http.StatusBadRequest, code, message)
			return
		}
		desiredNotifications[params.Name] = struct{}{}
		if restoreMode {
			if existing, ok := existingNotifications[params.Name]; ok {
				if _, err := s.notify.Update(c.Request.Context(), existing.ID, params); err != nil {
					s.handleNotificationError(c, err)
					return
				}
				notificationsUpdated++
				continue
			}
		}
		if _, err := s.notify.Create(c.Request.Context(), params); err != nil {
			if errors.Is(err, notifications.ErrDuplicateName) {
				notificationsSkipped++
				continue
			}
			s.handleNotificationError(c, err)
			return
		}
		notificationsCreated++
	}
	if restoreMode {
		for name, item := range existingNotifications {
			if _, ok := desiredNotifications[name]; ok {
				continue
			}
			if err := s.notify.Delete(c.Request.Context(), item.ID); err != nil && !errors.Is(err, notifications.ErrNotFound) {
				s.handleNotificationError(c, err)
				return
			}
			notificationsDeleted++
		}

		var err error
		defaultVariablesRestored, defaultVariablesDeleted, err = s.restoreDefaultVariables(c.Request.Context(), request.DefaultVariables)
		if err != nil {
			fail(c, http.StatusBadRequest, "CONFIG_IMPORT_FAILED", err.Error())
			return
		}
		rcloneConfigRestored, rcloneConfigRemoved, err = restoreRcloneConfig(s.cfg.RcloneConfig, request.RcloneConfig)
		if err != nil {
			fail(c, http.StatusBadRequest, "CONFIG_IMPORT_FAILED", err.Error())
			return
		}
	}

	ok(c, gin.H{
		"repositoriesCreated":      repositoriesCreated,
		"repositoriesUpdated":      repositoriesUpdated,
		"repositoriesDeleted":      repositoriesDeleted,
		"repositoriesSkipped":      repositoriesSkipped,
		"notificationsCreated":     notificationsCreated,
		"notificationsUpdated":     notificationsUpdated,
		"notificationsDeleted":     notificationsDeleted,
		"notificationsSkipped":     notificationsSkipped,
		"defaultVariablesRestored": defaultVariablesRestored,
		"defaultVariablesDeleted":  defaultVariablesDeleted,
		"rcloneConfigRestored":     rcloneConfigRestored,
		"rcloneConfigRemoved":      rcloneConfigRemoved,
	})
}

func exportRcloneConfig(path string) (rcloneConfigExport, error) {
	result := rcloneConfigExport{Path: path}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return result, err
	}
	if info.IsDir() {
		return result, nil
	}
	if info.Size() > maxRcloneConfigBytes {
		return result, fmt.Errorf("rclone.conf 超过 %d bytes，未导出", maxRcloneConfigBytes)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return result, err
	}
	result.Included = true
	result.Content = string(content)
	return result, nil
}

func restoreRcloneConfig(path string, item rcloneConfigExport) (bool, bool, error) {
	if item.Included {
		if len([]byte(item.Content)) > maxRcloneConfigBytes {
			return false, false, fmt.Errorf("rclone.conf 超过 %d bytes，未导入", maxRcloneConfigBytes)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return false, false, errors.New("rclone 配置目录创建失败")
		}
		if err := os.WriteFile(path, []byte(item.Content), 0o600); err != nil {
			return false, false, errors.New("rclone.conf 写入失败")
		}
		return true, false, nil
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return false, false, nil
		}
		return false, false, errors.New("rclone.conf 删除失败")
	}
	return false, true, nil
}

func (s server) changePassword(c *gin.Context) {
	var request changePasswordRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		fail(c, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON request body.")
		return
	}
	if err := s.auth.ChangePassword(request.CurrentPassword, request.NewPassword); err != nil {
		fail(c, http.StatusBadRequest, "PASSWORD_CHANGE_FAILED", err.Error())
		return
	}
	setSessionCookie(c, "", -1)
	ok(c, gin.H{"changed": true})
}

func (s server) repositoryParams(request repositoryRequest) (repositories.Params, string, string) {
	backend := strings.TrimSpace(request.Backend)
	variables := normalizeVariables(request.Variables)
	secretFields := repositorySecretFields(backend)
	params := repositories.Params{
		Name:         strings.TrimSpace(request.Name),
		Backend:      backend,
		RepoURL:      strings.TrimSpace(request.RepoURL),
		Variables:    variables,
		SecretFields: secretFields,
		Description:  strings.TrimSpace(request.Description),
	}
	if params.Name == "" || len(params.Name) > 80 {
		return params, "INVALID_REPOSITORY_NAME", "仓库名称不能为空且最多 80 个字符。"
	}
	if !backendAllowed(params.Backend) {
		return params, "INVALID_REPOSITORY_BACKEND", "暂只支持 R2、B2、S3 和 rclone。"
	}
	if params.RepoURL == "" || len(params.RepoURL) > 1024 {
		return params, "INVALID_REPOSITORY_URL", "Repository URL 不能为空。"
	}
	if request.Password == "" {
		return params, "INVALID_REPOSITORY_PASSWORD", "restic 仓库密码不能为空。"
	}
	password, err := s.secrets.Encrypt(request.Password)
	if err != nil {
		return params, "SECRET_ENCRYPT_FAILED", "仓库密码加密失败。"
	}
	params.PasswordCiphertext = password
	for _, field := range secretFields {
		value := variables[field]
		if value == "" || value == "********" {
			continue
		}
		encrypted, err := s.secrets.Encrypt(value)
		if err != nil {
			return params, "SECRET_ENCRYPT_FAILED", "后端 secret 加密失败。"
		}
		params.Variables[field] = encrypted
	}
	return params, "", ""
}

func (s server) notificationParams(request notificationRequest) (notifications.Params, string, string) {
	notificationType := strings.TrimSpace(request.Type)
	secretFields := notificationSecretFields(notificationType)
	if secretFields == nil {
		return notifications.Params{}, "INVALID_NOTIFICATION_TYPE", "暂只支持 Telegram、Email 和 Webhook。"
	}
	settings := normalizeVariables(request.Settings)
	for _, field := range secretFields {
		value := settings[field]
		if value == "" || value == "********" {
			continue
		}
		encrypted, err := s.secrets.Encrypt(value)
		if err != nil {
			return notifications.Params{}, "SECRET_ENCRYPT_FAILED", "通知 secret 加密失败。"
		}
		settings[field] = encrypted
	}
	params := notifications.Params{
		Name:         strings.TrimSpace(request.Name),
		Type:         notificationType,
		Settings:     settings,
		SecretFields: secretFields,
	}
	if params.Name == "" || len(params.Name) > 80 {
		return params, "INVALID_NOTIFICATION_NAME", "通知名称不能为空且最多 80 个字符。"
	}
	return params, "", ""
}

func (s server) querySnapshots(ctx context.Context, repository repositories.Repository) ([]snapshot, error) {
	repoURL, env, err := s.resticAccess(ctx, repository)
	if err != nil {
		return nil, err
	}
	commandCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, "restic", "-r", repoURL, "snapshots", "--json")
	cmd.Env = env
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = "restic snapshots 执行失败"
		}
		return nil, errors.New(message)
	}
	items := []snapshot{}
	if strings.TrimSpace(stdout.String()) == "" {
		return items, nil
	}
	rawItems := []resticSnapshot{}
	if err := json.Unmarshal(stdout.Bytes(), &rawItems); err != nil {
		return nil, errors.New("restic snapshots JSON 解析失败")
	}
	for _, item := range rawItems {
		items = append(items, item.public())
	}
	return items, nil
}

func (item resticSnapshot) public() snapshot {
	shortID := strings.TrimSpace(item.ShortID)
	if shortID == "" && len(item.ID) > 12 {
		shortID = item.ID[:12]
	}
	return snapshot{
		ID:             item.ID,
		ShortID:        shortID,
		Time:           item.Time,
		Tree:           item.Tree,
		Paths:          item.Paths,
		Hostname:       item.Hostname,
		Username:       item.Username,
		UID:            item.UID,
		GID:            item.GID,
		Tags:           item.Tags,
		ProgramVersion: item.ProgramVersion,
		Parent:         item.Parent,
	}
}

func (s server) resticAccess(ctx context.Context, repository repositories.Repository) (string, []string, error) {
	password, err := s.secrets.Decrypt(repository.PasswordCiphertext)
	if err != nil {
		return "", nil, errors.New("仓库密码解密失败")
	}
	variables, err := s.decryptVariables(repository)
	if err != nil {
		return "", nil, err
	}
	return scriptgen.RepositoryURL(repository, variables), resticEnv(s.cfg, password, variables), nil
}

func (s server) decryptVariables(repository repositories.Repository) (map[string]string, error) {
	variables := map[string]string{}
	secretSet := map[string]struct{}{}
	for _, field := range repository.SecretFields {
		secretSet[field] = struct{}{}
	}
	for key, value := range repository.Variables {
		if _, secret := secretSet[key]; secret && value != "" {
			decrypted, err := s.secrets.Decrypt(value)
			if err != nil {
				return nil, errors.New("仓库 secret 解密失败")
			}
			variables[key] = decrypted
			continue
		}
		variables[key] = value
	}
	return variables, nil
}

func (s server) scriptNotificationChannels(ctx context.Context, inline bool, selectedIDs []string) ([]scriptgen.NotifyChannel, error) {
	items, err := s.notify.List(ctx)
	if inline {
		items, err = s.notify.ListPrivate(ctx)
	}
	if err != nil {
		return nil, err
	}
	selected := map[string]struct{}{}
	for _, id := range selectedIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			selected[id] = struct{}{}
		}
	}
	channels := []scriptgen.NotifyChannel{}
	if len(selected) == 0 {
		return channels, nil
	}
	for _, item := range items {
		if _, ok := selected[item.ID]; !ok {
			continue
		}
		settings := map[string]string{}
		for key, value := range item.Settings {
			settings[key] = value
		}
		if inline {
			secretSet := map[string]struct{}{}
			for _, field := range item.SecretFields {
				secretSet[field] = struct{}{}
			}
			for key, value := range settings {
				if _, secret := secretSet[key]; secret && value != "" {
					decrypted, err := s.secrets.Decrypt(value)
					if err != nil {
						return nil, errors.New("通知 secret 解密失败")
					}
					settings[key] = decrypted
				}
			}
		}
		channels = append(channels, scriptgen.NotifyChannel{Name: item.Name, Type: item.Type, Settings: settings})
	}
	return channels, nil
}

func (s server) decryptNotificationSettings(item notifications.Channel) (map[string]string, error) {
	settings := map[string]string{}
	secretSet := map[string]struct{}{}
	for _, field := range item.SecretFields {
		secretSet[field] = struct{}{}
	}
	for key, value := range item.Settings {
		if _, secret := secretSet[key]; secret && value != "" {
			decrypted, err := s.secrets.Decrypt(value)
			if err != nil {
				return nil, errors.New("通知 secret 解密失败")
			}
			settings[key] = decrypted
			continue
		}
		settings[key] = value
	}
	return settings, nil
}

func testNotificationChannel(ctx context.Context, notificationType string, settings map[string]string) error {
	title := "Urestic notification test"
	details := "This is a test message from Urestic WebUI."
	switch notificationType {
	case "telegram":
		return testTelegram(ctx, settings, title, details)
	case "email":
		return testEmail(settings, title, details)
	case "webhook":
		return testWebhook(ctx, settings, title, details)
	default:
		return errors.New("不支持的通知类型")
	}
}

func testTelegram(ctx context.Context, settings map[string]string, title string, details string) error {
	token := strings.TrimSpace(settings["bot_token"])
	chatID := strings.TrimSpace(settings["chat_id"])
	if token == "" || chatID == "" || token == "********" {
		return errors.New("Telegram 缺少 bot_token 或 chat_id")
	}
	payload, _ := json.Marshal(gin.H{"chat_id": chatID, "text": title + "\n" + details})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+token+"/sendMessage", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	client := http.Client{Timeout: 20 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Telegram 返回状态码 %d", response.StatusCode)
	}
	return nil
}

func testWebhook(ctx context.Context, settings map[string]string, title string, details string) error {
	url := strings.TrimSpace(settings["url"])
	if url == "" {
		return errors.New("Webhook 缺少 url")
	}
	payload, _ := json.Marshal(gin.H{"event": "notification_test", "title": title, "details": details})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	if token := strings.TrimSpace(settings["token"]); token != "" && token != "********" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	client := http.Client{Timeout: 20 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Webhook 返回状态码 %d", response.StatusCode)
	}
	return nil
}

func testEmail(settings map[string]string, title string, details string) error {
	host := strings.TrimSpace(settings["host"])
	if host == "" {
		return errors.New("Email 缺少 host")
	}
	port := strings.TrimSpace(settings["port"])
	if port == "" {
		port = "587"
	}
	if _, err := strconv.Atoi(port); err != nil {
		return errors.New("Email port 格式不正确")
	}
	from := strings.TrimSpace(settings["from"])
	if from == "" {
		from = strings.TrimSpace(settings["username"])
	}
	to := strings.TrimSpace(settings["to"])
	if from == "" || to == "" {
		return errors.New("Email 缺少 from 或 to")
	}
	address := net.JoinHostPort(host, port)
	var client *smtp.Client
	var err error
	if port == "465" {
		connection, err := tls.DialWithDialer(&net.Dialer{Timeout: 20 * time.Second}, "tcp", address, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err != nil {
			return err
		}
		client, err = smtp.NewClient(connection, host)
	} else {
		client, err = smtp.Dial(address)
	}
	if err != nil {
		return err
	}
	defer client.Close()
	if port != "465" {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
				return err
			}
		}
	}
	username := strings.TrimSpace(settings["username"])
	password := strings.TrimSpace(settings["password"])
	if username != "" && password != "" && password != "********" {
		if err := client.Auth(smtp.PlainAuth("", username, password, host)); err != nil {
			return err
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	message := "Subject: " + title + "\r\nFrom: " + from + "\r\nTo: " + to + "\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n" + details + "\r\n"
	if _, err := writer.Write([]byte(message)); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func resticEnv(cfg config.Config, password string, variables map[string]string) []string {
	env := append(os.Environ(), "RESTIC_PASSWORD="+password, "RCLONE_CONFIG="+cfg.RcloneConfig, "RCLONE_CACHE_DIR="+cfg.RcloneCacheDir)
	if variables["r2_access_key_id"] != "" {
		env = append(env, "AWS_ACCESS_KEY_ID="+variables["r2_access_key_id"])
	}
	if variables["r2_secret_access_key"] != "" {
		env = append(env, "AWS_SECRET_ACCESS_KEY="+variables["r2_secret_access_key"])
	}
	if variables["s3_access_key_id"] != "" {
		env = append(env, "AWS_ACCESS_KEY_ID="+variables["s3_access_key_id"])
	}
	if variables["s3_secret_access_key"] != "" {
		env = append(env, "AWS_SECRET_ACCESS_KEY="+variables["s3_secret_access_key"])
	}
	if variables["s3_region"] != "" {
		env = append(env, "AWS_DEFAULT_REGION="+variables["s3_region"])
	}
	if variables["b2_account_id"] != "" {
		env = append(env, "B2_ACCOUNT_ID="+variables["b2_account_id"])
	}
	if variables["b2_account_key"] != "" {
		env = append(env, "B2_ACCOUNT_KEY="+variables["b2_account_key"])
	}
	if variables["s3_region"] == "" {
		env = append(env, "AWS_DEFAULT_REGION=auto")
	}
	return env
}

func (s server) defaultVariables(ctx context.Context, redacted bool) (map[string]string, error) {
	items, err := s.settings.List(ctx)
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	for _, item := range items {
		if item.Secret {
			if redacted {
				values[item.Key] = "********"
				continue
			}
			decrypted, err := s.secrets.Decrypt(item.Value)
			if err != nil {
				return nil, err
			}
			values[item.Key] = decrypted
			continue
		}
		values[item.Key] = item.Value
	}
	return values, nil
}

func (s server) restoreDefaultVariables(ctx context.Context, values map[string]string) (int, int, error) {
	normalized := normalizeVariables(values)
	for key := range normalized {
		if !defaultVariableAllowed(key) {
			return 0, 0, errors.New("不支持的默认变量: " + key)
		}
	}
	currentItems, err := s.settings.List(ctx)
	if err != nil {
		return 0, 0, errors.New("默认变量读取失败")
	}
	current := map[string]struct{}{}
	for _, item := range currentItems {
		if defaultVariableAllowed(item.Key) {
			current[item.Key] = struct{}{}
		}
	}
	restored := 0
	for key, value := range normalized {
		if value == "" || value == "********" {
			continue
		}
		secret := defaultVariableSecret(key)
		stored := value
		if secret {
			encrypted, err := s.secrets.Encrypt(value)
			if err != nil {
				return restored, 0, errors.New("默认变量加密失败")
			}
			stored = encrypted
		}
		if err := s.settings.Upsert(ctx, key, stored, secret); err != nil {
			return restored, 0, errors.New("默认变量保存失败")
		}
		delete(current, key)
		restored++
	}
	deleted := 0
	for key := range current {
		if err := s.settings.Delete(ctx, key); err != nil {
			return restored, deleted, errors.New("默认变量删除失败")
		}
		deleted++
	}
	return restored, deleted, nil
}

func rememberLatest(items map[string]time.Time, key string, value time.Time) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	if existing, ok := items[key]; !ok || value.After(existing) {
		items[key] = value
	}
}

func insightItems(items map[string]time.Time, now time.Time, staleHours float64) []gin.H {
	result := []gin.H{}
	for key, last := range items {
		ageHours := now.Sub(last).Hours()
		result = append(result, gin.H{
			"name":       key,
			"lastBackup": last,
			"ageHours":   ageHours,
			"stale":      ageHours > staleHours,
		})
	}
	return result
}

func handleError(c *gin.Context, status int, code string, message string) {
	fail(c, status, code, message)
}

func validSnapshotID(value string) bool {
	if len(value) < 8 || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F') {
			continue
		}
		return false
	}
	return true
}

func (s server) handleRepositoryError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, repositories.ErrNotFound):
		handleError(c, http.StatusNotFound, "REPOSITORY_NOT_FOUND", "仓库不存在。")
	case errors.Is(err, repositories.ErrDuplicateName):
		handleError(c, http.StatusConflict, "REPOSITORY_NAME_EXISTS", "仓库名称已存在。")
	default:
		handleError(c, http.StatusInternalServerError, "REPOSITORY_OPERATION_FAILED", "仓库操作失败。")
	}
}

func (s server) handleNotificationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, notifications.ErrNotFound):
		handleError(c, http.StatusNotFound, "NOTIFICATION_NOT_FOUND", "通知渠道不存在。")
	case errors.Is(err, notifications.ErrDuplicateName):
		handleError(c, http.StatusConflict, "NOTIFICATION_NAME_EXISTS", "通知渠道名称已存在。")
	default:
		handleError(c, http.StatusInternalServerError, "NOTIFICATION_OPERATION_FAILED", "通知渠道操作失败。")
	}
}

func backendAllowed(backend string) bool {
	switch backend {
	case "r2", "s3", "b2", "rclone":
		return true
	default:
		return false
	}
}

func repositorySecretFields(backend string) []string {
	switch backend {
	case "r2":
		return []string{"r2_secret_access_key"}
	case "s3":
		return []string{"s3_secret_access_key"}
	case "b2":
		return []string{"b2_account_key"}
	default:
		return []string{}
	}
}

func defaultVariableAllowed(key string) bool {
	switch key {
	case "r2_s3_api", "r2_account_id", "r2_bucket", "r2_prefix", "r2_access_key_id", "r2_secret_access_key",
		"s3_endpoint", "s3_region", "s3_bucket", "s3_prefix", "s3_access_key_id", "s3_secret_access_key",
		"b2_bucket", "b2_prefix", "b2_account_id", "b2_account_key":
		return true
	default:
		return false
	}
}

func defaultVariableSecret(key string) bool {
	switch key {
	case "r2_secret_access_key", "s3_secret_access_key", "b2_account_key":
		return true
	default:
		return false
	}
}

func notificationSecretFields(notificationType string) []string {
	switch notificationType {
	case "telegram":
		return []string{"bot_token"}
	case "email":
		return []string{"password"}
	case "webhook":
		return []string{"token"}
	default:
		return nil
	}
}

func normalizeVariables(values map[string]string) map[string]string {
	normalized := map[string]string{}
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		normalized[key] = strings.TrimSpace(value)
	}
	return normalized
}

func extractToken(c *gin.Context) string {
	authorization := strings.TrimSpace(c.GetHeader("Authorization"))
	if strings.HasPrefix(strings.ToLower(authorization), "bearer ") {
		return strings.TrimSpace(authorization[7:])
	}
	token, err := c.Cookie(auth.CookieName)
	if err != nil {
		return ""
	}
	return token
}

func setSessionCookie(c *gin.Context, token string, maxAge int) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(auth.CookieName, token, maxAge, "/", "", c.Request.TLS != nil, true)
}

func serveWeb(r *gin.Engine, webDir string) {
	indexPath := filepath.Join(webDir, "index.html")
	webAvailable := webDir != "" && fileExists(indexPath)
	if webAvailable {
		r.Static("/assets", filepath.Join(webDir, "assets"))
		r.StaticFile("/favicon.ico", filepath.Join(webDir, "favicon.ico"))
		r.StaticFile("/", indexPath)
	}
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			fail(c, http.StatusNotFound, "NOT_FOUND", "API route not found.")
			return
		}
		if webAvailable {
			c.File(indexPath)
			return
		}
		fail(c, http.StatusNotFound, "NOT_FOUND", "Route not found.")
	})
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func limitText(value string, max int) string {
	value = strings.TrimSpace(value)
	items := []rune(value)
	if max <= 0 || len(items) <= max {
		return value
	}
	return string(items[:max]) + "..."
}

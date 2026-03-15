package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/freema/codeforge/internal/config"
	"github.com/freema/codeforge/internal/redisclient"
	"github.com/freema/codeforge/internal/task"
)

// WebhookReceiverHandler handles incoming webhooks from GitHub/GitLab.
type WebhookReceiverHandler struct {
	taskService *task.Service
	redis       *redisclient.Client
	cfg         config.CodeReviewConfig
}

// NewWebhookReceiverHandler creates a new webhook receiver handler.
func NewWebhookReceiverHandler(taskService *task.Service, redis *redisclient.Client, cfg config.CodeReviewConfig) *WebhookReceiverHandler {
	return &WebhookReceiverHandler{
		taskService: taskService,
		redis:       redis,
		cfg:         cfg,
	}
}

// --- GitHub webhook types ---

type githubPREvent struct {
	Action      string `json:"action"`
	Number      int    `json:"number"`
	PullRequest struct {
		Number  int    `json:"number"`
		Draft   bool   `json:"draft"`
		HTMLURL string `json:"html_url"`
		Head    struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
	} `json:"pull_request"`
	Repository struct {
		FullName string `json:"full_name"`
		CloneURL string `json:"clone_url"`
		HTMLURL  string `json:"html_url"`
	} `json:"repository"`
}

// --- GitLab webhook types ---

type gitlabMREvent struct {
	ObjectKind       string `json:"object_kind"`
	ObjectAttributes struct {
		IID            int    `json:"iid"`
		Action         string `json:"action"`
		SourceBranch   string `json:"source_branch"`
		TargetBranch   string `json:"target_branch"`
		WorkInProgress bool   `json:"work_in_progress"`
		Draft          bool   `json:"draft"`
		URL            string `json:"url"`
		Title          string `json:"title"`
		LastCommit     struct {
			ID string `json:"id"`
		} `json:"last_commit"`
	} `json:"object_attributes"`
	Project struct {
		PathWithNamespace string `json:"path_with_namespace"`
		HTTPURLToRepo     string `json:"http_url_to_repo"`
	} `json:"project"`
}

// GitHubWebhook handles POST /api/v1/webhooks/github.
func (h *WebhookReceiverHandler) GitHubWebhook(w http.ResponseWriter, r *http.Request) {
	log := slog.With("handler", "github_webhook")

	if h.cfg.WebhookSecrets.GitHub == "" {
		writeError(w, http.StatusServiceUnavailable, "GitHub webhook secret not configured")
		return
	}

	// Read body for signature verification
	body, err := io.ReadAll(io.LimitReader(r.Body, 5<<20)) // 5MB limit
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	// Verify HMAC-SHA256 signature
	sig := r.Header.Get("X-Hub-Signature-256")
	if !verifyGitHubSignature(body, sig, h.cfg.WebhookSecrets.GitHub) {
		log.Warn("github webhook: invalid signature")
		writeError(w, http.StatusUnauthorized, "invalid signature")
		return
	}

	// Only handle pull_request events
	eventType := r.Header.Get("X-GitHub-Event")
	if eventType != "pull_request" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "reason": "not a pull_request event"})
		return
	}

	var event githubPREvent
	if err := json.Unmarshal(body, &event); err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse webhook payload")
		return
	}

	// Only handle relevant actions
	switch event.Action {
	case "opened", "synchronize", "reopened":
		// proceed
	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "reason": fmt.Sprintf("action %s not handled", event.Action)})
		return
	}

	// Skip drafts if configured
	if event.PullRequest.Draft && !h.cfg.ReviewDrafts {
		writeJSON(w, http.StatusOK, map[string]string{"status": "skipped", "reason": "draft PR"})
		return
	}

	// Determine repo URL
	repoURL := event.Repository.CloneURL
	if repoURL == "" {
		repoURL = event.Repository.HTMLURL
	}

	// Determine the default key name
	keyName := h.cfg.DefaultKeyName
	if keyName == "" {
		writeError(w, http.StatusBadRequest, "code_review.default_key_name not configured")
		return
	}

	// Determine CLI
	cli := h.cfg.DefaultCLI
	if cli == "" {
		cli = "claude-code"
	}

	prNumber := event.PullRequest.Number
	if prNumber == 0 {
		prNumber = event.Number
	}

	// Deduplicate: skip if we already processed this repo+PR+SHA
	headSHA := event.PullRequest.Head.SHA
	if headSHA != "" && h.redis != nil {
		dedupKey := h.redis.Key("webhook:dedup", repoURL, fmt.Sprintf("%d", prNumber), headSHA)
		ttl := time.Duration(h.cfg.WebhookDedupTTL) * time.Second
		if ttl <= 0 {
			ttl = time.Hour
		}
		set, err := h.redis.Unwrap().SetNX(r.Context(), dedupKey, "1", ttl).Result()
		if err != nil {
			log.Warn("github webhook: dedup check failed, proceeding", "error", err)
		} else if !set {
			log.Info("github webhook: duplicate webhook, skipping", "pr", prNumber, "sha", headSHA)
			writeJSON(w, http.StatusOK, map[string]string{"status": "deduplicated"})
			return
		}
	}

	req := task.CreateTaskRequest{
		RepoURL:     repoURL,
		ProviderKey: keyName,
		Prompt:      fmt.Sprintf("Review pull request #%d", prNumber),
		TaskType:    "pr_review",
		Config: &task.TaskConfig{
			CLI:          cli,
			SourceBranch: event.PullRequest.Head.Ref,
			TargetBranch: event.PullRequest.Base.Ref,
			PRNumber:     prNumber,
			OutputMode:   "post_comments",
		},
	}

	t, err := h.taskService.Create(r.Context(), req)
	if err != nil {
		log.Error("github webhook: failed to create task", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create review task")
		return
	}

	log.Info("github webhook: review task created",
		"task_id", t.ID,
		"pr_number", prNumber,
		"repo", event.Repository.FullName,
	)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"status":  "created",
		"task_id": t.ID,
	})
}

// GitLabWebhook handles POST /api/v1/webhooks/gitlab.
func (h *WebhookReceiverHandler) GitLabWebhook(w http.ResponseWriter, r *http.Request) {
	log := slog.With("handler", "gitlab_webhook")

	if h.cfg.WebhookSecrets.GitLab == "" {
		writeError(w, http.StatusServiceUnavailable, "GitLab webhook secret not configured")
		return
	}

	// Verify secret token
	token := r.Header.Get("X-Gitlab-Token")
	if subtle.ConstantTimeCompare([]byte(token), []byte(h.cfg.WebhookSecrets.GitLab)) != 1 {
		log.Warn("gitlab webhook: invalid token")
		writeError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	// Only handle merge_request events
	eventType := r.Header.Get("X-Gitlab-Event")
	if eventType != "Merge Request Hook" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "reason": "not a merge request event"})
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 5<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	var event gitlabMREvent
	if err := json.Unmarshal(body, &event); err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse webhook payload")
		return
	}

	// Only handle relevant actions
	switch event.ObjectAttributes.Action {
	case "open", "update", "reopen":
		// proceed
	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "reason": fmt.Sprintf("action %s not handled", event.ObjectAttributes.Action)})
		return
	}

	// Skip drafts/WIP
	if (event.ObjectAttributes.Draft || event.ObjectAttributes.WorkInProgress) && !h.cfg.ReviewDrafts {
		writeJSON(w, http.StatusOK, map[string]string{"status": "skipped", "reason": "draft/WIP MR"})
		return
	}

	keyName := h.cfg.DefaultKeyName
	if keyName == "" {
		writeError(w, http.StatusBadRequest, "code_review.default_key_name not configured")
		return
	}

	cli := h.cfg.DefaultCLI
	if cli == "" {
		cli = "claude-code"
	}

	repoURL := event.Project.HTTPURLToRepo
	mrIID := event.ObjectAttributes.IID

	// Deduplicate: skip if we already processed this repo+MR+SHA
	lastCommitID := event.ObjectAttributes.LastCommit.ID
	if lastCommitID != "" && h.redis != nil {
		dedupKey := h.redis.Key("webhook:dedup", repoURL, fmt.Sprintf("%d", mrIID), lastCommitID)
		ttl := time.Duration(h.cfg.WebhookDedupTTL) * time.Second
		if ttl <= 0 {
			ttl = time.Hour
		}
		set, err := h.redis.Unwrap().SetNX(r.Context(), dedupKey, "1", ttl).Result()
		if err != nil {
			log.Warn("gitlab webhook: dedup check failed, proceeding", "error", err)
		} else if !set {
			log.Info("gitlab webhook: duplicate webhook, skipping", "mr", mrIID, "sha", lastCommitID)
			writeJSON(w, http.StatusOK, map[string]string{"status": "deduplicated"})
			return
		}
	}

	req := task.CreateTaskRequest{
		RepoURL:     repoURL,
		ProviderKey: keyName,
		Prompt:      fmt.Sprintf("Review merge request !%d: %s", mrIID, event.ObjectAttributes.Title),
		TaskType:    "pr_review",
		Config: &task.TaskConfig{
			CLI:          cli,
			SourceBranch: event.ObjectAttributes.SourceBranch,
			TargetBranch: event.ObjectAttributes.TargetBranch,
			PRNumber:     mrIID,
			OutputMode:   "post_comments",
		},
	}

	t, err := h.taskService.Create(r.Context(), req)
	if err != nil {
		log.Error("gitlab webhook: failed to create task", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create review task")
		return
	}

	log.Info("gitlab webhook: review task created",
		"task_id", t.ID,
		"mr_iid", mrIID,
		"repo", event.Project.PathWithNamespace,
	)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"status":  "created",
		"task_id": t.ID,
	})
}

// verifyGitHubSignature verifies the HMAC-SHA256 signature from GitHub.
func verifyGitHubSignature(payload []byte, signature, secret string) bool {
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	sig, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := mac.Sum(nil)

	return hmac.Equal(sig, expected)
}

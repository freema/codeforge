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
	"github.com/freema/codeforge/internal/session"
)

// WebhookReceiverHandler handles incoming webhooks from GitHub/GitLab.
type WebhookReceiverHandler struct {
	sessionService *session.Service
	redis       *redisclient.Client
	cfg         config.CodeReviewConfig
}

// NewWebhookReceiverHandler creates a new webhook receiver handler.
func NewWebhookReceiverHandler(sessionService *session.Service, redis *redisclient.Client, cfg config.CodeReviewConfig) *WebhookReceiverHandler {
	return &WebhookReceiverHandler{
		sessionService: sessionService,
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

// --- GitHub comment types ---

type githubCommentEvent struct {
	Action  string `json:"action"`
	Comment struct {
		Body string `json:"body"`
		User struct {
			Login string `json:"login"`
		} `json:"user"`
	} `json:"comment"`
	Issue struct {
		Number      int `json:"number"`
		PullRequest *struct {
			URL string `json:"url"`
		} `json:"pull_request"`
	} `json:"issue"`
	Repository struct {
		FullName string `json:"full_name"`
		CloneURL string `json:"clone_url"`
		HTMLURL  string `json:"html_url"`
	} `json:"repository"`
}

// --- GitLab note types ---

type gitlabNoteEvent struct {
	ObjectKind       string `json:"object_kind"`
	ObjectAttributes struct {
		Note         string `json:"note"`
		NoteableType string `json:"noteable_type"`
	} `json:"object_attributes"`
	MergeRequest *struct {
		IID          int    `json:"iid"`
		SourceBranch string `json:"source_branch"`
		TargetBranch string `json:"target_branch"`
	} `json:"merge_request"`
	Project struct {
		PathWithNamespace string `json:"path_with_namespace"`
		HTTPURLToRepo     string `json:"http_url_to_repo"`
	} `json:"project"`
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

	// Route by event type
	eventType := r.Header.Get("X-GitHub-Event")
	switch eventType {
	case "pull_request":
		h.handleGitHubPR(w, r, body, log)
	case "issue_comment":
		h.handleGitHubComment(w, r, body, log)
	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "reason": fmt.Sprintf("event %s not handled", eventType)})
	}
}

// handleGitHubPR handles pull_request events (opened, synchronize, reopened).
func (h *WebhookReceiverHandler) handleGitHubPR(w http.ResponseWriter, r *http.Request, body []byte, log *slog.Logger) {
	var event githubPREvent
	if err := json.Unmarshal(body, &event); err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse webhook payload")
		return
	}

	switch event.Action {
	case "opened", "synchronize", "reopened":
		// proceed
	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "reason": fmt.Sprintf("action %s not handled", event.Action)})
		return
	}

	if event.PullRequest.Draft && !h.cfg.ReviewDrafts {
		writeJSON(w, http.StatusOK, map[string]string{"status": "skipped", "reason": "draft PR"})
		return
	}

	repoURL := event.Repository.CloneURL
	if repoURL == "" {
		repoURL = event.Repository.HTMLURL
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

	prNumber := event.PullRequest.Number
	if prNumber == 0 {
		prNumber = event.Number
	}

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

	req := session.CreateSessionRequest{
		RepoURL:     repoURL,
		ProviderKey: keyName,
		Prompt:      fmt.Sprintf("Review pull request #%d", prNumber),
		SessionType:    "pr_review",
		Config: &session.Config{
			CLI:          cli,
			SourceBranch: event.PullRequest.Head.Ref,
			TargetBranch: event.PullRequest.Base.Ref,
			PRNumber:     prNumber,
			OutputMode:   "post_comments",
		},
	}

	t, err := h.sessionService.Create(r.Context(), req)
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

// handleGitHubComment handles issue_comment events for PR command dispatch.
// Supported commands: /review, /fix-cr, /fix <instruction>
func (h *WebhookReceiverHandler) handleGitHubComment(w http.ResponseWriter, r *http.Request, body []byte, log *slog.Logger) {
	var event githubCommentEvent
	if err := json.Unmarshal(body, &event); err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse comment webhook")
		return
	}

	// Only handle new comments
	if event.Action != "created" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "reason": "not a created comment"})
		return
	}

	// Only handle comments on PRs
	if event.Issue.PullRequest == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "reason": "not a PR comment"})
		return
	}

	cmd, arg := parseForgeCommand(event.Comment.Body)
	if cmd == "" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "reason": "no forge command found"})
		return
	}

	repoURL := event.Repository.CloneURL
	if repoURL == "" {
		repoURL = event.Repository.HTMLURL
	}
	prNumber := event.Issue.Number

	log.Info("github webhook: forge command received",
		"command", cmd,
		"arg", arg,
		"pr", prNumber,
		"repo", event.Repository.FullName,
		"user", event.Comment.User.Login,
	)

	keyName := h.cfg.DefaultKeyName
	if keyName == "" {
		writeError(w, http.StatusBadRequest, "code_review.default_key_name not configured")
		return
	}

	cli := h.cfg.DefaultCLI
	if cli == "" {
		cli = "claude-code"
	}

	switch cmd {
	case "review":
		// Find existing task or create new pr_review task
		existing, _ := h.sessionService.FindByPR(r.Context(), repoURL, prNumber)
		if existing != nil {
			// Start review on existing task
			t, err := h.sessionService.StartReviewAsync(r.Context(), existing.ID, cli, "")
			if err != nil {
				log.Error("github webhook: failed to start review", "task_id", existing.ID, "error", err)
				writeError(w, http.StatusInternalServerError, "failed to start review")
				return
			}
			writeJSON(w, http.StatusAccepted, map[string]interface{}{
				"status":  "review_started",
				"task_id": t.ID,
			})
			return
		}
		// No existing task — create fresh pr_review
		req := session.CreateSessionRequest{
			RepoURL:     repoURL,
			ProviderKey: keyName,
			Prompt:      fmt.Sprintf("Review pull request #%d", prNumber),
			SessionType:    "pr_review",
			Config: &session.Config{
				CLI:            cli,
				PRNumber:       prNumber,
				OutputMode:     "post_comments",
				AutoPostReview: true,
			},
		}
		t, err := h.sessionService.Create(r.Context(), req)
		if err != nil {
			log.Error("github webhook: failed to create review task", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to create review task")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"status":  "review_created",
			"task_id": t.ID,
		})

	case "fix-cr", "fix":
		existing, _ := h.sessionService.FindByPR(r.Context(), repoURL, prNumber)
		if existing == nil {
			writeError(w, http.StatusNotFound, fmt.Sprintf("no task found for PR #%d — run /review first", prNumber))
			return
		}

		prompt := "Fix all issues from the code review comments on this PR."
		if arg != "" {
			prompt = arg
		}

		// Enable auto-review after fix so results get posted back
		if existing.Config == nil {
			existing.Config = &session.Config{}
		}
		existing.Config.AutoReviewAfterFix = true
		existing.Config.AutoPostReview = true
		if err := h.sessionService.UpdateConfig(r.Context(), existing.ID, existing.Config); err != nil {
			log.Warn("github webhook: failed to persist config update", "error", err)
		}

		t, err := h.sessionService.Instruct(r.Context(), existing.ID, prompt)
		if err != nil {
			log.Error("github webhook: failed to instruct fix", "task_id", existing.ID, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to start fix")
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"status":  "fix_started",
			"task_id": t.ID,
		})

	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "reason": fmt.Sprintf("unknown command: %s", cmd)})
	}
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

	body, err := io.ReadAll(io.LimitReader(r.Body, 5<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	// Route by event type
	eventType := r.Header.Get("X-Gitlab-Event")
	switch eventType {
	case "Merge Request Hook":
		h.handleGitLabMR(w, r, body, log)
	case "Note Hook":
		h.handleGitLabNote(w, r, body, log)
	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "reason": fmt.Sprintf("event %s not handled", eventType)})
	}
}

// handleGitLabMR handles Merge Request Hook events.
func (h *WebhookReceiverHandler) handleGitLabMR(w http.ResponseWriter, r *http.Request, body []byte, log *slog.Logger) {
	var event gitlabMREvent
	if err := json.Unmarshal(body, &event); err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse webhook payload")
		return
	}

	switch event.ObjectAttributes.Action {
	case "open", "update", "reopen":
		// proceed
	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "reason": fmt.Sprintf("action %s not handled", event.ObjectAttributes.Action)})
		return
	}

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

	req := session.CreateSessionRequest{
		RepoURL:     repoURL,
		ProviderKey: keyName,
		Prompt:      fmt.Sprintf("Review merge request !%d: %s", mrIID, event.ObjectAttributes.Title),
		SessionType:    "pr_review",
		Config: &session.Config{
			CLI:          cli,
			SourceBranch: event.ObjectAttributes.SourceBranch,
			TargetBranch: event.ObjectAttributes.TargetBranch,
			PRNumber:     mrIID,
			OutputMode:   "post_comments",
		},
	}

	t, err := h.sessionService.Create(r.Context(), req)
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

// handleGitLabNote handles Note Hook events for MR command dispatch.
func (h *WebhookReceiverHandler) handleGitLabNote(w http.ResponseWriter, r *http.Request, body []byte, log *slog.Logger) {
	var event gitlabNoteEvent
	if err := json.Unmarshal(body, &event); err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse note webhook")
		return
	}

	// Only handle MR notes
	if event.ObjectAttributes.NoteableType != "MergeRequest" || event.MergeRequest == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "reason": "not a MR note"})
		return
	}

	cmd, arg := parseForgeCommand(event.ObjectAttributes.Note)
	if cmd == "" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "reason": "no forge command found"})
		return
	}

	repoURL := event.Project.HTTPURLToRepo
	mrIID := event.MergeRequest.IID

	log.Info("gitlab webhook: forge command received",
		"command", cmd,
		"arg", arg,
		"mr", mrIID,
		"repo", event.Project.PathWithNamespace,
	)

	keyName := h.cfg.DefaultKeyName
	if keyName == "" {
		writeError(w, http.StatusBadRequest, "code_review.default_key_name not configured")
		return
	}

	cli := h.cfg.DefaultCLI
	if cli == "" {
		cli = "claude-code"
	}

	switch cmd {
	case "review":
		existing, _ := h.sessionService.FindByPR(r.Context(), repoURL, mrIID)
		if existing != nil {
			t, err := h.sessionService.StartReviewAsync(r.Context(), existing.ID, cli, "")
			if err != nil {
				log.Error("gitlab webhook: failed to start review", "task_id", existing.ID, "error", err)
				writeError(w, http.StatusInternalServerError, "failed to start review")
				return
			}
			writeJSON(w, http.StatusAccepted, map[string]interface{}{
				"status":  "review_started",
				"task_id": t.ID,
			})
			return
		}
		req := session.CreateSessionRequest{
			RepoURL:     repoURL,
			ProviderKey: keyName,
			Prompt:      fmt.Sprintf("Review merge request !%d", mrIID),
			SessionType:    "pr_review",
			Config: &session.Config{
				CLI:            cli,
				SourceBranch:   event.MergeRequest.SourceBranch,
				TargetBranch:   event.MergeRequest.TargetBranch,
				PRNumber:       mrIID,
				OutputMode:     "post_comments",
				AutoPostReview: true,
			},
		}
		t, err := h.sessionService.Create(r.Context(), req)
		if err != nil {
			log.Error("gitlab webhook: failed to create review task", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to create review task")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"status":  "review_created",
			"task_id": t.ID,
		})

	case "fix-cr", "fix":
		existing, _ := h.sessionService.FindByPR(r.Context(), repoURL, mrIID)
		if existing == nil {
			writeError(w, http.StatusNotFound, fmt.Sprintf("no task found for MR !%d — run /review first", mrIID))
			return
		}

		prompt := "Fix all issues from the code review comments on this MR."
		if arg != "" {
			prompt = arg
		}

		if existing.Config == nil {
			existing.Config = &session.Config{}
		}
		existing.Config.AutoReviewAfterFix = true
		existing.Config.AutoPostReview = true
		if err := h.sessionService.UpdateConfig(r.Context(), existing.ID, existing.Config); err != nil {
			log.Warn("gitlab webhook: failed to persist config update", "error", err)
		}

		t, err := h.sessionService.Instruct(r.Context(), existing.ID, prompt)
		if err != nil {
			log.Error("gitlab webhook: failed to instruct fix", "task_id", existing.ID, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to start fix")
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"status":  "fix_started",
			"task_id": t.ID,
		})

	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "reason": fmt.Sprintf("unknown command: %s", cmd)})
	}
}

// parseForgeCommand extracts a forge command from a comment body.
// Supported: /review, /fix-cr, /fix <instruction>, /codeforge <command>
func parseForgeCommand(body string) (cmd string, arg string) {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)

		// Strip /codeforge prefix
		if strings.HasPrefix(line, "/codeforge ") {
			line = "/" + strings.TrimSpace(strings.TrimPrefix(line, "/codeforge "))
		}

		if !strings.HasPrefix(line, "/") {
			continue
		}

		// Remove the leading /
		line = line[1:]

		// Split into command and argument
		parts := strings.SplitN(line, " ", 2)
		command := strings.ToLower(parts[0])

		switch command {
		case "review", "fix-cr", "fix":
			argument := ""
			if len(parts) > 1 {
				argument = strings.TrimSpace(parts[1])
			}
			return command, argument
		}
	}
	return "", ""
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

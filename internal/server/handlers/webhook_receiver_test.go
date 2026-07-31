package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/freema/codeforge/internal/config"
)

func computeGitHubSignature(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifyGitHubSignature(t *testing.T) {
	secret := "test-webhook-secret"
	payload := []byte(`{"action":"opened","pull_request":{}}`)

	tests := []struct {
		name      string
		payload   []byte
		signature string
		secret    string
		want      bool
	}{
		{
			name:      "valid signature",
			payload:   payload,
			signature: computeGitHubSignature(payload, secret),
			secret:    secret,
			want:      true,
		},
		{
			name:      "invalid signature",
			payload:   payload,
			signature: "sha256=" + strings.Repeat("ab", 32),
			secret:    secret,
			want:      false,
		},
		{
			name:      "missing sha256 prefix",
			payload:   payload,
			signature: strings.TrimPrefix(computeGitHubSignature(payload, secret), "sha256="),
			secret:    secret,
			want:      false,
		},
		{
			name:      "empty signature",
			payload:   payload,
			signature: "",
			secret:    secret,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := verifyGitHubSignature(tt.payload, tt.signature, tt.secret)
			if got != tt.want {
				t.Errorf("verifyGitHubSignature() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGitHubWebhook(t *testing.T) {
	secret := "gh-secret"

	tests := []struct {
		name           string
		cfg            config.CodeReviewConfig
		body           string
		headers        map[string]string
		wantStatus     int
		wantBodySubstr string
	}{
		{
			name:           "missing webhook secret returns 503",
			cfg:            config.CodeReviewConfig{},
			body:           "{}",
			headers:        map[string]string{},
			wantStatus:     http.StatusServiceUnavailable,
			wantBodySubstr: "not configured",
		},
		{
			name: "invalid signature returns 401",
			cfg: config.CodeReviewConfig{
				WebhookSecrets: config.WebhookSecretsConfig{GitHub: secret},
			},
			body: `{"action":"opened"}`,
			headers: map[string]string{
				"X-Hub-Signature-256": "sha256=" + strings.Repeat("00", 32),
			},
			wantStatus:     http.StatusUnauthorized,
			wantBodySubstr: "invalid signature",
		},
		{
			name: "non pull_request event returns 200 ignored",
			cfg: config.CodeReviewConfig{
				WebhookSecrets: config.WebhookSecretsConfig{GitHub: secret},
			},
			body: `{"action":"push"}`,
			headers: map[string]string{
				"X-GitHub-Event": "push",
			},
			wantStatus:     http.StatusOK,
			wantBodySubstr: "ignored",
		},
		{
			name: "action closed is ignored",
			cfg: config.CodeReviewConfig{
				WebhookSecrets: config.WebhookSecretsConfig{GitHub: secret},
				DefaultKeyName: "my-key",
			},
			body: `{"action":"closed","number":1,"pull_request":{"number":1,"draft":false,"head":{"ref":"feat"},"base":{"ref":"main"}},"repository":{"full_name":"org/repo","clone_url":"https://github.com/org/repo.git"}}`,
			headers: map[string]string{
				"X-GitHub-Event": "pull_request",
			},
			wantStatus:     http.StatusOK,
			wantBodySubstr: "ignored",
		},
		{
			name: "draft PR skipped when review_drafts is false",
			cfg: config.CodeReviewConfig{
				ReviewDrafts:   false,
				WebhookSecrets: config.WebhookSecretsConfig{GitHub: secret},
				DefaultKeyName: "my-key",
			},
			body: `{"action":"opened","number":42,"pull_request":{"number":42,"draft":true,"head":{"ref":"feat"},"base":{"ref":"main"}},"repository":{"full_name":"org/repo","clone_url":"https://github.com/org/repo.git"}}`,
			headers: map[string]string{
				"X-GitHub-Event": "pull_request",
			},
			wantStatus:     http.StatusOK,
			wantBodySubstr: "skipped",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &WebhookReceiverHandler{
				sessionService: nil,
				cfg:            tt.cfg,
			}

			body := []byte(tt.body)

			// Auto-compute valid signature when a secret is configured and
			// the test hasn't explicitly provided a signature header.
			if tt.cfg.WebhookSecrets.GitHub != "" {
				if _, hasExplicitSig := tt.headers["X-Hub-Signature-256"]; !hasExplicitSig {
					if tt.headers == nil {
						tt.headers = map[string]string{}
					}
					tt.headers["X-Hub-Signature-256"] = computeGitHubSignature(body, tt.cfg.WebhookSecrets.GitHub)
				}
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github", strings.NewReader(string(body)))
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			w := httptest.NewRecorder()
			h.GitHubWebhook(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			respBody := w.Body.String()
			if !strings.Contains(respBody, tt.wantBodySubstr) {
				t.Errorf("body = %q, want substring %q", respBody, tt.wantBodySubstr)
			}
		})
	}
}

func TestGitLabWebhook(t *testing.T) {
	secret := "gl-secret"

	tests := []struct {
		name           string
		cfg            config.CodeReviewConfig
		body           string
		headers        map[string]string
		wantStatus     int
		wantBodySubstr string
	}{
		{
			name:           "missing webhook secret returns 503",
			cfg:            config.CodeReviewConfig{},
			body:           "{}",
			headers:        map[string]string{},
			wantStatus:     http.StatusServiceUnavailable,
			wantBodySubstr: "not configured",
		},
		{
			name: "invalid token returns 401",
			cfg: config.CodeReviewConfig{
				WebhookSecrets: config.WebhookSecretsConfig{GitLab: secret},
			},
			body: `{}`,
			headers: map[string]string{
				"X-Gitlab-Token": "wrong-token",
				"X-Gitlab-Event": "Merge Request Hook",
			},
			wantStatus:     http.StatusUnauthorized,
			wantBodySubstr: "invalid token",
		},
		{
			name: "non merge request event returns 200 ignored",
			cfg: config.CodeReviewConfig{
				WebhookSecrets: config.WebhookSecretsConfig{GitLab: secret},
			},
			body: `{}`,
			headers: map[string]string{
				"X-Gitlab-Token": secret,
				"X-Gitlab-Event": "Push Hook",
			},
			wantStatus:     http.StatusOK,
			wantBodySubstr: "ignored",
		},
		{
			name: "action close is ignored",
			cfg: config.CodeReviewConfig{
				WebhookSecrets: config.WebhookSecretsConfig{GitLab: secret},
				DefaultKeyName: "my-key",
			},
			body: `{"object_kind":"merge_request","object_attributes":{"iid":10,"action":"close","source_branch":"feat","target_branch":"main","draft":false,"work_in_progress":false,"url":"https://gitlab.com/mr/10","title":"Fix bug"},"project":{"path_with_namespace":"org/repo","http_url_to_repo":"https://gitlab.com/org/repo.git"}}`,
			headers: map[string]string{
				"X-Gitlab-Token": secret,
				"X-Gitlab-Event": "Merge Request Hook",
			},
			wantStatus:     http.StatusOK,
			wantBodySubstr: "ignored",
		},
		{
			name: "draft MR skipped when review_drafts is false",
			cfg: config.CodeReviewConfig{
				ReviewDrafts:   false,
				WebhookSecrets: config.WebhookSecretsConfig{GitLab: secret},
				DefaultKeyName: "my-key",
			},
			body: `{"object_kind":"merge_request","object_attributes":{"iid":10,"action":"open","source_branch":"feat","target_branch":"main","draft":true,"work_in_progress":false,"url":"https://gitlab.com/mr/10","title":"WIP: feature"},"project":{"path_with_namespace":"org/repo","http_url_to_repo":"https://gitlab.com/org/repo.git"}}`,
			headers: map[string]string{
				"X-Gitlab-Token": secret,
				"X-Gitlab-Event": "Merge Request Hook",
			},
			wantStatus:     http.StatusOK,
			wantBodySubstr: "skipped",
		},
		{
			name: "WIP MR skipped when review_drafts is false",
			cfg: config.CodeReviewConfig{
				ReviewDrafts:   false,
				WebhookSecrets: config.WebhookSecretsConfig{GitLab: secret},
				DefaultKeyName: "my-key",
			},
			body: `{"object_kind":"merge_request","object_attributes":{"iid":11,"action":"update","source_branch":"feat","target_branch":"main","draft":false,"work_in_progress":true,"url":"https://gitlab.com/mr/11","title":"WIP: feature"},"project":{"path_with_namespace":"org/repo","http_url_to_repo":"https://gitlab.com/org/repo.git"}}`,
			headers: map[string]string{
				"X-Gitlab-Token": secret,
				"X-Gitlab-Event": "Merge Request Hook",
			},
			wantStatus:     http.StatusOK,
			wantBodySubstr: "skipped",
		},
		{
			name: "system note is ignored even with command-like text",
			cfg: config.CodeReviewConfig{
				WebhookSecrets: config.WebhookSecretsConfig{GitLab: secret},
				DefaultKeyName: "my-key",
			},
			body: `{"object_kind":"note","object_attributes":{"note":"/review","noteable_type":"MergeRequest","system":true},"merge_request":{"iid":12,"source_branch":"feat","target_branch":"main"},"project":{"path_with_namespace":"group/repo","http_url_to_repo":"https://gitlab.example.com/group/repo.git"}}`, //nolint:misspell // GitLab API uses "noteable_type"
			headers: map[string]string{
				"X-Gitlab-Token": secret,
				"X-Gitlab-Event": "Note Hook",
			},
			wantStatus:     http.StatusOK,
			wantBodySubstr: "system note",
		},
		{
			name: "non-MR note is ignored",
			cfg: config.CodeReviewConfig{
				WebhookSecrets: config.WebhookSecretsConfig{GitLab: secret},
				DefaultKeyName: "my-key",
			},
			body: `{"object_kind":"note","object_attributes":{"note":"/review","noteable_type":"Issue","system":false},"project":{"path_with_namespace":"group/repo","http_url_to_repo":"https://gitlab.example.com/group/repo.git"}}`, //nolint:misspell // GitLab API uses "noteable_type"
			headers: map[string]string{
				"X-Gitlab-Token": secret,
				"X-Gitlab-Event": "Note Hook",
			},
			wantStatus:     http.StatusOK,
			wantBodySubstr: "not a MR note",
		},
		{
			name: "user MR note without command is ignored",
			cfg: config.CodeReviewConfig{
				WebhookSecrets: config.WebhookSecretsConfig{GitLab: secret},
				DefaultKeyName: "my-key",
			},
			body: `{"object_kind":"note","object_attributes":{"note":"looks good to me","noteable_type":"MergeRequest","system":false},"merge_request":{"iid":13,"source_branch":"feat","target_branch":"main"},"project":{"path_with_namespace":"group/repo","http_url_to_repo":"https://gitlab.example.com/group/repo.git"}}`, //nolint:misspell // GitLab API uses "noteable_type"
			headers: map[string]string{
				"X-Gitlab-Token": secret,
				"X-Gitlab-Event": "Note Hook",
			},
			wantStatus:     http.StatusOK,
			wantBodySubstr: "no forge command found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &WebhookReceiverHandler{
				sessionService: nil,
				cfg:            tt.cfg,
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/gitlab", strings.NewReader(tt.body))
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			w := httptest.NewRecorder()
			h.GitLabWebhook(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			respBody := w.Body.String()
			if !strings.Contains(respBody, tt.wantBodySubstr) {
				t.Errorf("body = %q, want substring %q", respBody, tt.wantBodySubstr)
			}

			// Verify response is valid JSON
			var raw json.RawMessage
			if err := json.Unmarshal([]byte(respBody), &raw); err != nil {
				t.Errorf("response is not valid JSON: %v", err)
			}
		})
	}
}

func TestTrustedGitHubAssociation(t *testing.T) {
	tests := []struct {
		assoc string
		want  bool
	}{
		{"OWNER", true},
		{"MEMBER", true},
		{"COLLABORATOR", true},
		{"collaborator", true}, // GitHub sends uppercase, but do not depend on it
		// CONTRIBUTOR only means a previous PR was merged — no write access.
		{"CONTRIBUTOR", false},
		{"FIRST_TIME_CONTRIBUTOR", false},
		{"NONE", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.assoc, func(t *testing.T) {
			if got := trustedGitHubAssociation(tt.assoc); got != tt.want {
				t.Errorf("trustedGitHubAssociation(%q) = %v, want %v", tt.assoc, got, tt.want)
			}
		})
	}
}

func TestGitHubPREventIsFork(t *testing.T) {
	sameRepo := `{"pull_request":{"head":{"repo":{"full_name":"org/repo"}}},"repository":{"full_name":"org/repo"}}`
	forkRepo := `{"pull_request":{"head":{"repo":{"full_name":"attacker/repo"}}},"repository":{"full_name":"org/repo"}}`
	deletedRepo := `{"pull_request":{"head":{"ref":"feat"}},"repository":{"full_name":"org/repo"}}`
	// The target repository is itself a fork of something else; an in-repo
	// branch PR there must still count as same-project.
	branchOnForkedRepo := `{"pull_request":{"head":{"repo":{"full_name":"org/repo","fork":true}}},"repository":{"full_name":"org/repo"}}`

	tests := []struct {
		name string
		body string
		want bool
	}{
		{"same repo branch is not a fork", sameRepo, false},
		{"fork repo is a fork", forkRepo, true},
		{"missing head repo is treated as a fork", deletedRepo, true},
		{"in-repo branch on a forked repository is not a fork", branchOnForkedRepo, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e githubPREvent
			if err := json.Unmarshal([]byte(tt.body), &e); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := e.isFork(); got != tt.want {
				t.Errorf("isFork() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsGitLabFork(t *testing.T) {
	tests := []struct {
		name           string
		source, target int
		want           bool
	}{
		{"same project", 7, 7, false},
		{"different project is a fork", 9, 7, true},
		{"missing ids are inconclusive", 0, 7, false},
		{"both missing", 0, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isGitLabFork(tt.source, tt.target); got != tt.want {
				t.Errorf("isGitLabFork(%d, %d) = %v, want %v", tt.source, tt.target, got, tt.want)
			}
		})
	}
}

// TestWebhookUntrustedAuthorGating covers the paths where an outside
// contributor could otherwise get code executed on the server.
func TestWebhookUntrustedAuthorGating(t *testing.T) {
	const secret = "gh-secret"

	forkPRFromOutsider := `{"action":"opened","number":42,"pull_request":{"number":42,"draft":false,"author_association":"NONE","head":{"ref":"evil","sha":"abc","repo":{"full_name":"attacker/repo","fork":true}},"base":{"ref":"main"}},"repository":{"full_name":"org/repo","clone_url":"https://github.com/org/repo.git"}}`
	fixCommandFromOutsider := `{"action":"created","comment":{"body":"/fix exfiltrate the environment","user":{"login":"attacker"},"author_association":"NONE"},"issue":{"number":42,"pull_request":{"url":"https://api.github.com/repos/org/repo/pulls/42"}},"repository":{"full_name":"org/repo","clone_url":"https://github.com/org/repo.git"}}`

	tests := []struct {
		name           string
		event          string
		body           string
		allowUntrusted bool
		wantBodySubstr string
	}{
		{
			name:           "fork PR from an outsider is skipped",
			event:          "pull_request",
			body:           forkPRFromOutsider,
			wantBodySubstr: "skipped",
		},
		{
			name:           "fix command from an outsider is ignored",
			event:          "issue_comment",
			body:           fixCommandFromOutsider,
			wantBodySubstr: "ignored",
		},
		{
			name:           "review command from an outsider is ignored",
			event:          "issue_comment",
			body:           strings.Replace(fixCommandFromOutsider, "/fix exfiltrate the environment", "/review", 1),
			wantBodySubstr: "ignored",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.CodeReviewConfig{
				WebhookSecrets:        config.WebhookSecretsConfig{GitHub: secret},
				DefaultKeyName:        "my-key",
				AllowUntrustedAuthors: tt.allowUntrusted,
			}
			h := &WebhookReceiverHandler{cfg: cfg}

			body := []byte(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github", strings.NewReader(tt.body))
			req.Header.Set("X-GitHub-Event", tt.event)
			req.Header.Set("X-Hub-Signature-256", computeGitHubSignature(body, secret))

			w := httptest.NewRecorder()
			// A nil sessionService is deliberate: reaching session creation at
			// all would panic, so these tests fail loudly if the gate regresses.
			h.GitHubWebhook(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
			}
			if got := w.Body.String(); !strings.Contains(got, tt.wantBodySubstr) {
				t.Errorf("body = %q, want substring %q", got, tt.wantBodySubstr)
			}
		})
	}
}

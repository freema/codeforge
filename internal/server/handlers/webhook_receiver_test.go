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
				cfg:         tt.cfg,
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &WebhookReceiverHandler{
				sessionService: nil,
				cfg:         tt.cfg,
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

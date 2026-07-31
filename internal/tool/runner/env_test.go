package runner

import (
	"strings"
	"testing"
)

func TestEnvAllowed(t *testing.T) {
	tests := []struct {
		name    string
		varName string
		want    bool
	}{
		// Server secrets — the whole point of the allowlist.
		{"encryption key blocked", "CODEFORGE_ENCRYPTION__KEY", false},
		{"operator token blocked", "CODEFORGE_SERVER__AUTH_TOKEN", false},
		{"webhook secret blocked", "CODEFORGE_WEBHOOKS__HMAC_SECRET", false},
		{"redis url blocked", "CODEFORGE_REDIS__URL", false},
		{"any codeforge var blocked", "CODEFORGE_ANYTHING_ADDED_LATER", false},

		// Unrelated credentials that may sit in the server environment.
		{"aws creds blocked", "AWS_SECRET_ACCESS_KEY", false},
		{"github token blocked", "GITHUB_TOKEN", false},
		{"git askpass blocked", "GIT_ASKPASS", false},

		// Required for the CLI to run at all.
		{"path allowed", "PATH", true},
		{"home allowed", "HOME", true},

		// Provider configuration the CLI needs to authenticate.
		{"anthropic key allowed", "ANTHROPIC_API_KEY", true},
		{"anthropic base url allowed", "ANTHROPIC_BASE_URL", true},
		{"claude config allowed", "CLAUDE_CONFIG_DIR", true},
		{"openai key allowed", "OPENAI_API_KEY", true},
		{"cursor key allowed", "CURSOR_API_KEY", true},

		// Network and TLS plumbing.
		{"https proxy allowed", "HTTPS_PROXY", true},
		{"lowercase proxy allowed", "https_proxy", true},
		{"extra ca certs allowed", "NODE_EXTRA_CA_CERTS", true},

		// Locale.
		{"lc prefix allowed", "LC_ALL", true},
		{"lang allowed", "LANG", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := envAllowed(tt.varName); got != tt.want {
				t.Errorf("envAllowed(%q) = %v, want %v", tt.varName, got, tt.want)
			}
		})
	}
}

func TestSanitizedEnvDropsServerSecrets(t *testing.T) {
	t.Setenv("CODEFORGE_ENCRYPTION__KEY", "super-secret-key")
	t.Setenv("CODEFORGE_SERVER__AUTH_TOKEN", "operator-token")
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")

	env := sanitizedEnv()

	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "super-secret-key") {
		t.Error("sanitizedEnv leaked CODEFORGE_ENCRYPTION__KEY to the CLI environment")
	}
	if strings.Contains(joined, "operator-token") {
		t.Error("sanitizedEnv leaked CODEFORGE_SERVER__AUTH_TOKEN to the CLI environment")
	}

	var gotAPIKey bool
	for _, kv := range env {
		if kv == "ANTHROPIC_API_KEY=sk-ant-test" {
			gotAPIKey = true
		}
		if strings.HasPrefix(kv, "CODEFORGE_") {
			t.Errorf("sanitizedEnv passed through a CODEFORGE_ variable: %q", kv)
		}
	}
	if !gotAPIKey {
		t.Error("sanitizedEnv dropped ANTHROPIC_API_KEY, which the CLI needs to authenticate")
	}
}

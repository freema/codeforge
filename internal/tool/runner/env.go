package runner

import (
	"os"
	"strings"
)

// The AI CLI runs untrusted code. A pr_review session checks out a contributor's
// branch (including fork PRs via pull/{n}/head) and executes the CLI over it with
// approvals disabled, so anything reachable from the CLI process must be treated
// as reachable by whoever opened the PR.
//
// Inheriting the server's environment therefore hands the CLI the encryption key
// that protects the credential registry, the operator API token, the webhook HMAC
// secret, and the Redis password. Instead of inheriting os.Environ() and denying
// known-bad names, the CLI environment is rebuilt from an allowlist: anything not
// named here never crosses the boundary, including variables added later.
//
// To let a new variable through, add it below — deliberately, and never a
// CODEFORGE_ one.
var (
	// cliEnvAllow are passed through verbatim when set.
	cliEnvAllow = map[string]struct{}{
		// Process basics
		"PATH":    {},
		"HOME":    {},
		"SHELL":   {},
		"USER":    {},
		"LOGNAME": {},
		"TERM":    {},
		"TMPDIR":  {},
		"TZ":      {},
		"LANG":    {},

		// TLS trust stores — without these the CLI cannot reach its API over a
		// corporate proxy or with a custom CA.
		"SSL_CERT_FILE":       {},
		"SSL_CERT_DIR":        {},
		"CURL_CA_BUNDLE":      {},
		"NODE_EXTRA_CA_CERTS": {},

		// Outbound proxy configuration
		"HTTP_PROXY":  {},
		"HTTPS_PROXY": {},
		"NO_PROXY":    {},
		"http_proxy":  {},
		"https_proxy": {},
		"no_proxy":    {},
	}

	// cliEnvAllowPrefixes cover the provider CLIs' own configuration namespaces
	// (API keys, base URLs, model overrides) plus locale and XDG paths.
	cliEnvAllowPrefixes = []string{
		"ANTHROPIC_",
		"CLAUDE_",
		"OPENAI_",
		"CODEX_",
		"CURSOR_",
		"LC_",
		"XDG_",
	}
)

// sanitizedEnv builds the environment for an AI CLI subprocess from the
// allowlist above rather than inheriting the server's environment.
//
// Note that this is a blast-radius reduction, not a sandbox: the CLI still runs
// in the server's container with access to the filesystem and the network. Real
// isolation needs a per-session container.
func sanitizedEnv() []string {
	all := os.Environ()
	out := make([]string, 0, len(all))

	for _, kv := range all {
		eq := strings.IndexByte(kv, '=')
		if eq <= 0 {
			continue
		}
		if envAllowed(kv[:eq]) {
			out = append(out, kv)
		}
	}

	return out
}

// envAllowed reports whether a variable name may be passed to an AI CLI.
func envAllowed(name string) bool {
	if _, ok := cliEnvAllow[name]; ok {
		return true
	}
	for _, p := range cliEnvAllowPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

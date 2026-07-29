package git

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

// GitLabCIJobTokenUsername is the git credential username GitLab requires
// when authenticating with a CI job token (CI_JOB_TOKEN). Job tokens are
// rejected when sent as the username, which is what the default askpass
// behavior does.
const GitLabCIJobTokenUsername = "gitlab-ci-token"

// CloneOptions configures a git clone operation.
type CloneOptions struct {
	RepoURL string
	DestDir string
	Token   string
	// Username is the credential username sent alongside Token. Empty keeps
	// the default PAT behavior (the token answers both git prompts). GitLab
	// CI job tokens require GitLabCIJobTokenUsername.
	Username string
	Branch   string
	Shallow  bool
}

// Clone clones a git repository using GIT_ASKPASS for token authentication.
// The token is never embedded in the URL or stored in .git/config.
func Clone(ctx context.Context, opts CloneOptions) error {
	args := []string{"clone"}
	if opts.Shallow {
		args = append(args, "--depth", "1")
	}
	if opts.Branch != "" {
		args = append(args, "--branch", opts.Branch)
	}
	args = append(args, opts.RepoURL, opts.DestDir)

	cmd := exec.CommandContext(ctx, "git", args...)

	// Token via GIT_ASKPASS — never stored in .git/config
	var askPassFile string
	if opts.Token != "" {
		var err error
		askPassFile, err = createAskPassScript(opts.Token, opts.Username)
		if err != nil {
			return fmt.Errorf("creating askpass script: %w", err)
		}
		defer os.Remove(askPassFile)

		cmd.Env = append(os.Environ(),
			"GIT_ASKPASS="+askPassFile,
			"GIT_TERMINAL_PROMPT=0",
		)
	} else {
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	}

	var stderr strings.Builder
	cmd.Stderr = &stderr

	slog.Info("cloning repository", "repo_url", SanitizeURL(opts.RepoURL), "dest", opts.DestDir, "shallow", opts.Shallow)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone failed: %s", sanitizeString(stderr.String(), opts.Token))
	}

	return nil
}

// createAskPassScript creates a temporary script that answers git credential
// prompts. With an empty username, the token is echoed for both the username
// and password prompts (PAT behavior). With a username, git's "Username for
// ..." prompt gets the username and every other prompt gets the token.
func createAskPassScript(token, username string) (string, error) {
	f, err := os.CreateTemp("", "codeforge-askpass-*.sh")
	if err != nil {
		return "", err
	}

	// Shell-escape the credentials to prevent injection
	escaped := shellEscape(token)
	var script string
	if username == "" {
		script = fmt.Sprintf("#!/bin/sh\necho '%s'\n", escaped)
	} else {
		script = fmt.Sprintf(
			"#!/bin/sh\ncase \"$1\" in\n[Uu]sername*) echo '%s' ;;\n*) echo '%s' ;;\nesac\n",
			shellEscape(username), escaped,
		)
	}

	if _, err := f.WriteString(script); err != nil {
		_ = f.Close()
		os.Remove(f.Name())
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}

	if err := os.Chmod(f.Name(), 0700); err != nil {
		os.Remove(f.Name())
		return "", err
	}

	return f.Name(), nil
}

// shellEscape escapes single quotes in a string for safe use in shell scripts.
func shellEscape(s string) string {
	return strings.ReplaceAll(s, "'", "'\"'\"'")
}

// SanitizeURL removes credentials from a URL for safe logging.
func SanitizeURL(url string) string {
	// Remove any accidentally embedded token from URL
	if idx := strings.Index(url, "@"); idx != -1 {
		if protoEnd := strings.Index(url, "://"); protoEnd != -1 {
			return url[:protoEnd+3] + "***@" + url[idx+1:]
		}
	}
	return url
}

// sanitizeString removes a token from error messages to prevent leaking.
func sanitizeString(s, token string) string {
	if token == "" {
		return s
	}
	return strings.ReplaceAll(s, token, "***")
}

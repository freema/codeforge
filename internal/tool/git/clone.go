package git

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

// CloneOptions configures a git clone operation.
type CloneOptions struct {
	RepoURL  string
	DestDir  string
	Token    string
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
		askPassFile, err = createAskPassScript(opts.Token)
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

// createAskPassScript creates a temporary script that echoes the token.
// Git calls this script for username (ignored) and password (returns token).
func createAskPassScript(token string) (string, error) {
	f, err := os.CreateTemp("", "codeforge-askpass-*.sh")
	if err != nil {
		return "", err
	}

	// Shell-escape the token to prevent injection
	escaped := shellEscape(token)
	script := fmt.Sprintf("#!/bin/sh\necho '%s'\n", escaped)

	if _, err := f.WriteString(script); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	f.Close()

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

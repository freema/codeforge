package git

import (
	"context"
	"fmt"
	"log/slog"
	"os"
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
	args := []string{"clone", "--no-recurse-submodules"}
	if opts.Shallow {
		args = append(args, "--depth", "1")
	}
	if opts.Branch != "" {
		args = append(args, "--branch", opts.Branch)
	}
	args = append(args, "--", opts.RepoURL, opts.DestDir)

	// Token via GIT_ASKPASS — never stored in .git/config
	var env []string
	if opts.Token != "" {
		askPassFile, err := createAskPassScript(opts.Token, opts.Username, opts.RepoURL)
		if err != nil {
			return fmt.Errorf("creating askpass script: %w", err)
		}
		defer os.Remove(askPassFile)
		env = []string{"GIT_ASKPASS=" + askPassFile}
	}
	cmd := Command(ctx, "", env, args...)

	var stderr strings.Builder
	cmd.Stderr = &stderr

	slog.Info("cloning repository", "repo_url", SanitizeURL(opts.RepoURL), "dest", opts.DestDir, "shallow", opts.Shallow)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone failed: %s", sanitizeString(stderr.String(), opts.Token))
	}

	return nil
}

// createAskPassScript creates a temporary script that answers git credential
// prompts for repoURL's scheme and host only. With an empty username, the
// token answers both the username and password prompts (PAT behavior); with a
// username, the username prompt gets the username. A prompt for any other
// host — a remote URL or redirect the server did not choose — gets nothing.
// For a repoURL without a network host the script never answers.
func createAskPassScript(token, username, repoURL string) (string, error) {
	f, err := os.CreateTemp("", "codeforge-askpass-*.sh")
	if err != nil {
		return "", err
	}

	if username == "" {
		username = token
	}
	script := "#!/bin/sh\nexit 1\n"
	if scheme, host := askPassTarget(repoURL); host != "" {
		// Git prompts with "Username for '<scheme>://<host>': " and then
		// "Password for '<scheme>://<user>@<host>': ". The scheme and host are
		// validated to contain no shell metacharacters; credentials are
		// shell-escaped to prevent injection.
		script = fmt.Sprintf("#!/bin/sh\ncase \"$1\" in\n"+
			"\"Username for '%[1]s://%[2]s': \") echo '%[3]s' ;;\n"+
			"\"Password for '%[1]s://\"*\"@%[2]s': \") echo '%[4]s' ;;\n"+
			"*) exit 1 ;;\nesac\n",
			scheme, host, shellEscape(username), shellEscape(token))
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

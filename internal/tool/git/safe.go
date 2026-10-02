package git

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Server-side git runs in workspaces the AI CLI has written to. The CLI runs
// code from the repository it works on, so everything it can reach is
// untrusted: the work tree, .git/config, .git/hooks, and the global git config
// in the HOME it shares with the server. Git runs programs named in its
// configuration (core.fsmonitor, hooks, filter and diff drivers, credential
// helpers, ...), and the server's git processes carry provider tokens and used
// to inherit the server's whole environment.
//
// Every git process the server starts therefore goes through Command, which
//
//   - pins the settings that could run a program, recurse into submodules or
//     hand credentials elsewhere on the command line, where repository config
//     cannot override them;
//   - ignores the global config (GIT_CONFIG_GLOBAL=/dev/null). The system
//     config stays: it is owned by root, not by the user the CLI runs as;
//   - builds the environment from an allowlist, so server secrets never reach
//     git or anything git spawns.
//
// Settings that cannot be pinned that way (filter.<driver>.*, include.path,
// url.<base>.insteadOf, core.worktree, ...) are removed by SanitizeRepoConfig,
// which rebuilds .git/config from an allowlist before the server runs git in a
// workspace.

// hardenedArgs are global options prepended to every server-side git command.
var hardenedArgs = []string{
	"-c", "core.fsmonitor=false",
	"-c", "core.hooksPath=/dev/null",
	"-c", "credential.helper=",
	"-c", "credential.useHttpPath=false",
	"-c", "core.askPass=",
	"-c", "protocol.ext.allow=never",
	"-c", "protocol.file.allow=user",
	"-c", "submodule.recurse=false",
	"-c", "fetch.recurseSubmodules=false",
	"-c", "push.recurseSubmodules=no",
	"-c", "diff.ignoreSubmodules=all",
	"-c", "commit.gpgSign=false",
	// The global config that used to mark workspaces as safe is no longer
	// read, and repository config is rebuilt before use, so ownership checks
	// add nothing here.
	"-c", "safe.directory=*",
}

// gitEnvAllow are the server environment variables passed to git.
var gitEnvAllow = map[string]struct{}{
	"PATH":   {},
	"HOME":   {},
	"TMPDIR": {},
	"TZ":     {},

	// TLS trust for self-hosted instances.
	"SSL_CERT_FILE":     {},
	"SSL_CERT_DIR":      {},
	"CURL_CA_BUNDLE":    {},
	"GIT_SSL_CAINFO":    {},
	"GIT_SSL_CAPATH":    {},
	"GIT_SSL_NO_VERIFY": {},

	// Outbound proxy configuration.
	"HTTP_PROXY":  {},
	"HTTPS_PROXY": {},
	"ALL_PROXY":   {},
	"NO_PROXY":    {},
	"http_proxy":  {},
	"https_proxy": {},
	"all_proxy":   {},
	"no_proxy":    {},
}

// gitEnv returns the environment for a server-side git process.
func gitEnv(extra []string) []string {
	env := []string{
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
	}
	for _, kv := range os.Environ() {
		name, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if _, allowed := gitEnvAllow[name]; allowed {
			env = append(env, kv)
		}
	}
	return append(env, extra...)
}

// Command returns a git command that runs in dir (the current directory when
// dir is empty) with the hardened configuration and an allowlisted
// environment plus extraEnv.
func Command(ctx context.Context, dir string, extraEnv []string, args ...string) *exec.Cmd {
	full := make([]string, 0, len(hardenedArgs)+len(args))
	full = append(full, hardenedArgs...)
	full = append(full, args...)

	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Dir = dir
	cmd.Env = gitEnv(extraEnv)
	return cmd
}

var (
	objectFormats = map[string]bool{"sha1": true, "sha256": true}
	refStorages   = map[string]bool{"files": true, "reftable": true}
	boolValue     = regexp.MustCompile(`^(true|false)$`)
)

// SanitizeRepoConfig rebuilds workDir/.git/config from an allowlist so that
// none of the settings git would execute or follow survive from the workspace:
// only the repository format, a few filesystem probes from clone time, and the
// origin remote are kept.
//
// originURL, when non-empty, becomes remote.origin.url — callers that talk to
// the remote pass the URL the server cloned from, so a changed origin cannot
// redirect a fetch or push (and the token with it). With an empty originURL the
// existing origin URL is carried over as plain data, for local-only commands.
//
// A .git that is not a real directory (a symlink, or a gitfile pointing
// elsewhere) or that redirects to a common directory is refused.
func SanitizeRepoConfig(ctx context.Context, workDir, originURL string) error {
	gitDir := filepath.Join(workDir, ".git")
	info, err := os.Lstat(gitDir)
	if err != nil {
		return fmt.Errorf("inspecting .git: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("refusing to run git: %s is not a directory", gitDir)
	}
	if _, err := os.Lstat(filepath.Join(gitDir, "commondir")); err == nil {
		return errors.New("refusing to run git: .git/commondir redirects the repository")
	}
	if strings.ContainsAny(originURL, "\x00\r\n") {
		return errors.New("refusing to run git: invalid origin URL")
	}

	cfgPath := filepath.Join(gitDir, "config")
	get := func(key string, typ string) string {
		args := []string{"config", "--file", cfgPath, "--no-includes"}
		if typ != "" {
			args = append(args, "--type="+typ)
		}
		// Run outside the workspace so git does not discover the repository.
		out, err := Command(ctx, os.TempDir(), nil, append(args, "--get", key)...).Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}

	var b strings.Builder
	version := get("core.repositoryformatversion", "int")
	if version != "1" {
		version = "0"
	}
	b.WriteString("[core]\n")
	fmt.Fprintf(&b, "\trepositoryformatversion = %s\n", version)
	b.WriteString("\tbare = false\n")
	b.WriteString("\tlogallrefupdates = true\n")
	for _, key := range []string{"filemode", "ignorecase", "precomposeunicode", "symlinks"} {
		if v := get("core."+key, "bool"); boolValue.MatchString(v) {
			fmt.Fprintf(&b, "\t%s = %s\n", key, v)
		}
	}

	if version == "1" {
		var ext strings.Builder
		if v := get("extensions.objectformat", ""); objectFormats[v] {
			fmt.Fprintf(&ext, "\tobjectformat = %s\n", v)
		}
		if v := get("extensions.refstorage", ""); refStorages[v] {
			fmt.Fprintf(&ext, "\trefstorage = %s\n", v)
		}
		if ext.Len() > 0 {
			b.WriteString("[extensions]\n")
			b.WriteString(ext.String())
		}
	}

	origin := originURL
	if origin == "" {
		origin = get("remote.origin.url", "")
		if strings.ContainsAny(origin, "\x00\r\n") {
			origin = ""
		}
	}
	if origin != "" {
		b.WriteString("[remote \"origin\"]\n")
		fmt.Fprintf(&b, "\turl = %s\n", quoteConfigValue(origin))
		b.WriteString("\tfetch = +refs/heads/*:refs/remotes/origin/*\n")
	}

	// Replace the file rather than editing it: rename swaps out a symlinked
	// config instead of writing through it.
	tmp, err := os.CreateTemp(gitDir, "config.codeforge-*")
	if err != nil {
		return fmt.Errorf("writing git config: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(b.String()); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing git config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing git config: %w", err)
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return fmt.Errorf("writing git config: %w", err)
	}
	if err := os.Rename(tmp.Name(), cfgPath); err != nil {
		return fmt.Errorf("writing git config: %w", err)
	}
	return nil
}

// quoteConfigValue quotes a value for a git config file.
func quoteConfigValue(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	return `"` + v + `"`
}

// askPassTarget returns the scheme and host[:port] the askpass script may
// answer for, or empty strings when repoURL names no network host (a local
// path or file:// URL never prompts for credentials).
func askPassTarget(repoURL string) (scheme, host string) {
	u, err := url.Parse(repoURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return "", ""
	}
	if !validAskPassHost.MatchString(u.Host) {
		return "", ""
	}
	return u.Scheme, u.Host
}

var validAskPassHost = regexp.MustCompile(`^[A-Za-z0-9.\-]+(:[0-9]+)?$|^\[[0-9A-Fa-f:.]+\](:[0-9]+)?$`)

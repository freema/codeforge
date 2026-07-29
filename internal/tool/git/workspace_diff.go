package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// File statuses reported in a WorkspaceDiff.
const (
	FileStatusAdded    = "added"
	FileStatusModified = "modified"
	FileStatusDeleted  = "deleted"
	FileStatusRenamed  = "renamed"
)

// maxDiffBytes caps the unified diff payload returned to API clients at 1 MB.
const maxDiffBytes = 1 << 20

// FileDiff describes a single changed file in a workspace diff.
type FileDiff struct {
	Path      string `json:"path"`
	Status    string `json:"status"` // added | modified | deleted | renamed
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

// WorkspaceDiff is the full uncommitted diff of a workspace against HEAD,
// including untracked files.
type WorkspaceDiff struct {
	Files          []FileDiff `json:"files"`
	Diff           string     `json:"diff"` // unified diff, capped at maxDiffBytes
	TotalAdditions int        `json:"total_additions"`
	TotalDeletions int        `json:"total_deletions"`
	Truncated      bool       `json:"truncated"`
}

// Diff produces the uncommitted diff of the workspace at workDir against HEAD.
// Untracked files are registered with intent-to-add (git add -A -N) so they
// appear in the diff; this does not stage any content, so a later `git add -A`
// (e.g. during create-pr) is unaffected. The unified diff is capped at 1 MB,
// cut at a line boundary with Truncated set.
func Diff(ctx context.Context, workDir string) (*WorkspaceDiff, error) {
	// Intent-to-add so untracked files show up in `git diff HEAD`.
	if _, err := runGit(ctx, workDir, "add", "-A", "-N", "."); err != nil {
		return nil, err
	}

	// --no-renames keeps the unified diff, numstat, and porcelain status
	// consistent: a moved file is reported as one deletion + one addition
	// everywhere instead of a rename entry in some outputs only.
	unified, err := runGit(ctx, workDir, "diff", "--no-renames", "HEAD")
	if err != nil {
		return nil, err
	}

	numstat, err := runGit(ctx, workDir, "diff", "--no-renames", "--numstat", "HEAD")
	if err != nil {
		return nil, err
	}

	status, err := runGit(ctx, workDir, "status", "--porcelain")
	if err != nil {
		return nil, err
	}

	statuses := parsePorcelainStatuses(string(status))

	result := &WorkspaceDiff{Files: []FileDiff{}}
	for _, line := range strings.Split(strings.TrimRight(string(numstat), "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		additions, _ := strconv.Atoi(parts[0]) // "-" for binary files → 0
		deletions, _ := strconv.Atoi(parts[1])
		path := strings.Trim(parts[2], `"`)

		st, ok := statuses[path]
		if !ok {
			st = FileStatusModified
		}

		result.Files = append(result.Files, FileDiff{
			Path:      path,
			Status:    st,
			Additions: additions,
			Deletions: deletions,
		})
		result.TotalAdditions += additions
		result.TotalDeletions += deletions
	}

	diffText := string(unified)
	if len(diffText) > maxDiffBytes {
		diffText = truncateAtLineBoundary(diffText, maxDiffBytes)
		result.Truncated = true
	}
	result.Diff = diffText

	return result, nil
}

// runGit executes git with explicit args against the given directory
// (git -C <dir> ...) and returns stdout. On failure, stderr from the git
// process is folded into the returned error.
func runGit(ctx context.Context, workDir string, args ...string) ([]byte, error) {
	full := append([]string{"-C", workDir}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return out, fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return out, fmt.Errorf("git %s: %w", args[0], err)
	}
	return out, nil
}

// parsePorcelainStatuses maps file paths to added/modified/deleted/renamed
// from `git status --porcelain` output. Intent-to-add entries appear with an
// "A" in either column depending on git version, and untracked as "??" —
// all map to added. Staged renames ("R  old -> new") record both paths.
func parsePorcelainStatuses(out string) map[string]string {
	statuses := make(map[string]string)
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 4 {
			continue
		}
		xy := line[:2]
		path := line[3:]
		st := FileStatusModified
		switch {
		case strings.Contains(xy, "R"):
			st = FileStatusRenamed
			if idx := strings.Index(path, " -> "); idx >= 0 {
				statuses[strings.Trim(path[:idx], `"`)] = st
				path = path[idx+4:]
			}
		case xy == "??" || strings.Contains(xy, "A"):
			st = FileStatusAdded
		case strings.Contains(xy, "D"):
			st = FileStatusDeleted
		}
		statuses[strings.Trim(path, `"`)] = st
	}
	return statuses
}

// truncateAtLineBoundary cuts s to at most max bytes, ending at the last
// complete line within the limit.
func truncateAtLineBoundary(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := strings.LastIndexByte(s[:max], '\n')
	if cut < 0 {
		return s[:max]
	}
	return s[:cut+1]
}

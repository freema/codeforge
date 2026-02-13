package git

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// ChangesSummary holds git diff statistics after CLI execution.
type ChangesSummary struct {
	FilesModified int    `json:"files_modified"`
	FilesCreated  int    `json:"files_created"`
	FilesDeleted  int    `json:"files_deleted"`
	DiffStats     string `json:"diff_stats"`
}

// CalculateChanges computes a summary of workspace changes after CLI execution.
// It runs git status and git diff --shortstat (both staged and unstaged).
func CalculateChanges(ctx context.Context, workDir string) (*ChangesSummary, error) {
	// git status --porcelain for file counts
	statusCmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	statusCmd.Dir = workDir
	statusOut, err := statusCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status: %w", err)
	}

	var modified, created, deleted int
	for _, line := range strings.Split(string(statusOut), "\n") {
		if len(line) < 3 {
			continue
		}
		xy := line[:2]
		switch {
		case xy == "??" || xy == "A " || xy == " A":
			created++
		case xy == " D" || xy == "D ":
			deleted++
		case xy == " M" || xy == "M " || xy == "MM":
			modified++
		case xy == "AM":
			created++ // added then modified
		case xy == "RM" || xy == "R ":
			modified++ // renamed
		}
	}

	// git diff --shortstat for unstaged changes
	unstagedIns, unstagedDel := shortStat(ctx, workDir, false)

	// git diff --cached --shortstat for staged changes
	stagedIns, stagedDel := shortStat(ctx, workDir, true)

	diffStats := fmt.Sprintf("+%d -%d", unstagedIns+stagedIns, unstagedDel+stagedDel)

	return &ChangesSummary{
		FilesModified: modified,
		FilesCreated:  created,
		FilesDeleted:  deleted,
		DiffStats:     diffStats,
	}, nil
}

var shortStatRegex = regexp.MustCompile(`(\d+) insertions?\(\+\).*?(\d+) deletions?\(-\)|(\d+) insertions?\(\+\)|(\d+) deletions?\(-\)`)

func shortStat(ctx context.Context, workDir string, cached bool) (insertions, deletions int) {
	args := []string{"diff", "--shortstat"}
	if cached {
		args = []string{"diff", "--cached", "--shortstat"}
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return 0, 0
	}

	return parseShortStat(string(out))
}

// parseShortStat parses git diff --shortstat output like:
// "3 files changed, 142 insertions(+), 38 deletions(-)"
func parseShortStat(s string) (insertions, deletions int) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0
	}

	matches := shortStatRegex.FindStringSubmatch(s)
	if len(matches) == 0 {
		return 0, 0
	}

	// Full match: insertions and deletions
	if matches[1] != "" && matches[2] != "" {
		insertions, _ = strconv.Atoi(matches[1])
		deletions, _ = strconv.Atoi(matches[2])
		return
	}
	// Only insertions
	if matches[3] != "" {
		insertions, _ = strconv.Atoi(matches[3])
		return
	}
	// Only deletions
	if matches[4] != "" {
		deletions, _ = strconv.Atoi(matches[4])
		return
	}

	return 0, 0
}

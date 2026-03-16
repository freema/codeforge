package git

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DiffLineSet maps filename to the set of valid new-file line numbers in the PR diff.
type DiffLineSet map[string]map[int]bool

// Contains checks whether a file+line is within the PR diff hunks.
func (d DiffLineSet) Contains(file string, line int) bool {
	lines, ok := d[file]
	if !ok {
		return false
	}
	return lines[line]
}

// prFile represents a single file in the GitHub PR files response.
type prFile struct {
	Filename string `json:"filename"`
	Patch    string `json:"patch"`
}

// FetchPRDiffLines fetches PR files from the GitHub API and returns
// the set of valid new-file line numbers per file (lines in diff hunks).
func FetchPRDiffLines(ctx context.Context, client *http.Client, apiURL, owner, repo, token string, prNumber int) (DiffLineSet, error) {
	result := make(DiffLineSet)
	page := 1

	for {
		url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/files?per_page=100&page=%d",
			apiURL, owner, repo, prNumber, page)

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("creating PR files request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

		if client == nil {
			client = &http.Client{Timeout: 30 * time.Second}
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching PR files: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading PR files response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("PR files API returned %d: %s", resp.StatusCode, truncateBytes(body, 500))
		}

		var files []prFile
		if err := json.Unmarshal(body, &files); err != nil {
			return nil, fmt.Errorf("parsing PR files response: %w", err)
		}

		for _, f := range files {
			if f.Patch == "" {
				continue // binary file or empty diff
			}
			lines := ParsePatch(f.Patch)
			if len(lines) > 0 {
				result[f.Filename] = lines
			}
		}

		// No more pages if we got fewer than 100 files
		if len(files) < 100 {
			break
		}
		page++
	}

	slog.Debug("fetched PR diff lines", "files", len(result))
	return result, nil
}

// hunkHeaderRe matches unified diff hunk headers: @@ -old,count +new,count @@
var hunkHeaderRe = regexp.MustCompile(`^@@\s+-\d+(?:,\d+)?\s+\+(\d+)(?:,(\d+))?\s+@@`)

// ParsePatch parses a unified diff patch string and returns the set of
// valid new-file line numbers (added and context lines within hunks).
func ParsePatch(patch string) map[int]bool {
	lines := make(map[int]bool)
	if patch == "" {
		return lines
	}

	var newLine int
	inHunk := false

	for _, line := range strings.Split(patch, "\n") {
		if m := hunkHeaderRe.FindStringSubmatch(line); m != nil {
			start, _ := strconv.Atoi(m[1])
			newLine = start
			inHunk = true
			continue
		}

		if !inHunk {
			continue
		}

		if len(line) == 0 {
			// Empty line in diff = context line (newline with no prefix)
			lines[newLine] = true
			newLine++
			continue
		}

		switch line[0] {
		case '+':
			lines[newLine] = true
			newLine++
		case '-':
			// Deletion from old file — don't increment new line counter
		case ' ':
			lines[newLine] = true
			newLine++
		case '\\':
			// "\ No newline at end of file" — skip
		default:
			// Unknown prefix in hunk — treat as context
			lines[newLine] = true
			newLine++
		}
	}

	return lines
}

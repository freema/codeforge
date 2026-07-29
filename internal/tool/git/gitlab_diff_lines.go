package git

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// gitlabFileDiff holds per-file diff positions for a GitLab MR: the old path
// (differs from the new path on renames) and a map of new-file line numbers
// to old-file line numbers (0 = added line, >0 = context line).
type gitlabFileDiff struct {
	oldPath string
	lines   map[int]int
}

// gitlabDiffSet maps new file paths to their diff positions.
type gitlabDiffSet map[string]gitlabFileDiff

// contains reports whether file+line is within the MR diff hunks.
func (d gitlabDiffSet) contains(file string, line int) bool {
	fd, ok := d[file]
	if !ok {
		return false
	}
	_, ok = fd.lines[line]
	return ok
}

// gitlabMRDiff is a single file entry in the GitLab MR diffs response.
type gitlabMRDiff struct {
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path"`
	Diff    string `json:"diff"`
}

// fetchMRDiffLines fetches the MR diff from the GitLab API (paginated
// GET /merge_requests/{iid}/diffs) and returns per-file line positions used
// to validate inline comments and to resolve old_path/old_line for context
// lines.
func fetchMRDiffLines(ctx context.Context, client *http.Client, apiURL, projectPath, token string, mrIID int) (gitlabDiffSet, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	result := make(gitlabDiffSet)
	page := 1

	for {
		endpoint := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/diffs?per_page=100&page=%d",
			apiURL, projectPath, mrIID, page)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("creating MR diffs request: %w", err)
		}
		req.Header.Set("PRIVATE-TOKEN", token)

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching MR diffs: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading MR diffs response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("MR diffs API returned %d: %s", resp.StatusCode, truncateBytes(body, 500))
		}

		var files []gitlabMRDiff
		if err := json.Unmarshal(body, &files); err != nil {
			return nil, fmt.Errorf("parsing MR diffs response: %w", err)
		}

		for _, f := range files {
			if f.Diff == "" {
				continue // binary file or empty diff
			}
			lines := ParsePatchPositions(f.Diff)
			if len(lines) > 0 {
				result[f.NewPath] = gitlabFileDiff{oldPath: f.OldPath, lines: lines}
			}
		}

		// No more pages if we got fewer than 100 files
		if len(files) < 100 {
			break
		}
		page++
	}

	slog.Debug("fetched MR diff lines", "files", len(result))
	return result, nil
}

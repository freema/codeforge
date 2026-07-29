package git

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/freema/codeforge/internal/review"
)

// gitlabFake is a fake GitLab API server for MR review-posting tests.
type gitlabFake struct {
	mu             sync.Mutex
	versionsStatus int // 0 = 200 OK
	diffsStatus    int // 0 = 200 OK
	approveStatus  int // 0 = 201 Created
	diffs          []map[string]any
	discussions    []map[string]any // captured POST /discussions bodies
	approveCalls   int
}

func (f *gitlabFake) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()

		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/versions"):
			if f.versionsStatus != 0 {
				w.WriteHeader(f.versionsStatus)
				return
			}
			_ = json.NewEncoder(w).Encode([]map[string]string{{
				"base_commit_sha":  "base-sha",
				"start_commit_sha": "start-sha",
				"head_commit_sha":  "head-sha",
			}})
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/diffs"):
			if f.diffsStatus != 0 {
				w.WriteHeader(f.diffsStatus)
				return
			}
			_ = json.NewEncoder(w).Encode(f.diffs)
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/discussions"):
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.discussions = append(f.discussions, body)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"disc-1","notes":[{"id":42}]}`))
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/approve"):
			f.approveCalls++
			if f.approveStatus != 0 {
				w.WriteHeader(f.approveStatus)
				return
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	})
}

// gitlabTestRepo builds a RepoInfo pointing at the httptest server,
// preserving its scheme and port.
func gitlabTestRepo(t *testing.T, srvURL string) *RepoInfo {
	t.Helper()
	u, err := url.Parse(srvURL)
	if err != nil {
		t.Fatalf("parsing test server URL: %v", err)
	}
	return &RepoInfo{
		Provider: ProviderGitLab,
		Host:     u.Hostname(),
		Port:     u.Port(),
		Scheme:   u.Scheme,
		Owner:    "group",
		Repo:     "repo",
	}
}

// testFormatSummary records the issues folded into the summary comment.
func testFormatSummary(captured *[]review.ReviewIssue) func(*review.ReviewResult, []review.ReviewIssue) string {
	return func(_ *review.ReviewResult, issues []review.ReviewIssue) string {
		*captured = issues
		return "summary"
	}
}

func testFormatIssue(issue review.ReviewIssue) string {
	return "issue: " + issue.Description
}

func discussionPosition(t *testing.T, discussion map[string]any) map[string]any {
	t.Helper()
	pos, ok := discussion["position"].(map[string]any)
	if !ok {
		t.Fatalf("discussion has no position: %v", discussion)
	}
	return pos
}

func assertPositionField(t *testing.T, pos map[string]any, key string, want any) {
	t.Helper()
	if got := pos[key]; got != want {
		t.Errorf("position[%q] = %v, want %v", key, got, want)
	}
}

const mainGoPatch = `@@ -3,4 +3,5 @@
 ctx1
 ctx2
+added
 ctx3
 ctx4`

const renamedPatch = `@@ -1,3 +1,3 @@
 alpha
-beta
+gamma
 delta`

func TestPostMRReview_PositionPayloadAndDiffValidation(t *testing.T) {
	fake := &gitlabFake{diffs: []map[string]any{
		{"old_path": "main.go", "new_path": "main.go", "diff": mainGoPatch},
		{"old_path": "old_name.go", "new_path": "new_name.go", "diff": renamedPatch},
	}}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	repo := gitlabTestRepo(t, srv.URL)
	poster := &GitLabReviewPoster{client: srv.Client()}

	result := &review.ReviewResult{
		Verdict: review.VerdictComment,
		Issues: []review.ReviewIssue{
			{File: "main.go", Line: 5, Description: "on added line"},
			{File: "new_name.go", Line: 3, Description: "on context line"},
			{File: "main.go", Line: 99, Description: "outside diff"},
			{Description: "general note"},
		},
	}

	var summaryIssues []review.ReviewIssue
	post, err := poster.PostMRReview(context.Background(), repo, "test-token", 7, result,
		testFormatSummary(&summaryIssues), testFormatIssue)
	if err != nil {
		t.Fatalf("PostMRReview: %v", err)
	}

	if post.CommentsPosted != 2 {
		t.Errorf("CommentsPosted = %d, want 2", post.CommentsPosted)
	}

	wantURL := srv.URL + "/group/repo/-/merge_requests/7#note_42"
	if post.ReviewURL != wantURL {
		t.Errorf("ReviewURL = %q, want %q", post.ReviewURL, wantURL)
	}

	if len(fake.discussions) != 3 {
		t.Fatalf("expected 3 discussions (2 inline + summary), got %d", len(fake.discussions))
	}

	// Added line: new_path/new_line only, no old_path/old_line
	pos := discussionPosition(t, fake.discussions[0])
	assertPositionField(t, pos, "new_path", "main.go")
	assertPositionField(t, pos, "new_line", float64(5))
	assertPositionField(t, pos, "base_sha", "base-sha")
	assertPositionField(t, pos, "start_sha", "start-sha")
	assertPositionField(t, pos, "head_sha", "head-sha")
	if _, ok := pos["old_path"]; ok {
		t.Errorf("added-line position must not contain old_path: %v", pos)
	}
	if _, ok := pos["old_line"]; ok {
		t.Errorf("added-line position must not contain old_line: %v", pos)
	}

	// Context line on a renamed file: old_path/old_line resolved from the diff
	pos = discussionPosition(t, fake.discussions[1])
	assertPositionField(t, pos, "new_path", "new_name.go")
	assertPositionField(t, pos, "new_line", float64(3))
	assertPositionField(t, pos, "old_path", "old_name.go")
	assertPositionField(t, pos, "old_line", float64(3))

	// Summary: no position; contains the dropped and the general issue
	if _, ok := fake.discussions[2]["position"]; ok {
		t.Errorf("summary discussion must not have a position")
	}
	if len(summaryIssues) != 2 {
		t.Fatalf("summary issues = %d, want 2 (dropped + general): %v", len(summaryIssues), summaryIssues)
	}
	if summaryIssues[0].Description != "outside diff" || summaryIssues[1].Description != "general note" {
		t.Errorf("unexpected summary issues: %v", summaryIssues)
	}

	if fake.approveCalls != 0 {
		t.Errorf("approve called %d times on comment verdict, want 0", fake.approveCalls)
	}
}

func TestPostMRReview_VerdictApproveMapping(t *testing.T) {
	tests := []struct {
		name         string
		verdict      review.Verdict
		wantApproves int
	}{
		{"approve calls approvals API", review.VerdictApprove, 1},
		{"comment does not approve", review.VerdictComment, 0},
		{"request_changes does not approve", review.VerdictRequestChanges, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &gitlabFake{}
			srv := httptest.NewServer(fake.handler())
			defer srv.Close()

			repo := gitlabTestRepo(t, srv.URL)
			poster := &GitLabReviewPoster{client: srv.Client()}

			var summaryIssues []review.ReviewIssue
			_, err := poster.PostMRReview(context.Background(), repo, "test-token", 1,
				&review.ReviewResult{Verdict: tt.verdict},
				testFormatSummary(&summaryIssues), testFormatIssue)
			if err != nil {
				t.Fatalf("PostMRReview: %v", err)
			}
			if fake.approveCalls != tt.wantApproves {
				t.Errorf("approve calls = %d, want %d", fake.approveCalls, tt.wantApproves)
			}
		})
	}
}

func TestPostMRReview_ApproveFailureIsNotFatal(t *testing.T) {
	fake := &gitlabFake{approveStatus: http.StatusForbidden}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	repo := gitlabTestRepo(t, srv.URL)
	poster := &GitLabReviewPoster{client: srv.Client()}

	var summaryIssues []review.ReviewIssue
	post, err := poster.PostMRReview(context.Background(), repo, "test-token", 1,
		&review.ReviewResult{Verdict: review.VerdictApprove},
		testFormatSummary(&summaryIssues), testFormatIssue)
	if err != nil {
		t.Fatalf("PostMRReview must not fail when approve is rejected: %v", err)
	}
	if fake.approveCalls != 1 {
		t.Errorf("approve calls = %d, want 1", fake.approveCalls)
	}
	if len(fake.discussions) != 1 {
		t.Errorf("expected summary discussion to still be posted, got %d discussions", len(fake.discussions))
	}
	if post.ReviewURL == "" {
		t.Errorf("ReviewURL must be set even when approve fails")
	}
}

func TestPostMRReview_SummaryOnlyWhenVersionsFail(t *testing.T) {
	fake := &gitlabFake{versionsStatus: http.StatusInternalServerError}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	repo := gitlabTestRepo(t, srv.URL)
	poster := &GitLabReviewPoster{client: srv.Client()}

	result := &review.ReviewResult{
		Verdict: review.VerdictComment,
		Issues: []review.ReviewIssue{
			{File: "main.go", Line: 5, Description: "inline"},
			{Description: "general"},
		},
	}

	var summaryIssues []review.ReviewIssue
	post, err := poster.PostMRReview(context.Background(), repo, "test-token", 3, result,
		testFormatSummary(&summaryIssues), testFormatIssue)
	if err != nil {
		t.Fatalf("PostMRReview: %v", err)
	}

	if post.CommentsPosted != 0 {
		t.Errorf("CommentsPosted = %d, want 0", post.CommentsPosted)
	}
	if len(fake.discussions) != 1 {
		t.Fatalf("expected exactly 1 summary discussion, got %d", len(fake.discussions))
	}
	if _, ok := fake.discussions[0]["position"]; ok {
		t.Errorf("summary-only discussion must not have a position")
	}
	if len(summaryIssues) != 2 {
		t.Errorf("all issues must fold into the summary, got %v", summaryIssues)
	}
	wantURL := srv.URL + "/group/repo/-/merge_requests/3#note_42"
	if post.ReviewURL != wantURL {
		t.Errorf("ReviewURL = %q, want %q", post.ReviewURL, wantURL)
	}
}

func TestPostMRReview_DiffsFetchFailurePostsUnvalidated(t *testing.T) {
	fake := &gitlabFake{diffsStatus: http.StatusInternalServerError}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	repo := gitlabTestRepo(t, srv.URL)
	poster := &GitLabReviewPoster{client: srv.Client()}

	result := &review.ReviewResult{
		Verdict: review.VerdictComment,
		Issues:  []review.ReviewIssue{{File: "main.go", Line: 5, Description: "inline"}},
	}

	var summaryIssues []review.ReviewIssue
	post, err := poster.PostMRReview(context.Background(), repo, "test-token", 1, result,
		testFormatSummary(&summaryIssues), testFormatIssue)
	if err != nil {
		t.Fatalf("PostMRReview: %v", err)
	}

	if post.CommentsPosted != 1 {
		t.Errorf("CommentsPosted = %d, want 1 (posted without diff validation)", post.CommentsPosted)
	}
	if len(fake.discussions) != 2 {
		t.Fatalf("expected inline + summary discussions, got %d", len(fake.discussions))
	}
	pos := discussionPosition(t, fake.discussions[0])
	assertPositionField(t, pos, "new_path", "main.go")
	assertPositionField(t, pos, "new_line", float64(5))
	if _, ok := pos["old_path"]; ok {
		t.Errorf("position must not contain old_path without diff data: %v", pos)
	}
}

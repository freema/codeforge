package git

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/freema/codeforge/internal/review"
)

func TestPostPRReview_OwnPRFallbackToComment(t *testing.T) {
	var events []string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r) // diff fetch — nil diffLines path
			return
		}
		var body struct {
			Event string `json:"event"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		events = append(events, body.Event)
		if body.Event == "REQUEST_CHANGES" {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"message":"Unprocessable Entity","errors":["Can not request changes on your own pull request"]}`))
			return
		}
		_, _ = w.Write([]byte(`{"html_url":"https://example.invalid/review/1"}`))
	}))
	defer srv.Close()

	repo := &RepoInfo{Provider: ProviderGitHub, Host: strings.TrimPrefix(srv.URL, "https://"), Owner: "acme", Repo: "demo"}
	poster := &GitHubReviewPoster{client: srv.Client()}

	result := &review.ReviewResult{
		Verdict: review.VerdictRequestChanges,
		Issues:  []review.ReviewIssue{{Description: "issue without file"}},
	}

	post, err := poster.PostPRReview(context.Background(), repo, "test-token", 1, result,
		func(*review.ReviewResult, []review.ReviewIssue) string { return "summary" },
		func(review.ReviewIssue) string { return "issue" },
	)
	if err != nil {
		t.Fatalf("expected fallback to COMMENT to succeed, got error: %v", err)
	}
	if post.ReviewURL != "https://example.invalid/review/1" {
		t.Errorf("unexpected review URL: %s", post.ReviewURL)
	}
	if len(events) != 2 || events[0] != "REQUEST_CHANGES" || events[1] != "COMMENT" {
		t.Errorf("expected events [REQUEST_CHANGES COMMENT], got %v", events)
	}
}

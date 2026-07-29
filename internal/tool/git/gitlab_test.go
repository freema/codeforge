package git

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetMRStatus(t *testing.T) {
	tests := []struct {
		name       string
		response   string
		wantState  string
		wantMerged bool
		wantBy     string
	}{
		{
			name:      "opened maps to open",
			response:  `{"state":"opened","title":"Add feature"}`,
			wantState: "open",
		},
		{
			name:      "locked maps to open",
			response:  `{"state":"locked","title":"Add feature"}`,
			wantState: "open",
		},
		{
			name:      "closed stays closed",
			response:  `{"state":"closed","title":"Add feature"}`,
			wantState: "closed",
		},
		{
			name:       "merge_user preferred over deprecated merged_by",
			response:   `{"state":"merged","title":"Add feature","merge_user":{"username":"alice"},"merged_by":{"username":"bob"}}`,
			wantState:  "merged",
			wantMerged: true,
			wantBy:     "alice",
		},
		{
			name:       "merged_by fallback when merge_user is null",
			response:   `{"state":"merged","title":"Add feature","merge_user":null,"merged_by":{"username":"bob"}}`,
			wantState:  "merged",
			wantMerged: true,
			wantBy:     "bob",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tt.response))
			}))
			defer srv.Close()

			repo := gitlabTestRepo(t, srv.URL)
			creator := &GitLabMRCreator{client: srv.Client()}

			status, err := creator.GetMRStatus(context.Background(), repo, "test-token", 1)
			if err != nil {
				t.Fatalf("GetMRStatus: %v", err)
			}
			if status.State != tt.wantState {
				t.Errorf("State = %q, want %q", status.State, tt.wantState)
			}
			if status.Merged != tt.wantMerged {
				t.Errorf("Merged = %v, want %v", status.Merged, tt.wantMerged)
			}
			if status.MergedBy != tt.wantBy {
				t.Errorf("MergedBy = %q, want %q", status.MergedBy, tt.wantBy)
			}
		})
	}
}

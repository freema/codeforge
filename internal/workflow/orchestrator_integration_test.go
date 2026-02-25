//go:build integration

package workflow

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/freema/codeforge/internal/redisclient"
)

func getRedisURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("CODEFORGE_REDIS__URL")
	if url == "" {
		url = "redis://localhost:6379"
	}
	return url
}

func TestOrchestrator_Integration_FullRun_FetchOnly(t *testing.T) {
	db := setupTestDB(t)
	reg := NewSQLiteRegistry(db)
	store := NewSQLiteRunStore(db)

	rdb, err := redisclient.New(getRedisURL(t), "test:workflow:")
	if err != nil {
		t.Skipf("skipping: redis not available: %v", err)
	}
	defer rdb.Close()

	ctx := context.Background()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"title": "Test Issue",
			"body":  "Fix this please",
		})
	}))
	defer ts.Close()

	def := WorkflowDefinition{
		Name:        "fetch-only",
		Description: "test",
		Steps: []StepDefinition{
			{
				Name: "fetch_data",
				Type: StepTypeFetch,
				Config: mustJSON(FetchConfig{
					URL: ts.URL + "/api",
					Outputs: map[string]string{
						"title": "$.title",
						"body":  "$.body",
					},
				}),
			},
		},
	}
	_ = reg.Create(ctx, def)

	fetchExec := NewFetchExecutor(nil)
	streamer := NewStreamer(rdb, time.Hour)
	orch := NewOrchestrator(reg, store, fetchExec, nil, nil, streamer, rdb, OrchestratorConfig{
		ContextTTLHours:   24,
		MaxRunDurationSec: 60,
	})

	run, err := orch.CreateRun(ctx, "fetch-only", map[string]string{})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	// Dequeue to simulate what Start() does
	rdb.Unwrap().LPop(ctx, rdb.Key("queue:workflows"))

	// Execute directly
	orch.executeRun(ctx, run.ID)

	// Verify completed
	got, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.Status != RunStatusCompleted {
		t.Fatalf("expected completed, got %s (error: %s)", got.Status, got.Error)
	}

	// Verify steps
	steps, err := store.GetSteps(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetSteps: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(steps))
	}
	if steps[0].Result["title"] != "Test Issue" {
		t.Errorf("expected title 'Test Issue', got %q", steps[0].Result["title"])
	}
}

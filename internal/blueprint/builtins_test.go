package blueprint

import (
	"context"
	"encoding/json"
	"testing"
)

func TestSeedBuiltins_SeedsAll(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := SeedBuiltins(ctx, store); err != nil {
		t.Fatalf("SeedBuiltins: %v", err)
	}

	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != len(BuiltinBlueprints) {
		t.Fatalf("List = %d blueprints, want %d", len(list), len(BuiltinBlueprints))
	}
	seeded := make(map[string]*Blueprint, len(list))
	for _, b := range list {
		if !b.Builtin {
			t.Errorf("blueprint %q builtin = false, want true", b.Name)
		}
		seeded[b.Name] = b
	}
	for _, want := range []string{"sentry-fixer", "knowledge-update", "repo-review"} {
		if seeded[want] == nil {
			t.Errorf("builtin %q not seeded", want)
		}
	}
}

func TestSeedBuiltins_SentryFixerBuilds(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := SeedBuiltins(ctx, store); err != nil {
		t.Fatalf("SeedBuiltins: %v", err)
	}
	bp, err := store.GetByName(ctx, "sentry-fixer")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}

	reg := &fakeKeyRegistry{tokens: map[string]string{"sentry-key": "tok-123"}}
	req, err := Build(ctx, bp, map[string]string{
		"sentry_org":     "acme",
		"sentry_project": "api",
		"repo_url":       "https://example.com/acme/api.git",
		"key_name":       "sentry-key",
	}, reg)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if req.RepoURL != "https://example.com/acme/api.git" {
		t.Errorf("RepoURL = %q, want rendered repo_url", req.RepoURL)
	}
	if req.Config == nil {
		t.Fatal("Config = nil, want populated")
	}
	if !req.Config.AutoCreatePR {
		t.Error("AutoCreatePR = false, want true")
	}
	if req.Config.PRTitle != "fix: resolve Sentry errors" {
		t.Errorf("PRTitle = %q, want default pr_title", req.Config.PRTitle)
	}
	if len(req.Config.Tools) != 1 || req.Config.Tools[0].Name != "sentry" {
		t.Fatalf("Tools = %+v, want [sentry]", req.Config.Tools)
	}
	if got := req.Config.Tools[0].Config["auth_token"]; got != "tok-123" {
		t.Errorf("sentry auth_token = %q, want tok-123", got)
	}
}

func TestSeedBuiltins_KnowledgeAndReviewBuildWithEmptyPrompt(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := SeedBuiltins(ctx, store); err != nil {
		t.Fatalf("SeedBuiltins: %v", err)
	}

	tests := []struct {
		name        string
		wantType    string
		wantAutoPR  bool
		wantPRTitle string
	}{
		{name: "knowledge-update", wantType: "knowledge", wantAutoPR: true, wantPRTitle: "docs: update .codeforge knowledge"},
		{name: "repo-review", wantType: "review"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bp, err := store.GetByName(ctx, tt.name)
			if err != nil {
				t.Fatalf("GetByName: %v", err)
			}
			// focus is optional — empty prompt is valid for these session types.
			req, err := Build(ctx, bp, map[string]string{"repo_url": "https://example.com/o/r.git"}, nil)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			if req.SessionType != tt.wantType {
				t.Errorf("SessionType = %q, want %q", req.SessionType, tt.wantType)
			}
			if req.Prompt != "" {
				t.Errorf("Prompt = %q, want empty", req.Prompt)
			}
			gotAutoPR := req.Config != nil && req.Config.AutoCreatePR
			if gotAutoPR != tt.wantAutoPR {
				t.Errorf("AutoCreatePR = %v, want %v", gotAutoPR, tt.wantAutoPR)
			}
			if tt.wantPRTitle != "" && (req.Config == nil || req.Config.PRTitle != tt.wantPRTitle) {
				t.Errorf("PRTitle = %+v, want %q", req.Config, tt.wantPRTitle)
			}
		})
	}
}

func TestSeedBuiltins_Idempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := SeedBuiltins(ctx, store); err != nil {
		t.Fatalf("first SeedBuiltins: %v", err)
	}
	first, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	ids := make(map[string]string, len(first))
	for _, b := range first {
		ids[b.Name] = b.ID
	}

	if err := SeedBuiltins(ctx, store); err != nil {
		t.Fatalf("second SeedBuiltins: %v", err)
	}
	second, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(second) != len(BuiltinBlueprints) {
		t.Fatalf("List after reseed = %d blueprints, want %d", len(second), len(BuiltinBlueprints))
	}
	for _, b := range second {
		if ids[b.Name] != b.ID {
			t.Errorf("blueprint %q ID changed on reseed: %s → %s", b.Name, ids[b.Name], b.ID)
		}
	}
}

func TestSeedBuiltins_RemovesStale(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Simulate a builtin from an older release that is no longer in code.
	stale := &Blueprint{
		Name:    "old-builtin",
		Builtin: true,
		Request: json.RawMessage(`{"repo_url":"https://example.com/o/r.git"}`),
	}
	if err := store.Create(ctx, stale); err != nil {
		t.Fatalf("Create stale builtin: %v", err)
	}

	if err := SeedBuiltins(ctx, store); err != nil {
		t.Fatalf("SeedBuiltins: %v", err)
	}

	if _, err := store.GetByName(ctx, "old-builtin"); err == nil {
		t.Error("stale builtin still present after SeedBuiltins")
	}
	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != len(BuiltinBlueprints) {
		t.Errorf("List = %d blueprints, want %d", len(list), len(BuiltinBlueprints))
	}
}

func TestSeedBuiltins_NeverClobbersUserRows(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// A user blueprint holding a builtin name must survive seeding untouched.
	userRequest := json.RawMessage(`{"repo_url":"https://example.com/mine.git","prompt":"my own review"}`)
	user := &Blueprint{Name: "repo-review", Request: userRequest}
	if err := store.Create(ctx, user); err != nil {
		t.Fatalf("Create user blueprint: %v", err)
	}
	// A plain user blueprint must survive too.
	other := seedBlueprint(t, store, "my-preset")

	if err := SeedBuiltins(ctx, store); err != nil {
		t.Fatalf("SeedBuiltins: %v", err)
	}

	got, err := store.GetByName(ctx, "repo-review")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got.Builtin {
		t.Error("user blueprint was converted to builtin")
	}
	if string(got.Request) != string(userRequest) {
		t.Errorf("user blueprint request clobbered: %s", got.Request)
	}
	if _, err := store.Get(ctx, other.ID); err != nil {
		t.Errorf("unrelated user blueprint missing after seed: %v", err)
	}
}

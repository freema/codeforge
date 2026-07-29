package blueprint

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/freema/codeforge/internal/database"
)

// newLegacyDB opens an in-memory database with the full schema (including the
// legacy 006-era tables, which the SQL migrations still create) and seeds it
// with a preset, a user workflow definition, a builtin sentry-fixer-style
// definition, and workflow configs.
func newLegacyDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Legacy preset.
	mustExec(t, db,
		`INSERT INTO session_presets (id, name, description, request_json, created_at, updated_at)
		 VALUES ('p1', 'nightly-deps', 'bump deps', '{"repo_url":"https://example.com/org/repo.git","prompt":"update deps"}',
		         '2026-01-02T03:04:05.000000006Z', '2026-01-02T03:04:05.000000006Z')`)

	// Legacy user workflow definition (single session step).
	mustExec(t, db,
		`INSERT INTO workflow_definitions (name, description, builtin, steps_json, params_json)
		 VALUES ('deps-bump-flow', 'bump dependencies', 0,
		         '[{"name":"run","type":"session","config":{"repo_url":"{{.Params.repo_url}}","prompt":"bump {{.Params.thing}}","auto_create_pr":true,"pr_title":"chore: bump"}}]',
		         '[{"name":"repo_url","required":true},{"name":"thing","default":"deps"}]')`)

	// Legacy builtin definition row, as the old SeedBuiltins would have
	// written it (representative subset of the sentry-fixer step config).
	mustExec(t, db,
		`INSERT INTO workflow_definitions (name, description, builtin, steps_json, params_json)
		 VALUES ('sentry-fixer', 'fix sentry errors', 1,
		         '[{"name":"fix_bugs","type":"session","config":{"repo_url":"{{.Params.repo_url}}","prompt":"fix sentry for {{.Params.sentry_org}}/{{.Params.sentry_project}}","provider_key":"{{.Params.provider_key}}","tool_key_ref":"{{.Params.key_name}}","tools":[{"name":"sentry"}],"auto_create_pr":true,"pr_title":"{{.Params.pr_title}}","target_branch":"{{.Params.target_branch}}"}}]',
		         '[{"name":"sentry_org","required":true},{"name":"sentry_project","required":true},{"name":"repo_url","required":true},{"name":"key_name","required":true},{"name":"provider_key"},{"name":"max_issues","default":"5"},{"name":"pr_title","default":"fix: resolve Sentry errors"},{"name":"target_branch"}]')`)

	// Legacy saved config for the builtin workflow, with a timeout.
	mustExec(t, db,
		`INSERT INTO workflow_configs (name, workflow, params, timeout_seconds)
		 VALUES ('acme-sentry', 'sentry-fixer',
		         '{"sentry_org":"acme","sentry_project":"api","repo_url":"https://example.com/acme/api.git","key_name":"sentry-key"}',
		         900)`)

	return db
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatalf("counting %s: %v", table, err)
	}
	return n
}

func requestMap(t *testing.T, b *Blueprint) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b.Request, &m); err != nil {
		t.Fatalf("parsing request of %q: %v", b.Name, err)
	}
	return m
}

func TestMigrateLegacy(t *testing.T) {
	db := newLegacyDB(t)
	store := NewStore(db)
	ctx := context.Background()

	if err := MigrateLegacy(db); err != nil {
		t.Fatalf("MigrateLegacy: %v", err)
	}

	t.Run("preset copied verbatim", func(t *testing.T) {
		got, err := store.GetByName(ctx, "nightly-deps")
		if err != nil {
			t.Fatalf("GetByName: %v", err)
		}
		if got.ID != "p1" {
			t.Errorf("ID = %q, want legacy id p1", got.ID)
		}
		if got.Builtin {
			t.Error("Builtin = true, want false")
		}
		if got.Description != "bump deps" {
			t.Errorf("Description = %q, want bump deps", got.Description)
		}
		if len(got.Parameters) != 0 {
			t.Errorf("Parameters = %+v, want empty", got.Parameters)
		}
		req := requestMap(t, got)
		if req["repo_url"] != "https://example.com/org/repo.git" || req["prompt"] != "update deps" {
			t.Errorf("request = %v, want preset request", req)
		}
	})

	t.Run("user definition transformed", func(t *testing.T) {
		got, err := store.GetByName(ctx, "deps-bump-flow")
		if err != nil {
			t.Fatalf("GetByName: %v", err)
		}
		req := requestMap(t, got)
		if req["repo_url"] != "{{.Params.repo_url}}" {
			t.Errorf("repo_url = %v, want template placeholder kept intact", req["repo_url"])
		}
		if req["prompt"] != "bump {{.Params.thing}}" {
			t.Errorf("prompt = %v, want template placeholder kept intact", req["prompt"])
		}
		config, _ := req["config"].(map[string]any)
		if config == nil || config["auto_create_pr"] != true || config["pr_title"] != "chore: bump" {
			t.Errorf("config = %v, want auto_create_pr + pr_title", config)
		}
		if len(got.Parameters) != 2 || got.Parameters[0].Name != "repo_url" || !got.Parameters[0].Required ||
			got.Parameters[1].Name != "thing" || got.Parameters[1].Default != "deps" {
			t.Errorf("Parameters = %+v, want ported [repo_url required, thing default deps]", got.Parameters)
		}
	})

	t.Run("builtin definition not copied", func(t *testing.T) {
		// Only non-builtin definitions migrate — the builtin is replaced by
		// the seeded builtin blueprint (seeding is a separate startup step).
		if _, err := store.GetByName(ctx, "sentry-fixer"); err == nil {
			t.Error("builtin workflow definition was copied, want skipped")
		}
	})

	t.Run("config derived with defaults and timeout", func(t *testing.T) {
		got, err := store.GetByName(ctx, "acme-sentry")
		if err != nil {
			t.Fatalf("GetByName: %v", err)
		}
		if got.Builtin {
			t.Error("Builtin = true, want false")
		}

		req := requestMap(t, got)
		if req["tool_key_ref"] != "{{.Params.key_name}}" {
			t.Errorf("tool_key_ref = %v, want template placeholder kept intact", req["tool_key_ref"])
		}
		config, _ := req["config"].(map[string]any)
		if config == nil {
			t.Fatalf("request config missing: %v", req)
		}
		if config["timeout_seconds"] != float64(900) {
			t.Errorf("timeout_seconds = %v (%T), want baked-in JSON number 900", config["timeout_seconds"], config["timeout_seconds"])
		}
		if config["auto_create_pr"] != true {
			t.Errorf("auto_create_pr = %v, want true", config["auto_create_pr"])
		}

		defaults := make(map[string]ParameterDefinition, len(got.Parameters))
		for _, p := range got.Parameters {
			defaults[p.Name] = p
		}
		for name, want := range map[string]string{
			"sentry_org":     "acme",
			"sentry_project": "api",
			"repo_url":       "https://example.com/acme/api.git",
			"key_name":       "sentry-key",
		} {
			p, ok := defaults[name]
			if !ok {
				t.Errorf("parameter %q missing", name)
				continue
			}
			if p.Default != want {
				t.Errorf("parameter %q default = %q, want stored value %q", name, p.Default, want)
			}
			if !p.Required {
				t.Errorf("parameter %q required = false, want required kept (now satisfiable via default)", name)
			}
		}
		if defaults["max_issues"].Default != "5" {
			t.Errorf("max_issues default = %q, want parent default 5", defaults["max_issues"].Default)
		}

		// The derived blueprint must build without any input params.
		reg := &fakeKeyRegistry{tokens: map[string]string{"sentry-key": "tok-123"}}
		builtReq, err := Build(ctx, got, nil, reg)
		if err != nil {
			t.Fatalf("Build derived config blueprint: %v", err)
		}
		if builtReq.RepoURL != "https://example.com/acme/api.git" {
			t.Errorf("built RepoURL = %q, want config value", builtReq.RepoURL)
		}
		if builtReq.Config == nil || builtReq.Config.TimeoutSeconds != 900 {
			t.Errorf("built Config = %+v, want TimeoutSeconds 900", builtReq.Config)
		}
		if got := builtReq.Config.Tools[0].Config["auth_token"]; got != "tok-123" {
			t.Errorf("built auth_token = %q, want tok-123", got)
		}
	})

	t.Run("legacy tables left intact", func(t *testing.T) {
		if n := countRows(t, db, "session_presets"); n != 1 {
			t.Errorf("session_presets rows = %d, want 1", n)
		}
		if n := countRows(t, db, "workflow_definitions"); n != 2 {
			t.Errorf("workflow_definitions rows = %d, want 2", n)
		}
		if n := countRows(t, db, "workflow_configs"); n != 1 {
			t.Errorf("workflow_configs rows = %d, want 1", n)
		}
	})

	t.Run("second run is a no-op", func(t *testing.T) {
		before := countRows(t, db, "blueprints")
		if err := MigrateLegacy(db); err != nil {
			t.Fatalf("second MigrateLegacy: %v", err)
		}
		if after := countRows(t, db, "blueprints"); after != before {
			t.Errorf("blueprints after rerun = %d, want %d (no duplicates)", after, before)
		}
	})
}

func TestMigrateLegacy_NameCollisionSuffix(t *testing.T) {
	db := newLegacyDB(t)
	store := NewStore(db)
	ctx := context.Background()

	// A preset and a workflow definition share the name "dup" with different
	// content — the later one must be suffixed, not dropped or clobbered.
	mustExec(t, db,
		`INSERT INTO session_presets (id, name, description, request_json, created_at, updated_at)
		 VALUES ('p2', 'dup', '', '{"repo_url":"https://example.com/a.git"}',
		         '2026-01-02T03:04:05Z', '2026-01-02T03:04:05Z')`)
	mustExec(t, db,
		`INSERT INTO workflow_definitions (name, description, builtin, steps_json, params_json)
		 VALUES ('dup', 'different content', 0,
		         '[{"name":"run","type":"session","config":{"repo_url":"{{.Params.repo_url}}","prompt":"other"}}]',
		         '[{"name":"repo_url","required":true}]')`)

	if err := MigrateLegacy(db); err != nil {
		t.Fatalf("MigrateLegacy: %v", err)
	}

	dup, err := store.GetByName(ctx, "dup")
	if err != nil {
		t.Fatalf("GetByName dup: %v", err)
	}
	if len(dup.Parameters) != 0 {
		t.Errorf("dup should be the preset copy, got parameters %+v", dup.Parameters)
	}

	dup2, err := store.GetByName(ctx, "dup-2")
	if err != nil {
		t.Fatalf("GetByName dup-2: %v", err)
	}
	if len(dup2.Parameters) != 1 || dup2.Parameters[0].Name != "repo_url" {
		t.Errorf("dup-2 should be the definition copy, got parameters %+v", dup2.Parameters)
	}

	// Re-run: both keep matching by name+content, so nothing new appears.
	before := countRows(t, db, "blueprints")
	if err := MigrateLegacy(db); err != nil {
		t.Fatalf("second MigrateLegacy: %v", err)
	}
	if after := countRows(t, db, "blueprints"); after != before {
		t.Errorf("blueprints after rerun = %d, want %d", after, before)
	}
}

func TestMigrateLegacy_ConfigFallsBackToBuiltinBlueprint(t *testing.T) {
	db := newLegacyDB(t)
	store := NewStore(db)
	ctx := context.Background()

	// A config whose parent definition row is gone resolves against the
	// current builtin blueprints (their requests are the faithful conversion
	// of the old builtin definitions).
	mustExec(t, db,
		`INSERT INTO workflow_configs (name, workflow, params, timeout_seconds)
		 VALUES ('weekly-review', 'repo-review', '{"repo_url":"https://example.com/x/y.git"}', 600)`)

	if err := MigrateLegacy(db); err != nil {
		t.Fatalf("MigrateLegacy: %v", err)
	}

	got, err := store.GetByName(ctx, "weekly-review")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	req := requestMap(t, got)
	if req["session_type"] != "review" {
		t.Errorf("session_type = %v, want review (from builtin blueprint)", req["session_type"])
	}
	config, _ := req["config"].(map[string]any)
	if config == nil || config["timeout_seconds"] != float64(600) {
		t.Errorf("config = %v, want timeout_seconds 600 baked in", config)
	}

	defaults := make(map[string]string, len(got.Parameters))
	for _, p := range got.Parameters {
		defaults[p.Name] = p.Default
	}
	if defaults["repo_url"] != "https://example.com/x/y.git" {
		t.Errorf("repo_url default = %q, want stored config value", defaults["repo_url"])
	}
}

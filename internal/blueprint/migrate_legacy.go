package blueprint

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// maxNameSuffix bounds the "-2", "-3", … suffix search when resolving
// blueprint name collisions during the legacy migration.
const maxNameSuffix = 100

// legacyStep mirrors the legacy workflow StepDefinition JSON shape.
type legacyStep struct {
	Name   string          `json:"name"`
	Type   string          `json:"type"`
	Config json.RawMessage `json:"config"`
}

// legacyStepConfig mirrors the legacy workflow SessionStepConfig JSON shape.
type legacyStepConfig struct {
	RepoURL      string `json:"repo_url"`
	Prompt       string `json:"prompt"`
	SessionType  string `json:"session_type,omitempty"`
	ProviderKey  string `json:"provider_key,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`
	CLI          string `json:"cli,omitempty"`
	AIModel      string `json:"ai_model,omitempty"`
	SourceBranch string `json:"source_branch,omitempty"`
	TargetBranch string `json:"target_branch,omitempty"`

	PRNumber   int    `json:"pr_number,omitempty"`
	OutputMode string `json:"output_mode,omitempty"`

	AutoCreatePR bool   `json:"auto_create_pr,omitempty"`
	PRTitle      string `json:"pr_title,omitempty"`

	Tools      json.RawMessage `json:"tools,omitempty"`
	MCPServers json.RawMessage `json:"mcp_servers,omitempty"`
	ToolKeyRef string          `json:"tool_key_ref,omitempty"`
}

// legacyRow is a blueprint candidate derived from a legacy table row.
type legacyRow struct {
	id          string // preferred ID ("" = generate); only used when the base name is free
	name        string
	description string
	requestJSON string
	paramsJSON  string
	createdAt   string // "" = now
	updatedAt   string // "" = now
}

// MigrateLegacy copies legacy session presets, workflow definitions, and
// workflow configs into the blueprints table. It must be called once at
// startup AFTER SQL migrations have run.
//
// The migration is strictly non-destructive: legacy tables and their rows are
// left fully intact as a rollback archive (a future release may drop them).
// It is idempotent — a legacy row whose derived blueprint already exists
// (matched by name + content) is skipped, so re-runs are no-ops. Genuine name
// collisions with different content get a "-2", "-3", … suffix.
func MigrateLegacy(db *sql.DB) error {
	ctx := context.Background()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning legacy blueprint migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var presets, defs, cfgs int

	if ok, err := tableExists(ctx, tx, "session_presets"); err != nil {
		return err
	} else if ok {
		if presets, err = migratePresets(ctx, tx); err != nil {
			return fmt.Errorf("migrating session presets: %w", err)
		}
	}

	defsExist, err := tableExists(ctx, tx, "workflow_definitions")
	if err != nil {
		return err
	}
	if defsExist {
		if defs, err = migrateDefinitions(ctx, tx); err != nil {
			return fmt.Errorf("migrating workflow definitions: %w", err)
		}
	}

	if ok, err := tableExists(ctx, tx, "workflow_configs"); err != nil {
		return err
	} else if ok {
		if cfgs, err = migrateConfigs(ctx, tx, defsExist); err != nil {
			return fmt.Errorf("migrating workflow configs: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing legacy blueprint migration: %w", err)
	}

	if presets+defs+cfgs > 0 {
		slog.Info("migrated legacy rows to blueprints",
			"presets", presets, "workflow_definitions", defs, "workflow_configs", cfgs)
	}
	return nil
}

// migratePresets copies session_presets rows into blueprints verbatim
// (same id/name/description/request, no parameters).
func migratePresets(ctx context.Context, tx *sql.Tx) (int, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT id, name, description, request_json, created_at, updated_at FROM session_presets`)
	if err != nil {
		return 0, fmt.Errorf("reading session presets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var candidates []legacyRow
	for rows.Next() {
		var r legacyRow
		var description sql.NullString
		if err := rows.Scan(&r.id, &r.name, &description, &r.requestJSON, &r.createdAt, &r.updatedAt); err != nil {
			return 0, fmt.Errorf("scanning session preset: %w", err)
		}
		r.description = description.String
		r.paramsJSON = "[]"
		candidates = append(candidates, r)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterating session presets: %w", err)
	}

	return insertAll(ctx, tx, candidates)
}

// migrateDefinitions converts non-builtin workflow_definitions rows into
// blueprints, transforming the first session step's config into a request
// template ({{.Params.x}} placeholders are kept intact).
func migrateDefinitions(ctx context.Context, tx *sql.Tx) (int, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT name, description, steps_json, params_json FROM workflow_definitions WHERE builtin = 0`)
	if err != nil {
		return 0, fmt.Errorf("reading workflow definitions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type defRow struct {
		name, description, stepsJSON, paramsJSON string
	}
	var defRows []defRow
	for rows.Next() {
		var d defRow
		if err := rows.Scan(&d.name, &d.description, &d.stepsJSON, &d.paramsJSON); err != nil {
			return 0, fmt.Errorf("scanning workflow definition: %w", err)
		}
		defRows = append(defRows, d)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterating workflow definitions: %w", err)
	}

	candidates := make([]legacyRow, 0, len(defRows))
	for _, d := range defRows {
		requestJSON, err := stepsToRequest(d.stepsJSON, 0)
		if err != nil {
			slog.Warn("skipping legacy workflow definition", "name", d.name, "error", err)
			continue
		}
		paramsJSON, err := normalizeParams(d.paramsJSON)
		if err != nil {
			slog.Warn("skipping legacy workflow definition", "name", d.name, "error", err)
			continue
		}
		candidates = append(candidates, legacyRow{
			name:        d.name,
			description: d.description,
			requestJSON: requestJSON,
			paramsJSON:  paramsJSON,
		})
	}

	return insertAll(ctx, tx, candidates)
}

// migrateConfigs converts workflow_configs rows into blueprints. The parent
// definition's template is transformed into a request (with timeout_seconds
// baked into config when set), and its parameters get the config's stored
// values as defaults so the saved configuration keeps working as-is.
func migrateConfigs(ctx context.Context, tx *sql.Tx, defsExist bool) (int, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT name, workflow, params, timeout_seconds FROM workflow_configs`)
	if err != nil {
		return 0, fmt.Errorf("reading workflow configs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type cfgRow struct {
		name, workflow, paramsJSON string
		timeoutSeconds             int
	}
	var cfgRows []cfgRow
	for rows.Next() {
		var c cfgRow
		if err := rows.Scan(&c.name, &c.workflow, &c.paramsJSON, &c.timeoutSeconds); err != nil {
			return 0, fmt.Errorf("scanning workflow config: %w", err)
		}
		cfgRows = append(cfgRows, c)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterating workflow configs: %w", err)
	}

	candidates := make([]legacyRow, 0, len(cfgRows))
	for _, c := range cfgRows {
		var values map[string]string
		if err := json.Unmarshal([]byte(c.paramsJSON), &values); err != nil {
			slog.Warn("skipping legacy workflow config", "name", c.name, "error", err)
			continue
		}

		// The legacy runtime injected timeouts via the magic _timeout_seconds
		// param; blueprints bake them into request config.timeout_seconds.
		timeout := c.timeoutSeconds
		if timeout <= 0 {
			if v, err := strconv.Atoi(values["_timeout_seconds"]); err == nil && v > 0 {
				timeout = v
			}
		}

		description, requestJSON, params, err := lookupParent(ctx, tx, defsExist, c.workflow, timeout)
		if err != nil {
			slog.Warn("skipping legacy workflow config", "name", c.name, "workflow", c.workflow, "error", err)
			continue
		}

		paramsJSON, err := overrideDefaults(params, values)
		if err != nil {
			slog.Warn("skipping legacy workflow config", "name", c.name, "error", err)
			continue
		}

		candidates = append(candidates, legacyRow{
			name:        c.name,
			description: description,
			requestJSON: requestJSON,
			paramsJSON:  paramsJSON,
		})
	}

	return insertAll(ctx, tx, candidates)
}

// lookupParent resolves a config's parent workflow definition into a request
// template + parameters. It reads the workflow_definitions row (SeedBuiltins
// used to write builtin rows there too); when the row is gone it falls back
// to the current builtin blueprints, whose requests are the faithful
// conversion of the old builtin definitions.
func lookupParent(ctx context.Context, tx *sql.Tx, defsExist bool, workflowName string, timeoutSeconds int) (description, requestJSON, paramsJSON string, err error) {
	if defsExist {
		var stepsJSON, params string
		err := tx.QueryRowContext(ctx,
			`SELECT description, steps_json, params_json FROM workflow_definitions WHERE name = ?`,
			workflowName,
		).Scan(&description, &stepsJSON, &params)
		switch {
		case err == nil:
			requestJSON, err = stepsToRequest(stepsJSON, timeoutSeconds)
			if err != nil {
				return "", "", "", err
			}
			paramsJSON, err = normalizeParams(params)
			if err != nil {
				return "", "", "", err
			}
			return description, requestJSON, paramsJSON, nil
		case errors.Is(err, sql.ErrNoRows):
			// fall through to builtin blueprints
		default:
			return "", "", "", fmt.Errorf("reading parent definition %q: %w", workflowName, err)
		}
	}

	for i := range BuiltinBlueprints {
		if BuiltinBlueprints[i].Name != workflowName {
			continue
		}
		requestJSON, err := bakeTimeout(BuiltinBlueprints[i].Request, timeoutSeconds)
		if err != nil {
			return "", "", "", err
		}
		paramsJSON, err := marshalParams(BuiltinBlueprints[i].Parameters)
		if err != nil {
			return "", "", "", err
		}
		return BuiltinBlueprints[i].Description, requestJSON, paramsJSON, nil
	}

	return "", "", "", fmt.Errorf("parent workflow %q not found", workflowName)
}

// stepsToRequest transforms a legacy steps_json blob (first session step)
// into a CreateSessionRequest-shaped JSON template. String values keep their
// {{.Params.x}} placeholders — they are rendered at build time, not here.
func stepsToRequest(stepsJSON string, timeoutSeconds int) (string, error) {
	var steps []legacyStep
	if err := json.Unmarshal([]byte(stepsJSON), &steps); err != nil {
		return "", fmt.Errorf("parsing steps: %w", err)
	}

	var stepCfg *legacyStepConfig
	for i := range steps {
		if steps[i].Type == "session" {
			var cfg legacyStepConfig
			if err := json.Unmarshal(steps[i].Config, &cfg); err != nil {
				return "", fmt.Errorf("parsing session step config: %w", err)
			}
			stepCfg = &cfg
			break
		}
	}
	if stepCfg == nil {
		return "", errors.New("no session step found")
	}

	req := map[string]any{}
	setIfNotEmpty(req, "repo_url", stepCfg.RepoURL)
	setIfNotEmpty(req, "prompt", stepCfg.Prompt)
	setIfNotEmpty(req, "session_type", stepCfg.SessionType)
	setIfNotEmpty(req, "provider_key", stepCfg.ProviderKey)
	setIfNotEmpty(req, "access_token", stepCfg.AccessToken)
	setIfNotEmpty(req, "tool_key_ref", stepCfg.ToolKeyRef)

	config := map[string]any{}
	setIfNotEmpty(config, "cli", stepCfg.CLI)
	setIfNotEmpty(config, "ai_model", stepCfg.AIModel)
	setIfNotEmpty(config, "source_branch", stepCfg.SourceBranch)
	setIfNotEmpty(config, "target_branch", stepCfg.TargetBranch)
	setIfNotEmpty(config, "output_mode", stepCfg.OutputMode)
	setIfNotEmpty(config, "pr_title", stepCfg.PRTitle)
	if stepCfg.PRNumber > 0 {
		config["pr_number"] = stepCfg.PRNumber
	}
	if stepCfg.AutoCreatePR {
		config["auto_create_pr"] = true
	}
	if len(stepCfg.Tools) > 0 {
		config["tools"] = stepCfg.Tools
	}
	if len(stepCfg.MCPServers) > 0 {
		config["mcp_servers"] = stepCfg.MCPServers
	}
	if timeoutSeconds > 0 {
		config["timeout_seconds"] = timeoutSeconds
	}
	if len(config) > 0 {
		req["config"] = config
	}

	b, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshaling request template: %w", err)
	}
	return string(b), nil
}

// bakeTimeout sets config.timeout_seconds (as a JSON number) in a request
// template. A timeout <= 0 leaves the template unchanged.
func bakeTimeout(requestJSON json.RawMessage, timeoutSeconds int) (string, error) {
	if timeoutSeconds <= 0 {
		return string(requestJSON), nil
	}
	var m map[string]any
	if err := json.Unmarshal(requestJSON, &m); err != nil {
		return "", fmt.Errorf("parsing request template: %w", err)
	}
	config, _ := m["config"].(map[string]any)
	if config == nil {
		config = map[string]any{}
	}
	config["timeout_seconds"] = timeoutSeconds
	m["config"] = config
	b, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("marshaling request template: %w", err)
	}
	return string(b), nil
}

// normalizeParams re-marshals a legacy params_json blob so equal parameter
// lists always serialize identically (needed for content-match idempotency).
func normalizeParams(paramsJSON string) (string, error) {
	var params []ParameterDefinition
	if paramsJSON != "" {
		if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
			return "", fmt.Errorf("parsing parameters: %w", err)
		}
	}
	return marshalParams(params)
}

// overrideDefaults applies a config's stored param values onto the parent's
// parameter definitions as defaults (required parameters stay required but
// become satisfiable). Values for undeclared params — the legacy runtime fed
// every config param to the templates — are appended as optional parameters,
// sorted by name for determinism. The magic _timeout_seconds param is
// dropped: timeouts are baked into request config.timeout_seconds instead.
func overrideDefaults(paramsJSON string, values map[string]string) (string, error) {
	var params []ParameterDefinition
	if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
		return "", fmt.Errorf("parsing parent parameters: %w", err)
	}

	declared := make(map[string]bool, len(params))
	for i := range params {
		declared[params[i].Name] = true
		if v, ok := values[params[i].Name]; ok && v != "" {
			params[i].Default = v
		}
	}

	extras := make([]string, 0, len(values))
	for name, v := range values {
		if name == "_timeout_seconds" || declared[name] || v == "" {
			continue
		}
		extras = append(extras, name)
	}
	sort.Strings(extras)
	for _, name := range extras {
		params = append(params, ParameterDefinition{Name: name, Default: values[name]})
	}

	return marshalParams(params)
}

// insertAll inserts each candidate via insertUnique and returns how many
// rows were actually inserted (skipped duplicates don't count).
func insertAll(ctx context.Context, tx *sql.Tx, candidates []legacyRow) (int, error) {
	inserted := 0
	for i := range candidates {
		ok, err := insertUnique(ctx, tx, candidates[i])
		if err != nil {
			return inserted, err
		}
		if ok {
			inserted++
		}
	}
	return inserted, nil
}

// insertUnique inserts a derived blueprint under its name, or under a
// "-2", "-3", … suffixed name on collision. A row whose name AND content
// (request + parameters) already match is treated as previously migrated and
// skipped, which makes re-runs no-ops.
func insertUnique(ctx context.Context, tx *sql.Tx, r legacyRow) (bool, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	createdAt := r.createdAt
	if createdAt == "" {
		createdAt = now
	}
	updatedAt := r.updatedAt
	if updatedAt == "" {
		updatedAt = now
	}

	for n := 1; n <= maxNameSuffix; n++ {
		candidate := r.name
		if n > 1 {
			candidate = fmt.Sprintf("%s-%d", r.name, n)
		}

		var existingRequest, existingParams string
		err := tx.QueryRowContext(ctx,
			`SELECT request_json, params_json FROM blueprints WHERE name = ?`, candidate,
		).Scan(&existingRequest, &existingParams)
		if err == nil {
			if existingRequest == r.requestJSON && existingParams == r.paramsJSON {
				return false, nil // already migrated
			}
			continue // name taken by different content — try next suffix
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return false, fmt.Errorf("checking blueprint name %q: %w", candidate, err)
		}

		id := r.id
		if id == "" || n > 1 {
			id = uuid.NewString()
		} else {
			// The legacy ID could already be taken (e.g. the row was migrated
			// earlier under a name that has since changed) — fall back to a
			// fresh UUID rather than failing the whole migration.
			var count int
			if err := tx.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM blueprints WHERE id = ?`, id).Scan(&count); err != nil {
				return false, fmt.Errorf("checking blueprint id %q: %w", id, err)
			}
			if count > 0 {
				id = uuid.NewString()
			}
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO blueprints (id, name, description, builtin, request_json, params_json, created_at, updated_at)
			 VALUES (?, ?, ?, 0, ?, ?, ?, ?)`,
			id, candidate, r.description, r.requestJSON, r.paramsJSON, createdAt, updatedAt,
		); err != nil {
			return false, fmt.Errorf("inserting blueprint %q: %w", candidate, err)
		}
		return true, nil
	}
	return false, fmt.Errorf("could not find a free name for blueprint %q after %d attempts", r.name, maxNameSuffix)
}

func setIfNotEmpty(m map[string]any, key, value string) {
	if value != "" {
		m[key] = value
	}
}

func tableExists(ctx context.Context, tx *sql.Tx, name string) (bool, error) {
	var count int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name,
	).Scan(&count); err != nil {
		return false, fmt.Errorf("checking table %q: %w", name, err)
	}
	return count > 0, nil
}

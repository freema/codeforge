package blueprint

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/freema/codeforge/internal/keys"
)

// fakeKeyRegistry is a minimal keys.Registry for Build tests.
type fakeKeyRegistry struct {
	tokens map[string]string
}

func (f *fakeKeyRegistry) Create(_ context.Context, _ keys.Key) error { return nil }
func (f *fakeKeyRegistry) List(_ context.Context) ([]keys.Key, error) { return nil, nil }
func (f *fakeKeyRegistry) Delete(_ context.Context, _ string) error   { return nil }

func (f *fakeKeyRegistry) Resolve(_ context.Context, _, _ string) (string, error) {
	return "", errors.New("not implemented")
}

func (f *fakeKeyRegistry) Verify(_ context.Context, _ string) (*keys.VerifyResult, string, error) {
	return nil, "", errors.New("not implemented")
}

func (f *fakeKeyRegistry) ResolveByName(_ context.Context, name string) (string, string, error) {
	token, ok := f.tokens[name]
	if !ok {
		return "", "", errors.New("key not found")
	}
	return token, "sentry", nil
}

func (f *fakeKeyRegistry) ResolveFullByName(ctx context.Context, name string) (string, string, string, error) {
	token, provider, err := f.ResolveByName(ctx, name)
	return token, provider, "", err
}

func TestBuild_ParamMerge(t *testing.T) {
	bp := &Blueprint{
		Name:    "merge-test",
		Request: json.RawMessage(`{"repo_url":"{{.Params.repo_url}}","prompt":"{{.Params.focus}}","session_type":"{{.Params.kind}}"}`),
		Parameters: []ParameterDefinition{
			{Name: "repo_url", Required: true},
			{Name: "kind", Default: "review"},
			{Name: "focus", Required: false},
		},
	}

	tests := []struct {
		name       string
		params     map[string]string
		wantErr    bool
		wantMissed string // expected MissingParameterError.Parameter
		wantPrompt string
		wantType   string
	}{
		{
			name:       "missing required",
			params:     map[string]string{},
			wantErr:    true,
			wantMissed: "repo_url",
		},
		{
			name:       "defaults and empty optional applied",
			params:     map[string]string{"repo_url": "https://example.com/o/r.git"},
			wantPrompt: "",
			wantType:   "review",
		},
		{
			name:       "explicit values win over defaults",
			params:     map[string]string{"repo_url": "https://example.com/o/r.git", "kind": "feature", "focus": "security"},
			wantPrompt: "security",
			wantType:   "feature",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := Build(context.Background(), bp, tt.params, nil)
			if tt.wantErr {
				var missing *MissingParameterError
				if !errors.As(err, &missing) {
					t.Fatalf("Build error = %v, want MissingParameterError", err)
				}
				if missing.Parameter != tt.wantMissed || missing.Blueprint != bp.Name {
					t.Errorf("MissingParameterError = %+v, want parameter %q", missing, tt.wantMissed)
				}
				return
			}
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			if req.RepoURL != tt.params["repo_url"] {
				t.Errorf("RepoURL = %q, want %q", req.RepoURL, tt.params["repo_url"])
			}
			if req.Prompt != tt.wantPrompt {
				t.Errorf("Prompt = %q, want %q", req.Prompt, tt.wantPrompt)
			}
			if req.SessionType != tt.wantType {
				t.Errorf("SessionType = %q, want %q", req.SessionType, tt.wantType)
			}
		})
	}
}

func TestBuild_UndeclaredParamInTemplate(t *testing.T) {
	bp := &Blueprint{
		Name:    "bad-template",
		Request: json.RawMessage(`{"repo_url":"{{.Params.undeclared}}"}`),
	}
	if _, err := Build(context.Background(), bp, map[string]string{}, nil); err == nil {
		t.Fatal("Build with undeclared param in template should fail (missingkey=error)")
	}
}

func TestBuild_StringLeafRendering_SpecialCharacters(t *testing.T) {
	// Param values with newlines, quotes, and backslashes must survive intact:
	// each string leaf is rendered individually, never the JSON blob as one
	// template, so they cannot break the document.
	bp := &Blueprint{
		Name:    "special-chars",
		Request: json.RawMessage(`{"repo_url":"https://example.com/o/r.git","prompt":"Fix this:\n{{.Params.details}}"}`),
		Parameters: []ParameterDefinition{
			{Name: "details", Required: true},
		},
	}
	details := "line1\nline2 \"quoted\" and \\backslash\\ end"

	req, err := Build(context.Background(), bp, map[string]string{"details": details}, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	want := "Fix this:\n" + details
	if req.Prompt != want {
		t.Errorf("Prompt = %q, want %q", req.Prompt, want)
	}
}

func TestBuild_NonStringLeavesPreserved(t *testing.T) {
	bp := &Blueprint{
		Name:    "leaves",
		Request: json.RawMessage(`{"repo_url":"https://example.com/o/r.git","config":{"timeout_seconds":900,"auto_create_pr":true,"pr_number":42}}`),
	}
	req, err := Build(context.Background(), bp, nil, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if req.Config == nil {
		t.Fatal("Config = nil, want populated")
	}
	if req.Config.TimeoutSeconds != 900 {
		t.Errorf("TimeoutSeconds = %d, want 900", req.Config.TimeoutSeconds)
	}
	if !req.Config.AutoCreatePR {
		t.Error("AutoCreatePR = false, want true")
	}
	if req.Config.PRNumber != 42 {
		t.Errorf("PRNumber = %d, want 42", req.Config.PRNumber)
	}
}

func TestBuild_ToolKeyRefInjection(t *testing.T) {
	bp := &Blueprint{
		Name: "with-tools",
		Request: json.RawMessage(`{
			"repo_url": "https://example.com/o/r.git",
			"tool_key_ref": "{{.Params.key_name}}",
			"config": {
				"tools": [
					{"name": "sentry"},
					{"name": "other", "config": {"auth_token": "keep-me"}}
				]
			}
		}`),
		Parameters: []ParameterDefinition{
			{Name: "key_name", Required: true},
		},
	}
	reg := &fakeKeyRegistry{tokens: map[string]string{"sentry-key": "tok-123"}}

	req, err := Build(context.Background(), bp, map[string]string{"key_name": "sentry-key"}, reg)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if req.Config == nil || len(req.Config.Tools) != 2 {
		t.Fatalf("Config.Tools = %+v, want 2 tools", req.Config)
	}
	if got := req.Config.Tools[0].Config["auth_token"]; got != "tok-123" {
		t.Errorf("tools[0] auth_token = %q, want injected tok-123", got)
	}
	if got := req.Config.Tools[1].Config["auth_token"]; got != "keep-me" {
		t.Errorf("tools[1] auth_token = %q, want existing keep-me preserved", got)
	}
}

func TestBuild_ToolKeyRefResolveFailure(t *testing.T) {
	// A failed key lookup logs a warning and skips injection — it never
	// fails the build (matches the legacy workflow behavior).
	bp := &Blueprint{
		Name: "with-tools",
		Request: json.RawMessage(`{
			"repo_url": "https://example.com/o/r.git",
			"tool_key_ref": "missing-key",
			"config": {"tools": [{"name": "sentry"}]}
		}`),
	}
	reg := &fakeKeyRegistry{tokens: map[string]string{}}

	req, err := Build(context.Background(), bp, nil, reg)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, ok := req.Config.Tools[0].Config["auth_token"]; ok {
		t.Error("auth_token injected despite failed key resolution")
	}
}

func TestBuild_EmptyToolKeyRefSkipsRegistry(t *testing.T) {
	bp := &Blueprint{
		Name: "no-key",
		Request: json.RawMessage(`{
			"repo_url": "https://example.com/o/r.git",
			"tool_key_ref": "{{.Params.key_name}}",
			"config": {"tools": [{"name": "sentry"}]}
		}`),
		Parameters: []ParameterDefinition{
			{Name: "key_name", Required: false},
		},
	}
	// key_name unset → renders "" → no lookup, no injection.
	req, err := Build(context.Background(), bp, nil, &fakeKeyRegistry{tokens: map[string]string{"": "boom"}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, ok := req.Config.Tools[0].Config["auth_token"]; ok {
		t.Error("auth_token injected for empty tool_key_ref")
	}
}

func TestBuild_EmptyRequest(t *testing.T) {
	bp := &Blueprint{Name: "empty"}
	if _, err := Build(context.Background(), bp, nil, nil); err == nil {
		t.Fatal("Build with empty request should fail")
	}
}

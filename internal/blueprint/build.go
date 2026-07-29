package blueprint

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/freema/codeforge/internal/keys"
	"github.com/freema/codeforge/internal/session"
)

// MissingParameterError reports a required blueprint parameter that was
// neither provided by the caller nor covered by a default.
type MissingParameterError struct {
	Blueprint string
	Parameter string
}

func (e *MissingParameterError) Error() string {
	return fmt.Sprintf("missing required parameter %q for blueprint %q", e.Parameter, e.Blueprint)
}

// Build renders a blueprint's request template with the given params and
// returns a CreateSessionRequest ready to submit to the session service.
//
// Parameters are merged with declared defaults (missing required → typed
// error; declared-but-missing optional → ""), every string leaf of the
// request JSON is rendered individually through the template engine, and the
// optional top-level "tool_key_ref" field is resolved via the key registry
// into each tool's auth_token config.
func Build(ctx context.Context, bp *Blueprint, params map[string]string, keyReg keys.Registry) (*session.CreateSessionRequest, error) {
	if len(bp.Request) == 0 {
		return nil, fmt.Errorf("blueprint %q has no request template", bp.Name)
	}

	merged, err := mergeParams(bp, params)
	if err != nil {
		return nil, err
	}

	var tree any
	if err := json.Unmarshal(bp.Request, &tree); err != nil {
		return nil, fmt.Errorf("parsing blueprint %q request: %w", bp.Name, err)
	}

	// Render each string leaf on its own — never the whole blob as one
	// template — so parameter values containing quotes or newlines cannot
	// break the JSON.
	tctx := TemplateContext{Params: merged}
	rendered, err := renderTree(tree, tctx)
	if err != nil {
		return nil, fmt.Errorf("rendering blueprint %q request: %w", bp.Name, err)
	}

	blob, err := json.Marshal(rendered)
	if err != nil {
		return nil, fmt.Errorf("marshaling rendered request for blueprint %q: %w", bp.Name, err)
	}

	var req struct {
		session.CreateSessionRequest
		ToolKeyRef string `json:"tool_key_ref"`
	}
	if err := json.Unmarshal(blob, &req); err != nil {
		return nil, fmt.Errorf("decoding rendered request for blueprint %q: %w", bp.Name, err)
	}

	// Resolve tool key reference — inject auth_token from the key registry
	// into each tool's config (existing auth_token values win).
	if req.ToolKeyRef != "" && keyReg != nil && req.Config != nil && len(req.Config.Tools) > 0 {
		token, _, err := keyReg.ResolveByName(ctx, req.ToolKeyRef)
		if err != nil {
			slog.Warn("failed to resolve tool key ref", "key", req.ToolKeyRef, "error", err)
		} else {
			for i := range req.Config.Tools {
				if req.Config.Tools[i].Config == nil {
					req.Config.Tools[i].Config = make(map[string]string)
				}
				if _, exists := req.Config.Tools[i].Config["auth_token"]; !exists {
					req.Config.Tools[i].Config["auth_token"] = token
				}
			}
		}
	}

	out := req.CreateSessionRequest
	return &out, nil
}

// mergeParams overlays caller params onto the blueprint's declared
// parameters. Declared parameters missing from the input get their default;
// missing required parameters are an error; declared optional parameters
// without a default render as "" so missingkey=error never fires for them.
func mergeParams(bp *Blueprint, params map[string]string) (map[string]string, error) {
	merged := make(map[string]string, len(params)+len(bp.Parameters))
	for k, v := range params {
		merged[k] = v
	}
	for _, p := range bp.Parameters {
		if _, ok := merged[p.Name]; ok {
			continue
		}
		switch {
		case p.Default != "":
			merged[p.Name] = p.Default
		case p.Required:
			return nil, &MissingParameterError{Blueprint: bp.Name, Parameter: p.Name}
		default:
			merged[p.Name] = ""
		}
	}
	return merged, nil
}

// renderTree walks an unmarshaled JSON tree and renders every string leaf
// through the template engine. Non-string leaves pass through untouched.
func renderTree(v any, tctx TemplateContext) (any, error) {
	switch node := v.(type) {
	case string:
		return Render(node, tctx)
	case map[string]any:
		out := make(map[string]any, len(node))
		for k, child := range node {
			r, err := renderTree(child, tctx)
			if err != nil {
				return nil, fmt.Errorf("field %q: %w", k, err)
			}
			out[k] = r
		}
		return out, nil
	case []any:
		out := make([]any, len(node))
		for i, child := range node {
			r, err := renderTree(child, tctx)
			if err != nil {
				return nil, fmt.Errorf("index %d: %w", i, err)
			}
			out[i] = r
		}
		return out, nil
	default:
		return v, nil
	}
}

// Package blueprint implements the unified "Blueprint" concept: a named,
// reusable session request template with optional input parameters. A preset
// is a blueprint without parameters; a workflow is a blueprint with
// parameters. Running a blueprint merges parameters, renders every string
// leaf of the request template, and submits the result as a normal
// session.CreateSessionRequest.
package blueprint

import (
	"encoding/json"
	"time"
)

// Blueprint is a named, reusable session request template.
type Blueprint struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Builtin     bool   `json:"builtin"`

	// Request is a session.CreateSessionRequest-shaped JSON template. String
	// values may contain Go template expressions over {{.Params.x}}. One
	// extra optional top-level field "tool_key_ref" names a key registry
	// entry whose token is injected into each tool's auth_token config when
	// the blueprint is built.
	Request json.RawMessage `json:"request"`

	// Parameters declares the blueprint's inputs. Empty for presets.
	Parameters []ParameterDefinition `json:"parameters"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ParameterDefinition describes a blueprint input parameter.
type ParameterDefinition struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Default  string `json:"default,omitempty"`
}

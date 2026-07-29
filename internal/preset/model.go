// Package preset implements saved, reusable session configurations: named
// session.CreateSessionRequest templates that can be run on demand.
package preset

import (
	"encoding/json"
	"time"
)

// Preset is a named, reusable session request template.
type Preset struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`

	// Request is the stored session.CreateSessionRequest JSON used as the
	// template each time the preset is run (prompt may be omitted — the run
	// endpoint can supply it as an override).
	Request json.RawMessage `json:"request"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

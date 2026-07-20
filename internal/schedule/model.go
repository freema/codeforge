// Package schedule implements recurring (cron) sessions: stored session
// templates that a background scheduler fires on their cron expression.
package schedule

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

// Schedule is a recurring session template.
type Schedule struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Cron    string `json:"cron"` // standard 5-field cron or @daily/@every descriptors
	Enabled bool   `json:"enabled"`

	// SessionRequest is the stored session.CreateSessionRequest JSON used
	// verbatim each time the schedule fires.
	SessionRequest json.RawMessage `json:"session_request"`

	LastRunAt     *time.Time `json:"last_run_at,omitempty"`
	LastSessionID string     `json:"last_session_id,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`

	// NextRunAt is computed from Cron on read, never stored.
	NextRunAt *time.Time `json:"next_run_at,omitempty"`
}

// cronParser accepts the standard 5-field spec plus @daily/@weekly/@every descriptors.
var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// ParseCron validates a cron expression and returns its schedule.
func ParseCron(spec string) (cron.Schedule, error) {
	s, err := cronParser.Parse(spec)
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression %q: %w", spec, err)
	}
	return s, nil
}

// FillNextRun computes NextRunAt relative to now (zero for disabled or
// unparseable schedules).
func (s *Schedule) FillNextRun(now time.Time) {
	if !s.Enabled {
		s.NextRunAt = nil
		return
	}
	spec, err := ParseCron(s.Cron)
	if err != nil {
		s.NextRunAt = nil
		return
	}
	next := spec.Next(now)
	s.NextRunAt = &next
}

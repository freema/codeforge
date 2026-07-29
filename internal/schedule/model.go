// Package schedule implements recurring (cron) sessions: stored session
// templates that a background scheduler fires on their cron expression.
package schedule

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

// Run trigger values — what caused a schedule occurrence.
const (
	TriggerCron   = "cron"
	TriggerManual = "manual"
)

// Run status values — lifecycle of a single schedule occurrence.
const (
	RunFired            = "fired"             // session created
	RunFireFailed       = "fire_failed"       // session creation failed
	RunSkippedOverlap   = "skipped_overlap"   // previous session still active
	RunSessionCompleted = "session_completed" // fired session finished successfully
	RunSessionFailed    = "session_failed"    // fired session failed
)

// MaxConsecutiveFailures is the failure count at which a schedule is
// automatically disabled.
const MaxConsecutiveFailures = 5

// Schedule is a recurring session template.
type Schedule struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Cron    string `json:"cron"` // standard 5-field cron or @daily/@every descriptors
	Enabled bool   `json:"enabled"`

	// Timezone is an optional IANA timezone name (e.g. "Europe/Prague") the
	// cron expression is evaluated in. Empty = process-local time.
	Timezone string `json:"timezone,omitempty"`

	// SessionRequest is the stored session.CreateSessionRequest JSON used
	// verbatim each time the schedule fires. Empty for blueprint-backed
	// schedules — a schedule is EITHER inline (SessionRequest set) OR
	// blueprint-backed (BlueprintID set), never both.
	SessionRequest json.RawMessage `json:"session_request,omitempty"`

	// BlueprintID references the blueprint rendered on each firing (with
	// BlueprintParams). Empty for inline schedules.
	BlueprintID string `json:"blueprint_id,omitempty"`

	// BlueprintParams are the parameter values passed to blueprint.Build when
	// the schedule fires. Only meaningful when BlueprintID is set.
	BlueprintParams map[string]string `json:"blueprint_params,omitempty"`

	// ConsecutiveFailures counts fire/session failures since the last success;
	// at MaxConsecutiveFailures the schedule is auto-disabled with DisabledReason.
	ConsecutiveFailures int    `json:"consecutive_failures"`
	DisabledReason      string `json:"disabled_reason,omitempty"`

	LastRunAt     *time.Time `json:"last_run_at,omitempty"`
	LastSessionID string     `json:"last_session_id,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`

	// NextRunAt is computed from Cron on read, never stored.
	NextRunAt *time.Time `json:"next_run_at,omitempty"`
}

// Run is one recorded occurrence of a schedule (fired, failed or skipped),
// later stamped with the fired session's terminal outcome.
type Run struct {
	ID         string    `json:"id"`
	ScheduleID string    `json:"schedule_id"`
	SessionID  string    `json:"session_id,omitempty"`
	Trigger    string    `json:"trigger"` // TriggerCron | TriggerManual
	Status     string    `json:"status"`  // Run* status constants
	Error      string    `json:"error,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// cronParser accepts the standard 5-field spec plus @daily/@weekly/@every descriptors.
var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// ParseCron validates a cron expression and returns its schedule evaluated in
// process-local time.
func ParseCron(spec string) (cron.Schedule, error) {
	s, err := cronParser.Parse(spec)
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression %q: %w", spec, err)
	}
	return s, nil
}

// ParseCronIn validates a cron expression and returns its schedule evaluated
// in the given IANA timezone (process-local when empty).
func ParseCronIn(spec, timezone string) (cron.Schedule, error) {
	if timezone == "" {
		return ParseCron(spec)
	}
	if err := ValidateTimezone(timezone); err != nil {
		return nil, err
	}
	// robfig/cron evaluates a CRON_TZ= prefix in the named location.
	s, err := cronParser.Parse("CRON_TZ=" + timezone + " " + spec)
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression %q: %w", spec, err)
	}
	return s, nil
}

// ValidateTimezone checks an IANA timezone name (empty = process local, valid).
func ValidateTimezone(timezone string) error {
	if timezone == "" {
		return nil
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return fmt.Errorf("invalid timezone %q: %w", timezone, err)
	}
	return nil
}

// CronSchedule returns the schedule's cron evaluated in its timezone.
func (s *Schedule) CronSchedule() (cron.Schedule, error) {
	return ParseCronIn(s.Cron, s.Timezone)
}

// FillNextRun computes NextRunAt relative to now (zero for disabled or
// unparseable schedules).
func (s *Schedule) FillNextRun(now time.Time) {
	if !s.Enabled {
		s.NextRunAt = nil
		return
	}
	spec, err := s.CronSchedule()
	if err != nil {
		s.NextRunAt = nil
		return
	}
	next := spec.Next(now)
	s.NextRunAt = &next
}

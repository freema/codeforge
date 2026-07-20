// Package notify posts terminal-session notifications to chat webhooks
// (Slack incoming webhooks, Discord webhooks and Microsoft Teams webhooks).
// Best-effort: delivery failures are logged and never affect session processing.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/freema/codeforge/internal/config"
)

// Event types emitted by the executor.
const (
	EventSessionCompleted = "session_completed"
	EventSessionFailed    = "session_failed"
	EventPRCreated        = "pr_created"
	EventReviewCompleted  = "review_completed"
)

// Event describes a terminal session state worth telling a human about.
type Event struct {
	Type            string
	SessionID       string
	SessionType     string
	RepoURL         string
	Error           string
	DurationSeconds int
	InputTokens     int
	OutputTokens    int
	ReviewScore     int
}

// Notifier delivers events to the configured chat webhooks.
type Notifier struct {
	slackURL   string
	discordURL string
	teamsURL   string
	uiBaseURL  string
	events     map[string]bool // empty = all events
	client     *http.Client
}

// New builds a Notifier from config. Returns nil when no webhook URL is
// configured, so callers can treat notifications as absent.
func New(cfg config.NotificationsConfig) *Notifier {
	if cfg.SlackWebhookURL == "" && cfg.DiscordWebhookURL == "" && cfg.TeamsWebhookURL == "" {
		return nil
	}
	// Entries may arrive comma-joined (env var) or as a YAML list — accept both.
	events := make(map[string]bool, len(cfg.Events))
	for _, e := range cfg.Events {
		for _, part := range strings.Split(e, ",") {
			if p := strings.TrimSpace(part); p != "" {
				events[p] = true
			}
		}
	}
	return &Notifier{
		slackURL:   cfg.SlackWebhookURL,
		discordURL: cfg.DiscordWebhookURL,
		teamsURL:   cfg.TeamsWebhookURL,
		uiBaseURL:  strings.TrimRight(cfg.UIBaseURL, "/"),
		events:     events,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

// Notify formats and posts the event to every configured webhook.
// Errors are logged, never returned — notifications must not fail sessions.
func (n *Notifier) Notify(ctx context.Context, ev Event) {
	if n == nil {
		return
	}
	if len(n.events) > 0 && !n.events[ev.Type] {
		return
	}

	text := n.format(ev)
	if n.slackURL != "" {
		n.post(ctx, n.slackURL, map[string]string{"text": text}, "slack", ev.SessionID)
	}
	if n.discordURL != "" {
		n.post(ctx, n.discordURL, map[string]string{"content": text}, "discord", ev.SessionID)
	}
	if n.teamsURL != "" {
		n.post(ctx, n.teamsURL, teamsPayload(n.teamsURL, text), "teams", ev.SessionID)
	}
}

// teamsPayload builds the Teams webhook payload for the given webhook URL.
// Classic incoming webhooks (hosted on webhook.office.com) accept a plain
// {"text": ...} message; Power Automate / Teams Workflows endpoints (e.g.
// *.logic.azure.com) expect an Adaptive Card envelope.
func teamsPayload(webhookURL, text string) map[string]any {
	if u, err := url.Parse(webhookURL); err == nil && strings.Contains(u.Host, "webhook.office.com") {
		return map[string]any{"text": text}
	}
	return map[string]any{
		"type": "message",
		"attachments": []map[string]any{{
			"contentType": "application/vnd.microsoft.card.adaptive",
			"content": map[string]any{
				"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
				"type":    "AdaptiveCard",
				"version": "1.4",
				"body": []map[string]any{{
					"type": "TextBlock",
					"text": text,
					"wrap": true,
				}},
			},
		}},
	}
}

func (n *Notifier) format(ev Event) string {
	var b strings.Builder

	switch ev.Type {
	case EventSessionFailed:
		b.WriteString("❌ Session failed")
	case EventPRCreated:
		b.WriteString("🔀 Session completed — PR created")
	case EventReviewCompleted:
		b.WriteString(fmt.Sprintf("📋 Review completed (score %d/10)", ev.ReviewScore))
	default:
		b.WriteString("✅ Session completed")
	}

	b.WriteString(fmt.Sprintf(" — %s", shortRepo(ev.RepoURL)))
	if ev.SessionType != "" {
		b.WriteString(fmt.Sprintf(" (%s)", ev.SessionType))
	}

	if ev.Error != "" {
		b.WriteString("\n")
		b.WriteString(truncate(ev.Error, 300))
	}

	var stats []string
	if ev.DurationSeconds > 0 {
		stats = append(stats, formatDuration(ev.DurationSeconds))
	}
	if ev.InputTokens > 0 || ev.OutputTokens > 0 {
		stats = append(stats, fmt.Sprintf("%s in / %s out tokens", formatTokens(ev.InputTokens), formatTokens(ev.OutputTokens)))
	}
	if len(stats) > 0 {
		b.WriteString("\n⏱ ")
		b.WriteString(strings.Join(stats, " · "))
	}

	if n.uiBaseURL != "" {
		b.WriteString(fmt.Sprintf("\n%s/sessions/%s", n.uiBaseURL, ev.SessionID))
	}

	return b.String()
}

func (n *Notifier) post(ctx context.Context, webhookURL string, payload any, target, sessionID string) {
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}

	// Detach from the caller's context: a canceled session must still notify.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		slog.Warn("notification request build failed", "target", target, "session_id", sessionID, "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		slog.Warn("notification delivery failed", "target", target, "session_id", sessionID, "error", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		slog.Warn("notification rejected", "target", target, "session_id", sessionID, "status", resp.StatusCode)
	}
}

// shortRepo reduces a clone URL (https or scp-style) to "owner/repo" for readability.
func shortRepo(repoURL string) string {
	s := strings.TrimSuffix(repoURL, ".git")
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
		if j := strings.Index(s, "/"); j >= 0 {
			s = s[j+1:]
		}
	} else if i := strings.Index(s, ":"); i >= 0 && strings.Contains(s, "@") {
		s = s[i+1:]
	}
	if s == "" {
		return repoURL
	}
	return s
}

func formatDuration(seconds int) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	return fmt.Sprintf("%dm%02ds", seconds/60, seconds%60)
}

func formatTokens(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

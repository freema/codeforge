package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/freema/codeforge/internal/config"
)

func TestNew_DisabledWithoutURLs(t *testing.T) {
	if n := New(config.NotificationsConfig{}); n != nil {
		t.Fatal("expected nil notifier when no webhook URLs are configured")
	}
}

func TestNotify_NilReceiverIsNoop(t *testing.T) {
	var n *Notifier
	n.Notify(context.Background(), Event{Type: EventSessionCompleted}) // must not panic
}

func TestNotify_SlackAndDiscordPayloads(t *testing.T) {
	var slackBody, discordBody map[string]string

	slack := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&slackBody)
	}))
	defer slack.Close()
	discord := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&discordBody)
	}))
	defer discord.Close()

	n := New(config.NotificationsConfig{
		SlackWebhookURL:   slack.URL,
		DiscordWebhookURL: discord.URL,
		UIBaseURL:         "https://cf.example.com/",
	})
	n.Notify(context.Background(), Event{
		Type:            EventSessionCompleted,
		SessionID:       "sess-1",
		SessionType:     "code",
		RepoURL:         "https://github.com/acme/widget.git",
		DurationSeconds: 95,
		InputTokens:     12345,
		OutputTokens:    678,
	})

	for name, body := range map[string]map[string]string{"slack": slackBody, "discord": discordBody} {
		key := "text"
		if name == "discord" {
			key = "content"
		}
		msg, ok := body[key]
		if !ok {
			t.Fatalf("%s payload missing %q field: %v", name, key, body)
		}
		for _, want := range []string{"✅", "acme/widget", "(code)", "1m35s", "12.3k in / 678 out", "https://cf.example.com/sessions/sess-1"} {
			if !strings.Contains(msg, want) {
				t.Errorf("%s message missing %q:\n%s", name, want, msg)
			}
		}
	}
}

func TestNotify_EventFilter(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
	}))
	defer srv.Close()

	n := New(config.NotificationsConfig{
		SlackWebhookURL: srv.URL,
		Events:          []string{EventSessionFailed},
	})
	n.Notify(context.Background(), Event{Type: EventSessionCompleted, SessionID: "s1"})
	if calls != 0 {
		t.Fatalf("filtered event delivered, calls = %d", calls)
	}
	n.Notify(context.Background(), Event{Type: EventSessionFailed, SessionID: "s1", Error: "boom"})
	if calls != 1 {
		t.Fatalf("allowed event not delivered, calls = %d", calls)
	}
}

func TestNotify_SurvivesCanceledContext(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	n := New(config.NotificationsConfig{SlackWebhookURL: srv.URL})
	n.Notify(ctx, Event{Type: EventSessionFailed, SessionID: "s1"})
	if calls != 1 {
		t.Fatalf("notification dropped on canceled context, calls = %d", calls)
	}
}

func TestFormat_FailedIncludesTruncatedError(t *testing.T) {
	n := &Notifier{}
	msg := n.format(Event{
		Type:    EventSessionFailed,
		RepoURL: "git@github.com:acme/widget.git",
		Error:   strings.Repeat("x", 400),
	})
	if !strings.Contains(msg, "❌") || !strings.Contains(msg, "acme/widget") {
		t.Errorf("unexpected failed message: %s", msg)
	}
	if !strings.Contains(msg, "…") {
		t.Errorf("long error not truncated: %s", msg)
	}
}

func TestShortRepo(t *testing.T) {
	cases := map[string]string{
		"https://github.com/acme/widget.git": "acme/widget",
		"git@github.com:acme/widget.git":     "acme/widget",
		"https://gitlab.com/grp/sub/repo":    "grp/sub/repo",
		"":                                   "",
	}
	for in, want := range cases {
		if got := shortRepo(in); got != want {
			t.Errorf("shortRepo(%q) = %q, want %q", in, got, want)
		}
	}
}

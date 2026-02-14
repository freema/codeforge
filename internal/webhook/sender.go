package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"time"

	gitpkg "github.com/freema/codeforge/internal/git"
	"github.com/freema/codeforge/internal/metrics"
	"github.com/freema/codeforge/internal/task"
)

// Payload is the webhook request body.
type Payload struct {
	TaskID         string                `json:"task_id"`
	Status         string                `json:"status"`
	Result         string                `json:"result,omitempty"`
	Error          string                `json:"error,omitempty"`
	ChangesSummary *gitpkg.ChangesSummary `json:"changes_summary,omitempty"`
	Usage          *task.UsageInfo        `json:"usage,omitempty"`
	TraceID        string                `json:"trace_id,omitempty"`
	FinishedAt     time.Time             `json:"finished_at"`
}

// Sender delivers webhook callbacks with HMAC-SHA256 signatures.
type Sender struct {
	client     *http.Client
	secret     string
	maxRetries int
	baseDelay  time.Duration
}

// NewSender creates a webhook sender.
func NewSender(secret string, maxRetries int, baseDelay time.Duration) *Sender {
	return &Sender{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		secret:     secret,
		maxRetries: maxRetries,
		baseDelay:  baseDelay,
	}
}

// Send delivers a webhook to the callback URL with retries and exponential backoff.
func (s *Sender) Send(ctx context.Context, callbackURL string, payload Payload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling webhook payload: %w", err)
	}

	sig := s.sign(body)
	eventType := "task." + payload.Status

	for attempt := 0; attempt <= s.maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(math.Pow(5, float64(attempt-1))) * s.baseDelay
			slog.Info("webhook retry", "attempt", attempt, "delay", delay, "url", callbackURL)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, callbackURL, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("creating webhook request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Signature-256", "sha256="+sig)
		req.Header.Set("X-CodeForge-Event", eventType)
		if payload.TraceID != "" {
			req.Header.Set("X-Trace-ID", payload.TraceID)
		}

		resp, err := s.client.Do(req)
		if err != nil {
			slog.Warn("webhook request failed", "attempt", attempt, "error", err, "url", callbackURL)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			slog.Info("webhook delivered", "url", callbackURL, "status", resp.StatusCode, "attempt", attempt)
			metrics.WebhookDeliveries.WithLabelValues("success").Inc()
			return nil
		}

		slog.Warn("webhook non-2xx response", "attempt", attempt, "status", resp.StatusCode, "url", callbackURL)
	}

	metrics.WebhookDeliveries.WithLabelValues("failed").Inc()
	return fmt.Errorf("webhook delivery failed after %d attempts to %s", s.maxRetries+1, callbackURL)
}

func (s *Sender) sign(body []byte) string {
	mac := hmac.New(sha256.New, []byte(s.secret))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

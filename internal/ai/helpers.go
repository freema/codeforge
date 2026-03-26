package ai

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/freema/codeforge/internal/prompt"
)

// PRMetadata holds generated PR title and description.
type PRMetadata struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// GeneratePRMetadata generates a PR title and description from the diff and original prompt.
// Returns nil if AI is not available or fails (caller should use fallback).
func GeneratePRMetadata(ctx context.Context, client Client, diff, sessionPrompt string) *PRMetadata {
	if client == nil {
		return nil
	}

	system, err := prompt.LoadRaw("pr_metadata")
	if err != nil {
		slog.Warn("failed to load pr_metadata prompt", "error", err)
		return nil
	}

	// Truncate diff to avoid token waste
	if len(diff) > 4000 {
		diff = diff[:4000] + "\n... (truncated)"
	}

	user := "## Original task\n" + sessionPrompt + "\n\n## Diff\n" + diff

	response, err := client.Generate(ctx, system, user)
	if err != nil {
		slog.Warn("AI PR metadata generation failed", "error", err)
		return nil
	}

	// Parse JSON from response (strip markdown fences if present)
	response = stripJSONFences(response)

	var meta PRMetadata
	if err := json.Unmarshal([]byte(response), &meta); err != nil {
		slog.Warn("failed to parse AI PR metadata", "error", err, "response", truncate(response, 200))
		return nil
	}

	// Validate
	if meta.Title == "" {
		return nil
	}
	if len(meta.Title) > 72 {
		meta.Title = meta.Title[:72]
	}

	return &meta
}

// GenerateCommitMessage generates a commit message from the diff.
// Returns empty string if AI is not available or fails.
func GenerateCommitMessage(ctx context.Context, client Client, diff, taskPrompt string) string {
	if client == nil {
		return ""
	}

	system, err := prompt.LoadRaw("commit_message")
	if err != nil {
		slog.Warn("failed to load commit_message prompt", "error", err)
		return ""
	}

	if len(diff) > 4000 {
		diff = diff[:4000] + "\n... (truncated)"
	}

	user := diff
	if taskPrompt != "" {
		user = "## Task\n" + taskPrompt + "\n\n## Diff\n" + diff
	}

	response, err := client.Generate(ctx, system, user)
	if err != nil {
		slog.Warn("AI commit message generation failed", "error", err)
		return ""
	}

	// Clean up: take first line, trim whitespace and quotes
	msg := strings.TrimSpace(response)
	if idx := strings.IndexByte(msg, '\n'); idx > 0 {
		msg = msg[:idx]
	}
	msg = strings.Trim(msg, "\"'`")

	if len(msg) > 72 {
		msg = msg[:72]
	}

	return msg
}

func stripJSONFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```json") {
		s = strings.TrimPrefix(s, "```json")
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = s[:idx]
		}
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = s[:idx]
		}
	}
	return strings.TrimSpace(s)
}

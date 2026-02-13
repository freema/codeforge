package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// AnalysisResult holds auto-generated PR metadata.
type AnalysisResult struct {
	BranchSlug  string
	PRTitle     string
	Description string
}

// Analyzer uses the Anthropic API to generate PR metadata from a task prompt.
type Analyzer struct {
	apiKey string
	client *http.Client
}

// NewAnalyzer creates a prompt analyzer.
func NewAnalyzer(apiKey string) *Analyzer {
	return &Analyzer{
		apiKey: apiKey,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Analyze generates branch slug, PR title, and description from task prompt and changes.
// Falls back to generic values on any error.
func (a *Analyzer) Analyze(ctx context.Context, prompt, diffStats string, taskID string) *AnalysisResult {
	if a.apiKey == "" {
		return fallbackResult(prompt, taskID)
	}

	result, err := a.callAPI(ctx, prompt, diffStats)
	if err != nil {
		return fallbackResult(prompt, taskID)
	}

	return result
}

func (a *Analyzer) callAPI(ctx context.Context, prompt, diffStats string) (*AnalysisResult, error) {
	systemPrompt := `You generate metadata for a git pull request. Given a task description and diff stats, produce:
1. branch_slug: a short kebab-case slug (max 40 chars, no special chars except hyphens)
2. pr_title: a concise PR title (max 72 chars)
3. description: a 1-3 sentence PR description

Respond ONLY with valid JSON: {"branch_slug":"...","pr_title":"...","description":"..."}`

	userMsg := fmt.Sprintf("Task: %s\n\nChanges: %s", truncateStr(prompt, 1000), diffStats)

	body := map[string]interface{}{
		"model":      "claude-haiku-4-5-20250929",
		"max_tokens": 256,
		"messages": []map[string]string{
			{"role": "user", "content": systemPrompt + "\n\n" + userMsg},
		},
	}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic API returned %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return parseAnalyzerResponse(respBody)
}

func parseAnalyzerResponse(body []byte) (*AnalysisResult, error) {
	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	if len(resp.Content) == 0 {
		return nil, fmt.Errorf("empty response from analyzer")
	}

	var result struct {
		BranchSlug  string `json:"branch_slug"`
		PRTitle     string `json:"pr_title"`
		Description string `json:"description"`
	}

	text := resp.Content[0].Text
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return nil, fmt.Errorf("parsing analyzer output: %w", err)
	}

	slug := sanitizeSlug(result.BranchSlug)
	if slug == "" {
		return nil, fmt.Errorf("empty branch slug from analyzer")
	}

	return &AnalysisResult{
		BranchSlug:  slug,
		PRTitle:     result.PRTitle,
		Description: result.Description,
	}, nil
}

func fallbackResult(prompt, taskID string) *AnalysisResult {
	shortID := taskID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}

	title := truncateStr(prompt, 60)
	if len(prompt) > 60 {
		title += "..."
	}

	return &AnalysisResult{
		BranchSlug:  "task-" + shortID,
		PRTitle:     "CodeForge: " + title,
		Description: "Automated changes by CodeForge.",
	}
}

var slugRegex = regexp.MustCompile(`[^a-z0-9-]`)

func sanitizeSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugRegex.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	// Collapse multiple hyphens
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	if len(s) > 40 {
		s = s[:40]
		s = strings.TrimRight(s, "-")
	}
	return s
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

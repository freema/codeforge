package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Client generates short text completions via AI API.
type Client interface {
	Generate(ctx context.Context, system, user string) (string, error)
}

// anthropicClient calls the Anthropic Messages API.
type anthropicClient struct {
	apiKey string
	model  string
	client *http.Client
}

// NewAnthropicClient creates a client for the Anthropic API.
func NewAnthropicClient(apiKey string) Client {
	return &anthropicClient{
		apiKey: apiKey,
		model:  "claude-haiku-4-5-20251001",
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *anthropicClient) Generate(ctx context.Context, system, user string) (string, error) {
	body := map[string]interface{}{
		"model":      c.model,
		"max_tokens": 1024,
		"system":     system,
		"messages":   []map[string]string{{"role": "user", "content": user}},
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(bodyJSON))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}
	if len(result.Content) == 0 {
		return "", fmt.Errorf("empty response from API")
	}

	return result.Content[0].Text, nil
}

// openaiClient calls the OpenAI Chat Completions API.
type openaiClient struct {
	apiKey string
	model  string
	client *http.Client
}

// NewOpenAIClient creates a client for the OpenAI API.
func NewOpenAIClient(apiKey string) Client {
	return &openaiClient{
		apiKey: apiKey,
		model:  "gpt-4.1-mini",
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *openaiClient) Generate(ctx context.Context, system, user string) (string, error) {
	messages := []map[string]string{
		{"role": "system", "content": system},
		{"role": "user", "content": user},
	}
	body := map[string]interface{}{
		"model":      c.model,
		"max_tokens": 1024,
		"messages":   messages,
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(bodyJSON))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("empty response from API")
	}

	return result.Choices[0].Message.Content, nil
}

// NewClientFromRegistry creates an AI client by trying providers in order:
// 1. Anthropic key -> AnthropicClient
// 2. OpenAI key -> OpenAIClient
// 3. No key -> returns nil (caller should use fallback)
func NewClientFromRegistry(ctx context.Context, keys KeyResolver) Client {
	// Try Anthropic first
	if token, err := keys.ResolveAIKey(ctx, "anthropic"); err == nil && token != "" {
		slog.Info("AI helper using Anthropic API")
		return NewAnthropicClient(token)
	}
	// Try OpenAI
	if token, err := keys.ResolveAIKey(ctx, "openai"); err == nil && token != "" {
		slog.Info("AI helper using OpenAI API")
		return NewOpenAIClient(token)
	}
	slog.Info("AI helper disabled (no API key found)")
	return nil
}

// KeyResolver is the subset of keys.Resolver needed by the AI client.
type KeyResolver interface {
	ResolveAIKey(ctx context.Context, provider string) (string, error)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

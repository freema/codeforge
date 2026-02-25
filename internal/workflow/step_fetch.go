package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// KeyResolver looks up a stored key by name and returns the token and provider.
type KeyResolver interface {
	ResolveByName(ctx context.Context, name string) (token, provider string, err error)
}

// FetchExecutor executes fetch steps — HTTP calls to external APIs.
type FetchExecutor struct {
	keys   KeyResolver
	client *http.Client
}

// NewFetchExecutor creates a new fetch step executor.
func NewFetchExecutor(keys KeyResolver) *FetchExecutor {
	return &FetchExecutor{
		keys:   keys,
		client: http.DefaultClient,
	}
}

// Execute runs a fetch step: makes an HTTP request, extracts outputs via JSONPath.
func (e *FetchExecutor) Execute(ctx context.Context, stepDef StepDefinition, tctx TemplateContext) (map[string]string, error) {
	var cfg FetchConfig
	if err := json.Unmarshal(stepDef.Config, &cfg); err != nil {
		return nil, fmt.Errorf("parsing fetch config: %w", err)
	}

	// Render templates in config
	url, err := Render(cfg.URL, tctx)
	if err != nil {
		return nil, fmt.Errorf("rendering URL template: %w", err)
	}

	method := cfg.Method
	if method == "" {
		method = "GET"
	}

	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating HTTP request: %w", err)
	}

	// Set static headers
	for k, v := range cfg.Headers {
		rendered, err := Render(v, tctx)
		if err != nil {
			return nil, fmt.Errorf("rendering header %s: %w", k, err)
		}
		req.Header.Set(k, rendered)
	}

	// Resolve auth key if specified
	if cfg.KeyName != "" {
		keyName, err := Render(cfg.KeyName, tctx)
		if err != nil {
			return nil, fmt.Errorf("rendering key_name: %w", err)
		}
		if keyName != "" && e.keys != nil {
			token, provider, err := e.keys.ResolveByName(ctx, keyName)
			if err != nil {
				return nil, fmt.Errorf("resolving key '%s': %w", keyName, err)
			}
			switch provider {
			case "github":
				req.Header.Set("Authorization", "Bearer "+token)
			case "gitlab":
				req.Header.Set("PRIVATE-TOKEN", token)
			default:
				req.Header.Set("Authorization", "Bearer "+token)
			}
		}
	}

	slog.Debug("fetch step executing", "method", method, "url", url)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateString(string(body), 200))
	}

	// Parse JSON response
	var raw interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parsing JSON response: %w", err)
	}

	// Extract outputs via JSONPath expressions
	outputs := make(map[string]string)
	for name, path := range cfg.Outputs {
		value := jsonPathExtract(raw, path)
		outputs[name] = value
	}

	return outputs, nil
}

// jsonPathExtract does minimal JSONPath extraction supporting $.key and $.a.b.c patterns.
func jsonPathExtract(data interface{}, path string) string {
	path = strings.TrimPrefix(path, "$.")
	if path == "" || path == "$" {
		return jsonToString(data)
	}

	parts := strings.Split(path, ".")
	current := data
	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return ""
		}
		current, ok = m[part]
		if !ok {
			return ""
		}
	}

	return jsonToString(current)
}

func jsonToString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case bool:
		return fmt.Sprintf("%v", val)
	default:
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(b)
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

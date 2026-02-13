//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

const defaultBaseURL = "http://localhost:8080"

func baseURL() string {
	if v := os.Getenv("CODEFORGE_TEST_URL"); v != "" {
		return v
	}
	return defaultBaseURL
}

func authToken() string {
	if v := os.Getenv("CODEFORGE_TEST_TOKEN"); v != "" {
		return v
	}
	return "dev-token"
}

// apiRequest makes an authenticated HTTP request and returns the response.
func apiRequest(t *testing.T, method, path string, body interface{}) *http.Response {
	t.Helper()

	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, baseURL()+path, bodyReader)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+authToken())
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

// decodeJSON decodes a JSON response body into dst.
func decodeJSON(t *testing.T, resp *http.Response, dst interface{}) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

// waitForStatus polls a task until it reaches the expected status or times out.
func waitForStatus(t *testing.T, taskID string, expected string, timeout time.Duration) map[string]interface{} {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for task %s to reach status %s", taskID, expected)
			return nil
		default:
		}

		resp := apiRequest(t, "GET", "/api/v1/tasks/"+taskID, nil)
		var result map[string]interface{}
		decodeJSON(t, resp, &result)

		status, _ := result["status"].(string)
		if status == expected {
			return result
		}
		if status == "failed" && expected != "failed" {
			t.Fatalf("task %s failed unexpectedly: %v", taskID, result["error"])
		}

		time.Sleep(500 * time.Millisecond)
	}
}

func TestHealthEndpoint(t *testing.T) {
	resp, err := http.Get(baseURL() + "/health")
	if err != nil {
		t.Fatalf("health request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if result["status"] != "ok" {
		t.Errorf("expected status ok, got %v", result["status"])
	}
	if result["redis"] != "connected" {
		t.Errorf("expected redis connected, got %v", result["redis"])
	}
}

func TestReadyEndpoint(t *testing.T) {
	resp, err := http.Get(baseURL() + "/ready")
	if err != nil {
		t.Fatalf("ready request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	resp, err := http.Get(baseURL() + "/metrics")
	if err != nil {
		t.Fatalf("metrics request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// Only check gauges which are always present; counters like tasks_total
	// only appear after the first increment.
	expectedMetrics := []string{
		"codeforge_tasks_in_progress",
		"codeforge_workers_total",
		"codeforge_http_requests_total",
	}
	for _, m := range expectedMetrics {
		if !bytes.Contains(body, []byte(m)) {
			t.Errorf("metrics response missing %s; body: %s", m, bodyStr[:min(500, len(bodyStr))])
		}
	}
}

func TestAuthRequired(t *testing.T) {
	// No auth token
	req, _ := http.NewRequest("GET", baseURL()+"/api/v1/tasks/some-id", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAuthWrongToken(t *testing.T) {
	req, _ := http.NewRequest("GET", baseURL()+"/api/v1/tasks/some-id", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestGetNonExistentTask(t *testing.T) {
	resp := apiRequest(t, "GET", "/api/v1/tasks/nonexistent-id", nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestCreateTaskValidation(t *testing.T) {
	tests := []struct {
		name string
		body map[string]interface{}
		code int
	}{
		{
			name: "missing repo_url",
			body: map[string]interface{}{
				"prompt": "do something",
			},
			code: http.StatusBadRequest,
		},
		{
			name: "missing prompt",
			body: map[string]interface{}{
				"repo_url": "https://github.com/user/repo.git",
			},
			code: http.StatusBadRequest,
		},
		{
			name: "invalid repo_url",
			body: map[string]interface{}{
				"repo_url": "not-a-url",
				"prompt":   "do something",
			},
			code: http.StatusBadRequest,
		},
		{
			name: "invalid callback_url",
			body: map[string]interface{}{
				"repo_url":     "https://github.com/user/repo.git",
				"prompt":       "do something",
				"callback_url": "not-a-url",
			},
			code: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := apiRequest(t, "POST", "/api/v1/tasks", tt.body)
			defer resp.Body.Close()
			if resp.StatusCode != tt.code {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("expected %d, got %d: %s", tt.code, resp.StatusCode, body)
			}
		})
	}
}

func TestCancelNonRunningTask(t *testing.T) {
	resp := apiRequest(t, "POST", "/api/v1/tasks/nonexistent/cancel", nil)
	defer resp.Body.Close()

	// Should be 404 (task not found)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestKeyCRUD(t *testing.T) {
	keyName := fmt.Sprintf("test-key-%d", time.Now().UnixNano())

	// Create key
	resp := apiRequest(t, "POST", "/api/v1/keys", map[string]string{
		"name":     keyName,
		"provider": "github",
		"token":    "ghp_test_token_123",
	})
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create key: expected 201, got %d: %s", resp.StatusCode, body)
	}
	resp.Body.Close()

	// List keys
	resp = apiRequest(t, "GET", "/api/v1/keys", nil)
	var listResult map[string]interface{}
	decodeJSON(t, resp, &listResult)

	keys, ok := listResult["keys"].([]interface{})
	if !ok {
		t.Fatal("expected keys array in response")
	}

	found := false
	for _, k := range keys {
		km, _ := k.(map[string]interface{})
		if km["name"] == keyName {
			found = true
			if km["provider"] != "github" {
				t.Errorf("expected provider github, got %v", km["provider"])
			}
			// Token should NOT be in list response
			if _, hasToken := km["token"]; hasToken {
				t.Error("token should not be exposed in list response")
			}
			break
		}
	}
	if !found {
		t.Errorf("created key %s not found in list", keyName)
	}

	// Delete key
	resp = apiRequest(t, "DELETE", "/api/v1/keys/"+keyName, nil)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("delete key: expected 200, got %d: %s", resp.StatusCode, body)
	}
	resp.Body.Close()
}

func TestMCPServerCRUD(t *testing.T) {
	serverName := fmt.Sprintf("test-mcp-%d", time.Now().UnixNano())

	// Create
	resp := apiRequest(t, "POST", "/api/v1/mcp/servers", map[string]interface{}{
		"name":    serverName,
		"package": "@test/mcp-server",
		"args":    []string{"--port", "3000"},
	})
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create MCP: expected 201, got %d: %s", resp.StatusCode, body)
	}
	resp.Body.Close()

	// List
	resp = apiRequest(t, "GET", "/api/v1/mcp/servers", nil)
	var listResult map[string]interface{}
	decodeJSON(t, resp, &listResult)

	servers, ok := listResult["servers"].([]interface{})
	if !ok {
		t.Fatal("expected servers array in response")
	}

	found := false
	for _, s := range servers {
		sm, _ := s.(map[string]interface{})
		if sm["name"] == serverName {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("created MCP server %s not found in list", serverName)
	}

	// Delete
	resp = apiRequest(t, "DELETE", "/api/v1/mcp/servers/"+serverName, nil)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("delete MCP: expected 200, got %d: %s", resp.StatusCode, body)
	}
	resp.Body.Close()
}

func TestWorkspaceList(t *testing.T) {
	resp := apiRequest(t, "GET", "/api/v1/workspaces", nil)
	var result map[string]interface{}
	decodeJSON(t, resp, &result)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	if _, ok := result["workspaces"]; !ok {
		t.Error("expected workspaces key in response")
	}
	if _, ok := result["total_count"]; !ok {
		t.Error("expected total_count key in response")
	}
}

func TestDeleteNonExistentWorkspace(t *testing.T) {
	resp := apiRequest(t, "DELETE", "/api/v1/workspaces/nonexistent-id", nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

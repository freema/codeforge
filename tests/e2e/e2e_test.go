//go:build integration

// E2E tests for the full task lifecycle.
// Run with: task test:e2e
//
// These tests require:
//   - Running dev server with mock CLI (CODEFORGE_CLI__CLAUDE_CODE__PATH=/app/bin/mock-claude)
//   - Redis available
//   - Shared /data/workspaces volume between test and app containers
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func baseURL() string {
	if v := os.Getenv("CODEFORGE_TEST_URL"); v != "" {
		return v
	}
	return "http://app:8080"
}

func authToken() string {
	if v := os.Getenv("CODEFORGE_TEST_TOKEN"); v != "" {
		return v
	}
	return "dev-token"
}

func apiRequest(t *testing.T, method, path string, body interface{}) *http.Response {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
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
		t.Fatalf("request %s %s: %v", method, path, err)
	}
	return resp
}

func decodeJSON(t *testing.T, resp *http.Response, dst interface{}) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

// createTestRepo creates a bare git repo with one commit on the shared volume
// so both the test container and the app container can access it.
func createTestRepo(t *testing.T, name string) string {
	t.Helper()

	baseDir := "/data/workspaces/_e2e_repos"
	os.MkdirAll(baseDir, 0755)

	repoDir := filepath.Join(baseDir, fmt.Sprintf("%s-%d.git", name, time.Now().UnixNano()))
	workDir := repoDir + "-work"

	cmds := []struct {
		args []string
		dir  string
	}{
		{[]string{"git", "init", "--bare", repoDir}, ""},
		{[]string{"git", "clone", repoDir, workDir}, ""},
	}
	for _, c := range cmds {
		cmd := exec.Command(c.args[0], c.args[1:]...)
		if c.dir != "" {
			cmd.Dir = c.dir
		}
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("cmd %v: %v\n%s", c.args, err, out)
		}
	}

	// Add initial commit
	os.WriteFile(filepath.Join(workDir, "README.md"), []byte("# Test\n"), 0644)

	gitCmds := [][]string{
		{"git", "-C", workDir, "add", "."},
		{"git", "-C", workDir, "-c", "user.name=Test", "-c", "user.email=t@t.com", "commit", "-m", "init"},
		{"git", "-C", workDir, "push", "origin", "HEAD"},
	}
	for _, args := range gitCmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("cmd %v: %v\n%s", args, err, out)
		}
	}

	t.Cleanup(func() {
		os.RemoveAll(repoDir)
		os.RemoveAll(workDir)
	})

	return repoDir
}

func waitForTerminal(t *testing.T, taskID string, timeout time.Duration) map[string]interface{} {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp := apiRequest(t, "GET", "/api/v1/tasks/"+taskID, nil)
		var result map[string]interface{}
		decodeJSON(t, resp, &result)
		status := result["status"].(string)
		if status == "completed" || status == "failed" || status == "pr_created" {
			return result
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for task %s to reach terminal status", taskID)
	return nil
}

func waitForStatus(t *testing.T, taskID, expected string, timeout time.Duration) map[string]interface{} {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp := apiRequest(t, "GET", "/api/v1/tasks/"+taskID, nil)
		var result map[string]interface{}
		decodeJSON(t, resp, &result)
		if result["status"] == expected {
			return result
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for status %s", expected)
	return nil
}

func createTask(t *testing.T, body map[string]interface{}) string {
	t.Helper()
	resp := apiRequest(t, "POST", "/api/v1/tasks", body)
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create task: expected 201, got %d: %s", resp.StatusCode, b)
	}
	var result map[string]interface{}
	decodeJSON(t, resp, &result)
	return result["id"].(string)
}

// --- E2E Tests ---

func TestE2ETaskSuccess(t *testing.T) {
	repoDir := createTestRepo(t, "success")

	taskID := createTask(t, map[string]interface{}{
		"repo_url": "file://" + repoDir,
		"prompt":   "Add a hello world function",
	})
	t.Logf("task created: %s", taskID)

	result := waitForTerminal(t, taskID, 60*time.Second)
	status := result["status"].(string)
	t.Logf("final status: %s", status)

	if status != "completed" {
		t.Fatalf("expected completed, got %s (error: %v)", status, result["error"])
	}

	// Verify result
	if result["result"] == nil || result["result"] == "" {
		t.Error("expected non-empty result")
	}

	// Verify usage
	usage, ok := result["usage"].(map[string]interface{})
	if !ok {
		t.Fatal("expected usage info")
	}
	if usage["input_tokens"].(float64) <= 0 {
		t.Error("expected positive input_tokens")
	}
	if usage["output_tokens"].(float64) <= 0 {
		t.Error("expected positive output_tokens")
	}
	if usage["duration_seconds"].(float64) < 0 {
		t.Error("expected non-negative duration")
	}

	// Verify iteration
	if result["iteration"].(float64) != 1 {
		t.Errorf("expected iteration 1, got %v", result["iteration"])
	}

	t.Log("SUCCESS: full task lifecycle completed")
}

func TestE2ETaskCLIFailure(t *testing.T) {
	repoDir := createTestRepo(t, "fail")

	taskID := createTask(t, map[string]interface{}{
		"repo_url": "file://" + repoDir,
		"prompt":   "FAIL", // mock CLI exits with code 1
	})
	t.Logf("task created: %s", taskID)

	result := waitForTerminal(t, taskID, 60*time.Second)
	if result["status"] != "failed" {
		t.Fatalf("expected failed, got %v", result["status"])
	}

	errMsg, _ := result["error"].(string)
	if errMsg == "" {
		t.Error("expected non-empty error message")
	}
	t.Logf("failure error: %s", errMsg)
}

func TestE2ETaskTimeout(t *testing.T) {
	repoDir := createTestRepo(t, "timeout")

	taskID := createTask(t, map[string]interface{}{
		"repo_url": "file://" + repoDir,
		"prompt":   "TIMEOUT", // mock CLI sleeps for 10min
		"config": map[string]interface{}{
			"timeout_seconds": 5,
		},
	})
	t.Logf("task created: %s", taskID)

	result := waitForTerminal(t, taskID, 30*time.Second)
	if result["status"] != "failed" {
		t.Fatalf("expected failed (timeout), got %v", result["status"])
	}

	errMsg, _ := result["error"].(string)
	if !bytes.Contains([]byte(errMsg), []byte("timed out")) {
		t.Errorf("expected 'timed out' in error, got: %s", errMsg)
	}
	t.Logf("timeout error: %s", errMsg)
}

func TestE2ETaskCancel(t *testing.T) {
	repoDir := createTestRepo(t, "cancel")

	taskID := createTask(t, map[string]interface{}{
		"repo_url": "file://" + repoDir,
		"prompt":   "TIMEOUT", // will hang until cancelled
		"config": map[string]interface{}{
			"timeout_seconds": 120,
		},
	})
	t.Logf("task created: %s", taskID)

	// Wait for running
	waitForStatus(t, taskID, "running", 30*time.Second)
	t.Log("task is running, cancelling...")

	resp := apiRequest(t, "POST", fmt.Sprintf("/api/v1/tasks/%s/cancel", taskID), nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Logf("cancel returned %d (might already be done)", resp.StatusCode)
	}

	result := waitForTerminal(t, taskID, 15*time.Second)
	if result["status"] != "failed" {
		t.Fatalf("expected failed after cancel, got %v", result["status"])
	}

	errMsg, _ := result["error"].(string)
	if !bytes.Contains([]byte(errMsg), []byte("cancelled")) {
		t.Logf("cancel error (may vary): %s", errMsg)
	}
	t.Log("cancel test passed")
}

func TestE2EFollowUp(t *testing.T) {
	repoDir := createTestRepo(t, "followup")

	taskID := createTask(t, map[string]interface{}{
		"repo_url": "file://" + repoDir,
		"prompt":   "Initial task",
	})

	// Wait for first iteration
	result := waitForTerminal(t, taskID, 60*time.Second)
	if result["status"] != "completed" {
		t.Fatalf("first iteration: expected completed, got %v (error: %v)", result["status"], result["error"])
	}

	// Send follow-up
	resp := apiRequest(t, "POST", fmt.Sprintf("/api/v1/tasks/%s/instruct", taskID), map[string]string{
		"prompt": "Now add tests",
	})
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("instruct: expected 200, got %d: %s", resp.StatusCode, b)
	}
	var instrResult map[string]interface{}
	decodeJSON(t, resp, &instrResult)

	if instrResult["iteration"].(float64) != 2 {
		t.Errorf("expected iteration 2, got %v", instrResult["iteration"])
	}

	// Wait for second iteration
	result2 := waitForTerminal(t, taskID, 60*time.Second)
	if result2["status"] != "completed" {
		t.Fatalf("second iteration: expected completed, got %v", result2["status"])
	}

	// Verify iterations
	resp = apiRequest(t, "GET", fmt.Sprintf("/api/v1/tasks/%s?include=iterations", taskID), nil)
	var full map[string]interface{}
	decodeJSON(t, resp, &full)

	iterations, ok := full["iterations"].([]interface{})
	if !ok || len(iterations) < 2 {
		t.Errorf("expected at least 2 iterations, got %d", len(iterations))
	}

	t.Logf("follow-up test passed: %d iterations", len(iterations))
}

func TestE2ECloneFailure(t *testing.T) {
	taskID := createTask(t, map[string]interface{}{
		"repo_url": "file:///nonexistent/repo.git",
		"prompt":   "should fail at clone",
	})

	result := waitForTerminal(t, taskID, 30*time.Second)
	if result["status"] != "failed" {
		t.Fatalf("expected failed, got %v", result["status"])
	}

	errMsg, _ := result["error"].(string)
	if !bytes.Contains([]byte(errMsg), []byte("clone")) {
		t.Logf("clone error: %s", errMsg)
	}
	t.Log("clone failure test passed")
}

//go:build integration

// E2E tests for the full session lifecycle.
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

func waitForTerminal(t *testing.T, sessionID string, timeout time.Duration) map[string]interface{} {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp := apiRequest(t, "GET", "/api/v1/sessions/"+sessionID, nil)
		var result map[string]interface{}
		decodeJSON(t, resp, &result)
		status := result["status"].(string)
		if status == "completed" || status == "failed" || status == "pr_created" || status == "canceled" {
			return result
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for session %s to reach terminal status", sessionID)
	return nil
}

func waitForStatus(t *testing.T, sessionID, expected string, timeout time.Duration) map[string]interface{} {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp := apiRequest(t, "GET", "/api/v1/sessions/"+sessionID, nil)
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

func createSession(t *testing.T, body map[string]interface{}) string {
	t.Helper()
	resp := apiRequest(t, "POST", "/api/v1/sessions", body)
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create session: expected 201, got %d: %s", resp.StatusCode, b)
	}
	var result map[string]interface{}
	decodeJSON(t, resp, &result)
	return result["id"].(string)
}

// --- E2E Tests ---

func TestE2ESessionSuccess(t *testing.T) {
	repoDir := createTestRepo(t, "success")

	sessionID := createSession(t, map[string]interface{}{
		"repo_url": "file://" + repoDir,
		"prompt":   "Add a hello world function",
	})
	t.Logf("session created: %s", sessionID)

	result := waitForTerminal(t, sessionID, 60*time.Second)
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

	t.Log("SUCCESS: full session lifecycle completed")
}

func TestE2ESessionCLIFailure(t *testing.T) {
	repoDir := createTestRepo(t, "fail")

	sessionID := createSession(t, map[string]interface{}{
		"repo_url": "file://" + repoDir,
		"prompt":   "FAIL", // mock CLI exits with code 1
	})
	t.Logf("session created: %s", sessionID)

	result := waitForTerminal(t, sessionID, 60*time.Second)
	if result["status"] != "failed" {
		t.Fatalf("expected failed, got %v", result["status"])
	}

	errMsg, _ := result["error"].(string)
	if errMsg == "" {
		t.Error("expected non-empty error message")
	}
	t.Logf("failure error: %s", errMsg)
}

func TestE2ESessionTimeout(t *testing.T) {
	repoDir := createTestRepo(t, "timeout")

	sessionID := createSession(t, map[string]interface{}{
		"repo_url": "file://" + repoDir,
		"prompt":   "TIMEOUT", // mock CLI sleeps for 10min
		"config": map[string]interface{}{
			"timeout_seconds": 5,
		},
	})
	t.Logf("session created: %s", sessionID)

	result := waitForTerminal(t, sessionID, 30*time.Second)
	if result["status"] != "completed" {
		t.Fatalf("expected completed (graceful timeout), got %v", result["status"])
	}

	resultText, _ := result["result"].(string)
	if !bytes.Contains([]byte(resultText), []byte("timed out")) {
		t.Errorf("expected 'timed out' in result, got: %s", resultText)
	}
	t.Logf("timeout result: %s", resultText)
}

func TestE2ESessionCancel(t *testing.T) {
	repoDir := createTestRepo(t, "cancel")

	sessionID := createSession(t, map[string]interface{}{
		"repo_url": "file://" + repoDir,
		"prompt":   "TIMEOUT", // will hang until canceled
		"config": map[string]interface{}{
			"timeout_seconds": 120,
		},
	})
	t.Logf("session created: %s", sessionID)

	// Wait for running
	waitForStatus(t, sessionID, "running", 30*time.Second)
	t.Log("session is running, canceling...")

	resp := apiRequest(t, "POST", fmt.Sprintf("/api/v1/sessions/%s/cancel", sessionID), nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Logf("cancel returned %d (might already be done)", resp.StatusCode)
	}

	// SIGTERM grace: the CLI gets up to 15s before SIGKILL escalation.
	result := waitForTerminal(t, sessionID, 30*time.Second)
	if result["status"] != "canceled" {
		t.Fatalf("expected canceled after cancel, got %v", result["status"])
	}
	t.Log("cancel test passed")
}

func TestE2EFollowUp(t *testing.T) {
	repoDir := createTestRepo(t, "followup")

	sessionID := createSession(t, map[string]interface{}{
		"repo_url": "file://" + repoDir,
		"prompt":   "Initial task",
	})

	// Wait for first iteration
	result := waitForTerminal(t, sessionID, 60*time.Second)
	if result["status"] != "completed" {
		t.Fatalf("first iteration: expected completed, got %v (error: %v)", result["status"], result["error"])
	}

	// Send follow-up
	resp := apiRequest(t, "POST", fmt.Sprintf("/api/v1/sessions/%s/instruct", sessionID), map[string]string{
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
	result2 := waitForTerminal(t, sessionID, 60*time.Second)
	if result2["status"] != "completed" {
		t.Fatalf("second iteration: expected completed, got %v", result2["status"])
	}

	// Verify iterations
	resp = apiRequest(t, "GET", fmt.Sprintf("/api/v1/sessions/%s?include=iterations", sessionID), nil)
	var full map[string]interface{}
	decodeJSON(t, resp, &full)

	iterations, ok := full["iterations"].([]interface{})
	if !ok || len(iterations) < 2 {
		t.Errorf("expected at least 2 iterations, got %d", len(iterations))
	}

	t.Logf("follow-up test passed: %d iterations", len(iterations))
}

func TestE2ECloneFailure(t *testing.T) {
	sessionID := createSession(t, map[string]interface{}{
		"repo_url": "file:///nonexistent/repo.git",
		"prompt":   "should fail at clone",
	})

	result := waitForTerminal(t, sessionID, 30*time.Second)
	if result["status"] != "failed" {
		t.Fatalf("expected failed, got %v", result["status"])
	}

	errMsg, _ := result["error"].(string)
	if !bytes.Contains([]byte(errMsg), []byte("clone")) {
		t.Logf("clone error: %s", errMsg)
	}
	t.Log("clone failure test passed")
}

func TestE2ESessionWithCLI(t *testing.T) {
	repoDir := createTestRepo(t, "with-cli")

	sessionID := createSession(t, map[string]interface{}{
		"repo_url": "file://" + repoDir,
		"prompt":   "Add a hello world function",
		"config": map[string]interface{}{
			"cli": "claude-code",
		},
	})
	t.Logf("session created with explicit CLI: %s", sessionID)

	result := waitForTerminal(t, sessionID, 60*time.Second)
	status := result["status"].(string)
	t.Logf("final status: %s", status)

	if status != "completed" {
		t.Fatalf("expected completed, got %s (error: %v)", status, result["error"])
	}

	if result["result"] == nil || result["result"] == "" {
		t.Error("expected non-empty result")
	}

	t.Log("SUCCESS: session with explicit CLI parameter completed")
}

// TestE2ECodeReviewWorkflow removed — code-review builtin workflow was consolidated
// into the review-as-action flow (POST /sessions/:id/review).

// usageOf extracts the usage map from a session GET response.
func usageOf(t *testing.T, result map[string]interface{}) map[string]interface{} {
	t.Helper()
	usage, ok := result["usage"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected usage info, got %v", result["usage"])
	}
	return usage
}

func numField(m map[string]interface{}, key string) float64 {
	v, _ := m[key].(float64)
	return v
}

// TestE2EUsageCostAndCLISessionID verifies that a completed session reports
// cost, cache token counts, and the CLI-native session id from the mock CLI's
// result event.
func TestE2EUsageCostAndCLISessionID(t *testing.T) {
	repoDir := createTestRepo(t, "usage-cost")

	sessionID := createSession(t, map[string]interface{}{
		"repo_url": "file://" + repoDir,
		"prompt":   "Report your usage",
	})

	result := waitForTerminal(t, sessionID, 60*time.Second)
	if result["status"] != "completed" {
		t.Fatalf("expected completed, got %v (error: %v)", result["status"], result["error"])
	}

	usage := usageOf(t, result)
	if numField(usage, "input_tokens") <= 0 {
		t.Errorf("expected positive input_tokens, got %v", usage["input_tokens"])
	}
	if numField(usage, "output_tokens") <= 0 {
		t.Errorf("expected positive output_tokens, got %v", usage["output_tokens"])
	}
	if numField(usage, "cost_usd") <= 0 {
		t.Errorf("expected positive cost_usd, got %v", usage["cost_usd"])
	}
	if numField(usage, "cache_read_tokens") <= 0 {
		t.Errorf("expected positive cache_read_tokens, got %v", usage["cache_read_tokens"])
	}
	if numField(usage, "cache_creation_tokens") <= 0 {
		t.Errorf("expected positive cache_creation_tokens, got %v", usage["cache_creation_tokens"])
	}

	cliSessionID, _ := result["cli_session_id"].(string)
	if cliSessionID == "" {
		t.Error("expected non-empty cli_session_id after completed run")
	}
	t.Logf("usage: %v, cli_session_id: %s", usage, cliSessionID)
}

// TestE2EResumeAccumulatesUsage instructs a completed session and verifies:
//   - the second iteration resumes the CLI-native session (--resume with the
//     stored cli_session_id — the mock CLI echoes it into the result text)
//   - session usage accumulates across iterations instead of being replaced
func TestE2EResumeAccumulatesUsage(t *testing.T) {
	repoDir := createTestRepo(t, "resume-usage")

	sessionID := createSession(t, map[string]interface{}{
		"repo_url": "file://" + repoDir,
		"prompt":   "First turn",
	})

	first := waitForTerminal(t, sessionID, 60*time.Second)
	if first["status"] != "completed" {
		t.Fatalf("first iteration: expected completed, got %v (error: %v)", first["status"], first["error"])
	}
	firstUsage := usageOf(t, first)
	firstInput := numField(firstUsage, "input_tokens")
	firstOutput := numField(firstUsage, "output_tokens")
	firstCost := numField(firstUsage, "cost_usd")
	firstCLISessionID, _ := first["cli_session_id"].(string)
	if firstCLISessionID == "" {
		t.Fatal("expected cli_session_id after first iteration")
	}

	// Follow-up turn in the same workspace — should take the --resume path.
	resp := apiRequest(t, "POST", fmt.Sprintf("/api/v1/sessions/%s/instruct", sessionID), map[string]string{
		"prompt": "Second turn",
	})
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("instruct: expected 200, got %d: %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	// Poll until the second iteration finished AND usage grew past the first
	// run's totals — immune to catching a stale "completed" right after instruct.
	var second map[string]interface{}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		r := apiRequest(t, "GET", "/api/v1/sessions/"+sessionID, nil)
		var cur map[string]interface{}
		decodeJSON(t, r, &cur)
		if cur["status"] == "failed" {
			t.Fatalf("second iteration failed: %v", cur["error"])
		}
		if cur["status"] == "completed" {
			if u, ok := cur["usage"].(map[string]interface{}); ok && numField(u, "input_tokens") > firstInput {
				second = cur
				break
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	if second == nil {
		t.Fatal("timed out waiting for second iteration with accumulated usage")
	}

	// Resume assertion: mock CLI appends "[resumed:<id>]" when run with --resume.
	resultText, _ := second["result"].(string)
	wantMarker := "[resumed:" + firstCLISessionID + "]"
	if !bytes.Contains([]byte(resultText), []byte(wantMarker)) {
		t.Errorf("expected result to contain %q (resume path), got: %s", wantMarker, resultText)
	}

	// Accumulation assertions: mock CLI emits fixed usage per run, so the
	// totals must be exactly double after two iterations.
	usage := usageOf(t, second)
	if got := numField(usage, "input_tokens"); got != 2*firstInput {
		t.Errorf("expected accumulated input_tokens %v, got %v", 2*firstInput, got)
	}
	if got := numField(usage, "output_tokens"); got != 2*firstOutput {
		t.Errorf("expected accumulated output_tokens %v, got %v", 2*firstOutput, got)
	}
	if got := numField(usage, "cost_usd"); got < 1.5*firstCost {
		t.Errorf("expected accumulated cost_usd >= %v, got %v", 1.5*firstCost, got)
	}

	// The resumed run forks to a new CLI session id.
	secondCLISessionID, _ := second["cli_session_id"].(string)
	if secondCLISessionID == "" || secondCLISessionID == firstCLISessionID {
		t.Errorf("expected a new cli_session_id after resume, first=%q second=%q", firstCLISessionID, secondCLISessionID)
	}
	t.Logf("accumulated usage: %v", usage)
}

// TestE2EWorkspaceDiff verifies GET /sessions/{id}/diff returns the mock CLI's
// file change as a non-empty unified diff with per-file stats.
func TestE2EWorkspaceDiff(t *testing.T) {
	repoDir := createTestRepo(t, "diff")

	sessionID := createSession(t, map[string]interface{}{
		"repo_url": "file://" + repoDir,
		"prompt":   "Make a change for the diff test",
	})

	result := waitForTerminal(t, sessionID, 60*time.Second)
	if result["status"] != "completed" {
		t.Fatalf("expected completed, got %v (error: %v)", result["status"], result["error"])
	}

	resp := apiRequest(t, "GET", fmt.Sprintf("/api/v1/sessions/%s/diff", sessionID), nil)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("diff: expected 200, got %d: %s", resp.StatusCode, b)
	}
	var diff map[string]interface{}
	decodeJSON(t, resp, &diff)

	files, ok := diff["files"].([]interface{})
	if !ok || len(files) < 1 {
		t.Fatalf("expected at least 1 changed file, got %v", diff["files"])
	}
	foundMockChange := false
	for _, f := range files {
		fm, _ := f.(map[string]interface{})
		if fm["path"] == "MOCK_CHANGES.md" {
			foundMockChange = true
			if fm["status"] != "added" {
				t.Errorf("expected MOCK_CHANGES.md status added, got %v", fm["status"])
			}
			if numField(fm, "additions") < 1 {
				t.Errorf("expected MOCK_CHANGES.md additions >= 1, got %v", fm["additions"])
			}
		}
	}
	if !foundMockChange {
		t.Errorf("expected MOCK_CHANGES.md in diff files, got %v", files)
	}

	unified, _ := diff["diff"].(string)
	if unified == "" {
		t.Error("expected non-empty unified diff")
	}
	if !bytes.Contains([]byte(unified), []byte("MOCK_CHANGES.md")) {
		t.Errorf("expected unified diff to mention MOCK_CHANGES.md, got: %.500s", unified)
	}
	if numField(diff, "total_additions") < 1 {
		t.Errorf("expected total_additions >= 1, got %v", diff["total_additions"])
	}
	if truncated, _ := diff["truncated"].(bool); truncated {
		t.Error("did not expect small diff to be truncated")
	}
}

// TestE2EPresetLifecycle covers the preset CRUD + run flow: create a preset,
// find it in the list, run it (which must produce a completing session), and
// delete it.
func TestE2EPresetLifecycle(t *testing.T) {
	repoDir := createTestRepo(t, "preset")
	presetName := fmt.Sprintf("e2e-preset-%d", time.Now().UnixNano())

	// Create
	resp := apiRequest(t, "POST", "/api/v1/presets", map[string]interface{}{
		"name":        presetName,
		"description": "E2E preset",
		"request": map[string]interface{}{
			"repo_url": "file://" + repoDir,
			"prompt":   "Run from preset",
		},
	})
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create preset: expected 201, got %d: %s", resp.StatusCode, b)
	}
	var created map[string]interface{}
	decodeJSON(t, resp, &created)
	presetID, _ := created["id"].(string)
	if presetID == "" {
		t.Fatal("expected preset id in create response")
	}
	t.Cleanup(func() {
		r := apiRequest(t, "DELETE", "/api/v1/presets/"+presetID, nil)
		r.Body.Close()
	})

	// List
	resp = apiRequest(t, "GET", "/api/v1/presets", nil)
	var list map[string]interface{}
	decodeJSON(t, resp, &list)
	presets, _ := list["presets"].([]interface{})
	found := false
	for _, p := range presets {
		pm, _ := p.(map[string]interface{})
		if pm["id"] == presetID {
			found = true
			if pm["name"] != presetName {
				t.Errorf("expected preset name %q, got %v", presetName, pm["name"])
			}
		}
	}
	if !found {
		t.Fatalf("created preset %s not found in list", presetID)
	}

	// Run — creates a session through the normal create path. The run endpoint
	// shares the session-creation rate limiter, and earlier tests in this suite
	// may have exhausted the window — retry on 429.
	deadline := time.Now().Add(30 * time.Second)
	resp = apiRequest(t, "POST", fmt.Sprintf("/api/v1/presets/%s/run", presetID), nil)
	for resp.StatusCode == http.StatusTooManyRequests && time.Now().Before(deadline) {
		resp.Body.Close()
		time.Sleep(2 * time.Second)
		resp = apiRequest(t, "POST", fmt.Sprintf("/api/v1/presets/%s/run", presetID), nil)
	}
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("run preset: expected 201, got %d: %s", resp.StatusCode, b)
	}
	var runResult map[string]interface{}
	decodeJSON(t, resp, &runResult)
	sessionID, _ := runResult["id"].(string)
	if sessionID == "" {
		t.Fatal("expected session id from preset run")
	}

	result := waitForTerminal(t, sessionID, 60*time.Second)
	if result["status"] != "completed" {
		t.Fatalf("preset session: expected completed, got %v (error: %v)", result["status"], result["error"])
	}
	resultText, _ := result["result"].(string)
	if !bytes.Contains([]byte(resultText), []byte("Run from preset")) {
		t.Errorf("expected preset prompt in result, got: %s", resultText)
	}

	// Delete
	resp = apiRequest(t, "DELETE", "/api/v1/presets/"+presetID, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete preset: expected 204, got %d", resp.StatusCode)
	}

	// Gone
	resp = apiRequest(t, "GET", "/api/v1/presets/"+presetID, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("get deleted preset: expected 404, got %d", resp.StatusCode)
	}
}

// TestE2EScheduleManualRun creates a schedule (cron far in the future), fires
// it manually, waits for the resulting session to complete, and verifies the
// run history records the manual trigger and the session outcome.
func TestE2EScheduleManualRun(t *testing.T) {
	repoDir := createTestRepo(t, "schedule")
	scheduleName := fmt.Sprintf("e2e-schedule-%d", time.Now().UnixNano())

	resp := apiRequest(t, "POST", "/api/v1/schedules", map[string]interface{}{
		"name": scheduleName,
		"cron": "0 3 1 1 *", // once a year — never fires during the test
		"session_request": map[string]interface{}{
			"repo_url": "file://" + repoDir,
			"prompt":   "Run from schedule",
		},
	})
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create schedule: expected 201, got %d: %s", resp.StatusCode, b)
	}
	var created map[string]interface{}
	decodeJSON(t, resp, &created)
	scheduleID, _ := created["id"].(string)
	if scheduleID == "" {
		t.Fatal("expected schedule id in create response")
	}
	t.Cleanup(func() {
		r := apiRequest(t, "DELETE", "/api/v1/schedules/"+scheduleID, nil)
		r.Body.Close()
	})

	// Fire manually
	resp = apiRequest(t, "POST", fmt.Sprintf("/api/v1/schedules/%s/run", scheduleID), nil)
	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("run schedule: expected 202, got %d: %s", resp.StatusCode, b)
	}
	var runResp map[string]interface{}
	decodeJSON(t, resp, &runResp)
	sessionID, _ := runResp["session_id"].(string)
	if sessionID == "" {
		t.Fatal("expected session_id from manual schedule run")
	}

	result := waitForTerminal(t, sessionID, 60*time.Second)
	if result["status"] != "completed" {
		t.Fatalf("schedule session: expected completed, got %v (error: %v)", result["status"], result["error"])
	}

	// The run row starts as trigger=manual/status=fired and is stamped
	// session_completed once the executor records the outcome — poll for it.
	deadline := time.Now().Add(15 * time.Second)
	var lastRuns []interface{}
	for time.Now().Before(deadline) {
		r := apiRequest(t, "GET", fmt.Sprintf("/api/v1/schedules/%s/runs", scheduleID), nil)
		var runsResp map[string]interface{}
		decodeJSON(t, r, &runsResp)
		lastRuns, _ = runsResp["runs"].([]interface{})
		for _, run := range lastRuns {
			rm, _ := run.(map[string]interface{})
			if rm["session_id"] != sessionID {
				continue
			}
			if rm["trigger"] != "manual" {
				t.Fatalf("expected trigger manual, got %v", rm["trigger"])
			}
			if rm["status"] == "session_completed" {
				t.Logf("schedule run history verified: %v", rm)
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for session_completed run row; last runs: %v", lastRuns)
}

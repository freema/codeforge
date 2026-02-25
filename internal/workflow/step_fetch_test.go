package workflow

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockKeyResolver struct {
	token    string
	provider string
	err      error
}

func (m *mockKeyResolver) ResolveByName(_ context.Context, _ string) (string, string, error) {
	return m.token, m.provider, m.err
}

func TestFetchExecutor_JSONPathExtraction(t *testing.T) {
	// Setup a test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected auth header, got: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"title": "NullPointerException in handler",
			"metadata": map[string]interface{}{
				"value": "null reference at line 42",
			},
			"number":   123,
			"platform": "go",
		})
	}))
	defer ts.Close()

	keys := &mockKeyResolver{token: "test-token", provider: "github"}
	executor := NewFetchExecutor(keys)

	stepDef := StepDefinition{
		Name: "fetch_issue",
		Type: StepTypeFetch,
		Config: mustJSON(FetchConfig{
			URL:     ts.URL + "/api/issue",
			Method:  "GET",
			KeyName: "my-key",
			Outputs: map[string]string{
				"title":    "$.title",
				"message":  "$.metadata.value",
				"number":   "$.number",
				"platform": "$.platform",
			},
		}),
	}

	tctx := TemplateContext{Params: map[string]string{}, Steps: map[string]map[string]string{}}
	outputs, err := executor.Execute(context.Background(), stepDef, tctx)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if outputs["title"] != "NullPointerException in handler" {
		t.Errorf("title = %q", outputs["title"])
	}
	if outputs["message"] != "null reference at line 42" {
		t.Errorf("message = %q", outputs["message"])
	}
	if outputs["number"] != "123" {
		t.Errorf("number = %q", outputs["number"])
	}
	if outputs["platform"] != "go" {
		t.Errorf("platform = %q", outputs["platform"])
	}
}

func TestFetchExecutor_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	}))
	defer ts.Close()

	executor := NewFetchExecutor(nil)
	stepDef := StepDefinition{
		Name:   "fetch",
		Type:   StepTypeFetch,
		Config: mustJSON(FetchConfig{URL: ts.URL, Outputs: map[string]string{}}),
	}

	_, err := executor.Execute(context.Background(), stepDef, TemplateContext{})
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestFetchExecutor_TemplateInURL(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
	}))
	defer ts.Close()

	executor := NewFetchExecutor(nil)
	stepDef := StepDefinition{
		Name: "fetch",
		Type: StepTypeFetch,
		Config: mustJSON(FetchConfig{
			URL:     ts.URL + "/repos/{{.Params.owner}}/issues",
			Outputs: map[string]string{},
		}),
	}

	tctx := TemplateContext{Params: map[string]string{"owner": "testorg"}}
	_, err := executor.Execute(context.Background(), stepDef, tctx)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestJSONPathExtract(t *testing.T) {
	data := map[string]interface{}{
		"title":  "Bug",
		"nested": map[string]interface{}{"deep": "value"},
		"number": float64(42),
		"labels": []interface{}{"bug", "critical"},
	}

	tests := []struct {
		path string
		want string
	}{
		{"$.title", "Bug"},
		{"$.nested.deep", "value"},
		{"$.number", "42"},
		{"$.nonexistent", ""},
		{"$.nested.nonexistent", ""},
	}

	for _, tc := range tests {
		got := jsonPathExtract(data, tc.path)
		if got != tc.want {
			t.Errorf("jsonPathExtract(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

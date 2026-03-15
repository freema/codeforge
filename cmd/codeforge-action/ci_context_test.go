package main

import (
	"testing"
)

func TestDetectPlatform(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		expected CIPlatform
	}{
		{
			name:     "github actions",
			env:      map[string]string{"GITHUB_ACTIONS": "true"},
			expected: PlatformGitHub,
		},
		{
			name:     "gitlab ci",
			env:      map[string]string{"GITLAB_CI": "true"},
			expected: PlatformGitLab,
		},
		{
			name:     "unknown",
			env:      map[string]string{},
			expected: PlatformUnknown,
		},
		{
			name:     "github takes priority",
			env:      map[string]string{"GITHUB_ACTIONS": "true", "GITLAB_CI": "true"},
			expected: PlatformGitHub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear platform env vars
			t.Setenv("GITHUB_ACTIONS", "")
			t.Setenv("GITLAB_CI", "")

			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			got := DetectPlatform()
			if got != tt.expected {
				t.Errorf("DetectPlatform() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestDetectCIContext_Unknown(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("GITLAB_CI", "")
	t.Setenv("WORKSPACE", "/test/dir")

	ctx, err := DetectCIContext()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ctx.Platform != PlatformUnknown {
		t.Errorf("Platform = %q, want %q", ctx.Platform, PlatformUnknown)
	}
	if ctx.WorkDir != "/test/dir" {
		t.Errorf("WorkDir = %q, want %q", ctx.WorkDir, "/test/dir")
	}
}

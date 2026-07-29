package main

import (
	"reflect"
	"testing"
)

func TestEnvProviderDomains(t *testing.T) {
	domainEnvVars := []string{"CI_SERVER_URL", "GITLAB_URL", "GITHUB_SERVER_URL", "GITHUB_URL"}

	tests := []struct {
		name string
		env  map[string]string
		want map[string]string
	}{
		{
			name: "no env set returns nil",
			env:  map[string]string{},
			want: nil,
		},
		{
			name: "self-managed GitLab via CI_SERVER_URL",
			env:  map[string]string{"CI_SERVER_URL": "https://gitlab.example.com"},
			want: map[string]string{"gitlab.example.com": "gitlab"},
		},
		{
			name: "CI_SERVER_URL with port adds host:port and bare host",
			env:  map[string]string{"CI_SERVER_URL": "http://gitlab.example.com:8080"},
			want: map[string]string{
				"gitlab.example.com:8080": "gitlab",
				"gitlab.example.com":      "gitlab",
			},
		},
		{
			name: "gitlab.com is skipped (natively detected)",
			env:  map[string]string{"CI_SERVER_URL": "https://gitlab.com"},
			want: nil,
		},
		{
			name: "github.com via GITHUB_SERVER_URL is skipped",
			env:  map[string]string{"GITHUB_SERVER_URL": "https://github.com"},
			want: nil,
		},
		{
			name: "GitHub Enterprise via GITHUB_SERVER_URL",
			env:  map[string]string{"GITHUB_SERVER_URL": "https://ghe.example.com"},
			want: map[string]string{"ghe.example.com": "github"},
		},
		{
			name: "manual GITLAB_URL and GITHUB_URL overrides",
			env: map[string]string{
				"GITLAB_URL": "https://git.example.com",
				"GITHUB_URL": "https://code.example.com",
			},
			want: map[string]string{
				"git.example.com":  "gitlab",
				"code.example.com": "github",
			},
		},
		{
			name: "scheme-less value is accepted",
			env:  map[string]string{"GITLAB_URL": "gitlab.example.com"},
			want: map[string]string{"gitlab.example.com": "gitlab"},
		},
		{
			name: "host is lowercased",
			env:  map[string]string{"CI_SERVER_URL": "https://GitLab.Example.COM"},
			want: map[string]string{"gitlab.example.com": "gitlab"},
		},
		{
			name: "earlier source wins on same host",
			env: map[string]string{
				"CI_SERVER_URL": "https://git.example.com",
				"GITHUB_URL":    "https://git.example.com",
			},
			want: map[string]string{"git.example.com": "gitlab"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range domainEnvVars {
				t.Setenv(key, "")
			}
			for key, val := range tt.env {
				t.Setenv(key, val)
			}

			got := envProviderDomains()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("envProviderDomains() = %v, want %v", got, tt.want)
			}
		})
	}
}

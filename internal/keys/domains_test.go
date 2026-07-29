package keys

import (
	"context"
	"fmt"
	"testing"
)

// listStubRegistry is a Registry stub with configurable List results.
type listStubRegistry struct {
	stubRegistry
	keys    []Key
	listErr error
}

func (s *listStubRegistry) List(_ context.Context) ([]Key, error) {
	return s.keys, s.listErr
}

func TestDomainSource_ProviderDomains(t *testing.T) {
	tests := []struct {
		name   string
		static map[string]string
		keys   []Key
		want   map[string]string
	}{
		{
			name: "gitlab key base_url derives domain",
			keys: []Key{{Name: "gl", Provider: "gitlab", BaseURL: "https://gitlab.example.com"}},
			want: map[string]string{"gitlab.example.com": "gitlab"},
		},
		{
			name: "ghe key base_url derives domain",
			keys: []Key{{Name: "ghe", Provider: "github", BaseURL: "https://ghe.example.com"}},
			want: map[string]string{"ghe.example.com": "github"},
		},
		{
			name: "base_url with port maps hostname",
			keys: []Key{{Name: "gl", Provider: "gitlab", BaseURL: "https://gitlab.example.com:8443"}},
			want: map[string]string{"gitlab.example.com": "gitlab"},
		},
		{
			name: "scheme-less base_url",
			keys: []Key{{Name: "gl", Provider: "gitlab", BaseURL: "gitlab.example.com"}},
			want: map[string]string{"gitlab.example.com": "gitlab"},
		},
		{
			name:   "static config wins over key-derived entry",
			static: map[string]string{"gitlab.example.com": "github"},
			keys:   []Key{{Name: "gl", Provider: "gitlab", BaseURL: "https://gitlab.example.com"}},
			want:   map[string]string{"gitlab.example.com": "github"},
		},
		{
			name:   "static entries carried over",
			static: map[string]string{"git.example.com": "gitlab"},
			keys:   []Key{{Name: "gl", Provider: "gitlab", BaseURL: "https://gitlab.example.com"}},
			want:   map[string]string{"git.example.com": "gitlab", "gitlab.example.com": "gitlab"},
		},
		{
			name: "well-known public hosts skipped",
			keys: []Key{
				{Name: "gh", Provider: "github", BaseURL: "https://github.com"},
				{Name: "gl", Provider: "gitlab", BaseURL: "https://gitlab.com"},
			},
			want: map[string]string{},
		},
		{
			name: "non-git providers and empty base_url ignored",
			keys: []Key{
				{Name: "sentry", Provider: "sentry", BaseURL: "https://sentry.example.com"},
				{Name: "gl-no-url", Provider: "gitlab"},
				{Name: "anthropic", Provider: "anthropic"},
			},
			want: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := NewDomainSource(&listStubRegistry{keys: tt.keys}, tt.static)
			got := src.ProviderDomains(context.Background())
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for host, provider := range tt.want {
				if got[host] != provider {
					t.Errorf("domains[%q] = %q, want %q", host, got[host], provider)
				}
			}
		})
	}
}

func TestDomainSource_ListError_FallsBackToStatic(t *testing.T) {
	static := map[string]string{"git.example.com": "gitlab"}
	src := NewDomainSource(&listStubRegistry{listErr: fmt.Errorf("db down")}, static)

	got := src.ProviderDomains(context.Background())
	if len(got) != 1 || got["git.example.com"] != "gitlab" {
		t.Errorf("got %v, want static map only", got)
	}
}

func TestDomainSource_NilRegistry(t *testing.T) {
	src := NewDomainSource(nil, map[string]string{"git.example.com": "github"})

	got := src.ProviderDomains(context.Background())
	if len(got) != 1 || got["git.example.com"] != "github" {
		t.Errorf("got %v, want static map only", got)
	}
}

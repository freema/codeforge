package blueprint

import (
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	tests := []struct {
		name    string
		tmpl    string
		params  map[string]string
		want    string
		wantErr bool
	}{
		{
			name:   "happy path",
			tmpl:   "Fix {{.Params.thing}} in {{.Params.repo_url}}",
			params: map[string]string{"thing": "bug", "repo_url": "https://github.com/owner/repo"},
			want:   "Fix bug in https://github.com/owner/repo",
		},
		{
			name: "no template passthrough",
			tmpl: "plain string",
			want: "plain string",
		},
		{
			name:    "missing key errors",
			tmpl:    "{{.Params.nonexistent}}",
			params:  map[string]string{},
			wantErr: true,
		},
		{
			name:   "repoPath helper",
			tmpl:   "{{repoPath .Params.repo_url}}",
			params: map[string]string{"repo_url": "https://github.com/owner/repo.git"},
			want:   "owner/repo",
		},
		{
			name:   "repoHost helper",
			tmpl:   "{{repoHost .Params.repo_url}}",
			params: map[string]string{"repo_url": "https://gitlab.example.com/group/project.git"},
			want:   "https://gitlab.example.com",
		},
		{
			name:   "urlEncode helper",
			tmpl:   "{{urlEncode (repoPath .Params.repo_url)}}",
			params: map[string]string{"repo_url": "https://gitlab.com/owner/repo.git"},
			want:   "owner%2Frepo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(tt.tmpl, TemplateContext{Params: tt.params})
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if got != tt.want {
				t.Errorf("Render = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRender_OutputLimit(t *testing.T) {
	bigValue := strings.Repeat("x", maxTemplateOutput+1)
	_, err := Render("{{.Params.big}}", TemplateContext{Params: map[string]string{"big": bigValue}})
	if err == nil {
		t.Fatal("expected error for output exceeding 1MB")
	}
	if !strings.Contains(err.Error(), "1MB") {
		t.Fatalf("expected 1MB error, got: %v", err)
	}
}

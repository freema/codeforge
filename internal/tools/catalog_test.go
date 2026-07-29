package tools

import "testing"

func TestBuiltinCatalog_HasAllTools(t *testing.T) {
	catalog := BuiltinCatalog()
	if len(catalog) != 6 {
		t.Fatalf("expected 6 built-in tools, got %d", len(catalog))
	}

	expected := []string{"sentry", "jira", "git", "github", "gitlab", "playwright"}
	for i, name := range expected {
		if catalog[i].Name != name {
			t.Errorf("catalog[%d].Name = %q, want %q", i, catalog[i].Name, name)
		}
	}
}

func TestBuiltinCatalog_AllHaveRequiredFields(t *testing.T) {
	for _, def := range BuiltinCatalog() {
		if def.Name == "" {
			t.Error("found tool with empty name")
		}
		if def.Type != ToolTypeMCP {
			t.Errorf("tool %q has type %q, want %q", def.Name, def.Type, ToolTypeMCP)
		}
		if def.Description == "" {
			t.Errorf("tool %q has empty description", def.Name)
		}
		if !def.Builtin {
			t.Errorf("tool %q has Builtin=false", def.Name)
		}
	}
}

func TestBuiltinCatalog_UniqueNames(t *testing.T) {
	seen := make(map[string]bool)
	for _, def := range BuiltinCatalog() {
		if seen[def.Name] {
			t.Errorf("duplicate built-in tool name: %q", def.Name)
		}
		seen[def.Name] = true
	}
}

func TestBuiltinCatalog_SensitiveFieldsHaveEnvVar(t *testing.T) {
	for _, def := range BuiltinCatalog() {
		for _, f := range def.RequiredConfig {
			if f.Sensitive && f.EnvVar == "" {
				t.Errorf("tool %q: sensitive field %q has no EnvVar", def.Name, f.Name)
			}
		}
		for _, f := range def.OptionalConfig {
			if f.Sensitive && f.EnvVar == "" {
				t.Errorf("tool %q: sensitive optional field %q has no EnvVar", def.Name, f.Name)
			}
		}
	}
}

func TestBuiltinCatalog_ProviderKeyWiring(t *testing.T) {
	tests := []struct {
		tool        string
		field       string
		envVar      string
		providerKey string
	}{
		{tool: "github", field: "token", envVar: "GITHUB_PERSONAL_ACCESS_TOKEN", providerKey: "github"},
		{tool: "gitlab", field: "token", envVar: "GITLAB_PERSONAL_ACCESS_TOKEN", providerKey: "gitlab"},
		{tool: "sentry", field: "auth_token", envVar: "SENTRY_ACCESS_TOKEN", providerKey: "sentry"},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			def := BuiltinByName(tt.tool)
			if def == nil {
				t.Fatalf("built-in tool %q not found", tt.tool)
			}
			var field *ConfigField
			for i := range def.RequiredConfig {
				if def.RequiredConfig[i].Name == tt.field {
					field = &def.RequiredConfig[i]
					break
				}
			}
			if field == nil {
				t.Fatalf("tool %q: required field %q not found", tt.tool, tt.field)
			}
			if field.EnvVar != tt.envVar {
				t.Errorf("EnvVar = %q, want %q", field.EnvVar, tt.envVar)
			}
			if field.ProviderKey != tt.providerKey {
				t.Errorf("ProviderKey = %q, want %q", field.ProviderKey, tt.providerKey)
			}
			if !field.Sensitive {
				t.Errorf("tool %q: field %q should be Sensitive", tt.tool, tt.field)
			}
		})
	}
}

func TestBuiltinCatalog_GitLabAPIURLOptional(t *testing.T) {
	def := BuiltinByName("gitlab")
	if def == nil {
		t.Fatal("built-in tool gitlab not found")
	}
	var found bool
	for _, f := range def.OptionalConfig {
		if f.Name == "api_url" {
			found = true
			if f.EnvVar != "GITLAB_API_URL" {
				t.Errorf("api_url EnvVar = %q, want GITLAB_API_URL", f.EnvVar)
			}
			if f.Sensitive {
				t.Error("api_url should not be Sensitive")
			}
		}
	}
	if !found {
		t.Error("gitlab tool missing optional api_url field")
	}
}

func TestBuiltinByName_Found(t *testing.T) {
	def := BuiltinByName("sentry")
	if def == nil {
		t.Fatal("expected to find sentry, got nil")
	}
	if def.Name != "sentry" {
		t.Errorf("Name = %q, want sentry", def.Name)
	}
}

func TestBuiltinByName_NotFound(t *testing.T) {
	def := BuiltinByName("nonexistent")
	if def != nil {
		t.Errorf("expected nil, got %+v", def)
	}
}

func TestBuiltinCatalog_IsACopy(t *testing.T) {
	catalog := BuiltinCatalog()
	catalog[0].Name = "modified"

	original := BuiltinCatalog()
	if original[0].Name == "modified" {
		t.Error("BuiltinCatalog returned a reference, not a copy")
	}
}

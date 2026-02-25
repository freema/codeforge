package tools

import "testing"

func TestBuiltinCatalog_HasAllTools(t *testing.T) {
	catalog := BuiltinCatalog()
	if len(catalog) != 5 {
		t.Fatalf("expected 5 built-in tools, got %d", len(catalog))
	}

	expected := []string{"sentry", "jira", "git", "github", "playwright"}
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

package tools

import (
	"strings"
	"testing"
)

func TestValidateConfig_RequiredPresent(t *testing.T) {
	def := &ToolDefinition{
		Name: "test",
		RequiredConfig: []ConfigField{
			{Name: "token", Type: "secret"},
			{Name: "org", Type: "string"},
		},
	}
	config := map[string]string{
		"token": "abc123",
		"org":   "myorg",
	}
	if err := ValidateConfig(def, config); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidateConfig_RequiredMissing(t *testing.T) {
	def := &ToolDefinition{
		Name: "test",
		RequiredConfig: []ConfigField{
			{Name: "token", Type: "secret"},
		},
	}
	err := ValidateConfig(def, map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing required field")
	}
	if !strings.Contains(err.Error(), "token") {
		t.Errorf("error should mention field name, got: %v", err)
	}
}

func TestValidateConfig_RequiredEmpty(t *testing.T) {
	def := &ToolDefinition{
		Name: "test",
		RequiredConfig: []ConfigField{
			{Name: "token", Type: "secret"},
		},
	}
	err := ValidateConfig(def, map[string]string{"token": ""})
	if err == nil {
		t.Fatal("expected error for empty required field")
	}
}

func TestValidateConfig_TypeURL_Valid(t *testing.T) {
	def := &ToolDefinition{
		Name: "test",
		RequiredConfig: []ConfigField{
			{Name: "endpoint", Type: "url"},
		},
	}
	config := map[string]string{"endpoint": "https://example.com"}
	if err := ValidateConfig(def, config); err != nil {
		t.Errorf("expected no error for valid URL, got %v", err)
	}
}

func TestValidateConfig_TypeURL_Invalid(t *testing.T) {
	def := &ToolDefinition{
		Name: "test",
		RequiredConfig: []ConfigField{
			{Name: "endpoint", Type: "url"},
		},
	}
	config := map[string]string{"endpoint": "not-a-url"}
	err := ValidateConfig(def, config)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
	if !strings.Contains(err.Error(), "valid URL") {
		t.Errorf("error should mention URL validation, got: %v", err)
	}
}

func TestValidateConfig_TypeInt_Valid(t *testing.T) {
	def := &ToolDefinition{
		Name: "test",
		RequiredConfig: []ConfigField{
			{Name: "port", Type: "int"},
		},
	}
	if err := ValidateConfig(def, map[string]string{"port": "8080"}); err != nil {
		t.Errorf("expected no error for valid int, got %v", err)
	}
}

func TestValidateConfig_TypeInt_Invalid(t *testing.T) {
	def := &ToolDefinition{
		Name: "test",
		RequiredConfig: []ConfigField{
			{Name: "port", Type: "int"},
		},
	}
	err := ValidateConfig(def, map[string]string{"port": "abc"})
	if err == nil {
		t.Fatal("expected error for invalid int")
	}
}

func TestValidateConfig_TypeBool_Valid(t *testing.T) {
	def := &ToolDefinition{
		Name: "test",
		OptionalConfig: []ConfigField{
			{Name: "enabled", Type: "bool"},
		},
	}
	for _, v := range []string{"true", "false", "1", "0", "True", "FALSE"} {
		if err := ValidateConfig(def, map[string]string{"enabled": v}); err != nil {
			t.Errorf("expected no error for bool %q, got %v", v, err)
		}
	}
}

func TestValidateConfig_TypeBool_Invalid(t *testing.T) {
	def := &ToolDefinition{
		Name: "test",
		OptionalConfig: []ConfigField{
			{Name: "enabled", Type: "bool"},
		},
	}
	err := ValidateConfig(def, map[string]string{"enabled": "yes"})
	if err == nil {
		t.Fatal("expected error for invalid bool")
	}
}

func TestValidateConfig_OptionalFieldSkipped(t *testing.T) {
	def := &ToolDefinition{
		Name: "test",
		OptionalConfig: []ConfigField{
			{Name: "org", Type: "string"},
		},
	}
	if err := ValidateConfig(def, map[string]string{}); err != nil {
		t.Errorf("expected no error for missing optional field, got %v", err)
	}
}

func TestValidateConfig_NoConfig(t *testing.T) {
	def := &ToolDefinition{Name: "test"}
	if err := ValidateConfig(def, nil); err != nil {
		t.Errorf("expected no error for tool with no config requirements, got %v", err)
	}
}

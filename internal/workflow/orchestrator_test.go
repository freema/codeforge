package workflow

import (
	"testing"
)

func TestValidateParams(t *testing.T) {
	defs := []ParameterDefinition{
		{Name: "required_param", Required: true},
		{Name: "optional_param", Required: false, Default: "default"},
		{Name: "optional_no_default", Required: false},
	}

	// Missing required
	_, err := validateParams(defs, map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing required param")
	}

	// All provided
	result, err := validateParams(defs, map[string]string{
		"required_param": "value",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["optional_param"] != "default" {
		t.Errorf("expected default, got %q", result["optional_param"])
	}
	if _, ok := result["optional_no_default"]; ok {
		t.Error("optional without default should not be in result")
	}
}

func TestValidateParams_EmptyDefs(t *testing.T) {
	result, err := validateParams(nil, map[string]string{"extra": "param"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["extra"] != "param" {
		t.Errorf("expected extra param preserved, got %v", result)
	}
}

func TestValidateParams_DefaultOverride(t *testing.T) {
	defs := []ParameterDefinition{
		{Name: "param", Required: false, Default: "default"},
	}

	result, err := validateParams(defs, map[string]string{"param": "override"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["param"] != "override" {
		t.Errorf("expected override, got %q", result["param"])
	}
}

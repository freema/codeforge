package tools

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ValidateConfig checks that all required config fields are present and
// that typed fields have valid values.
func ValidateConfig(def *ToolDefinition, config map[string]string) error {
	for _, f := range def.RequiredConfig {
		v, ok := config[f.Name]
		if !ok || v == "" {
			return fmt.Errorf("required config field %q is missing for tool %q", f.Name, def.Name)
		}
		if err := validateFieldType(f, v); err != nil {
			return err
		}
	}

	for _, f := range def.OptionalConfig {
		v, ok := config[f.Name]
		if !ok || v == "" {
			continue
		}
		if err := validateFieldType(f, v); err != nil {
			return err
		}
	}

	return nil
}

func validateFieldType(f ConfigField, value string) error {
	switch f.Type {
	case "url":
		u, err := url.Parse(value)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("config field %q must be a valid URL, got %q", f.Name, value)
		}
	case "int":
		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("config field %q must be an integer, got %q", f.Name, value)
		}
	case "bool":
		lower := strings.ToLower(value)
		if lower != "true" && lower != "false" && lower != "1" && lower != "0" {
			return fmt.Errorf("config field %q must be a boolean, got %q", f.Name, value)
		}
	}
	return nil
}

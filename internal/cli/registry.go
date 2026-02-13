package cli

import (
	"fmt"
	"log/slog"
	"os/exec"
)

// Registry manages available CLI tool runners.
type Registry struct {
	runners    map[string]Runner
	defaultCLI string
}

// NewRegistry creates a CLI registry with a default CLI name.
func NewRegistry(defaultCLI string) *Registry {
	return &Registry{
		runners:    make(map[string]Runner),
		defaultCLI: defaultCLI,
	}
}

// Register adds a runner under the given name.
func (r *Registry) Register(name string, runner Runner) {
	r.runners[name] = runner
	slog.Info("CLI registered", "name", name)
}

// Get returns the runner for the given name, or the default if name is empty.
func (r *Registry) Get(name string) (Runner, error) {
	if name == "" {
		name = r.defaultCLI
	}
	runner, ok := r.runners[name]
	if !ok {
		return nil, fmt.Errorf("unknown CLI: %s", name)
	}
	return runner, nil
}

// Available returns the names of all registered CLIs.
func (r *Registry) Available() []string {
	names := make([]string, 0, len(r.runners))
	for name := range r.runners {
		names = append(names, name)
	}
	return names
}

// CheckBinary returns true if the binary exists in PATH.
func CheckBinary(path string) bool {
	_, err := exec.LookPath(path)
	return err == nil
}

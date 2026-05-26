package runner

import (
	"fmt"
	"log/slog"
	"os/exec"
)

// registryEntry holds a runner and its metadata.
type registryEntry struct {
	runner Runner
	meta   RunnerMeta
}

// Registry manages available CLI tool runners.
type Registry struct {
	entries    map[string]registryEntry
	defaultCLI string
}

// NewRegistry creates a CLI registry with a default CLI name.
func NewRegistry(defaultCLI string) *Registry {
	return &Registry{
		entries:    make(map[string]registryEntry),
		defaultCLI: defaultCLI,
	}
}

// Register adds a runner under the given name with its metadata.
func (r *Registry) Register(name string, runner Runner, meta RunnerMeta) {
	r.entries[name] = registryEntry{runner: runner, meta: meta}
	slog.Info("CLI registered", "name", name)
}

// Get returns the runner for the given name, or the default if name is empty.
func (r *Registry) Get(name string) (Runner, error) {
	if name == "" {
		name = r.defaultCLI
	}
	entry, ok := r.entries[name]
	if !ok {
		return nil, fmt.Errorf("unknown CLI: %s", name)
	}
	return entry.runner, nil
}

// GetWithMeta returns the runner and its metadata for the given name.
func (r *Registry) GetWithMeta(name string) (Runner, RunnerMeta, error) {
	if name == "" {
		name = r.defaultCLI
	}
	entry, ok := r.entries[name]
	if !ok {
		return nil, RunnerMeta{}, fmt.Errorf("unknown CLI: %s", name)
	}
	return entry.runner, entry.meta, nil
}

// DefaultCLI returns the name of the default CLI.
func (r *Registry) DefaultCLI() string {
	return r.defaultCLI
}

// Available returns the names of all registered CLIs.
func (r *Registry) Available() []string {
	names := make([]string, 0, len(r.entries))
	for name := range r.entries {
		names = append(names, name)
	}
	return names
}

// CheckBinary returns true if the binary exists in PATH.
func CheckBinary(path string) bool {
	_, err := exec.LookPath(path)
	return err == nil
}

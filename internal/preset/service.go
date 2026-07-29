package preset

import "context"

// Service is a thin layer over the Store. Presets carry no runtime state, so
// the service only forwards calls; it exists so handlers stay decoupled from
// SQL and future logic (per-tenant presets, richer validation) has a home.
type Service struct {
	store *Store
}

// NewService creates a preset service.
func NewService(store *Store) *Service {
	return &Service{store: store}
}

// Create stores a new preset.
func (s *Service) Create(ctx context.Context, p *Preset) error {
	return s.store.Create(ctx, p)
}

// Get returns a preset by ID.
func (s *Service) Get(ctx context.Context, id string) (*Preset, error) {
	return s.store.Get(ctx, id)
}

// List returns all presets ordered by name.
func (s *Service) List(ctx context.Context) ([]*Preset, error) {
	return s.store.List(ctx)
}

// Update persists a preset's mutable fields.
func (s *Service) Update(ctx context.Context, p *Preset) error {
	return s.store.Update(ctx, p)
}

// Delete removes a preset.
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.store.Delete(ctx, id)
}

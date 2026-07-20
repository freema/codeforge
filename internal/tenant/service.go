package tenant

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/freema/codeforge/internal/crypto"
)

// Service provides tenant business logic.
type Service struct {
	store     *Store
	cryptoSvc *crypto.Service
}

// NewService creates a new tenant service.
func NewService(store *Store, cryptoSvc *crypto.Service) *Service {
	return &Service{store: store, cryptoSvc: cryptoSvc}
}

// Store returns the underlying store for direct access when needed.
func (s *Service) Store() *Store {
	return s.store
}

// CreateTenantResult holds the newly created tenant and its plain-text token (shown once).
type CreateTenantResult struct {
	Tenant     *Tenant `json:"tenant"`
	PlainToken string  `json:"api_token"`
}

// CreateTenant creates a new tenant with a generated API token.
func (s *Service) CreateTenant(ctx context.Context, name, slug, tier string) (*CreateTenantResult, error) {
	token, err := generateToken(32)
	if err != nil {
		return nil, fmt.Errorf("generating token: %w", err)
	}

	hash := hashToken(token)

	t := &Tenant{
		ID:                     generateID(),
		Name:                   name,
		Slug:                   slug,
		Tier:                   tier,
		APITokenHash:           hash,
		MaxSessionsPerDay:      defaultsForTier(tier).sessionsPerDay,
		MaxConcurrentSessions:  defaultsForTier(tier).concurrent,
		MaxBudgetUSDPerSession: defaultsForTier(tier).budgetUSD,
		AllowedCLIs:            defaultsForTier(tier).clis,
	}

	if err := s.store.CreateTenant(ctx, t); err != nil {
		return nil, err
	}

	return &CreateTenantResult{Tenant: t, PlainToken: token}, nil
}

// ResolveKeyFromPool returns a decrypted API key from the key pool for the given provider.
func (s *Service) ResolveKeyFromPool(ctx context.Context, provider string) (string, error) {
	entry, err := s.store.GetActiveKeyForProvider(ctx, provider)
	if err != nil {
		return "", err
	}
	decrypted, err := s.cryptoSvc.Decrypt(entry.EncryptedToken)
	if err != nil {
		return "", fmt.Errorf("decrypting pool key: %w", err)
	}
	return decrypted, nil
}

type tierDefaults struct {
	sessionsPerDay int
	concurrent     int
	budgetUSD      float64
	clis           string
}

func defaultsForTier(tier string) tierDefaults {
	switch tier {
	case TierPro:
		return tierDefaults{100, 10, 10.0, `["claude-code","codex","cursor","claude-agent"]`}
	case TierEnterprise:
		return tierDefaults{-1, 50, 50.0, `["claude-code","codex","cursor","claude-agent"]`}
	default:
		return tierDefaults{10, 2, 1.0, `["claude-code"]`}
	}
}

func generateToken(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "cfk_" + hex.EncodeToString(b), nil
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// HashToken exports the token hashing function for middleware use.
func HashToken(token string) string {
	return hashToken(token)
}

package keys

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Key represents a stored access token.
type Key struct {
	Name      string    `json:"name"`
	Provider  string    `json:"provider"`
	Token     string    `json:"token,omitempty"` // only in create request, never in responses
	Scope     string    `json:"scope,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// VerifyResult contains the result of a provider token verification.
type VerifyResult struct {
	Valid    bool   `json:"valid"`
	Username string `json:"username,omitempty"`
	Email    string `json:"email,omitempty"`
	Scopes   string `json:"scopes,omitempty"`
	Error    string `json:"error,omitempty"`
}

// Registry manages encrypted access tokens.
type Registry interface {
	Create(ctx context.Context, key Key) error
	List(ctx context.Context) ([]Key, error)
	Delete(ctx context.Context, name string) error
	Resolve(ctx context.Context, provider, name string) (string, error)
	Verify(ctx context.Context, name string) (*VerifyResult, string, error)
	// ResolveByName looks up a key by name (regardless of provider) and returns
	// the decrypted token and provider.
	ResolveByName(ctx context.Context, name string) (token, provider string, err error)
}

func verifyToken(ctx context.Context, provider, token string) *VerifyResult {
	switch provider {
	case "github":
		return verifyGitHub(ctx, token)
	case "gitlab":
		return verifyGitLab(ctx, token)
	case "sentry":
		return verifySentry(ctx, token)
	default:
		return &VerifyResult{Valid: false, Error: "unsupported provider"}
	}
}

func verifyGitHub(ctx context.Context, token string) *VerifyResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return &VerifyResult{Valid: false, Error: "failed to create request"}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &VerifyResult{Valid: false, Error: "connection failed"}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return &VerifyResult{Valid: false, Error: "invalid or expired token"}
	}
	if resp.StatusCode != http.StatusOK {
		return &VerifyResult{Valid: false, Error: fmt.Sprintf("unexpected status %d", resp.StatusCode)}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return &VerifyResult{Valid: true} // token works, just can't parse body
	}

	var user struct {
		Login string `json:"login"`
		Email string `json:"email"`
	}
	_ = json.Unmarshal(body, &user)

	return &VerifyResult{
		Valid:    true,
		Username: user.Login,
		Email:    user.Email,
		Scopes:   resp.Header.Get("X-OAuth-Scopes"),
	}
}

func verifySentry(ctx context.Context, token string) *VerifyResult {
	type sentryOrg struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	}

	// Sentry has regional endpoints — US (default) and EU.
	// A user's orgs may be spread across regions.
	regions := []string{
		"https://sentry.io/api/0/organizations/",
		"https://de.sentry.io/api/0/organizations/",
	}

	var allOrgs []sentryOrg
	validated := false

	for _, regionURL := range regions {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, regionURL, nil)
		if err != nil {
			continue
		}
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			continue
		}

		if !validated {
			if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
				resp.Body.Close()
				return &VerifyResult{Valid: false, Error: "invalid or expired token"}
			}
			validated = true
		}

		if resp.StatusCode == http.StatusOK {
			body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
			resp.Body.Close()
			if err == nil {
				var orgs []sentryOrg
				_ = json.Unmarshal(body, &orgs)
				allOrgs = append(allOrgs, orgs...)
			}
		} else {
			resp.Body.Close()
		}
	}

	if !validated {
		return &VerifyResult{Valid: false, Error: "connection failed"}
	}

	scopes := ""
	if len(allOrgs) > 0 {
		names := make([]string, len(allOrgs))
		for i, o := range allOrgs {
			names[i] = o.Slug
		}
		scopes = fmt.Sprintf("%d organization(s): %s", len(allOrgs), strings.Join(names, ", "))
	}

	return &VerifyResult{
		Valid:  true,
		Scopes: scopes,
	}
}

func verifyGitLab(ctx context.Context, token string) *VerifyResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://gitlab.com/api/v4/user", nil)
	if err != nil {
		return &VerifyResult{Valid: false, Error: "failed to create request"}
	}
	req.Header.Set("PRIVATE-TOKEN", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &VerifyResult{Valid: false, Error: "connection failed"}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return &VerifyResult{Valid: false, Error: "invalid or expired token"}
	}
	if resp.StatusCode != http.StatusOK {
		return &VerifyResult{Valid: false, Error: fmt.Sprintf("unexpected status %d", resp.StatusCode)}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return &VerifyResult{Valid: true}
	}

	var user struct {
		Username string `json:"username"`
		Email    string `json:"email"`
	}
	_ = json.Unmarshal(body, &user)

	return &VerifyResult{
		Valid:    true,
		Username: user.Username,
		Email:    user.Email,
	}
}

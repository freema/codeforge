package session

import (
	"context"
	"errors"
	"testing"
)

// recordingTokenResolver stands in for the operator's key registry and
// GITHUB_TOKEN/GITLAB_TOKEN fallback.
type recordingTokenResolver struct{ calls int }

func (r *recordingTokenResolver) ResolveToken(context.Context, string, string, string) (string, error) {
	r.calls++
	return "operator-token", nil
}

func TestResolveAccessToken(t *testing.T) {
	ctx := context.Background()

	t.Run("tenant session without a token gets no operator token", func(t *testing.T) {
		res := &recordingTokenResolver{}
		s := &PRService{tokenResolver: res}
		sess := &Session{RepoURL: "https://github.com/acme/repo.git", TenantID: "acme", ProviderKey: "operator-github"}
		if err := s.resolveAccessToken(ctx, sess); !errors.Is(err, errTenantNeedsAccessToken) {
			t.Fatalf("err = %v, want errTenantNeedsAccessToken", err)
		}
		if sess.AccessToken != "" || res.calls != 0 {
			t.Errorf("token = %q, resolver calls = %d; want neither", sess.AccessToken, res.calls)
		}
	})

	t.Run("tenant session keeps its own token", func(t *testing.T) {
		res := &recordingTokenResolver{}
		s := &PRService{tokenResolver: res}
		sess := &Session{TenantID: "acme", AccessToken: "tenant-token"}
		if err := s.resolveAccessToken(ctx, sess); err != nil {
			t.Fatal(err)
		}
		if sess.AccessToken != "tenant-token" || res.calls != 0 {
			t.Errorf("token = %q, resolver calls = %d", sess.AccessToken, res.calls)
		}
	})

	t.Run("operator session falls back to the resolver", func(t *testing.T) {
		res := &recordingTokenResolver{}
		s := &PRService{tokenResolver: res}
		sess := &Session{RepoURL: "https://github.com/acme/repo.git"}
		if err := s.resolveAccessToken(ctx, sess); err != nil {
			t.Fatal(err)
		}
		if sess.AccessToken != "operator-token" {
			t.Errorf("token = %q, want operator-token", sess.AccessToken)
		}
	})
}

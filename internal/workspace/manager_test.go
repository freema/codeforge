package workspace

import (
	"testing"
	"time"
)

func TestWorkspaceIsExpired(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name       string
		createdAt  time.Time
		lastAccess time.Time
		ttlSeconds int64
		expired    bool
	}{
		{
			name:       "fresh workspace, no last access",
			createdAt:  now.Add(-1 * time.Hour),
			ttlSeconds: 24 * 3600,
			expired:    false,
		},
		{
			name:       "old workspace, no last access",
			createdAt:  now.Add(-25 * time.Hour),
			ttlSeconds: 24 * 3600,
			expired:    true,
		},
		{
			name:       "old workspace kept alive by recent access",
			createdAt:  now.Add(-25 * time.Hour),
			lastAccess: now.Add(-1 * time.Hour),
			ttlSeconds: 24 * 3600,
			expired:    false,
		},
		{
			name:       "old workspace with stale last access",
			createdAt:  now.Add(-72 * time.Hour),
			lastAccess: now.Add(-25 * time.Hour),
			ttlSeconds: 24 * 3600,
			expired:    true,
		},
		{
			name:       "fresh workspace with last access equal to creation",
			createdAt:  now.Add(-10 * time.Minute),
			lastAccess: now.Add(-10 * time.Minute),
			ttlSeconds: 24 * 3600,
			expired:    false,
		},
		{
			name:       "zero-value last access falls back to created_at",
			createdAt:  now.Add(-2 * time.Hour),
			lastAccess: time.Time{},
			ttlSeconds: 3600,
			expired:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := &Workspace{
				CreatedAt:  tt.createdAt,
				LastAccess: tt.lastAccess,
				TTL:        tt.ttlSeconds,
			}
			if got := ws.IsExpired(); got != tt.expired {
				t.Errorf("IsExpired() = %v, want %v", got, tt.expired)
			}
		})
	}
}

func TestWorkspaceExpiresAt(t *testing.T) {
	created := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	accessed := time.Date(2026, 7, 2, 9, 30, 0, 0, time.UTC)

	tests := []struct {
		name       string
		ws         Workspace
		wantExpiry time.Time
	}{
		{
			name:       "no last access uses created_at",
			ws:         Workspace{CreatedAt: created, TTL: 3600},
			wantExpiry: created.Add(time.Hour),
		},
		{
			name:       "last access shifts expiry",
			ws:         Workspace{CreatedAt: created, LastAccess: accessed, TTL: 3600},
			wantExpiry: accessed.Add(time.Hour),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ws.ExpiresAt(); !got.Equal(tt.wantExpiry) {
				t.Errorf("ExpiresAt() = %v, want %v", got, tt.wantExpiry)
			}
		})
	}
}

func TestHashToWorkspaceLastAccess(t *testing.T) {
	created := "2026-07-01T12:00:00.000000000Z"
	accessed := "2026-07-02T09:30:00.000000000Z"

	tests := []struct {
		name           string
		fields         map[string]string
		wantLastAccess time.Time
	}{
		{
			name: "last_access present",
			fields: map[string]string{
				"task_id":     "s1",
				"created_at":  created,
				"last_access": accessed,
				"ttl":         "86400",
			},
			wantLastAccess: time.Date(2026, 7, 2, 9, 30, 0, 0, time.UTC),
		},
		{
			name: "legacy hash without last_access",
			fields: map[string]string{
				"task_id":    "s1",
				"created_at": created,
				"ttl":        "86400",
			},
			wantLastAccess: time.Time{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := hashToWorkspace(tt.fields)
			if ws == nil {
				t.Fatal("hashToWorkspace() returned nil")
			}
			if !ws.LastAccess.Equal(tt.wantLastAccess) {
				t.Errorf("LastAccess = %v, want %v", ws.LastAccess, tt.wantLastAccess)
			}
		})
	}
}

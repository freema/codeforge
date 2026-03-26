package session

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/freema/codeforge/internal/ai"
	"github.com/freema/codeforge/internal/slug"
	gitpkg "github.com/freema/codeforge/internal/tool/git"
	"github.com/freema/codeforge/internal/tool/runner"
)

// WorkspacePathResolver resolves the filesystem path for a session workspace.
type WorkspacePathResolver interface {
	WorkspacePath(ctx context.Context, sessionID string) string
}

// PRServiceConfig holds configuration for PR creation.
type PRServiceConfig struct {
	WorkspaceBase   string
	BranchPrefix    string
	CommitAuthor    string
	CommitEmail     string
	ProviderDomains map[string]string
}

// TokenResolver resolves access tokens for sessions.
type TokenResolver interface {
	ResolveToken(ctx context.Context, repoURL, accessToken, providerKey string) (string, error)
}

// PRService orchestrates the PR/MR creation workflow.
type PRService struct {
	sessionService    *Service
	analyzer          *runner.Analyzer
	workspaceResolver WorkspacePathResolver
	tokenResolver     TokenResolver
	cfg               PRServiceConfig
	ai                ai.Client // optional, nil = no AI commit messages
}

// NewPRService creates a PR service.
func NewPRService(sessionService *Service, analyzer *runner.Analyzer, workspaceResolver WorkspacePathResolver, tokenResolver TokenResolver, cfg PRServiceConfig, aiClient ...ai.Client) *PRService {
	svc := &PRService{
		sessionService:    sessionService,
		analyzer:          analyzer,
		workspaceResolver: workspaceResolver,
		tokenResolver:     tokenResolver,
		cfg:               cfg,
	}
	if len(aiClient) > 0 {
		svc.ai = aiClient[0]
	}
	return svc
}

// CreatePRRequest is the request body for POST /sessions/:id/create-pr.
type CreatePRRequest struct {
	Title        string `json:"title,omitempty"`
	Description  string `json:"description,omitempty"`
	TargetBranch string `json:"target_branch,omitempty"`
}

// CreatePRResponse is the response for a successful PR creation.
type CreatePRResponse struct {
	PRURL    string `json:"pr_url"`
	PRNumber int    `json:"pr_number"`
	Branch   string `json:"branch"`
}

// CreatePR orchestrates the full PR creation: analyze → branch → commit → push → create PR.
func (s *PRService) CreatePR(ctx context.Context, sessionID string, req CreatePRRequest) (*CreatePRResponse, error) {
	// Load session
	t, err := s.sessionService.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	// Validate state — allow PR creation from completed or pr_created (re-push after new iteration)
	if t.Status != StatusCompleted && t.Status != StatusPRCreated {
		return nil, fmt.Errorf("session must be in completed or pr_created status, currently: %s", t.Status)
	}

	// Resolve workDir early — needed for lazy change recalculation.
	workDir := filepath.Join(s.cfg.WorkspaceBase, sessionID)
	if s.workspaceResolver != nil {
		if resolved := s.workspaceResolver.WorkspacePath(ctx, sessionID); resolved != "" {
			workDir = resolved
		}
	}

	// Check for changes — lazy recalculation if summary is nil but workspace exists.
	if t.ChangesSummary == nil || (t.ChangesSummary.FilesModified == 0 && t.ChangesSummary.FilesCreated == 0 && t.ChangesSummary.FilesDeleted == 0) {
		recalc, err := gitpkg.CalculateChanges(ctx, workDir)
		if err == nil && recalc != nil && (recalc.FilesModified > 0 || recalc.FilesCreated > 0 || recalc.FilesDeleted > 0) {
			slog.Info("recalculated changes for PR", "session_id", sessionID, "modified", recalc.FilesModified, "created", recalc.FilesCreated, "deleted", recalc.FilesDeleted)
			t.ChangesSummary = recalc
		} else {
			return nil, fmt.Errorf("no changes to create PR for")
		}
	}

	// Resolve access token (inline → registry → env) if not already set.
	if s.tokenResolver != nil && t.AccessToken == "" {
		token, err := s.tokenResolver.ResolveToken(ctx, t.RepoURL, t.AccessToken, t.ProviderKey)
		if err != nil {
			return nil, fmt.Errorf("resolving access token for PR: %w", err)
		}
		t.AccessToken = token
	}

	// Remember previous status so we can revert on non-fatal errors
	previousStatus := t.Status

	// Transition to CREATING_PR
	if err := s.sessionService.UpdateStatus(ctx, sessionID, StatusCreatingPR); err != nil {
		return nil, fmt.Errorf("transitioning to creating_pr: %w", err)
	}

	// Parse repo URL to detect provider
	repoInfo, err := gitpkg.ParseRepoURL(t.RepoURL, s.cfg.ProviderDomains)
	if err != nil {
		s.failPR(ctx, sessionID, err)
		return nil, fmt.Errorf("parsing repo URL: %w", err)
	}

	if repoInfo.Provider == gitpkg.ProviderUnknown {
		err := fmt.Errorf("PR creation not supported for host: %s", repoInfo.Host)
		s.failPR(ctx, sessionID, err)
		return nil, err
	}

	// Auto-generate PR metadata if not provided
	title := req.Title
	description := req.Description
	var branchSlug string

	if title == "" || description == "" {
		analysis := s.analyzer.Analyze(ctx, t.Prompt, sessionID)
		if title == "" {
			title = analysis.PRTitle
		}
		if description == "" {
			description = analysis.Description
		}
		branchSlug = analysis.BranchSlug
	} else {
		branchSlug = slug.Generate(t.Prompt, sessionID)
	}

	baseBranch := req.TargetBranch
	if baseBranch == "" {
		if t.Config != nil && t.Config.TargetBranch != "" {
			baseBranch = t.Config.TargetBranch
		} else {
			// Detect default branch from the cloned repo
			if detected, err := gitpkg.DefaultBranch(ctx, workDir); err == nil && detected != "" {
				baseBranch = detected
			} else {
				baseBranch = "main"
			}
		}
	}

	// Generate branch name
	branchName := gitpkg.GenerateBranchName(ctx, workDir, s.cfg.BranchPrefix, branchSlug)

	// Create commit message — try AI, fall back to formatted message
	commitMsg := gitpkg.FormatCommitMessage(title, sessionID, s.cfg.CommitAuthor, s.cfg.CommitEmail)
	if s.ai != nil {
		if diffOut, diffErr := gitpkg.GetUnstagedDiff(ctx, workDir); diffErr == nil && diffOut != "" {
			if generated := ai.GenerateCommitMessage(ctx, s.ai, diffOut, t.Prompt); generated != "" {
				commitMsg = generated
			}
		}
	}

	// Create branch, commit, push
	err = gitpkg.CreateBranchAndPush(ctx, gitpkg.BranchOptions{
		WorkDir:     workDir,
		BranchName:  branchName,
		BaseBranch:  baseBranch,
		CommitMsg:   commitMsg,
		AuthorName:  s.cfg.CommitAuthor,
		AuthorEmail: s.cfg.CommitEmail,
		Token:       t.AccessToken,
	})
	if err != nil {
		// Revert status back instead of failing the session — user can retry or send new instructions
		_ = s.sessionService.UpdateStatus(ctx, sessionID, previousStatus)
		return nil, fmt.Errorf("creating branch and pushing: %w", err)
	}

	// Create PR/MR on provider
	prResult, err := gitpkg.CreatePR(ctx, repoInfo, t.AccessToken, gitpkg.PRCreateOptions{
		Title:       title,
		Description: description,
		Branch:      branchName,
		BaseBranch:  baseBranch,
	})
	if err != nil {
		s.failPR(ctx, sessionID, err)
		return nil, fmt.Errorf("creating PR: %w", err)
	}

	// Update session state with PR info
	stateKey := s.sessionService.redis.Key("session", sessionID, "state")
	s.sessionService.redis.Unwrap().HSet(ctx, stateKey, map[string]interface{}{
		"branch":    branchName,
		"pr_url":    prResult.URL,
		"pr_number": prResult.Number,
	})

	s.sessionService.persistToSQLite(func() error {
		return s.sessionService.sqlite.UpdatePR(ctx, sessionID, branchName, prResult.URL, prResult.Number)
	})

	// Transition to PR_CREATED
	if err := s.sessionService.UpdateStatus(ctx, sessionID, StatusPRCreated); err != nil {
		slog.Error("failed to transition to pr_created", "session_id", sessionID, "error", err)
	}

	slog.Info("PR created", "session_id", sessionID, "pr_url", prResult.URL, "branch", branchName)

	return &CreatePRResponse{
		PRURL:    prResult.URL,
		PRNumber: prResult.Number,
		Branch:   branchName,
	}, nil
}

// PushToPRResponse is the response for a successful push to an existing PR.
type PushToPRResponse struct {
	PRURL   string `json:"pr_url"`
	Branch  string `json:"branch"`
	Message string `json:"message"`
}

// PushToPR pushes new changes to an existing PR branch without creating a new PR/MR.
// The existing MR/PR on GitLab/GitHub auto-updates when the branch gets new commits.
func (s *PRService) PushToPR(ctx context.Context, sessionID string) (*PushToPRResponse, error) {
	// Load session
	t, err := s.sessionService.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	// Validate state
	if t.Status != StatusCompleted && t.Status != StatusPRCreated {
		return nil, fmt.Errorf("session must be in completed or pr_created status, currently: %s", t.Status)
	}

	// Validate that a PR was previously created
	if t.Branch == "" {
		return nil, fmt.Errorf("no existing PR — use create-pr first")
	}

	// Resolve workspace dir
	workDir := filepath.Join(s.cfg.WorkspaceBase, sessionID)
	if s.workspaceResolver != nil {
		if resolved := s.workspaceResolver.WorkspacePath(ctx, sessionID); resolved != "" {
			workDir = resolved
		}
	}

	// Resolve access token if not already set
	if s.tokenResolver != nil && t.AccessToken == "" {
		token, err := s.tokenResolver.ResolveToken(ctx, t.RepoURL, t.AccessToken, t.ProviderKey)
		if err != nil {
			return nil, fmt.Errorf("resolving access token for push: %w", err)
		}
		t.AccessToken = token
	}

	// Generate commit message — try AI, fall back to generic
	commitMsg := "follow-up changes"
	if s.ai != nil {
		if diffOut, diffErr := gitpkg.GetUnstagedDiff(ctx, workDir); diffErr == nil && diffOut != "" {
			if generated := ai.GenerateCommitMessage(ctx, s.ai, diffOut, t.Prompt); generated != "" {
				commitMsg = generated
			}
		}
	}

	// Stage, commit, and push to existing branch
	if err := gitpkg.CommitAndPushToExisting(ctx, gitpkg.PushExistingOptions{
		WorkDir:     workDir,
		BranchName:  t.Branch,
		CommitMsg:   commitMsg,
		AuthorName:  s.cfg.CommitAuthor,
		AuthorEmail: s.cfg.CommitEmail,
		Token:       t.AccessToken,
	}); err != nil {
		return nil, err
	}

	slog.Info("pushed to existing PR", "session_id", sessionID, "branch", t.Branch)

	// Recalculate changes summary
	recalc, err := gitpkg.CalculateChanges(ctx, workDir)
	if err == nil && recalc != nil {
		t.ChangesSummary = recalc
		stateKey := s.sessionService.redis.Key("session", sessionID, "state")
		s.sessionService.redis.Unwrap().HSet(ctx, stateKey, "changes_summary", MarshalChangesSummary(recalc))
	}

	// Ensure status is pr_created
	if t.Status != StatusPRCreated {
		if err := s.sessionService.UpdateStatus(ctx, sessionID, StatusPRCreated); err != nil {
			slog.Error("failed to transition to pr_created after push", "session_id", sessionID, "error", err)
		}
	}

	return &PushToPRResponse{
		PRURL:   t.PRURL,
		Branch:  t.Branch,
		Message: "Changes pushed to existing PR",
	}, nil
}

// GetPRStatus checks the current status of a session's PR/MR on the provider.
func (s *PRService) GetPRStatus(ctx context.Context, sessionID string) (*gitpkg.PRStatus, error) {
	t, err := s.sessionService.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if t.PRNumber == 0 {
		return nil, fmt.Errorf("session has no PR")
	}

	repoInfo, err := gitpkg.ParseRepoURL(t.RepoURL, s.cfg.ProviderDomains)
	if err != nil {
		return nil, fmt.Errorf("parsing repo URL: %w", err)
	}

	// Resolve token
	if s.tokenResolver != nil && t.AccessToken == "" {
		token, resolveErr := s.tokenResolver.ResolveToken(ctx, t.RepoURL, t.AccessToken, t.ProviderKey)
		if resolveErr != nil {
			return nil, fmt.Errorf("resolving token: %w", resolveErr)
		}
		t.AccessToken = token
	}

	status, err := gitpkg.GetPRStatus(ctx, repoInfo, t.AccessToken, t.PRNumber)
	if err != nil {
		return nil, err
	}

	// Sync session status with PR state:
	// PR merged or closed → transition session to completed (work is done)
	if (status.State == "merged" || status.State == "closed") && t.Status == StatusPRCreated {
		if err := s.sessionService.UpdateStatus(ctx, sessionID, StatusCompleted); err != nil {
			slog.Warn("failed to sync session status from PR", "session_id", sessionID, "pr_state", status.State, "error", err)
		} else {
			slog.Info("session status synced from PR", "session_id", sessionID, "pr_state", status.State)
		}
	}

	return status, nil
}

func (s *PRService) failPR(ctx context.Context, sessionID string, err error) {
	slog.Error("PR creation failed", "session_id", sessionID, "error", err)
	_ = s.sessionService.SetError(ctx, sessionID, fmt.Sprintf("PR creation failed: %v", err))
	_ = s.sessionService.UpdateStatus(ctx, sessionID, StatusFailed)
}

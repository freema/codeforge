package task

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/freema/codeforge/internal/tool/runner"
	gitpkg "github.com/freema/codeforge/internal/tool/git"
	"github.com/freema/codeforge/internal/slug"
)

// WorkspacePathResolver resolves the filesystem path for a task workspace.
type WorkspacePathResolver interface {
	WorkspacePath(ctx context.Context, taskID string) string
}

// PRServiceConfig holds configuration for PR creation.
type PRServiceConfig struct {
	WorkspaceBase   string
	BranchPrefix    string
	CommitAuthor    string
	CommitEmail     string
	ProviderDomains map[string]string
}

// TokenResolver resolves access tokens for tasks.
type TokenResolver interface {
	ResolveToken(ctx context.Context, repoURL, accessToken, providerKey string) (string, error)
}

// PRService orchestrates the PR/MR creation workflow.
type PRService struct {
	taskService       *Service
	analyzer          *runner.Analyzer
	workspaceResolver WorkspacePathResolver
	tokenResolver     TokenResolver
	cfg               PRServiceConfig
}

// NewPRService creates a PR service.
func NewPRService(taskService *Service, analyzer *runner.Analyzer, workspaceResolver WorkspacePathResolver, tokenResolver TokenResolver, cfg PRServiceConfig) *PRService {
	return &PRService{
		taskService:       taskService,
		analyzer:          analyzer,
		workspaceResolver: workspaceResolver,
		tokenResolver:     tokenResolver,
		cfg:               cfg,
	}
}

// CreatePRRequest is the request body for POST /tasks/:id/create-pr.
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
func (s *PRService) CreatePR(ctx context.Context, taskID string, req CreatePRRequest) (*CreatePRResponse, error) {
	// Load task
	t, err := s.taskService.Get(ctx, taskID)
	if err != nil {
		return nil, err
	}

	// Validate state
	if t.Status != StatusCompleted {
		return nil, fmt.Errorf("task must be in completed status, currently: %s", t.Status)
	}

	// Resolve workDir early — needed for lazy change recalculation.
	workDir := filepath.Join(s.cfg.WorkspaceBase, taskID)
	if s.workspaceResolver != nil {
		if resolved := s.workspaceResolver.WorkspacePath(ctx, taskID); resolved != "" {
			workDir = resolved
		}
	}

	// Check for changes — lazy recalculation if summary is nil but workspace exists.
	if t.ChangesSummary == nil || (t.ChangesSummary.FilesModified == 0 && t.ChangesSummary.FilesCreated == 0 && t.ChangesSummary.FilesDeleted == 0) {
		recalc, err := gitpkg.CalculateChanges(ctx, workDir)
		if err == nil && recalc != nil && (recalc.FilesModified > 0 || recalc.FilesCreated > 0 || recalc.FilesDeleted > 0) {
			slog.Info("recalculated changes for PR", "task_id", taskID, "modified", recalc.FilesModified, "created", recalc.FilesCreated, "deleted", recalc.FilesDeleted)
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

	// Transition to CREATING_PR
	if err := s.taskService.UpdateStatus(ctx, taskID, StatusCreatingPR); err != nil {
		return nil, fmt.Errorf("transitioning to creating_pr: %w", err)
	}

	// Parse repo URL to detect provider
	repoInfo, err := gitpkg.ParseRepoURL(t.RepoURL, s.cfg.ProviderDomains)
	if err != nil {
		s.failPR(ctx, taskID, err)
		return nil, fmt.Errorf("parsing repo URL: %w", err)
	}

	if repoInfo.Provider == gitpkg.ProviderUnknown {
		err := fmt.Errorf("PR creation not supported for host: %s", repoInfo.Host)
		s.failPR(ctx, taskID, err)
		return nil, err
	}

	// Auto-generate PR metadata if not provided
	title := req.Title
	description := req.Description
	var branchSlug string

	if title == "" || description == "" {
		analysis := s.analyzer.Analyze(ctx, t.Prompt, taskID)
		if title == "" {
			title = analysis.PRTitle
		}
		if description == "" {
			description = analysis.Description
		}
		branchSlug = analysis.BranchSlug
	} else {
		branchSlug = slug.Generate(t.Prompt, taskID)
	}

	baseBranch := req.TargetBranch
	if baseBranch == "" {
		if t.Config != nil && t.Config.TargetBranch != "" {
			baseBranch = t.Config.TargetBranch
		} else {
			baseBranch = "main"
		}
	}

	// Generate branch name
	branchName := gitpkg.GenerateBranchName(ctx, workDir, s.cfg.BranchPrefix, branchSlug)

	// Create commit message
	commitMsg := gitpkg.FormatCommitMessage(title, taskID, s.cfg.CommitAuthor, s.cfg.CommitEmail)

	// Create branch, commit, push
	err = gitpkg.CreateBranchAndPush(ctx, gitpkg.BranchOptions{
		WorkDir:     workDir,
		BranchName:  branchName,
		CommitMsg:   commitMsg,
		AuthorName:  s.cfg.CommitAuthor,
		AuthorEmail: s.cfg.CommitEmail,
		Token:       t.AccessToken,
	})
	if err != nil {
		s.failPR(ctx, taskID, err)
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
		s.failPR(ctx, taskID, err)
		return nil, fmt.Errorf("creating PR: %w", err)
	}

	// Update task state with PR info
	stateKey := s.taskService.redis.Key("task", taskID, "state")
	s.taskService.redis.Unwrap().HSet(ctx, stateKey, map[string]interface{}{
		"branch":    branchName,
		"pr_url":    prResult.URL,
		"pr_number": prResult.Number,
	})

	s.taskService.persistToSQLite(func() error {
		return s.taskService.sqlite.UpdatePR(ctx, taskID, branchName, prResult.URL, prResult.Number)
	})

	// Transition to PR_CREATED
	if err := s.taskService.UpdateStatus(ctx, taskID, StatusPRCreated); err != nil {
		slog.Error("failed to transition to pr_created", "task_id", taskID, "error", err)
	}

	slog.Info("PR created", "task_id", taskID, "pr_url", prResult.URL, "branch", branchName)

	return &CreatePRResponse{
		PRURL:    prResult.URL,
		PRNumber: prResult.Number,
		Branch:   branchName,
	}, nil
}

func (s *PRService) failPR(ctx context.Context, taskID string, err error) {
	slog.Error("PR creation failed", "task_id", taskID, "error", err)
	_ = s.taskService.SetError(ctx, taskID, fmt.Sprintf("PR creation failed: %v", err))
	_ = s.taskService.UpdateStatus(ctx, taskID, StatusFailed)
}

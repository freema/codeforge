package task

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/freema/codeforge/internal/cli"
	gitpkg "github.com/freema/codeforge/internal/git"
)

// PRServiceConfig holds configuration for PR creation.
type PRServiceConfig struct {
	WorkspaceBase   string
	BranchPrefix    string
	CommitAuthor    string
	CommitEmail     string
	ProviderDomains map[string]string
}

// PRService orchestrates the PR/MR creation workflow.
type PRService struct {
	taskService *Service
	analyzer    *cli.Analyzer
	cfg         PRServiceConfig
}

// NewPRService creates a PR service.
func NewPRService(taskService *Service, analyzer *cli.Analyzer, cfg PRServiceConfig) *PRService {
	return &PRService{
		taskService: taskService,
		analyzer:    analyzer,
		cfg:         cfg,
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

	// Check for changes
	if t.ChangesSummary == nil || (t.ChangesSummary.FilesModified == 0 && t.ChangesSummary.FilesCreated == 0 && t.ChangesSummary.FilesDeleted == 0) {
		return nil, fmt.Errorf("no changes to create PR for")
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
		diffStats := ""
		if t.ChangesSummary != nil {
			diffStats = t.ChangesSummary.DiffStats
		}
		analysis := s.analyzer.Analyze(ctx, t.Prompt, diffStats, taskID)
		if title == "" {
			title = analysis.PRTitle
		}
		if description == "" {
			description = analysis.Description
		}
		branchSlug = analysis.BranchSlug
	} else {
		branchSlug = "task-" + taskID[:8]
	}

	baseBranch := req.TargetBranch
	if baseBranch == "" {
		if t.Config != nil && t.Config.TargetBranch != "" {
			baseBranch = t.Config.TargetBranch
		} else {
			baseBranch = "main"
		}
	}

	workDir := filepath.Join(s.cfg.WorkspaceBase, taskID)

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
	s.taskService.SetError(ctx, taskID, fmt.Sprintf("PR creation failed: %v", err))
	s.taskService.UpdateStatus(ctx, taskID, StatusFailed)
}

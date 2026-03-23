package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/freema/codeforge/internal/keys"
	"github.com/freema/codeforge/internal/redisclient"
	"github.com/freema/codeforge/internal/session"
	"github.com/freema/codeforge/internal/tools"
)

// SessionCreator creates and retrieves CodeForge tasks.
type SessionCreator interface {
	Create(ctx context.Context, req session.CreateSessionRequest) (*session.Session, error)
	Get(ctx context.Context, taskID string) (*session.Session, error)
}

// SessionExecutor executes task steps — creates a CodeForge task and waits for completion.
type SessionExecutor struct {
	tasks SessionCreator
	redis *redisclient.Client
	keys  keys.Registry
}

// NewSessionExecutor creates a new session step executor.
func NewSessionExecutor(tasks SessionCreator, redis *redisclient.Client, keyReg ...keys.Registry) *SessionExecutor {
	e := &SessionExecutor{
		tasks: tasks,
		redis: redis,
	}
	if len(keyReg) > 0 {
		e.keys = keyReg[0]
	}
	return e
}

// Execute creates a CodeForge task and waits for it to complete via Redis done channel.
// Returns the session ID and result summary as outputs.
func (e *SessionExecutor) Execute(ctx context.Context, stepDef StepDefinition, tctx TemplateContext) (map[string]string, error) {
	var cfg SessionStepConfig
	if err := json.Unmarshal(stepDef.Config, &cfg); err != nil {
		return nil, fmt.Errorf("parsing session config: %w", err)
	}

	// Render templates
	repoURL, err := Render(cfg.RepoURL, tctx)
	if err != nil {
		return nil, fmt.Errorf("rendering repo_url: %w", err)
	}
	prompt, err := Render(cfg.Prompt, tctx)
	if err != nil {
		return nil, fmt.Errorf("rendering prompt: %w", err)
	}
	taskType, _ := Render(cfg.SessionType, tctx)
	providerKey, _ := Render(cfg.ProviderKey, tctx)
	accessToken, _ := Render(cfg.AccessToken, tctx)
	cli, _ := Render(cfg.CLI, tctx)
	aiModel, _ := Render(cfg.AIModel, tctx)
	sourceBranch, _ := Render(cfg.SourceBranch, tctx)
	targetBranch, _ := Render(cfg.TargetBranch, tctx)
	outputMode, _ := Render(cfg.OutputMode, tctx)

	// Resolve workspace session ID from referenced step
	var workspaceTaskID string
	if cfg.WorkspaceTaskRef != "" {
		if refStep, ok := tctx.Steps[cfg.WorkspaceTaskRef]; ok {
			workspaceTaskID = refStep["task_id"]
		}
	}

	// Decode tools and MCP servers from raw JSON
	var taskTools []tools.TaskTool
	if len(cfg.Tools) > 0 {
		_ = json.Unmarshal(cfg.Tools, &taskTools)
	}
	var mcpServers []session.MCPServer
	if len(cfg.MCPServers) > 0 {
		_ = json.Unmarshal(cfg.MCPServers, &mcpServers)
	}

	// Resolve tool key reference — inject auth_token from key registry into tool configs
	if cfg.ToolKeyRef != "" && e.keys != nil && len(taskTools) > 0 {
		toolKeyRef, _ := Render(cfg.ToolKeyRef, tctx)
		if toolKeyRef != "" {
			token, _, err := e.keys.ResolveByName(ctx, toolKeyRef)
			if err != nil {
				slog.Warn("failed to resolve tool key ref", "key", toolKeyRef, "error", err)
			} else {
				for i := range taskTools {
					if taskTools[i].Config == nil {
						taskTools[i].Config = make(map[string]string)
					}
					if _, exists := taskTools[i].Config["auth_token"]; !exists {
						taskTools[i].Config["auth_token"] = token
					}
				}
			}
		}
	}

	// Resolve timeout from workflow params (_timeout_seconds is injected by workflow config handler)
	var timeoutSeconds int
	if ts, ok := tctx.Params["_timeout_seconds"]; ok && ts != "" {
		if v, err := strconv.Atoi(ts); err == nil && v > 0 {
			timeoutSeconds = v
		}
	}

	// Build TaskConfig if any overrides are set
	var taskConfig *session.Config
	hasConfig := cli != "" || aiModel != "" || workspaceTaskID != "" || sourceBranch != "" ||
		targetBranch != "" || cfg.PRNumber > 0 || outputMode != "" ||
		len(taskTools) > 0 || len(mcpServers) > 0 || timeoutSeconds > 0
	if hasConfig {
		taskConfig = &session.Config{
			CLI:             cli,
			AIModel:         aiModel,
			WorkspaceTaskID: workspaceTaskID,
			SourceBranch:    sourceBranch,
			TargetBranch:    targetBranch,
			PRNumber:        cfg.PRNumber,
			OutputMode:      outputMode,
			Tools:           taskTools,
			MCPServers:      mcpServers,
			TimeoutSeconds:  timeoutSeconds,
		}
	}

	// Create the session
	req := session.CreateSessionRequest{
		RepoURL:       repoURL,
		Prompt:        prompt,
		SessionType:      taskType,
		ProviderKey:   providerKey,
		AccessToken:   accessToken,
		Config:        taskConfig,
		WorkflowRunID: tctx.RunID,
	}
	t, err := e.tasks.Create(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("creating session: %w", err)
	}

	slog.Info("workflow session created", "session_id", t.ID, "step", stepDef.Name)

	// Subscribe to done channel and wait
	doneKey := e.redis.Key("session", t.ID, "done")
	pubsub := e.redis.Unwrap().Subscribe(ctx, doneKey)
	defer func() { _ = pubsub.Close() }()

	msgCh := pubsub.Channel()

	// Poll with timeout from context
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("session %s: context canceled while waiting: %w", t.ID, ctx.Err())

		case msg, ok := <-msgCh:
			if !ok {
				return nil, fmt.Errorf("session %s: subscription closed", t.ID)
			}

			var done struct {
				TaskID string `json:"task_id"`
				Status string `json:"status"`
			}
			if err := json.Unmarshal([]byte(msg.Payload), &done); err != nil {
				slog.Warn("failed to parse done message", "error", err)
				continue
			}

			if done.Status == string(session.StatusFailed) {
				// Fetch full session to get error
				ft, _ := e.tasks.Get(ctx, t.ID)
				errMsg := "session failed"
				if ft != nil && ft.Error != "" {
					errMsg = ft.Error
				}
				return nil, fmt.Errorf("session %s failed: %s", t.ID, errMsg)
			}

			// Session completed — fetch result
			ft, err := e.tasks.Get(ctx, t.ID)
			if err != nil {
				return nil, fmt.Errorf("getting completed session: %w", err)
			}

			outputs := map[string]string{
				"task_id": ft.ID,
				"status":  string(ft.Status),
				"result":  truncateString(ft.Result, 2000),
			}
			return outputs, nil

		case <-time.After(5 * time.Second):
			// Periodic check in case we missed the pub/sub message
			ft, err := e.tasks.Get(ctx, t.ID)
			if err != nil {
				continue
			}
			if ft.Status == session.StatusCompleted || ft.Status == session.StatusFailed || ft.Status == session.StatusPRCreated {
				if ft.Status == session.StatusFailed {
					return nil, fmt.Errorf("session %s failed: %s", ft.ID, ft.Error)
				}
				outputs := map[string]string{
					"task_id": ft.ID,
					"status":  string(ft.Status),
					"result":  truncateString(ft.Result, 2000),
				}
				return outputs, nil
			}
		}
	}
}

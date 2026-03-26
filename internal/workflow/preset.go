package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/freema/codeforge/internal/keys"
	"github.com/freema/codeforge/internal/session"
	"github.com/freema/codeforge/internal/tools"
)

// BuildSessionRequest builds a CreateSessionRequest from a workflow definition,
// preset params, and key registry. It finds the first "session" step in the
// definition and renders its config with the provided params.
func BuildSessionRequest(ctx context.Context, def WorkflowDefinition, params map[string]string, keyReg keys.Registry) (*session.CreateSessionRequest, error) {
	// Find first session step
	var stepDef *StepDefinition
	for i := range def.Steps {
		if def.Steps[i].Type == StepTypeSession {
			stepDef = &def.Steps[i]
			break
		}
	}
	if stepDef == nil {
		return nil, fmt.Errorf("workflow %q has no session step", def.Name)
	}

	var cfg SessionStepConfig
	if err := json.Unmarshal(stepDef.Config, &cfg); err != nil {
		return nil, fmt.Errorf("parsing session config: %w", err)
	}

	// Build template context (no step outputs or run ID in preset mode)
	tctx := TemplateContext{
		Params: params,
		Steps:  make(map[string]map[string]string),
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
	sessionType, _ := Render(cfg.SessionType, tctx)
	providerKey, _ := Render(cfg.ProviderKey, tctx)
	accessToken, _ := Render(cfg.AccessToken, tctx)
	cli, _ := Render(cfg.CLI, tctx)
	aiModel, _ := Render(cfg.AIModel, tctx)
	sourceBranch, _ := Render(cfg.SourceBranch, tctx)
	targetBranch, _ := Render(cfg.TargetBranch, tctx)
	outputMode, _ := Render(cfg.OutputMode, tctx)

	// Decode tools and MCP servers from raw JSON
	var taskTools []tools.TaskTool
	if len(cfg.Tools) > 0 {
		_ = json.Unmarshal(cfg.Tools, &taskTools)
	}
	var mcpServers []session.MCPServer
	if len(cfg.MCPServers) > 0 {
		_ = json.Unmarshal(cfg.MCPServers, &mcpServers)
	}

	// Resolve tool key reference — inject auth_token from key registry
	if cfg.ToolKeyRef != "" && keyReg != nil && len(taskTools) > 0 {
		toolKeyRef, _ := Render(cfg.ToolKeyRef, tctx)
		if toolKeyRef != "" {
			token, _, err := keyReg.ResolveByName(ctx, toolKeyRef)
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

	// Resolve timeout from params
	var timeoutSeconds int
	if ts, ok := params["_timeout_seconds"]; ok && ts != "" {
		if v, err := strconv.Atoi(ts); err == nil && v > 0 {
			timeoutSeconds = v
		}
	}

	// Build Config if any overrides are set
	var taskConfig *session.Config
	hasConfig := cli != "" || aiModel != "" || sourceBranch != "" ||
		targetBranch != "" || cfg.PRNumber > 0 || outputMode != "" ||
		len(taskTools) > 0 || len(mcpServers) > 0 || timeoutSeconds > 0
	if hasConfig {
		taskConfig = &session.Config{
			CLI:            cli,
			AIModel:        aiModel,
			SourceBranch:   sourceBranch,
			TargetBranch:   targetBranch,
			PRNumber:       cfg.PRNumber,
			OutputMode:     outputMode,
			Tools:          taskTools,
			MCPServers:     mcpServers,
			TimeoutSeconds: timeoutSeconds,
		}
	}

	req := &session.CreateSessionRequest{
		RepoURL:     repoURL,
		Prompt:      prompt,
		SessionType: sessionType,
		ProviderKey: providerKey,
		AccessToken: accessToken,
		Config:      taskConfig,
	}

	return req, nil
}

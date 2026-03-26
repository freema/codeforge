import { useState, useMemo, useEffect, type FormEvent } from "react";
import { useNavigate } from "react-router";
import { useCreateSession } from "../hooks/useSessionMutations";
import { useKeys } from "../hooks/useKeys";
import { useMCPServers } from "../hooks/useMCPServers";
import { useRepositories } from "../hooks/useRepositories";
import { useBranches } from "../hooks/useBranches";
import { usePullRequests } from "../hooks/usePullRequests";
import { useCLIs } from "../hooks/useCLIs";
import { useSessionTypes } from "../hooks/useSessionTypes";
import { usePageTitle } from "../hooks/usePageTitle";
import Select from "../components/Select";
import type {
  CreateSessionRequest,
  SessionConfig,
  Repository,
  PullRequest,
} from "../types";


const SESSION_TYPE_CONFIG: Record<
  string,
  { icon: string; label: string; desc: string; submit: string; submitIcon: string }
> = {
  code: {
    icon: "code",
    label: "Code",
    desc: "Write or modify code based on your instructions",
    submit: "Launch Session",
    submitIcon: "bolt",
  },
  plan: {
    icon: "map",
    label: "Plan",
    desc: "Analyze the codebase and produce an implementation plan",
    submit: "Start Planning",
    submitIcon: "map",
  },
  review: {
    icon: "rate_review",
    label: "Repo Review",
    desc: "Review the entire repository for code quality, security and architecture",
    submit: "Start Review",
    submitIcon: "rate_review",
  },
  pr_review: {
    icon: "difference",
    label: "MR / PR Review",
    desc: "Review a specific merge request or pull request diff and post comments",
    submit: "Start Review",
    submitIcon: "difference",
  },
};

const SESSION_TYPE_HINTS: Record<string, string> = {
  code: "Be specific about the changes you want. The agent will clone the repo and work autonomously.",
  plan: "Describe what you want planned. The agent will analyze the repo and produce a step-by-step plan.",
  review: "Describe the review focus, or leave empty for a general review.",
  pr_review:
    "Select a merge request / pull request to review. Leave the prompt empty for a standard review, or add specific focus areas.",
};

export default function NewSession() {
  usePageTitle("New Session");
  const navigate = useNavigate();
  const createSession = useCreateSession();
  const { data: allKeys } = useKeys();
  const keys = useMemo(
    () =>
      allKeys?.filter(
        (k) => k.provider === "github" || k.provider === "gitlab",
      ),
    [allKeys],
  );
  const hasAnthropicKey = useMemo(
    () => allKeys?.some((k) => k.provider === "anthropic") ?? false,
    [allKeys]
  );
  const hasOpenAIKey = useMemo(
    () => allKeys?.some((k) => k.provider === "openai") ?? false,
    [allKeys]
  );
  const { data: mcpServers } = useMCPServers();
  const { data: clis } = useCLIs();
  const { data: taskTypes } = useSessionTypes();

  // Session type — FIRST choice
  const [taskType, setTaskType] = useState("code");

  // Core fields
  const [providerKey, setProviderKey] = useState("");
  const [selectedRepo, setSelectedRepo] = useState<Repository | null>(null);
  const [repoUrl, setRepoUrl] = useState("");
  const [prompt, setPrompt] = useState("");
  const [sourceBranch, setSourceBranch] = useState("");
  const [targetBranch, setTargetBranch] = useState("");
  const [error, setError] = useState("");
  const [showAdvanced, setShowAdvanced] = useState(false);

  // PR Review fields
  const [prNumber, setPrNumber] = useState("");
  const [outputMode, setOutputMode] = useState("post_comments");

  // CLI & Model
  const [selectedCli, setSelectedCli] = useState("");
  const [aiModel, setAiModel] = useState("");

  // Advanced fields
  const [timeout, setTimeout] = useState("");
  const [maxTurns, setMaxTurns] = useState("");
  const [maxBudget, setMaxBudget] = useState("");
  const [callbackUrl, setCallbackUrl] = useState("");
  const [selectedMcp, setSelectedMcp] = useState<string[]>([]);

  // Derived booleans
  const isPrReview = taskType === "pr_review";
  const isPromptOptional = taskType === "review" || taskType === "pr_review";
  const showTargetBranch = taskType === "code";
  const showBranches = !isPrReview && (!!selectedRepo || !!repoUrl);

  // Fetch repositories when provider key is selected
  const { data: repos, isLoading: reposLoading } = useRepositories(
    providerKey || undefined,
  );

  // Fetch branches when repo is selected
  const { data: branches } = useBranches(
    providerKey || undefined,
    selectedRepo?.full_name,
  );

  // Fetch open PRs/MRs when repo is selected and type is pr_review
  const { data: pullRequests, isLoading: prsLoading } = usePullRequests(
    isPrReview ? providerKey || undefined : undefined,
    isPrReview ? selectedRepo?.full_name : undefined,
  );

  const branchOptions = useMemo(() => {
    if (!branches) return [];
    return branches.map((b) => ({ value: b.name, label: b.name }));
  }, [branches]);

  // Available CLIs only
  const availableClis = useMemo(
    () => clis?.filter((c) => c.available) ?? [],
    [clis],
  );

  // Auto-select first key on load
  useEffect(() => {
    if (keys && keys.length > 0 && !providerKey) {
      setProviderKey(keys[0]!.name);
    }
  }, [keys, providerKey]);

  // Auto-select default CLI
  useEffect(() => {
    if (availableClis.length > 0 && !selectedCli) {
      const defaultCli = availableClis.find((c) => c.is_default);
      setSelectedCli(defaultCli?.name ?? availableClis[0]!.name);
    }
  }, [availableClis, selectedCli]);

  // Models available for the selected CLI — from backend API, not hardcoded
  const selectedCliEntry = useMemo(
    () => availableClis.find((c) => c.name === selectedCli),
    [availableClis, selectedCli],
  );
  const cliModels = selectedCliEntry?.models ?? [];

  // Auto-select the first (newest) model when CLI changes and no model is set
  useEffect(() => {
    if (cliModels.length > 0 && !aiModel) {
      setAiModel(cliModels[0]!);
    }
  }, [cliModels, aiModel]);

  const typeConfig = SESSION_TYPE_CONFIG[taskType];
  const sessionTypeHint =
    SESSION_TYPE_HINTS[taskType] ?? typeConfig?.desc ?? "";
  const sessionTypePlaceholder =
    typeConfig?.desc ??
    "Describe what the AI agent should do with this repository...";

  const submitLabel = typeConfig?.submit ?? "Launch Session";
  const submitIcon = typeConfig?.submitIcon ?? "bolt";

  function handleSessionTypeChange(newType: string) {
    setTaskType(newType);
    // Reset type-specific fields
    setPrNumber("");
    setOutputMode("post_comments");
  }

  function handleRepoSelect(repo: Repository) {
    setSelectedRepo(repo);
    setRepoUrl(repo.clone_url);
    setSourceBranch(repo.default_branch);
    setTargetBranch(repo.default_branch);
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");

    const config: SessionConfig = {};
    if (selectedCli) config.cli = selectedCli;
    if (aiModel) config.ai_model = aiModel;
    if (timeout) config.timeout_seconds = Number(timeout);
    if (maxTurns) config.max_turns = Number(maxTurns);
    if (maxBudget) config.max_budget_usd = Number(maxBudget);
    if (selectedMcp.length > 0) {
      config.mcp_servers = selectedMcp.map((name) => ({ name }));
    }

    if (isPrReview) {
      if (prNumber) config.pr_number = Number(prNumber);
      config.output_mode = outputMode;
      // Send target_branch so backend knows which branch to clone
      // (repos may use "master" instead of "main")
      const defaultBranch = selectedRepo?.default_branch || targetBranch;
      if (defaultBranch) config.target_branch = defaultBranch;
    } else {
      if (sourceBranch) config.source_branch = sourceBranch;
      if (targetBranch && showTargetBranch) config.target_branch = targetBranch;
    }

    const req: CreateSessionRequest = {
      repo_url: repoUrl,
      prompt,
      session_type: taskType,
      ...(providerKey ? { provider_key: providerKey } : {}),
      ...(callbackUrl ? { callback_url: callbackUrl } : {}),
      ...(Object.keys(config).length > 0 ? { config } : {}),
    };

    try {
      const created = await createSession.mutateAsync(req);
      void navigate(`/sessions/${created.id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create session");
    }
  }

  function toggleMcp(name: string) {
    setSelectedMcp((prev) =>
      prev.includes(name) ? prev.filter((n) => n !== name) : [...prev, name],
    );
  }

  const isSubmitDisabled =
    createSession.isPending ||
    !repoUrl ||
    (isPrReview ? !prNumber : !isPromptOptional && !prompt);

  const inputCls =
    "w-full rounded-lg border border-edge bg-surface px-3 py-2.5 text-sm text-fg font-mono placeholder-fg-4 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent transition-colors";

  return (
    <div className="mx-auto max-w-3xl">
      <div className="mb-10">
        <h1 className="text-4xl font-bold tracking-tight text-fg">
          New Session
        </h1>
        <p className="mt-2 text-sm text-fg-3">
          Configure and launch an AI coding session
        </p>
      </div>

      <form onSubmit={handleSubmit} className="space-y-6">
        {/* 1. Session Type — FIRST */}
        {taskTypes && taskTypes.length > 0 && (
          <div>
            <label className="mb-2 block text-xs font-medium text-fg-3">
              Session Type
            </label>
            <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
              {taskTypes.map((tt) => {
                const cfg = SESSION_TYPE_CONFIG[tt.name];
                const isActive = taskType === tt.name;
                return (
                  <button
                    key={tt.name}
                    type="button"
                    onClick={() => handleSessionTypeChange(tt.name)}
                    className={`flex flex-col items-center gap-1 rounded-lg border px-3 py-3 text-center transition-all ${
                      isActive
                        ? "border-accent bg-accent/10 text-accent"
                        : "border-edge bg-surface text-fg-3 hover:border-fg-4 hover:text-fg"
                    }`}
                  >
                    <span className="material-symbols-outlined text-xl">
                      {cfg?.icon ?? "task"}
                    </span>
                    <span className="text-sm font-medium">
                      {cfg?.label ?? tt.label}
                    </span>
                    <span
                      className={`text-[10px] leading-tight ${
                        isActive ? "text-accent/70" : "text-fg-4"
                      }`}
                    >
                      {cfg?.desc ?? tt.description}
                    </span>
                  </button>
                );
              })}
            </div>
          </div>
        )}

        {/* 2. Provider */}
        {keys && keys.length > 0 && (
          <div>
            <label className="mb-2 block text-xs font-medium text-fg-3">
              Provider
            </label>
            <div className="flex gap-2">
              {keys.map((k) => (
                <button
                  key={k.name}
                  type="button"
                  onClick={() => {
                    setProviderKey(k.name);
                    setSelectedRepo(null);
                    setRepoUrl("");
                  }}
                  className={`flex items-center gap-2 rounded-lg border px-4 py-2.5 text-sm font-medium transition-all ${
                    providerKey === k.name
                      ? "border-accent bg-accent/10 text-accent"
                      : "border-edge bg-surface text-fg-3 hover:border-fg-4 hover:text-fg"
                  }`}
                >
                  <span className="material-symbols-outlined text-lg">
                    {k.provider === "github" ? "code" : "cloud"}
                  </span>
                  {k.name}
                  <span className="text-xs text-fg-4">({k.provider})</span>
                </button>
              ))}
            </div>
          </div>
        )}

        {/* 3. Repository */}
        <div>
          <label className="mb-2 block text-xs font-medium text-fg-3">
            Repository
          </label>
          {providerKey && repos && repos.length > 0 ? (
            <div className="space-y-3">
              <RepoSelector
                repos={repos}
                selected={selectedRepo}
                loading={reposLoading}
                onSelect={handleRepoSelect}
              />
              {selectedRepo && (
                <div className="flex items-center gap-3 rounded-lg border border-accent/20 bg-accent/5 p-3">
                  <span className="material-symbols-outlined text-accent">
                    check_circle
                  </span>
                  <div>
                    <p className="text-sm font-medium text-fg">
                      {selectedRepo.full_name}
                    </p>
                    <p className="text-xs text-fg-3">
                      {selectedRepo.description || "No description"}
                    </p>
                  </div>
                </div>
              )}
            </div>
          ) : providerKey && reposLoading ? (
            <div className="flex items-center gap-3 rounded-lg border border-edge bg-surface p-4">
              <span className="material-symbols-outlined animate-spin text-accent">
                progress_activity
              </span>
              <span className="text-sm text-fg-3">Loading repositories...</span>
            </div>
          ) : (
            <div>
              <input
                type="url"
                value={repoUrl}
                onChange={(e) => setRepoUrl(e.target.value)}
                placeholder="https://github.com/user/repo.git"
                required
                className={inputCls}
              />
              {!providerKey && keys && keys.length > 0 && (
                <p className="mt-2 text-xs text-fg-4">
                  Select a provider key above to browse repositories
                </p>
              )}
            </div>
          )}
        </div>

        {/* 4. Conditional fields based on session type */}

        {/* 4a. PR Review: PR selector + output mode */}
        {isPrReview && (selectedRepo || repoUrl) && (
          <div className="space-y-4">
            <div>
              <label className="mb-2 block text-xs font-medium text-fg-3">
                Pull Request / Merge Request
              </label>
              <PRSelector
                pullRequests={pullRequests ?? []}
                loading={prsLoading}
                selected={prNumber}
                onSelect={setPrNumber}
                inputCls={inputCls}
              />
            </div>
            <div>
              <label className="mb-2 block text-xs font-medium text-fg-3">
                Output Mode
              </label>
              <div className="flex gap-2">
                <button
                  type="button"
                  onClick={() => setOutputMode("post_comments")}
                  className={`flex flex-1 items-center justify-center gap-2 rounded-lg border px-3 py-2.5 text-sm font-medium transition-all ${
                    outputMode === "post_comments"
                      ? "border-accent bg-accent/10 text-accent"
                      : "border-edge bg-surface text-fg-3 hover:border-fg-4 hover:text-fg"
                  }`}
                >
                  <span className="material-symbols-outlined text-base">
                    comment
                  </span>
                  Post to PR
                </button>
                <button
                  type="button"
                  onClick={() => setOutputMode("api_only")}
                  className={`flex flex-1 items-center justify-center gap-2 rounded-lg border px-3 py-2.5 text-sm font-medium transition-all ${
                    outputMode === "api_only"
                      ? "border-accent bg-accent/10 text-accent"
                      : "border-edge bg-surface text-fg-3 hover:border-fg-4 hover:text-fg"
                  }`}
                >
                  <span className="material-symbols-outlined text-base">
                    api
                  </span>
                  API Only
                </button>
              </div>
            </div>
          </div>
        )}

        {/* 4b. Non-PR-Review: Branches */}
        {showBranches && (
          <div className={`grid gap-4 ${showTargetBranch ? "grid-cols-2" : "grid-cols-1"}`}>
            <div>
              <label className="mb-2 block text-xs font-medium text-fg-3">
                Source Branch
              </label>
              {branchOptions.length > 0 ? (
                <Select
                  value={sourceBranch}
                  onChange={(v) => {
                    setSourceBranch(v);
                    if (showTargetBranch) setTargetBranch(v);
                  }}
                  options={branchOptions}
                  placeholder={selectedRepo?.default_branch || "main"}
                />
              ) : (
                <input
                  type="text"
                  value={sourceBranch}
                  onChange={(e) => setSourceBranch(e.target.value)}
                  placeholder={selectedRepo?.default_branch || "main"}
                  className={inputCls}
                />
              )}
            </div>
            {showTargetBranch && (
              <div>
                <label className="mb-2 block text-xs font-medium text-fg-3">
                  Target Branch (for PR)
                </label>
                {branchOptions.length > 0 ? (
                  <Select
                    value={targetBranch}
                    onChange={setTargetBranch}
                    options={branchOptions}
                    placeholder={selectedRepo?.default_branch || "main"}
                  />
                ) : (
                  <input
                    type="text"
                    value={targetBranch}
                    onChange={(e) => setTargetBranch(e.target.value)}
                    placeholder={selectedRepo?.default_branch || "main"}
                    className={inputCls}
                  />
                )}
              </div>
            )}
          </div>
        )}

        {/* 5. Prompt */}
        <div>
          <label className="mb-2 block text-xs font-medium text-fg-3">
            {isPrReview
              ? "Additional review instructions (optional)"
              : "What should the agent do?"}
          </label>
          <textarea
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder={sessionTypePlaceholder}
            required={!isPromptOptional}
            minLength={isPromptOptional ? 0 : 10}
            rows={isPrReview ? 3 : 5}
            className={inputCls + " resize-none"}
          />
        </div>

        {/* 6. CLI & Model */}
        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="mb-2 block text-xs font-medium text-fg-3">
              CLI
            </label>
            <Select
              value={selectedCli}
              onChange={(v) => {
                setSelectedCli(v);
                setAiModel("");
              }}
              options={availableClis.map((cli) => ({
                value: cli.name,
                label: cli.name + (cli.is_default ? " (default)" : ""),
              }))}
              placeholder="Select CLI..."
            />
          </div>
          <div>
            <label className="mb-2 block text-xs font-medium text-fg-3">
              AI Model
            </label>
            <Select
              value={aiModel}
              onChange={setAiModel}
              options={
                cliModels.map((m) => ({ value: m, label: m }))
              }
              placeholder="Select model..."
            />
          </div>
        </div>

        {selectedCli === "claude-code" && !hasAnthropicKey && (
          <div className="flex items-start gap-2 rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-2">
            <span className="material-symbols-outlined mt-0.5 text-sm text-amber-400">warning</span>
            <p className="text-xs text-amber-400/80">
              No Anthropic API key configured.{" "}
              <a href="/settings?tab=ai" className="underline hover:text-amber-300">Add one in Settings</a>{" "}
              or set <code className="font-mono">ANTHROPIC_API_KEY</code> env var.
            </p>
          </div>
        )}
        {selectedCli === "codex" && !hasOpenAIKey && (
          <div className="flex items-start gap-2 rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-2">
            <span className="material-symbols-outlined mt-0.5 text-sm text-amber-400">warning</span>
            <p className="text-xs text-amber-400/80">
              No OpenAI API key configured.{" "}
              <a href="/settings?tab=ai" className="underline hover:text-amber-300">Add one in Settings</a>{" "}
              or set <code className="font-mono">OPENAI_API_KEY</code> env var.
            </p>
          </div>
        )}

        {/* 7. MCP Servers */}
        {mcpServers && mcpServers.length > 0 && (
          <div>
            <label className="mb-2 block text-xs font-medium text-fg-3">
              MCP Servers
            </label>
            <div className="flex flex-wrap gap-2">
              {mcpServers.map((s) => (
                <button
                  key={s.name}
                  type="button"
                  onClick={() => toggleMcp(s.name)}
                  className={`flex items-center gap-2 rounded-lg border px-3 py-2.5 text-sm font-medium transition-all ${
                    selectedMcp.includes(s.name)
                      ? "border-accent bg-accent/10 text-accent"
                      : "border-edge bg-surface text-fg-3 hover:border-fg-4 hover:text-fg"
                  }`}
                >
                  <span className="material-symbols-outlined text-lg">
                    {selectedMcp.includes(s.name)
                      ? "check_circle"
                      : "extension"}
                  </span>
                  {s.name}
                </button>
              ))}
            </div>
            <p className="mt-2 text-xs text-fg-4">
              Enable MCP servers to give the agent additional capabilities
            </p>
          </div>
        )}

        {/* 8. Advanced */}
        <div className="rounded-xl border border-edge bg-surface">
          <button
            type="button"
            onClick={() => setShowAdvanced(!showAdvanced)}
            className="flex w-full items-center justify-between p-4 text-sm font-medium text-fg-3 hover:text-fg transition-colors"
          >
            <div className="flex items-center gap-2">
              <span className="material-symbols-outlined text-base">tune</span>
              Advanced Configuration
            </div>
            <span
              className="material-symbols-outlined text-base transition-transform"
              style={{ transform: showAdvanced ? "rotate(180deg)" : "none" }}
            >
              expand_more
            </span>
          </button>

          {showAdvanced && (
            <div className="border-t border-edge p-4 space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="mb-1 block text-xs text-fg-3">
                    Max Iterations
                  </label>
                  <input
                    type="number"
                    value={maxTurns}
                    onChange={(e) => setMaxTurns(e.target.value)}
                    placeholder="default: unlimited"
                    className={inputCls}
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs text-fg-3">
                    Timeout (seconds)
                  </label>
                  <input
                    type="number"
                    value={timeout}
                    onChange={(e) => setTimeout(e.target.value)}
                    placeholder="600"
                    className={inputCls}
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs text-fg-3">
                    Max Budget ($)
                  </label>
                  <input
                    type="number"
                    step="0.50"
                    value={maxBudget}
                    onChange={(e) => setMaxBudget(e.target.value)}
                    placeholder="5.00"
                    className={inputCls}
                  />
                </div>
              </div>

              <div>
                <label className="mb-1 block text-xs text-fg-3">
                  Callback URL
                </label>
                <input
                  type="url"
                  value={callbackUrl}
                  onChange={(e) => setCallbackUrl(e.target.value)}
                  placeholder="https://your-app.com/webhook"
                  className={inputCls}
                />
              </div>
            </div>
          )}
        </div>

        {/* Error */}
        {error && (
          <div className="flex items-center gap-3 rounded-lg border border-red-900/50 bg-red-900/10 p-4">
            <span className="material-symbols-outlined text-red-400">
              error
            </span>
            <p className="text-sm text-red-400">{error}</p>
          </div>
        )}

        {/* Submit */}
        <div className="flex items-center justify-between pt-2">
          <button
            type="button"
            onClick={() => void navigate(-1)}
            className="rounded-lg border border-edge bg-surface px-6 py-3 text-sm font-medium text-fg-3 transition-colors hover:border-fg-4 hover:text-fg"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={isSubmitDisabled}
            className="flex items-center gap-2 rounded-lg bg-accent px-8 py-3 text-sm font-bold text-page shadow-[0_0_20px_rgba(0,255,64,0.3)] transition-all hover:bg-accent-hover hover:shadow-[0_0_30px_rgba(0,255,64,0.5)] disabled:opacity-40 disabled:shadow-none"
          >
            {createSession.isPending ? (
              <span className="material-symbols-outlined text-xl animate-spin">
                progress_activity
              </span>
            ) : (
              <span className="material-symbols-outlined text-xl">
                {submitIcon}
              </span>
            )}
            {submitLabel}
          </button>
        </div>
      </form>
    </div>
  );
}

/** Parse a PR/MR number from a URL like https://github.com/owner/repo/pull/123 or https://gitlab.com/group/repo/-/merge_requests/456 */
function parsePRNumberFromURL(input: string): string | null {
  // GitHub: /pull/123
  const ghMatch = input.match(/\/pull\/(\d+)/);
  if (ghMatch?.[1]) return ghMatch[1];
  // GitLab: /merge_requests/456
  const glMatch = input.match(/\/merge_requests\/(\d+)/);
  if (glMatch?.[1]) return glMatch[1];
  return null;
}

function PRSelector({
  pullRequests,
  loading,
  selected,
  onSelect,
  inputCls,
}: {
  pullRequests: PullRequest[];
  loading: boolean;
  selected: string;
  onSelect: (value: string) => void;
  inputCls: string;
}) {
  const [urlInput, setUrlInput] = useState("");

  function handleUrlPaste(value: string) {
    setUrlInput(value);
    const parsed = parsePRNumberFromURL(value);
    if (parsed) {
      onSelect(parsed);
      setUrlInput("");
    }
  }

  const selectedPR = pullRequests.find(
    (pr) => String(pr.number) === selected,
  );

  return (
    <div className="space-y-3">
      {/* Dropdown of open PRs */}
      {loading ? (
        <div className="flex items-center gap-3 rounded-lg border border-edge bg-surface p-3">
          <span className="material-symbols-outlined animate-spin text-accent text-base">
            progress_activity
          </span>
          <span className="text-sm text-fg-3">Loading open PRs...</span>
        </div>
      ) : pullRequests.length > 0 ? (
        <div className="space-y-2">
          <div className="max-h-48 overflow-y-auto rounded-lg border border-edge">
            {pullRequests.map((pr) => (
              <button
                key={pr.number}
                type="button"
                onClick={() => onSelect(String(pr.number))}
                className={`flex w-full items-center gap-3 border-b border-edge p-2.5 text-left transition-colors last:border-b-0 hover:bg-accent/5 ${
                  selected === String(pr.number) ? "bg-accent/10" : ""
                }`}
              >
                <span
                  className={`shrink-0 rounded border px-1.5 py-0.5 text-[10px] font-mono font-bold ${
                    selected === String(pr.number)
                      ? "border-accent/30 text-accent"
                      : "border-edge text-fg-3"
                  }`}
                >
                  #{pr.number}
                </span>
                <div className="flex-1 min-w-0">
                  <p className="truncate text-sm text-fg">{pr.title}</p>
                  <p className="text-[10px] text-fg-4">
                    {pr.source_branch} → {pr.target_branch}
                    {pr.author && ` · ${pr.author}`}
                  </p>
                </div>
                {selected === String(pr.number) && (
                  <span className="material-symbols-outlined text-accent text-base">
                    check_circle
                  </span>
                )}
              </button>
            ))}
          </div>
        </div>
      ) : (
        <p className="text-xs text-fg-4">
          No open PRs found. Paste a URL or enter a number below.
        </p>
      )}

      {/* Selected PR confirmation */}
      {selectedPR && (
        <div className="flex items-center gap-3 rounded-lg border border-accent/20 bg-accent/5 p-2.5">
          <span className="material-symbols-outlined text-accent text-base">
            check_circle
          </span>
          <span className="text-xs font-mono text-accent font-bold">
            #{selectedPR.number}
          </span>
          <span className="text-xs text-fg truncate">{selectedPR.title}</span>
        </div>
      )}

      {/* URL paste helper + manual number input */}
      <div className="flex gap-2">
        <div className="flex-1 relative">
          <span className="material-symbols-outlined absolute left-2.5 top-1/2 -translate-y-1/2 text-fg-4 text-base">
            link
          </span>
          <input
            type="text"
            value={urlInput}
            onChange={(e) => handleUrlPaste(e.target.value)}
            onPaste={(e) => {
              const text = e.clipboardData.getData("text");
              handleUrlPaste(text);
              e.preventDefault();
            }}
            placeholder="Paste PR/MR URL or enter number..."
            className={inputCls + " pl-9"}
          />
        </div>
        <input
          type="number"
          min="1"
          value={selected}
          onChange={(e) => onSelect(e.target.value)}
          placeholder="#"
          className={inputCls + " w-24 text-center"}
        />
      </div>
    </div>
  );
}

function RepoSelector({
  repos,
  selected,
  loading,
  onSelect,
}: {
  repos: Repository[];
  selected: Repository | null;
  loading: boolean;
  onSelect: (repo: Repository) => void;
}) {
  const [search, setSearch] = useState("");
  const [open, setOpen] = useState(!selected);

  const filtered = useMemo(() => {
    if (!search) return repos.slice(0, 20);
    const q = search.toLowerCase();
    return repos.filter(
      (r) =>
        r.full_name.toLowerCase().includes(q) ||
        r.description?.toLowerCase().includes(q),
    );
  }, [repos, search]);

  if (!open && selected) {
    return (
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="flex w-full items-center justify-between rounded-lg border border-edge bg-surface p-3 text-left transition-colors hover:border-fg-4"
      >
        <span className="text-sm text-fg font-mono">{selected.full_name}</span>
        <span className="material-symbols-outlined text-sm text-fg-3">
          unfold_more
        </span>
      </button>
    );
  }

  return (
    <div className="space-y-2">
      <div className="relative">
        <span className="material-symbols-outlined absolute left-3 top-1/2 -translate-y-1/2 text-fg-4 text-lg">
          search
        </span>
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search repositories..."
          className="w-full rounded-lg border border-edge bg-surface py-2.5 pl-10 pr-3 text-sm text-fg font-mono placeholder-fg-4 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent"
          autoFocus
        />
      </div>

      {loading ? (
        <div className="flex items-center justify-center py-8">
          <span className="material-symbols-outlined animate-spin text-accent">
            progress_activity
          </span>
        </div>
      ) : (
        <div className="max-h-64 overflow-y-auto rounded-lg border border-edge">
          {filtered.length === 0 ? (
            <p className="p-4 text-center text-sm text-fg-4">
              No repositories found
            </p>
          ) : (
            filtered.map((repo) => (
              <button
                key={repo.full_name}
                type="button"
                onClick={() => {
                  onSelect(repo);
                  setOpen(false);
                  setSearch("");
                }}
                className={`flex w-full items-center gap-3 border-b border-edge p-3 text-left transition-colors last:border-b-0 hover:bg-accent/5 ${
                  selected?.full_name === repo.full_name ? "bg-accent/10" : ""
                }`}
              >
                <span className="material-symbols-outlined text-fg-4 text-lg">
                  {repo.private ? "lock" : "public"}
                </span>
                <div className="flex-1 overflow-hidden">
                  <p className="truncate text-sm font-medium text-fg">
                    {repo.full_name}
                  </p>
                  {repo.description && (
                    <p className="truncate text-xs text-fg-4">
                      {repo.description}
                    </p>
                  )}
                </div>
                <span className="shrink-0 rounded border border-edge px-1.5 py-0.5 text-[10px] font-mono text-fg-3">
                  {repo.default_branch}
                </span>
              </button>
            ))
          )}
        </div>
      )}
    </div>
  );
}

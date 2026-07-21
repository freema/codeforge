import { useState, useMemo, useEffect, type FormEvent } from "react";
import { useNavigate } from "react-router";
import {
  Braces,
  ChevronDown,
  ChevronsUpDown,
  CircleAlert,
  CircleCheck,
  Code,
  GitPullRequest,
  Github,
  Gitlab,
  Link2,
  Loader2,
  Lock,
  Globe,
  MapIcon,
  MessageSquare,
  Puzzle,
  Search,
  SearchCode,
  SlidersHorizontal,
  SquareTerminal,
  TriangleAlert,
  Zap,
  type LucideIcon,
} from "lucide-react";
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
  {
    icon: LucideIcon;
    label: string;
    desc: string;
    submit: string;
    submitIcon: LucideIcon;
  }
> = {
  code: {
    icon: Code,
    label: "Code",
    desc: "Write or modify code based on your instructions",
    submit: "Launch session",
    submitIcon: Zap,
  },
  plan: {
    icon: MapIcon,
    label: "Plan",
    desc: "Analyze the codebase and produce an implementation plan",
    submit: "Start planning",
    submitIcon: MapIcon,
  },
  review: {
    icon: SearchCode,
    label: "Repo review",
    desc: "Review the entire repository for code quality, security and architecture",
    submit: "Start review",
    submitIcon: SearchCode,
  },
  pr_review: {
    icon: GitPullRequest,
    label: "MR / PR review",
    desc: "Review a specific merge request or pull request diff and post comments",
    submit: "Start review",
    submitIcon: GitPullRequest,
  },
};

export default function NewSession() {
  usePageTitle("New session");
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
    [allKeys],
  );
  const hasOpenAIKey = useMemo(
    () => allKeys?.some((k) => k.provider === "openai") ?? false,
    [allKeys],
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
  const [outputMode, setOutputMode] = useState("api_only");

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

  // Default to "" (auto) — CLI picks the best model for the API key
  // No auto-select needed; empty string means "auto"

  const typeConfig = SESSION_TYPE_CONFIG[taskType];
  const sessionTypePlaceholder =
    typeConfig?.desc ??
    "Describe what the AI agent should do with this repository…";

  const submitLabel = typeConfig?.submit ?? "Launch session";
  const SubmitIcon = typeConfig?.submitIcon ?? Zap;

  function handleSessionTypeChange(newType: string) {
    setTaskType(newType);
    // Reset type-specific fields
    setPrNumber("");
    setOutputMode("api_only");
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
    "w-full rounded-md border border-edge bg-input px-3 py-2 text-sm text-fg placeholder-fg-4 transition-colors focus:border-accent focus:outline-none";

  return (
    <div className="mx-auto max-w-3xl">
      <div className="mb-8">
        <p className="eyebrow mb-1">Sessions</p>
        <h2 className="font-expanded text-2xl font-extrabold tracking-tight text-fg">
          New session
        </h2>
      </div>

      <form onSubmit={handleSubmit} className="space-y-6">
        {/* 1. Session type — FIRST */}
        {taskTypes && taskTypes.length > 0 && (
          <div className="overflow-hidden rounded-md border border-edge bg-surface">
            <div className="border-b border-edge px-5 py-3.5">
              <span className="eyebrow">Session type</span>
            </div>
            <div className="p-5">
              <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
                {taskTypes.map((tt) => {
                  const cfg = SESSION_TYPE_CONFIG[tt.name];
                  const isActive = taskType === tt.name;
                  const Icon = cfg?.icon ?? SquareTerminal;
                  return (
                    <button
                      key={tt.name}
                      type="button"
                      onClick={() => handleSessionTypeChange(tt.name)}
                      className={`flex flex-col items-center gap-1.5 rounded-md border px-3 py-3 text-center transition-colors ${
                        isActive
                          ? "border-accent-muted bg-accent-soft text-accent"
                          : "border-edge bg-surface-alt text-fg-3 hover:border-fg-4 hover:text-fg"
                      }`}
                    >
                      <Icon className="size-5" />
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
          </div>
        )}

        {/* 2. Repository: provider, repo, PR / branches */}
        <div className="overflow-hidden rounded-md border border-edge bg-surface">
          <div className="border-b border-edge px-5 py-3.5">
            <span className="eyebrow">Repository</span>
          </div>
          <div className="space-y-5 p-5">
            {/* Provider */}
            {keys && keys.length > 0 && (
              <div>
                <label className="mb-2 block text-sm font-medium text-fg-2">
                  Provider
                </label>
                <div className="flex flex-wrap gap-2">
                  {keys.map((k) => (
                    <button
                      key={k.name}
                      type="button"
                      onClick={() => {
                        setProviderKey(k.name);
                        setSelectedRepo(null);
                        setRepoUrl("");
                      }}
                      className={`flex items-center gap-2 rounded-md border px-4 py-2 font-mono text-sm transition-colors ${
                        providerKey === k.name
                          ? "border-accent-muted bg-accent-soft text-accent"
                          : "border-edge bg-surface-alt text-fg-3 hover:border-fg-4 hover:text-fg"
                      }`}
                    >
                      {k.provider === "github" ? (
                        <Github className="size-4" />
                      ) : (
                        <Gitlab className="size-4" />
                      )}
                      {k.name}
                      <span className="text-xs text-fg-4">({k.provider})</span>
                    </button>
                  ))}
                </div>
              </div>
            )}

            {/* Repository */}
            <div>
              <label className="mb-2 block text-sm font-medium text-fg-2">
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
                    <div className="flex items-center gap-3 rounded-md border border-accent-muted bg-accent-soft p-3">
                      <CircleCheck className="size-4 shrink-0 text-accent" />
                      <div className="min-w-0">
                        <p className="truncate font-mono text-sm text-fg">
                          {selectedRepo.full_name}
                        </p>
                        <p className="truncate text-xs text-fg-3">
                          {selectedRepo.description || "No description"}
                        </p>
                      </div>
                    </div>
                  )}
                </div>
              ) : providerKey && reposLoading ? (
                <div className="flex items-center gap-3 rounded-md border border-edge bg-surface-alt p-4">
                  <Loader2 className="size-4 animate-spin text-accent" />
                  <span className="text-sm text-fg-3">
                    Loading repositories…
                  </span>
                </div>
              ) : (
                <div>
                  <input
                    type="url"
                    value={repoUrl}
                    onChange={(e) => setRepoUrl(e.target.value)}
                    placeholder="https://github.com/user/repo.git"
                    required
                    className={inputCls + " font-mono"}
                  />
                  {!providerKey && keys && keys.length > 0 && (
                    <p className="mt-2 text-xs text-fg-4">
                      Select a provider key above to browse repositories
                    </p>
                  )}
                </div>
              )}
            </div>

            {/* PR review: PR selector + output mode */}
            {isPrReview && (selectedRepo || repoUrl) && (
              <div className="space-y-4">
                <div>
                  <label className="mb-2 block text-sm font-medium text-fg-2">
                    Pull request / merge request
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
                  <label className="mb-2 block text-sm font-medium text-fg-2">
                    Output mode
                  </label>
                  <div className="flex gap-2">
                    <button
                      type="button"
                      onClick={() => setOutputMode("post_comments")}
                      className={`flex flex-1 items-center justify-center gap-2 rounded-md border px-3 py-2 text-sm font-medium transition-colors ${
                        outputMode === "post_comments"
                          ? "border-accent-muted bg-accent-soft text-accent"
                          : "border-edge bg-surface-alt text-fg-3 hover:border-fg-4 hover:text-fg"
                      }`}
                    >
                      <MessageSquare className="size-4" />
                      Post to PR
                    </button>
                    <button
                      type="button"
                      onClick={() => setOutputMode("api_only")}
                      className={`flex flex-1 items-center justify-center gap-2 rounded-md border px-3 py-2 text-sm font-medium transition-colors ${
                        outputMode === "api_only"
                          ? "border-accent-muted bg-accent-soft text-accent"
                          : "border-edge bg-surface-alt text-fg-3 hover:border-fg-4 hover:text-fg"
                      }`}
                    >
                      <Braces className="size-4" />
                      API only
                    </button>
                  </div>
                </div>
              </div>
            )}

            {/* Non-PR-review: branches */}
            {showBranches && (
              <div
                className={`grid gap-4 ${showTargetBranch ? "grid-cols-2" : "grid-cols-1"}`}
              >
                <div>
                  <label className="mb-2 block text-sm font-medium text-fg-2">
                    Source branch
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
                      className={inputCls + " font-mono"}
                    />
                  )}
                </div>
                {showTargetBranch && (
                  <div>
                    <label className="mb-2 block text-sm font-medium text-fg-2">
                      Target branch (for PR)
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
                        className={inputCls + " font-mono"}
                      />
                    )}
                  </div>
                )}
              </div>
            )}
          </div>
        </div>

        {/* 3. Instructions */}
        <div className="overflow-hidden rounded-md border border-edge bg-surface">
          <div className="border-b border-edge px-5 py-3.5">
            <span className="eyebrow">Instructions</span>
          </div>
          <div className="p-5">
            <label className="mb-2 block text-sm font-medium text-fg-2">
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
        </div>

        {/* 4. Runtime: CLI, model, MCP */}
        <div className="overflow-hidden rounded-md border border-edge bg-surface">
          <div className="border-b border-edge px-5 py-3.5">
            <span className="eyebrow">Runtime</span>
          </div>
          <div className="space-y-5 p-5">
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="mb-2 block text-sm font-medium text-fg-2">
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
                  placeholder="Select CLI…"
                />
              </div>
              <div>
                <label className="mb-2 block text-sm font-medium text-fg-2">
                  Model
                </label>
                <Select
                  value={aiModel}
                  onChange={setAiModel}
                  options={[
                    { value: "", label: "(auto)" },
                    ...cliModels.map((m) => ({ value: m, label: m })),
                  ]}
                  placeholder="Select model…"
                />
              </div>
            </div>

            {selectedCli === "claude-code" && !hasAnthropicKey && (
              <div className="flex items-start gap-2.5 rounded-md border border-warn/30 bg-warn/10 px-3 py-2.5">
                <TriangleAlert className="mt-0.5 size-4 shrink-0 text-warn" />
                <p className="text-xs text-warn">
                  No Anthropic API key configured.{" "}
                  <a
                    href="/settings?tab=ai"
                    className="underline transition-colors hover:text-fg"
                  >
                    Add one in Settings
                  </a>{" "}
                  or set <code className="font-mono">ANTHROPIC_API_KEY</code>{" "}
                  env var.
                </p>
              </div>
            )}
            {selectedCli === "codex" && !hasOpenAIKey && (
              <div className="flex items-start gap-2.5 rounded-md border border-warn/30 bg-warn/10 px-3 py-2.5">
                <TriangleAlert className="mt-0.5 size-4 shrink-0 text-warn" />
                <p className="text-xs text-warn">
                  No OpenAI API key configured.{" "}
                  <a
                    href="/settings?tab=ai"
                    className="underline transition-colors hover:text-fg"
                  >
                    Add one in Settings
                  </a>{" "}
                  or set <code className="font-mono">OPENAI_API_KEY</code> env
                  var.
                </p>
              </div>
            )}

            {mcpServers && mcpServers.length > 0 && (
              <div>
                <label className="mb-2 block text-sm font-medium text-fg-2">
                  MCP servers
                </label>
                <div className="flex flex-wrap gap-2">
                  {mcpServers.map((s) => (
                    <button
                      key={s.name}
                      type="button"
                      onClick={() => toggleMcp(s.name)}
                      className={`flex items-center gap-2 rounded-md border px-3 py-2 font-mono text-sm transition-colors ${
                        selectedMcp.includes(s.name)
                          ? "border-accent-muted bg-accent-soft text-accent"
                          : "border-edge bg-surface-alt text-fg-3 hover:border-fg-4 hover:text-fg"
                      }`}
                    >
                      {selectedMcp.includes(s.name) ? (
                        <CircleCheck className="size-4" />
                      ) : (
                        <Puzzle className="size-4" />
                      )}
                      {s.name}
                    </button>
                  ))}
                </div>
                <p className="mt-2 text-xs text-fg-4">
                  Enable MCP servers to give the agent additional capabilities
                </p>
              </div>
            )}
          </div>
        </div>

        {/* 5. Advanced */}
        <div className="overflow-hidden rounded-md border border-edge bg-surface">
          <button
            type="button"
            onClick={() => setShowAdvanced(!showAdvanced)}
            className="flex w-full items-center justify-between px-5 py-3.5 text-left transition-colors hover:bg-surface-alt"
          >
            <span className="flex items-center gap-2 text-sm font-medium text-fg-2">
              <SlidersHorizontal className="size-4 text-fg-3" />
              Advanced configuration
            </span>
            <ChevronDown
              className="size-4 text-fg-3"
              style={{ transform: showAdvanced ? "rotate(180deg)" : "none" }}
            />
          </button>

          {showAdvanced && (
            <div className="space-y-4 border-t border-edge p-5">
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="mb-2 block text-sm font-medium text-fg-2">
                    Max iterations
                  </label>
                  <input
                    type="number"
                    value={maxTurns}
                    onChange={(e) => setMaxTurns(e.target.value)}
                    placeholder="default: unlimited"
                    className={inputCls + " font-mono"}
                  />
                </div>
                <div>
                  <label className="mb-2 block text-sm font-medium text-fg-2">
                    Timeout (seconds)
                  </label>
                  <input
                    type="number"
                    value={timeout}
                    onChange={(e) => setTimeout(e.target.value)}
                    placeholder="600"
                    className={inputCls + " font-mono"}
                  />
                </div>
                <div>
                  <label className="mb-2 block text-sm font-medium text-fg-2">
                    Max budget ($)
                  </label>
                  <input
                    type="number"
                    step="0.50"
                    value={maxBudget}
                    onChange={(e) => setMaxBudget(e.target.value)}
                    placeholder="5.00"
                    className={inputCls + " font-mono"}
                  />
                </div>
              </div>

              <div>
                <label className="mb-2 block text-sm font-medium text-fg-2">
                  Callback URL
                </label>
                <input
                  type="url"
                  value={callbackUrl}
                  onChange={(e) => setCallbackUrl(e.target.value)}
                  placeholder="https://your-app.com/webhook"
                  className={inputCls + " font-mono"}
                />
              </div>
            </div>
          )}
        </div>

        {/* Error */}
        {error && (
          <div className="flex items-center gap-3 rounded-md border border-danger/30 bg-danger/10 p-4">
            <CircleAlert className="size-4 shrink-0 text-danger" />
            <p className="text-sm text-danger">{error}</p>
          </div>
        )}

        {/* Submit */}
        <div className="flex items-center justify-between pt-2">
          <button
            type="button"
            onClick={() => void navigate(-1)}
            className="rounded-md border border-edge bg-surface px-4 py-2 text-sm font-medium text-fg-2 transition-colors hover:border-fg-4 hover:text-fg"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={isSubmitDisabled}
            className="flex items-center gap-2 rounded-md bg-accent px-5 py-2 text-sm font-semibold text-white transition-colors hover:bg-accent-hover disabled:opacity-40"
          >
            {createSession.isPending ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <SubmitIcon className="size-4" />
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

  const selectedPR = pullRequests.find((pr) => String(pr.number) === selected);

  return (
    <div className="space-y-3">
      {/* Dropdown of open PRs */}
      {loading ? (
        <div className="flex items-center gap-3 rounded-md border border-edge bg-surface-alt p-3">
          <Loader2 className="size-4 animate-spin text-accent" />
          <span className="text-sm text-fg-3">Loading open PRs…</span>
        </div>
      ) : pullRequests.length > 0 ? (
        <div className="space-y-2">
          <div className="max-h-48 overflow-y-auto rounded-md border border-edge">
            {pullRequests.map((pr) => (
              <button
                key={pr.number}
                type="button"
                onClick={() => onSelect(String(pr.number))}
                className={`flex w-full items-center gap-3 border-b border-edge p-2.5 text-left transition-colors last:border-b-0 hover:bg-surface-alt ${
                  selected === String(pr.number) ? "bg-accent-soft" : ""
                }`}
              >
                <span
                  className={`shrink-0 rounded-[4px] border px-1.5 py-0.5 font-mono text-[10px] font-medium ${
                    selected === String(pr.number)
                      ? "border-accent-muted text-accent"
                      : "border-edge text-fg-3"
                  }`}
                >
                  #{pr.number}
                </span>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm text-fg">{pr.title}</p>
                  <p className="font-mono text-[10px] text-fg-4">
                    {pr.source_branch} → {pr.target_branch}
                    {pr.author && ` · ${pr.author}`}
                  </p>
                </div>
                {selected === String(pr.number) && (
                  <CircleCheck className="size-4 shrink-0 text-accent" />
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
        <div className="flex items-center gap-3 rounded-md border border-accent-muted bg-accent-soft p-2.5">
          <CircleCheck className="size-4 shrink-0 text-accent" />
          <span className="font-mono text-xs font-medium text-accent">
            #{selectedPR.number}
          </span>
          <span className="truncate text-xs text-fg">{selectedPR.title}</span>
        </div>
      )}

      {/* URL paste helper + manual number input */}
      <div className="flex gap-2">
        <div className="relative flex-1">
          <Link2 className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-fg-4" />
          <input
            type="text"
            value={urlInput}
            onChange={(e) => handleUrlPaste(e.target.value)}
            onPaste={(e) => {
              const text = e.clipboardData.getData("text");
              handleUrlPaste(text);
              e.preventDefault();
            }}
            placeholder="Paste a PR/MR URL or enter a number"
            className={inputCls + " pl-9 font-mono"}
          />
        </div>
        <input
          type="number"
          min="1"
          value={selected}
          onChange={(e) => onSelect(e.target.value)}
          placeholder="#"
          className={inputCls + " w-24 text-center font-mono"}
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
        className="flex w-full items-center justify-between rounded-md border border-edge bg-input p-3 text-left transition-colors hover:border-fg-4"
      >
        <span className="font-mono text-sm text-fg">{selected.full_name}</span>
        <ChevronsUpDown className="size-4 text-fg-3" />
      </button>
    );
  }

  return (
    <div className="space-y-2">
      <div className="relative">
        <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-fg-4" />
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search repositories…"
          className="w-full rounded-md border border-edge bg-input py-2 pr-3 pl-9 font-mono text-sm text-fg placeholder-fg-4 transition-colors focus:border-accent focus:outline-none"
          autoFocus
        />
      </div>

      {loading ? (
        <div className="flex items-center justify-center py-8">
          <Loader2 className="size-5 animate-spin text-accent" />
        </div>
      ) : (
        <div className="max-h-64 overflow-y-auto rounded-md border border-edge">
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
                className={`flex w-full items-center gap-3 border-b border-edge p-3 text-left transition-colors last:border-b-0 hover:bg-surface-alt ${
                  selected?.full_name === repo.full_name ? "bg-accent-soft" : ""
                }`}
              >
                {repo.private ? (
                  <Lock className="size-4 shrink-0 text-fg-4" />
                ) : (
                  <Globe className="size-4 shrink-0 text-fg-4" />
                )}
                <div className="flex-1 overflow-hidden">
                  <p className="truncate font-mono text-sm text-fg">
                    {repo.full_name}
                  </p>
                  {repo.description && (
                    <p className="truncate text-xs text-fg-4">
                      {repo.description}
                    </p>
                  )}
                </div>
                <span className="shrink-0 rounded-[4px] border border-edge bg-surface-alt px-1.5 py-0.5 font-mono text-[10px] text-fg-3">
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

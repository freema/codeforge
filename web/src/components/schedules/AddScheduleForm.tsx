import { useEffect, useMemo, useState, type FormEvent } from "react";
import {
  CircleCheck,
  Github,
  Gitlab,
  Link2,
  Loader2,
  Plus,
} from "lucide-react";
import { useCreateSchedule } from "../../hooks/useSchedules";
import { useCLIs } from "../../hooks/useCLIs";
import { useKeys } from "../../hooks/useKeys";
import { useRepositories } from "../../hooks/useRepositories";
import { useSessionTypes } from "../../hooks/useSessionTypes";
import { useToast } from "../../context/ToastContext";
import RepoSelector from "../RepoSelector";
import type { Repository } from "../../types";

const inputCls =
  "w-full rounded-md border border-edge bg-input px-3 py-2 text-sm text-fg placeholder-fg-4 transition-colors focus:border-accent focus:outline-none";

const labelCls = "mb-2 block text-sm font-medium text-fg-2";

const DEFAULT_CRON = "*/15 * * * *";

const CRON_PRESETS = [
  { value: DEFAULT_CRON, label: "Every 15 min" },
  { value: "0 * * * *", label: "Hourly" },
  { value: "0 3 * * *", label: "Daily 03:00" },
  { value: "0 3 * * 0", label: "Weekly Sun 03:00" },
  { value: "custom", label: "Custom" },
];

export default function AddScheduleForm() {
  const { toast } = useToast();
  const createSchedule = useCreateSchedule();
  const { data: clis } = useCLIs();
  const { data: allKeys } = useKeys();
  const { data: sessionTypes } = useSessionTypes();

  // pr_review needs a concrete PR number, so it cannot recur on a schedule
  const schedulableTypes = useMemo(
    () => sessionTypes?.filter((t) => t.name !== "pr_review") ?? [],
    [sessionTypes],
  );

  const keys = useMemo(
    () =>
      allKeys?.filter(
        (k) => k.provider === "github" || k.provider === "gitlab",
      ),
    [allKeys],
  );

  const [name, setName] = useState("");
  const [cronPreset, setCronPreset] = useState(DEFAULT_CRON);
  const [customCron, setCustomCron] = useState("");
  // null = not initialized yet (auto-select first key), "" = none / manual URL
  const [providerKey, setProviderKey] = useState<string | null>(null);
  const [selectedRepo, setSelectedRepo] = useState<Repository | null>(null);
  const [repoUrl, setRepoUrl] = useState("");
  const [sessionType, setSessionType] = useState("code");
  const [prompt, setPrompt] = useState("");
  const [cli, setCli] = useState("");
  const [timezone, setTimezone] = useState("");

  const activeKey = providerKey ?? "";

  // Prompt is required only for code and plan; review and knowledge
  // fall back to backend defaults when empty.
  const isPromptRequired = sessionType === "code" || sessionType === "plan";
  const selectedType = schedulableTypes.find((t) => t.name === sessionType);
  const promptPlaceholder = isPromptRequired
    ? "What should the agent do on each run?"
    : (selectedType?.description ?? "Optional additional instructions");

  const availableClis = useMemo(
    () => clis?.filter((c) => c.available) ?? [],
    [clis],
  );

  // Auto-select default CLI once the list loads (same pattern as NewSession)
  useEffect(() => {
    if (availableClis.length > 0 && !cli) {
      const defaultCli = availableClis.find((c) => c.is_default);
      setCli(defaultCli?.name ?? availableClis[0]!.name);
    }
  }, [availableClis, cli]);

  // Auto-select first provider key on load (same pattern as NewSession);
  // "" means the user explicitly chose manual URL entry, so leave that alone.
  useEffect(() => {
    if (providerKey === null && keys && keys.length > 0) {
      setProviderKey(keys[0]!.name);
    }
  }, [keys, providerKey]);

  // Fetch repositories when a provider key is selected
  const { data: repos, isLoading: reposLoading } = useRepositories(
    activeKey || undefined,
  );

  function handleProviderSelect(keyName: string) {
    setProviderKey(keyName);
    setSelectedRepo(null);
    setRepoUrl("");
  }

  function handleRepoSelect(repo: Repository) {
    setSelectedRepo(repo);
    setRepoUrl(repo.clone_url);
  }

  async function handleCreate(e: FormEvent) {
    e.preventDefault();
    const cron = cronPreset === "custom" ? customCron.trim() : cronPreset;
    try {
      await createSchedule.mutateAsync({
        name,
        cron,
        ...(timezone.trim() ? { timezone: timezone.trim() } : {}),
        session_request: {
          repo_url: repoUrl,
          ...(prompt.trim() ? { prompt: prompt.trim() } : {}),
          ...(sessionType !== "code" ? { session_type: sessionType } : {}),
          ...(activeKey ? { provider_key: activeKey } : {}),
          ...(cli ? { config: { cli } } : {}),
        },
      });
      toast("success", "Schedule created");
      setName("");
      setCronPreset(DEFAULT_CRON);
      setCustomCron("");
      setSelectedRepo(null);
      setRepoUrl("");
      setSessionType("code");
      setPrompt("");
      setCli("");
      setTimezone("");
    } catch (err) {
      toast(
        "error",
        `Create failed: ${err instanceof Error ? err.message : String(err)}`,
      );
    }
  }

  return (
    <form
      onSubmit={(e) => void handleCreate(e)}
      className="overflow-hidden rounded-md border border-edge bg-surface"
    >
      <div className="border-b border-edge px-5 py-3.5">
        <span className="eyebrow">New schedule</span>
      </div>
      <div className="space-y-5 p-5">
        {/* Name + cron */}
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label className={labelCls}>Name</label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Nightly dependency check"
              required
              className={inputCls}
            />
          </div>
          <div>
            <label className={labelCls}>Schedule</label>
            <select
              value={cronPreset}
              onChange={(e) => setCronPreset(e.target.value)}
              className={inputCls}
            >
              {CRON_PRESETS.map((p) => (
                <option key={p.value} value={p.value}>
                  {p.label}
                </option>
              ))}
            </select>
          </div>
          {cronPreset === "custom" && (
            <div className="sm:col-span-2">
              <label className={labelCls}>Cron expression</label>
              <input
                type="text"
                value={customCron}
                onChange={(e) => setCustomCron(e.target.value)}
                placeholder="0 6 * * 1-5"
                required
                className={`${inputCls} font-mono`}
              />
            </div>
          )}
        </div>

        {/* Provider key (optional) */}
        {keys && keys.length > 0 && (
          <div>
            <label className={labelCls}>Provider</label>
            <div className="flex flex-wrap gap-2">
              {keys.map((k) => (
                <button
                  key={k.name}
                  type="button"
                  onClick={() => handleProviderSelect(k.name)}
                  className={`flex items-center gap-2 rounded-md border px-4 py-2 font-mono text-sm transition-colors ${
                    activeKey === k.name
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
              <button
                type="button"
                onClick={() => handleProviderSelect("")}
                className={`flex items-center gap-2 rounded-md border px-4 py-2 text-sm transition-colors ${
                  activeKey === ""
                    ? "border-accent-muted bg-accent-soft text-accent"
                    : "border-edge bg-surface-alt text-fg-3 hover:border-fg-4 hover:text-fg"
                }`}
              >
                <Link2 className="size-4" />
                None / manual URL
              </button>
            </div>
          </div>
        )}

        {/* Repository */}
        <div>
          <label className={labelCls}>Repository</label>
          {activeKey && repos && repos.length > 0 ? (
            <div className="space-y-3">
              <RepoSelector
                repos={repos}
                selected={selectedRepo}
                loading={reposLoading}
                onSelect={handleRepoSelect}
                autoFocusSearch={false}
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
          ) : activeKey && reposLoading ? (
            <div className="flex items-center gap-3 rounded-md border border-edge bg-surface-alt p-4">
              <Loader2 className="size-4 animate-spin text-accent" />
              <span className="text-sm text-fg-3">Loading repositories…</span>
            </div>
          ) : (
            <div>
              <input
                type="url"
                value={repoUrl}
                onChange={(e) => setRepoUrl(e.target.value)}
                placeholder="https://github.com/user/repo.git"
                required
                className={`${inputCls} font-mono`}
              />
              {!activeKey && keys && keys.length > 0 && (
                <p className="mt-2 text-xs text-fg-4">
                  Select a provider key above to browse repositories
                </p>
              )}
            </div>
          )}
        </div>

        {/* Session type */}
        {schedulableTypes.length > 0 && (
          <div>
            <label className={labelCls}>Session type</label>
            <div className="flex flex-wrap gap-2">
              {schedulableTypes.map((t) => (
                <button
                  key={t.name}
                  type="button"
                  onClick={() => setSessionType(t.name)}
                  title={t.description}
                  className={`flex items-center gap-2 rounded-md border px-4 py-2 text-sm transition-colors ${
                    sessionType === t.name
                      ? "border-accent-muted bg-accent-soft text-accent"
                      : "border-edge bg-surface-alt text-fg-3 hover:border-fg-4 hover:text-fg"
                  }`}
                >
                  {t.label}
                </button>
              ))}
            </div>
          </div>
        )}

        {/* Prompt */}
        <div>
          <label className={labelCls}>
            {isPromptRequired ? "Prompt" : "Prompt (optional)"}
          </label>
          <textarea
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder={promptPlaceholder}
            required={isPromptRequired}
            rows={3}
            className={`${inputCls} resize-y`}
          />
          {sessionType === "knowledge" && (
            <p className="mt-2 text-xs text-fg-4">
              Writes .codeforge/ knowledge docs; open a PR from the resulting
              session to keep them.
            </p>
          )}
        </div>

        {/* CLI + timezone */}
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label className={labelCls}>CLI</label>
            <select
              value={cli}
              onChange={(e) => setCli(e.target.value)}
              className={`${inputCls} font-mono`}
            >
              {availableClis.map((c) => (
                <option key={c.name} value={c.name}>
                  {c.name + (c.is_default ? " (default)" : "")}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className={labelCls}>Timezone (optional)</label>
            <input
              type="text"
              value={timezone}
              onChange={(e) => setTimezone(e.target.value)}
              placeholder="Europe/Prague"
              title="Timezone for cron evaluation (IANA name)"
              className={`${inputCls} font-mono`}
            />
          </div>
        </div>

        <button
          type="submit"
          disabled={createSchedule.isPending || !repoUrl}
          className="flex items-center gap-2 rounded-md bg-accent px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-accent-hover disabled:opacity-50"
        >
          {createSchedule.isPending ? (
            <Loader2 className="size-4 animate-spin" />
          ) : (
            <Plus className="size-4" />
          )}
          Create schedule
        </button>
      </div>
    </form>
  );
}

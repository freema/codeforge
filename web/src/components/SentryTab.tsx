import {
  useState,
  useEffect,
  useMemo,
  useCallback,
  type FormEvent,
} from "react";
import { useKeys } from "../hooks/useKeys";
import { useRepositories } from "../hooks/useRepositories";
import {
  useSentryOrganizations,
  useSentryProjects,
  useSentryIssues,
  useSentryLatestEvent,
  useFixSentryIssue,
} from "../hooks/useSentry";
import { useWorkflowRuns } from "../hooks/useWorkflowRuns";
import { useWorkflowRunStream } from "../hooks/useWorkflowRuns";
import type {
  SentryConfig,
  SentryIssue,
  ProviderKey,
  WorkflowRun,
} from "../types";

const STORAGE_KEY = "sentry-configs";
const ACTIVE_KEY = "sentry-active-config";

function isValidConfig(cfg: SentryConfig): boolean {
  return !!(cfg.key_name && cfg.org_slug && cfg.project_slug && cfg.repo_url);
}

function loadConfigs(): SentryConfig[] {
  try {
    // Migrate from old single-config format
    const oldRaw = localStorage.getItem("sentry-config");
    if (oldRaw) {
      const old = JSON.parse(oldRaw) as SentryConfig;
      if (isValidConfig(old)) {
        if (!old.label) old.label = `${old.org_slug}/${old.project_slug}`;
        const configs = [old];
        localStorage.setItem(STORAGE_KEY, JSON.stringify(configs));
        localStorage.setItem(ACTIVE_KEY, "0");
        localStorage.removeItem("sentry-config");
        return configs;
      }
      localStorage.removeItem("sentry-config");
    }

    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const configs = JSON.parse(raw) as SentryConfig[];
    return configs.filter(isValidConfig);
  } catch {
    return [];
  }
}

function saveConfigs(configs: SentryConfig[]) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(configs));
}

function loadActiveIndex(): number {
  const raw = localStorage.getItem(ACTIVE_KEY);
  return raw ? parseInt(raw, 10) || 0 : 0;
}

function saveActiveIndex(idx: number) {
  localStorage.setItem(ACTIVE_KEY, String(idx));
}

const inputCls =
  "w-full rounded-lg border border-edge bg-surface px-3 py-2.5 text-sm text-fg font-mono placeholder-fg-4 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent transition-colors";

const selectCls =
  "w-full rounded-lg border border-edge bg-surface px-3 py-2.5 text-sm text-fg font-mono focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent transition-colors appearance-none";

// ─── Root ───────────────────────────────────────────────

export default function SentryTab({
  onSwitchToKeys,
}: {
  onSwitchToKeys: () => void;
}) {
  const { data: keys } = useKeys();
  const [configs, setConfigs] = useState<SentryConfig[]>(loadConfigs);
  const [activeIdx, setActiveIdx] = useState(loadActiveIndex);
  const [editingIdx, setEditingIdx] = useState<number | null>(null); // null=not editing, -1=new config
  const [confirmDeleteIdx, setConfirmDeleteIdx] = useState<number | null>(null);

  const sentryKeys = useMemo(
    () => keys?.filter((k) => k.provider === "sentry") ?? [],
    [keys],
  );

  const hasKey = sentryKeys.length > 0;
  const activeConfig = configs[activeIdx] ?? null;

  function handleSaveConfig(cfg: SentryConfig) {
    const newConfigs = [...configs];
    if (editingIdx === -1) {
      // Adding new
      newConfigs.push(cfg);
      const newIdx = newConfigs.length - 1;
      saveConfigs(newConfigs);
      saveActiveIndex(newIdx);
      setConfigs(newConfigs);
      setActiveIdx(newIdx);
    } else if (editingIdx !== null) {
      // Editing existing
      newConfigs[editingIdx] = cfg;
      saveConfigs(newConfigs);
      setConfigs(newConfigs);
    }
    setEditingIdx(null);
  }

  function handleDeleteConfig(idx: number) {
    const newConfigs = configs.filter((_, i) => i !== idx);
    saveConfigs(newConfigs);
    setConfigs(newConfigs);
    setConfirmDeleteIdx(null);
    // Adjust active index
    if (newConfigs.length === 0) {
      setActiveIdx(0);
      saveActiveIndex(0);
    } else if (activeIdx >= newConfigs.length) {
      const newIdx = newConfigs.length - 1;
      setActiveIdx(newIdx);
      saveActiveIndex(newIdx);
    }
  }

  function handleSwitchConfig(idx: number) {
    setActiveIdx(idx);
    saveActiveIndex(idx);
  }

  if (!hasKey) {
    return <SentrySetup onSwitchToKeys={onSwitchToKeys} />;
  }

  if (editingIdx !== null) {
    return (
      <SentryConfigForm
        sentryKeys={sentryKeys}
        allKeys={keys ?? []}
        initial={editingIdx === -1 ? null : (configs[editingIdx] ?? null)}
        onSave={handleSaveConfig}
        onCancel={() => setEditingIdx(null)}
      />
    );
  }

  if (configs.length === 0) {
    return (
      <SentryConfigForm
        sentryKeys={sentryKeys}
        allKeys={keys ?? []}
        initial={null}
        onSave={(cfg) => {
          const newConfigs = [cfg];
          saveConfigs(newConfigs);
          saveActiveIndex(0);
          setConfigs(newConfigs);
          setActiveIdx(0);
        }}
      />
    );
  }

  return (
    <div className="space-y-4">
      {/* Config switcher */}
      {configs.length > 0 && (
        <div className="flex items-center gap-2 flex-wrap">
          {configs.map((cfg, idx) => (
            <div key={idx} className="relative group">
              <button
                onClick={() => handleSwitchConfig(idx)}
                className={`flex items-center gap-2 rounded-lg border px-3 py-2 text-xs font-medium transition-all ${
                  idx === activeIdx
                    ? "border-accent bg-accent/10 text-accent"
                    : "border-edge bg-surface text-fg-3 hover:border-fg-4 hover:text-fg-2"
                }`}
              >
                <span className="material-symbols-outlined text-sm">
                  bug_report
                </span>
                <span className="font-mono">
                  {cfg.label || `${cfg.org_slug}/${cfg.project_slug}`}
                </span>
              </button>
              {/* Edit/Delete on hover */}
              <div className="absolute -top-1 -right-1 hidden group-hover:flex gap-0.5">
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    setEditingIdx(idx);
                  }}
                  className="flex h-5 w-5 items-center justify-center rounded-full bg-surface border border-edge text-fg-4 hover:text-accent hover:border-accent/30 transition-colors"
                  title="Edit"
                >
                  <span className="material-symbols-outlined text-[11px]">
                    edit
                  </span>
                </button>
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    setConfirmDeleteIdx(idx);
                  }}
                  className="flex h-5 w-5 items-center justify-center rounded-full bg-surface border border-edge text-fg-4 hover:text-red-400 hover:border-red-500/30 transition-colors"
                  title="Delete"
                >
                  <span className="material-symbols-outlined text-[11px]">
                    close
                  </span>
                </button>
              </div>
            </div>
          ))}
          <button
            onClick={() => setEditingIdx(-1)}
            className="flex items-center gap-1 rounded-lg border border-dashed border-edge px-3 py-2 text-xs text-fg-4 transition-colors hover:border-accent/30 hover:text-accent"
          >
            <span className="material-symbols-outlined text-sm">add</span>
            Add
          </button>
        </div>
      )}

      {/* Delete confirmation */}
      {confirmDeleteIdx !== null && (
        <div className="flex items-center gap-3 rounded-lg border border-red-900/50 bg-red-900/10 px-4 py-2.5">
          <span className="text-xs text-red-400">
            Delete "
            {configs[confirmDeleteIdx]?.label ||
              configs[confirmDeleteIdx]?.project_slug}
            "?
          </span>
          <button
            onClick={() => handleDeleteConfig(confirmDeleteIdx)}
            className="rounded bg-red-900/30 px-3 py-1 text-xs font-medium text-red-400 hover:bg-red-900/50"
          >
            Delete
          </button>
          <button
            onClick={() => setConfirmDeleteIdx(null)}
            className="text-xs text-fg-4 hover:text-fg-3"
          >
            Cancel
          </button>
        </div>
      )}

      {/* Issues view for active config */}
      {activeConfig && (
        <SentryIssuesView
          config={activeConfig}
          onOpenConfig={() => setEditingIdx(activeIdx)}
        />
      )}
    </div>
  );
}

// ─── Setup (no key) ─────────────────────────────────────

function SentrySetup({ onSwitchToKeys }: { onSwitchToKeys: () => void }) {
  return (
    <div className="flex flex-col items-center gap-4 rounded-xl border border-edge bg-surface/50 py-16">
      <div className="flex h-16 w-16 items-center justify-center rounded-2xl border border-edge bg-surface">
        <span className="material-symbols-outlined text-3xl text-fg-3">
          bug_report
        </span>
      </div>
      <div className="text-center">
        <h3 className="text-lg font-semibold text-fg">Connect Sentry</h3>
        <p className="mt-1 max-w-sm text-sm text-fg-3">
          Add a Sentry authentication token in Provider Keys to get started.
        </p>
      </div>
      <button
        onClick={onSwitchToKeys}
        className="flex items-center gap-2 rounded-lg bg-accent px-5 py-2 text-sm font-bold text-page transition-all hover:bg-accent-hover"
      >
        <span className="material-symbols-outlined text-lg">vpn_key</span>
        Go to Provider Keys
      </button>
    </div>
  );
}

// ─── Config Form ────────────────────────────────────────

function SentryConfigForm({
  sentryKeys,
  allKeys,
  initial,
  onSave,
  onCancel,
}: {
  sentryKeys: ProviderKey[];
  allKeys: ProviderKey[];
  initial: SentryConfig | null;
  onSave: (cfg: SentryConfig) => void;
  onCancel?: () => void;
}) {
  const [label, setLabel] = useState(initial?.label ?? "");
  const [keyName, setKeyName] = useState(
    initial?.key_name ?? sentryKeys[0]?.name ?? "",
  );
  const [orgSlug, setOrgSlug] = useState(initial?.org_slug ?? "");
  const [projectSlug, setProjectSlug] = useState(initial?.project_slug ?? "");
  const [repoUrl, setRepoUrl] = useState(initial?.repo_url ?? "");
  const [providerKey, setProviderKey] = useState(initial?.provider_key ?? "");

  // Auto-detect organizations from selected key
  const { data: sentryOrgs, isLoading: orgsLoading } = useSentryOrganizations(
    keyName || undefined,
  );

  // Auto-select org when there's only one
  useEffect(() => {
    if (!orgSlug && sentryOrgs && sentryOrgs.length === 1) {
      setOrgSlug(sentryOrgs[0]!.slug);
    }
  }, [sentryOrgs, orgSlug]);

  const { data: sentryProjects, isLoading: projectsLoading } =
    useSentryProjects(keyName || undefined, orgSlug || undefined);

  const gitKeys = useMemo(
    () =>
      allKeys.filter((k) => k.provider === "github" || k.provider === "gitlab"),
    [allKeys],
  );

  // Auto-select first git key if only one exists
  useEffect(() => {
    if (!providerKey && gitKeys.length === 1) {
      setProviderKey(gitKeys[0]!.name);
    }
  }, [gitKeys, providerKey]);

  const { data: repos, isLoading: reposLoading } = useRepositories(
    providerKey || undefined,
  );

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    onSave({
      label: label || `${orgSlug}/${projectSlug}`,
      key_name: keyName,
      org_slug: orgSlug,
      project_slug: projectSlug,
      repo_url: repoUrl,
      provider_key: providerKey || undefined,
    });
  }

  return (
    <form
      onSubmit={handleSubmit}
      className="rounded-xl border border-edge bg-surface/50 p-5"
    >
      <div className="mb-4 flex items-center justify-between">
        <h3 className="flex items-center gap-2 text-sm font-bold uppercase tracking-wider text-fg-2">
          <span className="material-symbols-outlined text-accent text-base">
            settings
          </span>
          Sentry Configuration
        </h3>
        {onCancel && (
          <button
            type="button"
            onClick={onCancel}
            className="flex items-center gap-1 text-xs text-fg-3 transition-colors hover:text-accent"
          >
            <span className="material-symbols-outlined text-sm">close</span>
            Cancel
          </button>
        )}
      </div>

      <div className="space-y-4">
        {/* Label */}
        <div>
          <label className="mb-1.5 block text-xs font-medium text-fg-3">
            Label
          </label>
          <input
            type="text"
            value={label}
            onChange={(e) => setLabel(e.target.value)}
            placeholder="e.g. Production, Staging..."
            className={inputCls}
          />
          <p className="mt-1 text-[11px] text-fg-4">
            Optional name to identify this configuration
          </p>
        </div>

        {/* Sentry Key */}
        <div>
          <label className="mb-1.5 block text-xs font-medium text-fg-3">
            Sentry Key
          </label>
          {sentryKeys.length === 1 ? (
            <div
              className={`flex items-center gap-2 rounded-lg border border-accent/30 bg-accent/5 px-3 py-2.5 text-sm text-accent font-mono`}
            >
              <span className="material-symbols-outlined text-base">
                vpn_key
              </span>
              {sentryKeys[0]!.name}
            </div>
          ) : (
            <select
              value={keyName}
              onChange={(e) => {
                setKeyName(e.target.value);
                setOrgSlug("");
                setProjectSlug("");
              }}
              className={selectCls}
            >
              {sentryKeys.map((k) => (
                <option key={k.name} value={k.name}>
                  {k.name}
                </option>
              ))}
            </select>
          )}
        </div>

        {/* Organization */}
        <div>
          <label className="mb-1.5 block text-xs font-medium text-fg-3">
            Organization
          </label>
          {orgsLoading ? (
            <div className="flex items-center gap-2 py-2 text-xs text-fg-4">
              <span className="material-symbols-outlined animate-spin text-sm">
                progress_activity
              </span>
              Loading organizations...
            </div>
          ) : sentryOrgs && sentryOrgs.length === 1 ? (
            <div
              className={`flex items-center gap-2 rounded-lg border border-accent/30 bg-accent/5 px-3 py-2.5 text-sm text-accent font-mono`}
            >
              <span className="material-symbols-outlined text-base">
                apartment
              </span>
              {sentryOrgs[0]!.name}{" "}
              <span className="text-fg-4">({sentryOrgs[0]!.slug})</span>
            </div>
          ) : sentryOrgs && sentryOrgs.length > 1 ? (
            <select
              value={orgSlug}
              onChange={(e) => {
                setOrgSlug(e.target.value);
                setProjectSlug("");
              }}
              required
              className={selectCls}
            >
              <option value="">Select organization...</option>
              {sentryOrgs.map((o) => (
                <option key={o.slug} value={o.slug}>
                  {o.name} ({o.slug})
                </option>
              ))}
            </select>
          ) : (
            <input
              type="text"
              value={orgSlug}
              onChange={(e) => {
                setOrgSlug(e.target.value);
                setProjectSlug("");
              }}
              placeholder="my-org"
              required
              className={inputCls}
            />
          )}
        </div>

        {/* Project */}
        <div>
          <label className="mb-1.5 block text-xs font-medium text-fg-3">
            Project
          </label>
          {projectsLoading && orgSlug ? (
            <div className="flex items-center gap-2 py-2 text-xs text-fg-4">
              <span className="material-symbols-outlined animate-spin text-sm">
                progress_activity
              </span>
              Loading projects...
            </div>
          ) : sentryProjects && sentryProjects.length > 0 ? (
            <select
              value={projectSlug}
              onChange={(e) => setProjectSlug(e.target.value)}
              required
              className={selectCls}
            >
              <option value="">Select project...</option>
              {sentryProjects.map((p) => (
                <option key={p.id} value={p.slug}>
                  {p.name} ({p.slug})
                </option>
              ))}
            </select>
          ) : orgSlug ? (
            <input
              type="text"
              value={projectSlug}
              onChange={(e) => setProjectSlug(e.target.value)}
              placeholder="my-project"
              required
              className={inputCls}
            />
          ) : (
            <p className="py-2 text-xs text-fg-4">
              Select an organization first
            </p>
          )}
        </div>

        {/* Git Provider Key */}
        <div>
          <label className="mb-1.5 block text-xs font-medium text-fg-3">
            Git Provider Key (for PRs)
          </label>
          {gitKeys.length > 0 ? (
            <select
              value={providerKey}
              onChange={(e) => setProviderKey(e.target.value)}
              className={selectCls}
            >
              <option value="">None</option>
              {gitKeys.map((k) => (
                <option key={k.name} value={k.name}>
                  {k.name} ({k.provider})
                </option>
              ))}
            </select>
          ) : (
            <p className="py-2 text-xs text-fg-4">
              No GitHub/GitLab keys configured
            </p>
          )}
        </div>

        {/* Repository URL */}
        <div>
          <label className="mb-1.5 block text-xs font-medium text-fg-3">
            Repository
          </label>
          {reposLoading && providerKey ? (
            <div className="flex items-center gap-2 py-2 text-xs text-fg-4">
              <span className="material-symbols-outlined animate-spin text-sm">
                progress_activity
              </span>
              Loading repositories...
            </div>
          ) : repos && repos.length > 0 ? (
            <select
              value={repoUrl}
              onChange={(e) => setRepoUrl(e.target.value)}
              required
              className={selectCls}
            >
              <option value="">Select a repository</option>
              {repos.map((r) => (
                <option key={r.clone_url} value={r.clone_url}>
                  {r.full_name}
                </option>
              ))}
            </select>
          ) : (
            <input
              type="url"
              value={repoUrl}
              onChange={(e) => setRepoUrl(e.target.value)}
              placeholder="https://github.com/org/repo"
              required
              className={inputCls}
            />
          )}
        </div>
      </div>

      <button
        type="submit"
        disabled={!keyName || !orgSlug || !projectSlug || !repoUrl}
        className="mt-5 flex items-center gap-2 rounded-lg bg-accent px-5 py-2 text-sm font-bold text-page transition-all hover:bg-accent-hover disabled:opacity-50"
      >
        <span className="material-symbols-outlined text-lg">save</span>
        Save Configuration
      </button>
    </form>
  );
}

// ─── Issues View ────────────────────────────────────────

type IssueSort = "date" | "freq" | "new" | "priority";

function SentryIssuesView({
  config,
  onOpenConfig,
}: {
  config: SentryConfig;
  onOpenConfig: () => void;
}) {
  const [search, setSearch] = useState("");
  const [sort, setSort] = useState<IssueSort>("freq");
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [drawerIssueId, setDrawerIssueId] = useState<string | null>(null);
  const [fixingIds, setFixingIds] = useState<Map<string, string>>(new Map()); // issueId -> runId

  const sortParam =
    sort === "freq"
      ? "freq"
      : sort === "new"
        ? "new"
        : sort === "priority"
          ? "priority"
          : "date";

  const {
    data: issues,
    isLoading,
    refetch,
  } = useSentryIssues(config.key_name, config.org_slug, config.project_slug, {
    query: search ? `is:unresolved ${search}` : "is:unresolved",
    sort: sortParam,
  });

  const fixIssue = useFixSentryIssue();

  const { data: runs } = useWorkflowRuns("sentry-fixer");
  const recentRuns = useMemo(() => (runs ?? []).slice(0, 10), [runs]);

  const toggleSelect = useCallback((id: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const toggleAll = useCallback(() => {
    if (!issues) return;
    setSelectedIds((prev) => {
      if (prev.size === issues.length) return new Set();
      return new Set(issues.map((i) => i.id));
    });
  }, [issues]);

  async function handleFixSingle(issue: SentryIssue) {
    try {
      const run = await fixIssue.mutateAsync({
        name: "sentry-fixer",
        params: {
          sentry_url: "https://sentry.io",
          issue_id: issue.id,
          repo_url: config.repo_url,
          key_name: config.key_name,
          ...(config.provider_key ? { provider_key: config.provider_key } : {}),
        },
      });
      setFixingIds((prev) => new Map(prev).set(issue.id, run.id));
    } catch {
      // error handled by mutation state
    }
  }

  async function handleBatchFix() {
    if (selectedIds.size === 0 || !issues) return;
    const selected = issues.filter((i) => selectedIds.has(i.id));
    const newFixing = new Map(fixingIds);

    await Promise.all(
      selected.map(async (issue) => {
        try {
          const run = await fixIssue.mutateAsync({
            name: "sentry-fixer",
            params: {
              sentry_url: "https://sentry.io",
              issue_id: issue.id,
              repo_url: config.repo_url,
              key_name: config.key_name,
              ...(config.provider_key
                ? { provider_key: config.provider_key }
                : {}),
            },
          });
          newFixing.set(issue.id, run.id);
        } catch {
          // skip failed ones
        }
      }),
    );

    setFixingIds(newFixing);
    setSelectedIds(new Set());
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between gap-4">
        <h3 className="flex items-center gap-2 text-lg font-semibold text-fg">
          <span className="material-symbols-outlined text-accent">
            bug_report
          </span>
          Sentry Issues
        </h3>
        <div className="flex items-center gap-2">
          <button
            onClick={onOpenConfig}
            className="flex items-center gap-1 rounded-lg border border-edge px-3 py-1.5 text-xs font-medium text-fg-3 transition-colors hover:border-accent/30 hover:text-accent"
          >
            <span className="material-symbols-outlined text-sm">settings</span>
            Config
          </button>
          <button
            onClick={() => void refetch()}
            className="flex items-center gap-1 rounded-lg border border-edge px-3 py-1.5 text-xs font-medium text-fg-3 transition-colors hover:border-accent/30 hover:text-accent"
          >
            <span className="material-symbols-outlined text-sm">refresh</span>
            Refresh
          </button>
        </div>
      </div>

      {/* Filters */}
      <div className="flex flex-wrap items-center gap-3">
        <select
          value={sort}
          onChange={(e) => setSort(e.target.value as IssueSort)}
          className="rounded-lg border border-edge bg-surface px-3 py-2 text-xs text-fg font-mono focus:border-accent focus:outline-none"
        >
          <option value="freq">Frequency</option>
          <option value="date">Last Seen</option>
          <option value="new">First Seen</option>
          <option value="priority">Priority</option>
        </select>
        <div className="relative flex-1">
          <span className="material-symbols-outlined absolute left-3 top-1/2 -translate-y-1/2 text-sm text-fg-4">
            search
          </span>
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search issues..."
            className="w-full rounded-lg border border-edge bg-surface py-2 pl-9 pr-3 text-xs text-fg font-mono placeholder-fg-4 focus:border-accent focus:outline-none"
          />
        </div>
      </div>

      {/* Batch actions */}
      {selectedIds.size > 0 && (
        <div className="flex items-center gap-3 rounded-lg border border-accent/30 bg-accent/5 px-4 py-2.5">
          <span className="text-xs font-medium text-accent">
            {selectedIds.size} selected
          </span>
          <button
            onClick={() => void handleBatchFix()}
            disabled={fixIssue.isPending}
            className="flex items-center gap-1.5 rounded-lg bg-accent px-4 py-1.5 text-xs font-bold text-page transition-all hover:bg-accent-hover disabled:opacity-50"
          >
            {fixIssue.isPending ? (
              <span className="material-symbols-outlined animate-spin text-sm">
                progress_activity
              </span>
            ) : (
              <span className="material-symbols-outlined text-sm">build</span>
            )}
            Fix Selected ({selectedIds.size})
          </button>
          <button
            onClick={() => setSelectedIds(new Set())}
            className="text-xs text-fg-3 hover:text-fg"
          >
            Clear
          </button>
        </div>
      )}

      {/* Issues list */}
      {isLoading ? (
        <div className="space-y-3">
          {[1, 2, 3, 4, 5].map((i) => (
            <div
              key={i}
              className="h-16 animate-pulse rounded-xl bg-surface-alt"
            />
          ))}
        </div>
      ) : issues && issues.length > 0 ? (
        <div className="rounded-xl border border-edge overflow-hidden">
          {/* Select all header */}
          <div className="flex items-center gap-3 border-b border-edge bg-surface/50 px-4 py-2">
            <input
              type="checkbox"
              checked={selectedIds.size === issues.length && issues.length > 0}
              onChange={toggleAll}
              className="h-3.5 w-3.5 rounded border-edge accent-[#00ff40]"
            />
            <span className="text-[10px] font-medium uppercase tracking-wider text-fg-4">
              {issues.length} issue{issues.length !== 1 && "s"}
            </span>
          </div>

          {issues.map((issue) => {
            const runId = fixingIds.get(issue.id);
            return (
              <IssueRow
                key={issue.id}
                issue={issue}
                selected={selectedIds.has(issue.id)}
                onToggle={() => toggleSelect(issue.id)}
                onClick={() => setDrawerIssueId(issue.id)}
                runId={runId}
                onFix={() => void handleFixSingle(issue)}
                isFixing={fixIssue.isPending}
              />
            );
          })}
        </div>
      ) : (
        <div className="flex flex-col items-center gap-3 py-12 text-center">
          <span className="material-symbols-outlined text-3xl text-fg-4">
            check_circle
          </span>
          <p className="text-sm text-fg-3">No unresolved issues found</p>
        </div>
      )}

      {/* Fix progress for active fixes */}
      {fixingIds.size > 0 && (
        <FixProgressSection fixingIds={fixingIds} issues={issues ?? []} />
      )}

      {/* Run history */}
      {recentRuns.length > 0 && <RunHistorySection runs={recentRuns} />}

      {/* Issue drawer */}
      {drawerIssueId && (
        <SentryIssueDrawer
          keyName={config.key_name}
          issueId={drawerIssueId}
          config={config}
          onClose={() => setDrawerIssueId(null)}
          onFix={(issue) => {
            void handleFixSingle(issue);
            setDrawerIssueId(null);
          }}
        />
      )}
    </div>
  );
}

// ─── Issue Row ──────────────────────────────────────────

const LEVEL_COLORS: Record<string, string> = {
  fatal: "bg-red-500",
  error: "bg-orange-500",
  warning: "bg-yellow-500",
  info: "bg-blue-500",
};

function relativeTime(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

function IssueRow({
  issue,
  selected,
  onToggle,
  onClick,
  runId,
  onFix,
  isFixing,
}: {
  issue: SentryIssue;
  selected: boolean;
  onToggle: () => void;
  onClick: () => void;
  runId?: string;
  onFix: () => void;
  isFixing: boolean;
}) {
  return (
    <div className="flex items-center gap-3 border-b border-edge px-4 py-3 transition-colors hover:bg-surface-alt/50 last:border-b-0">
      <input
        type="checkbox"
        checked={selected}
        onChange={onToggle}
        className="h-3.5 w-3.5 shrink-0 rounded border-edge accent-[#00ff40]"
      />

      <button
        onClick={onClick}
        className="flex flex-1 items-start gap-3 text-left min-w-0"
      >
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="shrink-0 font-mono text-[11px] text-fg-4">
              {issue.shortId}
            </span>
            <span className="truncate text-sm font-medium text-fg">
              {issue.title}
            </span>
          </div>
          <div className="mt-0.5 flex items-center gap-2 text-xs text-fg-4">
            <span className="truncate">{issue.culprit}</span>
            <span>·</span>
            <span className="shrink-0">{relativeTime(issue.lastSeen)}</span>
          </div>
        </div>

        <div className="flex shrink-0 items-center gap-3">
          <span className="rounded-full border border-edge bg-surface px-2 py-0.5 font-mono text-[11px] text-fg-3">
            {issue.count}x
          </span>
          <div
            className={`h-2.5 w-2.5 rounded-full ${LEVEL_COLORS[issue.level] ?? "bg-gray-500"}`}
          />
        </div>
      </button>

      {runId ? (
        <InlineRunStatus runId={runId} />
      ) : (
        <button
          onClick={(e) => {
            e.stopPropagation();
            onFix();
          }}
          disabled={isFixing}
          className="shrink-0 flex items-center gap-1 rounded-lg border border-edge px-2.5 py-1.5 text-[11px] font-medium text-fg-3 transition-colors hover:border-accent/30 hover:text-accent disabled:opacity-50"
        >
          <span className="material-symbols-outlined text-sm">build</span>
          Fix
        </button>
      )}
    </div>
  );
}

// ─── Inline Run Status ──────────────────────────────────

function InlineRunStatus({ runId }: { runId: string }) {
  const stream = useWorkflowRunStream(runId);

  const currentStep = useMemo(() => {
    const lastEvent = stream.events[stream.events.length - 1];
    if (lastEvent?.event === "step_update") {
      const data = lastEvent.data as { step_name?: string; status?: string };
      return data.step_name;
    }
    return null;
  }, [stream.events]);

  const statusIcon =
    stream.runStatus === "completed"
      ? "check_circle"
      : stream.runStatus === "failed"
        ? "error"
        : "progress_activity";

  const statusColor =
    stream.runStatus === "completed"
      ? "text-accent"
      : stream.runStatus === "failed"
        ? "text-red-400"
        : "text-yellow-400";

  return (
    <div className="flex shrink-0 items-center gap-1.5">
      <span
        className={`material-symbols-outlined text-sm ${statusColor} ${
          stream.runStatus === "running" || stream.runStatus === "pending"
            ? "animate-spin"
            : ""
        }`}
      >
        {statusIcon}
      </span>
      <span className="font-mono text-[10px] text-fg-4">
        {currentStep ?? stream.runStatus ?? "starting"}
      </span>
    </div>
  );
}

// ─── Issue Drawer ───────────────────────────────────────

function SentryIssueDrawer({
  keyName,
  issueId,
  onClose,
  onFix,
}: {
  keyName: string;
  issueId: string;
  config: SentryConfig;
  onClose: () => void;
  onFix: (issue: SentryIssue) => void;
}) {
  const { data: event, isLoading: eventLoading } = useSentryLatestEvent(
    keyName,
    issueId,
  );

  // Extract stacktrace from event entries
  const exceptionEntry = useMemo(
    () => event?.entries.find((e) => e.type === "exception"),
    [event],
  );

  const breadcrumbsEntry = useMemo(
    () => event?.entries.find((e) => e.type === "breadcrumbs"),
    [event],
  );

  const stackFrames = useMemo(() => {
    if (!exceptionEntry) return [];
    const data = exceptionEntry.data as {
      values?: Array<{
        type?: string;
        value?: string;
        stacktrace?: {
          frames?: Array<{
            filename?: string;
            function?: string;
            lineNo?: number;
            colNo?: number;
            context?: Array<[number, string]>;
            absPath?: string;
            inApp?: boolean;
          }>;
        };
      }>;
    };
    const values = data?.values ?? [];
    if (values.length === 0) return [];
    const frames = values[0]?.stacktrace?.frames ?? [];
    return [...frames].reverse().slice(0, 10);
  }, [exceptionEntry]);

  const exceptionMeta = useMemo(() => {
    if (!exceptionEntry) return null;
    const data = exceptionEntry.data as {
      values?: Array<{ type?: string; value?: string }>;
    };
    const val = data?.values?.[0];
    return val ? { type: val.type, value: val.value } : null;
  }, [exceptionEntry]);

  const breadcrumbs = useMemo(() => {
    if (!breadcrumbsEntry) return [];
    const data = breadcrumbsEntry.data as {
      values?: Array<{
        type?: string;
        category?: string;
        message?: string;
        timestamp?: string;
        level?: string;
      }>;
    };
    return (data?.values ?? []).slice(-15);
  }, [breadcrumbsEntry]);

  return (
    <div className="fixed inset-0 z-50 flex justify-end">
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-black/60 backdrop-blur-sm"
        onClick={onClose}
      />

      {/* Panel */}
      <div className="relative w-full max-w-2xl overflow-y-auto border-l border-edge bg-page">
        {/* Header */}
        <div className="sticky top-0 z-10 flex items-center justify-between border-b border-edge bg-page/95 backdrop-blur-sm px-6 py-4">
          <h3 className="text-sm font-bold uppercase tracking-wider text-fg-2">
            Issue Detail
          </h3>
          <div className="flex items-center gap-2">
            <button
              onClick={onClose}
              className="flex items-center gap-1 rounded-lg border border-edge px-3 py-1.5 text-xs font-medium text-fg-3 transition-colors hover:text-fg"
            >
              <span className="material-symbols-outlined text-sm">close</span>
              Close
            </button>
          </div>
        </div>

        <div className="space-y-6 p-6">
          {eventLoading ? (
            <div className="space-y-4">
              {[1, 2, 3].map((i) => (
                <div
                  key={i}
                  className="h-20 animate-pulse rounded-lg bg-surface-alt"
                />
              ))}
            </div>
          ) : (
            <>
              {/* Title / Exception */}
              {exceptionMeta && (
                <div className="rounded-xl border border-edge bg-surface-alt p-4">
                  <div className="flex items-center gap-2 mb-2">
                    <span className="material-symbols-outlined text-red-400">
                      error
                    </span>
                    <span className="font-mono text-sm font-bold text-red-400">
                      {exceptionMeta.type}
                    </span>
                  </div>
                  <p className="text-sm text-fg-2">{exceptionMeta.value}</p>
                </div>
              )}

              {event && !exceptionMeta && (
                <div className="rounded-xl border border-edge bg-surface-alt p-4">
                  <h4 className="text-sm font-medium text-fg">{event.title}</h4>
                </div>
              )}

              {/* Tags */}
              {event && event.tags.length > 0 && (
                <div>
                  <h4 className="mb-2 flex items-center gap-2 text-xs font-bold uppercase tracking-wider text-fg-3">
                    <span className="material-symbols-outlined text-sm">
                      label
                    </span>
                    Tags
                  </h4>
                  <div className="flex flex-wrap gap-1.5">
                    {event.tags.map((t) => (
                      <span
                        key={t.key}
                        className="rounded border border-edge bg-surface px-2 py-0.5 font-mono text-[11px] text-fg-3"
                      >
                        {t.key}: <span className="text-fg-2">{t.value}</span>
                      </span>
                    ))}
                  </div>
                </div>
              )}

              {/* Stack Trace */}
              {stackFrames.length > 0 && (
                <div>
                  <h4 className="mb-2 flex items-center gap-2 text-xs font-bold uppercase tracking-wider text-fg-3">
                    <span className="material-symbols-outlined text-sm">
                      layers
                    </span>
                    Stack Trace
                  </h4>
                  <div className="rounded-xl border border-edge overflow-hidden">
                    {stackFrames.map((frame, idx) => (
                      <div
                        key={idx}
                        className={`border-b border-edge px-4 py-2.5 last:border-b-0 ${
                          frame.inApp ? "bg-surface-alt" : "bg-surface/30"
                        }`}
                      >
                        <div className="flex items-center gap-2">
                          {frame.inApp && (
                            <span className="rounded border border-accent/30 bg-accent/10 px-1 py-0.5 text-[9px] font-bold uppercase text-accent">
                              app
                            </span>
                          )}
                          <span className="font-mono text-xs text-fg-2">
                            {frame.function ?? "<anonymous>"}
                          </span>
                        </div>
                        <div className="mt-0.5 font-mono text-[11px] text-fg-4">
                          {frame.filename}
                          {frame.lineNo != null && `:${frame.lineNo}`}
                          {frame.colNo != null && `:${frame.colNo}`}
                        </div>
                        {frame.context && frame.context.length > 0 && (
                          <pre className="mt-1.5 overflow-x-auto rounded border border-edge bg-page p-2 font-mono text-[11px] text-fg-3">
                            {frame.context.map(([lineNo, code]) => (
                              <div
                                key={lineNo}
                                className={
                                  lineNo === frame.lineNo
                                    ? "text-fg font-medium bg-accent/10 -mx-2 px-2"
                                    : ""
                                }
                              >
                                <span className="inline-block w-8 text-right text-fg-4 mr-3 select-none">
                                  {lineNo}
                                </span>
                                {code}
                              </div>
                            ))}
                          </pre>
                        )}
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {/* Breadcrumbs */}
              {breadcrumbs.length > 0 && (
                <div>
                  <h4 className="mb-2 flex items-center gap-2 text-xs font-bold uppercase tracking-wider text-fg-3">
                    <span className="material-symbols-outlined text-sm">
                      timeline
                    </span>
                    Breadcrumbs
                  </h4>
                  <div className="rounded-xl border border-edge overflow-hidden">
                    {breadcrumbs.map((bc, idx) => (
                      <div
                        key={idx}
                        className="flex items-start gap-3 border-b border-edge px-4 py-2 last:border-b-0"
                      >
                        <span
                          className={`mt-0.5 h-2 w-2 shrink-0 rounded-full ${
                            bc.level === "error"
                              ? "bg-red-500"
                              : bc.level === "warning"
                                ? "bg-yellow-500"
                                : "bg-fg-4/50"
                          }`}
                        />
                        <div className="min-w-0 flex-1">
                          <div className="flex items-center gap-2">
                            <span className="font-mono text-[11px] font-medium text-fg-3">
                              {bc.category ?? bc.type ?? "default"}
                            </span>
                            {bc.timestamp && (
                              <span className="text-[10px] text-fg-4">
                                {new Date(bc.timestamp).toLocaleTimeString()}
                              </span>
                            )}
                          </div>
                          {bc.message && (
                            <p className="mt-0.5 truncate font-mono text-[11px] text-fg-4">
                              {bc.message}
                            </p>
                          )}
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {/* User */}
              {event?.user && (
                <div>
                  <h4 className="mb-2 flex items-center gap-2 text-xs font-bold uppercase tracking-wider text-fg-3">
                    <span className="material-symbols-outlined text-sm">
                      person
                    </span>
                    User
                  </h4>
                  <div className="rounded-lg border border-edge bg-surface-alt px-4 py-3 font-mono text-xs text-fg-3">
                    {event.user.email && (
                      <div>
                        Email:{" "}
                        <span className="text-fg-2">{event.user.email}</span>
                      </div>
                    )}
                    {event.user.id && (
                      <div>
                        ID: <span className="text-fg-2">{event.user.id}</span>
                      </div>
                    )}
                    {event.user.ip_address && (
                      <div>
                        IP:{" "}
                        <span className="text-fg-2">
                          {event.user.ip_address}
                        </span>
                      </div>
                    )}
                  </div>
                </div>
              )}

              {/* Fix button — pass a synthetic issue for the callback */}
              <button
                onClick={() =>
                  onFix({
                    id: issueId,
                    shortId: "",
                    title: event?.title ?? "",
                    culprit: "",
                    count: "0",
                    userCount: 0,
                    firstSeen: "",
                    lastSeen: "",
                    level: "error",
                    status: "unresolved",
                    platform: "",
                    metadata: {},
                  })
                }
                className="flex w-full items-center justify-center gap-2 rounded-lg bg-accent px-5 py-3 text-sm font-bold text-page transition-all hover:bg-accent-hover"
              >
                <span className="material-symbols-outlined text-lg">build</span>
                Fix this issue
              </button>
            </>
          )}
        </div>
      </div>
    </div>
  );
}

// ─── Fix Progress Section ───────────────────────────────

function FixProgressSection({
  fixingIds,
  issues,
}: {
  fixingIds: Map<string, string>;
  issues: SentryIssue[];
}) {
  const entries = Array.from(fixingIds.entries());

  return (
    <div className="rounded-xl border border-edge bg-surface/50 p-4">
      <h4 className="mb-3 flex items-center gap-2 text-xs font-bold uppercase tracking-wider text-fg-2">
        <span className="material-symbols-outlined text-accent text-sm">
          build
        </span>
        Fixing {entries.length} issue{entries.length !== 1 && "s"}
      </h4>
      <div className="space-y-2">
        {entries.map(([issueId, runId]) => {
          const issue = issues.find((i) => i.id === issueId);
          return (
            <FixProgressRow
              key={issueId}
              issueId={issueId}
              shortId={issue?.shortId ?? issueId.slice(0, 8)}
              title={issue?.title ?? "Unknown"}
              runId={runId}
            />
          );
        })}
      </div>
    </div>
  );
}

function FixProgressRow({
  shortId,
  title,
  runId,
}: {
  issueId: string;
  shortId: string;
  title: string;
  runId: string;
}) {
  const stream = useWorkflowRunStream(runId);

  const currentStep = useMemo(() => {
    for (let i = stream.events.length - 1; i >= 0; i--) {
      const ev = stream.events[i]!;
      if (ev.event === "step_update") {
        const data = ev.data as { step_name?: string; status?: string };
        return data.step_name;
      }
    }
    return null;
  }, [stream.events]);

  const statusIcon =
    stream.runStatus === "completed"
      ? "check_circle"
      : stream.runStatus === "failed"
        ? "error"
        : "progress_activity";

  const statusColor =
    stream.runStatus === "completed"
      ? "text-accent"
      : stream.runStatus === "failed"
        ? "text-red-400"
        : "text-yellow-400 animate-spin";

  return (
    <div className="flex items-center gap-3 rounded-lg border border-edge bg-surface-alt px-3 py-2">
      <span className={`material-symbols-outlined text-base ${statusColor}`}>
        {statusIcon}
      </span>
      <span className="shrink-0 font-mono text-[11px] text-fg-4">
        {shortId}
      </span>
      <span className="min-w-0 truncate text-xs text-fg-2">{title}</span>
      <span className="ml-auto shrink-0 font-mono text-[10px] text-fg-4">
        {stream.runStatus === "completed"
          ? "done"
          : stream.runStatus === "failed"
            ? (stream.error ?? "failed")
            : (currentStep ?? "starting")}
      </span>
    </div>
  );
}

// ─── Run History ────────────────────────────────────────

const FALLBACK_STYLE = { icon: "schedule", color: "text-fg-4" };
const RUN_STATUS_STYLES: Record<string, { icon: string; color: string }> = {
  completed: { icon: "check_circle", color: "text-accent" },
  failed: { icon: "error", color: "text-red-400" },
  running: { icon: "progress_activity", color: "text-yellow-400" },
  pending: FALLBACK_STYLE,
};

function RunHistorySection({ runs }: { runs: WorkflowRun[] }) {
  return (
    <div>
      <h4 className="mb-3 flex items-center gap-2 text-xs font-bold uppercase tracking-wider text-fg-3">
        <span className="material-symbols-outlined text-sm">history</span>
        Recent Sentry Fixes
      </h4>
      <div className="rounded-xl border border-edge overflow-hidden">
        {runs.map((run) => {
          const style = RUN_STATUS_STYLES[run.status] ?? FALLBACK_STYLE;
          return (
            <a
              key={run.id}
              href={`/workflows/runs/${run.id}`}
              className="flex items-center gap-3 border-b border-edge px-4 py-2.5 transition-colors hover:bg-surface-alt/50 last:border-b-0"
            >
              <span
                className={`material-symbols-outlined text-base ${style.color} ${
                  run.status === "running" ? "animate-spin" : ""
                }`}
              >
                {style.icon}
              </span>
              <span className="font-mono text-[11px] text-fg-4">
                {run.id.slice(0, 8)}
              </span>
              <span className="min-w-0 truncate text-xs text-fg-3">
                {run.params?.issue_id
                  ? `Issue ${run.params.issue_id}`
                  : "sentry-fixer"}
              </span>
              <span className="ml-auto shrink-0 text-[10px] text-fg-4">
                {relativeTime(run.created_at)}
              </span>
            </a>
          );
        })}
      </div>
    </div>
  );
}

import { useState, useMemo } from "react";
import { useNavigate } from "react-router";
import {
  Building2,
  CircleCheck,
  FolderGit2,
  KeyRound,
  Loader2,
  Play,
} from "lucide-react";
import { useKeys } from "../hooks/useKeys";
import { useRepositories } from "../hooks/useRepositories";
import {
  useSentryOrganizations,
  useSentryProjects,
  useSentryIssues,
} from "../hooks/useSentry";
import { useRunWorkflow } from "../hooks/useWorkflowMutations";
import Select from "./Select";
import type { SentryConfig } from "../types";

function loadConfig(): SentryConfig | null {
  try {
    const raw = localStorage.getItem("sentry-configs");
    if (raw) {
      const configs = JSON.parse(raw) as SentryConfig[];
      const activeIdx =
        parseInt(localStorage.getItem("sentry-active-config") ?? "0", 10) || 0;
      const cfg = configs[activeIdx] ?? configs[0];
      if (cfg?.key_name && cfg.org_slug && cfg.project_slug && cfg.repo_url)
        return cfg;
    }
    const oldRaw = localStorage.getItem("sentry-config");
    if (oldRaw) {
      const cfg = JSON.parse(oldRaw) as SentryConfig;
      if (cfg.key_name && cfg.org_slug && cfg.project_slug && cfg.repo_url)
        return cfg;
    }
    return null;
  } catch {
    return null;
  }
}

/* Sentry severity → semantic status tokens */
const LEVEL_COLORS: Record<string, string> = {
  fatal: "bg-danger",
  error: "bg-danger",
  warning: "bg-warn",
  info: "bg-info",
};

export default function SentryFixerRunForm() {
  const navigate = useNavigate();
  const { data: allKeys } = useKeys();
  const config = loadConfig();
  const runWorkflow = useRunWorkflow();

  // ── Sentry key ──
  const sentryKeys = useMemo(
    () => allKeys?.filter((k) => k.provider === "sentry") ?? [],
    [allKeys],
  );
  const [sentryKey, setSentryKey] = useState(config?.key_name ?? "");
  const effectiveKey =
    sentryKey || (sentryKeys.length === 1 ? sentryKeys[0]!.name : "");

  // ── Organizations ──
  const { data: orgs, isLoading: orgsLoading } = useSentryOrganizations(
    effectiveKey || undefined,
  );
  const [orgSlug, setOrgSlug] = useState("");
  const [orgRegion, setOrgRegion] = useState("");
  const effectiveOrg = orgSlug || (orgs?.length === 1 ? orgs[0]!.slug : "");
  const effectiveRegion =
    orgRegion ||
    (orgs?.length === 1
      ? orgs[0]!.region
      : (orgs?.find((o) => o.slug === orgSlug)?.region ?? ""));

  // ── Projects ──
  const { data: sentryProjects, isLoading: projectsLoading } =
    useSentryProjects(
      effectiveKey || undefined,
      effectiveOrg || undefined,
      effectiveRegion || undefined,
    );
  const [projectSlug, setProjectSlug] = useState(config?.project_slug ?? "");

  // ── Git provider + repo ──
  const gitKeys = useMemo(
    () =>
      allKeys?.filter(
        (k) => k.provider === "github" || k.provider === "gitlab",
      ) ?? [],
    [allKeys],
  );
  const [gitKey, setGitKey] = useState(config?.provider_key ?? "");
  const effectiveGitKey =
    gitKey || (gitKeys.length === 1 ? gitKeys[0]!.name : "");
  const { data: repos, isLoading: reposLoading } = useRepositories(
    effectiveGitKey || undefined,
  );
  const [repoUrl, setRepoUrl] = useState(config?.repo_url ?? "");

  // ── Max issues ──
  const [maxIssues, setMaxIssues] = useState("5");

  // ── Issues preview (readonly) ──
  const { data: issues, isLoading: issuesLoading } = useSentryIssues(
    effectiveKey || undefined,
    effectiveOrg || undefined,
    projectSlug || undefined,
    {
      query: "is:unresolved",
      sort: "freq",
      region: effectiveRegion || undefined,
    },
  );

  const configComplete = !!(
    effectiveKey &&
    effectiveOrg &&
    projectSlug &&
    repoUrl &&
    effectiveGitKey
  );

  async function handleRun() {
    if (!configComplete) return;
    const run = await runWorkflow.mutateAsync({
      name: "sentry-fixer",
      params: {
        sentry_org: effectiveOrg,
        sentry_project: projectSlug,
        repo_url: repoUrl,
        key_name: effectiveKey,
        provider_key: effectiveGitKey,
        max_issues: maxIssues,
      },
    });
    void navigate(`/sessions/${run.session_id}`);
  }

  // Cascade step
  const step = !effectiveKey
    ? 0
    : !effectiveOrg
      ? 1
      : !projectSlug
        ? 2
        : !repoUrl
          ? 3
          : 4;

  return (
    <div className="overflow-hidden rounded-md border border-edge bg-surface">
      <div className="border-b border-edge px-5 py-3.5">
        <span className="eyebrow">Sentry fixer</span>
      </div>

      <div className="space-y-5 p-5">
        {/* ── Sentry key ── */}
        <div>
          <label className="mb-2 block text-sm font-medium text-fg-2">
            Sentry key
          </label>
          {sentryKeys.length > 0 ? (
            sentryKeys.length === 1 ? (
              <div className="flex items-center gap-2 rounded-md border border-accent-muted bg-accent-soft px-3 py-2.5 font-mono text-sm text-accent">
                <KeyRound className="size-4 shrink-0" />
                {sentryKeys[0]!.name}
              </div>
            ) : (
              <Select
                value={effectiveKey}
                onChange={(v) => {
                  setSentryKey(v);
                  setOrgSlug("");
                  setOrgRegion("");
                  setProjectSlug("");
                }}
                placeholder="Select a Sentry key…"
                options={sentryKeys.map((k) => ({
                  value: k.name,
                  label: k.name,
                }))}
              />
            )
          ) : (
            <p className="py-2 text-xs text-fg-4">
              No Sentry keys configured. Add one in Settings.
            </p>
          )}
        </div>

        {/* ── Organization ── */}
        {step >= 1 && (
          <div>
            <label className="mb-2 block text-sm font-medium text-fg-2">
              Organization
            </label>
            {orgsLoading ? (
              <div className="flex items-center gap-2 py-2.5 text-xs text-fg-4">
                <Loader2 className="size-3.5 animate-spin" />
                Loading organizations…
              </div>
            ) : orgs && orgs.length === 1 ? (
              <div className="flex items-center gap-2 rounded-md border border-accent-muted bg-accent-soft px-3 py-2.5 font-mono text-sm text-accent">
                <Building2 className="size-4 shrink-0" />
                {orgs[0]!.name}
                <span className="text-xs text-accent/60">
                  ({orgs[0]!.slug})
                </span>
              </div>
            ) : orgs && orgs.length > 1 ? (
              <Select
                value={effectiveOrg}
                onChange={(v) => {
                  setOrgSlug(v);
                  setOrgRegion(orgs.find((o) => o.slug === v)?.region ?? "");
                  setProjectSlug("");
                }}
                placeholder="Select an organization…"
                options={orgs.map((o) => ({
                  value: o.slug,
                  label: `${o.name} (${o.slug})${o.region !== "us" ? ` [${o.region.toUpperCase()}]` : ""}`,
                }))}
              />
            ) : (
              <p className="py-2 text-xs text-fg-4">
                No organizations found for this key.
              </p>
            )}
          </div>
        )}

        {/* ── Project ── */}
        {step >= 2 && (
          <div>
            <label className="mb-2 block text-sm font-medium text-fg-2">
              Project
            </label>
            {projectsLoading ? (
              <div className="flex items-center gap-2 py-2.5 text-xs text-fg-4">
                <Loader2 className="size-3.5 animate-spin" />
                Loading projects…
              </div>
            ) : sentryProjects && sentryProjects.length > 0 ? (
              <Select
                value={projectSlug}
                onChange={setProjectSlug}
                placeholder="Select a project…"
                options={sentryProjects.map((p) => ({
                  value: p.slug,
                  label: `${p.name}${p.platform ? ` (${p.platform})` : ""}`,
                }))}
              />
            ) : (
              <p className="py-2 text-xs text-fg-4">
                No projects found in this organization.
              </p>
            )}
          </div>
        )}

        {/* ── Git provider + repo ── */}
        {step >= 3 && (
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="mb-2 block text-sm font-medium text-fg-2">
                Git provider
              </label>
              {gitKeys.length > 0 ? (
                gitKeys.length === 1 ? (
                  <div className="flex items-center gap-2 rounded-md border border-accent-muted bg-accent-soft px-3 py-2.5 font-mono text-sm text-accent">
                    <FolderGit2 className="size-4 shrink-0" />
                    {gitKeys[0]!.name}
                  </div>
                ) : (
                  <Select
                    value={effectiveGitKey}
                    onChange={(v) => {
                      setGitKey(v);
                      setRepoUrl("");
                    }}
                    placeholder="Select a git key…"
                    options={gitKeys.map((k) => ({
                      value: k.name,
                      label: `${k.name} (${k.provider})`,
                    }))}
                  />
                )
              ) : (
                <p className="py-2 text-xs text-fg-4">
                  No GitHub or GitLab keys
                </p>
              )}
            </div>
            <div>
              <label className="mb-2 block text-sm font-medium text-fg-2">
                Repository
              </label>
              {reposLoading && effectiveGitKey ? (
                <div className="flex items-center gap-2 py-2.5 text-xs text-fg-4">
                  <Loader2 className="size-3.5 animate-spin" />
                  Loading…
                </div>
              ) : repos && repos.length > 0 ? (
                <Select
                  value={repoUrl}
                  onChange={setRepoUrl}
                  placeholder="Select a repository…"
                  options={repos.map((r) => ({
                    value: r.clone_url,
                    label: r.full_name,
                  }))}
                />
              ) : (
                <p className="py-2 text-xs text-fg-4">No repositories found.</p>
              )}
            </div>
          </div>
        )}

        {/* ── Max issues ── */}
        {step >= 4 && (
          <div>
            <label className="mb-2 block text-sm font-medium text-fg-2">
              Max issues to fix
            </label>
            <div className="flex items-center gap-4">
              <input
                type="range"
                min={1}
                max={20}
                value={maxIssues}
                onChange={(e) => setMaxIssues(e.target.value)}
                className="flex-1 accent-accent"
              />
              <span className="w-8 text-center font-mono text-sm font-semibold text-accent">
                {maxIssues}
              </span>
            </div>
            <p className="mt-1.5 text-xs text-fg-4">
              Claude will process the top {maxIssues} most critical issues by
              severity and frequency.
            </p>
          </div>
        )}

        {/* ── Issues preview (readonly) ── */}
        {step >= 4 && (
          <>
            <div>
              <div className="mb-2 flex items-center justify-between">
                <label className="text-sm font-medium text-fg-2">
                  Unresolved issues
                </label>
                {issues && (
                  <span className="font-mono text-[10px] text-fg-4">
                    {issues.length} loaded
                  </span>
                )}
              </div>
              {issuesLoading ? (
                <div className="space-y-1.5">
                  {[1, 2, 3].map((i) => (
                    <div
                      key={i}
                      className="animate-soft-pulse h-9 rounded-md bg-surface-alt"
                    />
                  ))}
                </div>
              ) : issues && issues.length > 0 ? (
                <div className="max-h-56 divide-y divide-edge overflow-y-auto rounded-md border border-edge">
                  {issues.map((issue) => (
                    <div
                      key={issue.id}
                      className="flex items-center gap-3 px-3 py-2"
                    >
                      <span
                        className={`size-2 shrink-0 rounded-full ${LEVEL_COLORS[issue.level] ?? "bg-fg-4"}`}
                      />
                      <span className="shrink-0 font-mono text-[10px] text-fg-4">
                        {issue.shortId}
                      </span>
                      <span className="min-w-0 flex-1 truncate font-mono text-xs text-fg">
                        {issue.title}
                      </span>
                      <span className="shrink-0 rounded-[4px] border border-edge bg-surface-alt px-1.5 py-0.5 font-mono text-[10px] text-fg-4">
                        {issue.count}x
                      </span>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="flex items-center justify-center gap-2 py-6 text-xs text-fg-4">
                  <CircleCheck className="size-4 text-ok" />
                  No unresolved issues
                </div>
              )}
            </div>

            {/* ── Run button ── */}
            <div className="flex flex-wrap items-center gap-4">
              <button
                onClick={() => void handleRun()}
                disabled={
                  !configComplete || runWorkflow.isPending || !issues?.length
                }
                className="flex items-center gap-2 rounded-md bg-accent px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-accent-hover disabled:opacity-50"
              >
                {runWorkflow.isPending ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <Play className="size-4" />
                )}
                Fix top {maxIssues} issues
              </button>
              {issues && issues.length > 0 && (
                <span className="text-xs text-fg-4">
                  {issues.length} unresolved — will process up to {maxIssues}
                </span>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  );
}

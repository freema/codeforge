import { useState, useMemo } from "react";
import { useNavigate } from "react-router";
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

const LEVEL_COLORS: Record<string, string> = {
  fatal: "bg-red-500",
  error: "bg-orange-500",
  warning: "bg-yellow-500",
  info: "bg-blue-500",
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
      : orgs?.find((o) => o.slug === orgSlug)?.region ?? "");

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
    void navigate(`/workflows/runs/${run.id}`);
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
    <div className="rounded-xl border border-edge bg-surface/50 p-6 space-y-5">
      <h3 className="flex items-center gap-2 text-sm font-bold uppercase tracking-wider text-fg-2">
        <span className="material-symbols-outlined text-accent text-base">
          bug_report
        </span>
        Sentry Fixer
      </h3>

      {/* ── Sentry Key ── */}
      <div>
        <label className="mb-1.5 block text-xs font-medium text-fg-3">
          Sentry Key
        </label>
        {sentryKeys.length > 0 ? (
          sentryKeys.length === 1 ? (
            <div className="flex items-center gap-2 rounded-lg border border-accent/30 bg-accent/5 px-3 py-2.5 text-sm font-mono text-accent">
              <span className="material-symbols-outlined text-base">
                vpn_key
              </span>
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
              placeholder="Select sentry key..."
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
          <label className="mb-1.5 block text-xs font-medium text-fg-3">
            Organization
          </label>
          {orgsLoading ? (
            <div className="flex items-center gap-2 py-2.5 text-xs text-fg-4">
              <span className="material-symbols-outlined animate-spin text-sm">
                progress_activity
              </span>
              Loading organizations...
            </div>
          ) : orgs && orgs.length === 1 ? (
            <div className="flex items-center gap-2 rounded-lg border border-accent/30 bg-accent/5 px-3 py-2.5 text-sm font-mono text-accent">
              <span className="material-symbols-outlined text-base">
                corporate_fare
              </span>
              {orgs[0]!.name}
              <span className="text-accent/50 text-xs">({orgs[0]!.slug})</span>
            </div>
          ) : orgs && orgs.length > 1 ? (
            <Select
              value={effectiveOrg}
              onChange={(v) => {
                setOrgSlug(v);
                setOrgRegion(orgs.find((o) => o.slug === v)?.region ?? "");
                setProjectSlug("");
              }}
              placeholder="Select organization..."
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
          <label className="mb-1.5 block text-xs font-medium text-fg-3">
            Project
          </label>
          {projectsLoading ? (
            <div className="flex items-center gap-2 py-2.5 text-xs text-fg-4">
              <span className="material-symbols-outlined animate-spin text-sm">
                progress_activity
              </span>
              Loading projects...
            </div>
          ) : sentryProjects && sentryProjects.length > 0 ? (
            <Select
              value={projectSlug}
              onChange={setProjectSlug}
              placeholder="Select project..."
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

      {/* ── Git Provider + Repo ── */}
      {step >= 3 && (
        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="mb-1.5 block text-xs font-medium text-fg-3">
              Git Provider
            </label>
            {gitKeys.length > 0 ? (
              gitKeys.length === 1 ? (
                <div className="flex items-center gap-2 rounded-lg border border-accent/30 bg-accent/5 px-3 py-2.5 text-sm font-mono text-accent">
                  <span className="material-symbols-outlined text-base">
                    code
                  </span>
                  {gitKeys[0]!.name}
                </div>
              ) : (
                <Select
                  value={effectiveGitKey}
                  onChange={(v) => {
                    setGitKey(v);
                    setRepoUrl("");
                  }}
                  placeholder="Select git key..."
                  options={gitKeys.map((k) => ({
                    value: k.name,
                    label: `${k.name} (${k.provider})`,
                  }))}
                />
              )
            ) : (
              <p className="py-2 text-xs text-fg-4">No GitHub/GitLab keys</p>
            )}
          </div>
          <div>
            <label className="mb-1.5 block text-xs font-medium text-fg-3">
              Repository
            </label>
            {reposLoading && effectiveGitKey ? (
              <div className="flex items-center gap-2 py-2.5 text-xs text-fg-4">
                <span className="material-symbols-outlined animate-spin text-sm">
                  progress_activity
                </span>
                Loading...
              </div>
            ) : repos && repos.length > 0 ? (
              <Select
                value={repoUrl}
                onChange={setRepoUrl}
                placeholder="Select repository..."
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

      {/* ── Max Issues ── */}
      {step >= 4 && (
        <div>
          <label className="mb-1.5 block text-xs font-medium text-fg-3">
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
            <span className="w-8 text-center font-mono text-sm font-bold text-accent">
              {maxIssues}
            </span>
          </div>
          <p className="mt-1 text-[10px] text-fg-4">
            Claude will process the top {maxIssues} most critical issues by severity and frequency.
          </p>
        </div>
      )}

      {/* ── Issues Preview (readonly) ── */}
      {step >= 4 && (
        <>
          <div>
            <div className="mb-2 flex items-center justify-between">
              <label className="text-xs font-medium text-fg-3">
                Unresolved Issues
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
                    className="h-9 animate-pulse rounded-lg bg-surface-alt"
                  />
                ))}
              </div>
            ) : issues && issues.length > 0 ? (
              <div className="max-h-56 overflow-y-auto rounded-lg border border-edge divide-y divide-edge">
                {issues.map((issue) => (
                  <div
                    key={issue.id}
                    className="flex items-center gap-3 px-3 py-2"
                  >
                    <div
                      className={`h-2 w-2 shrink-0 rounded-full ${LEVEL_COLORS[issue.level] ?? "bg-gray-500"}`}
                    />
                    <span className="shrink-0 font-mono text-[10px] text-fg-4">
                      {issue.shortId}
                    </span>
                    <span className="min-w-0 flex-1 truncate text-xs text-fg">
                      {issue.title}
                    </span>
                    <span className="shrink-0 rounded border border-edge bg-surface px-1.5 py-0.5 font-mono text-[10px] text-fg-4">
                      {issue.count}x
                    </span>
                  </div>
                ))}
              </div>
            ) : (
              <div className="flex items-center justify-center gap-2 py-6 text-xs text-fg-4">
                <span className="material-symbols-outlined text-base">
                  check_circle
                </span>
                No unresolved issues
              </div>
            )}
          </div>

          {/* ── Run Button ── */}
          <div className="flex items-center gap-4">
            <button
              onClick={() => void handleRun()}
              disabled={
                !configComplete || runWorkflow.isPending || !issues?.length
              }
              className="flex items-center gap-2 rounded-lg bg-accent px-6 py-2.5 text-sm font-bold text-page shadow-[0_0_15px_rgba(0,255,64,0.3)] transition-all hover:bg-accent-hover disabled:opacity-50"
            >
              {runWorkflow.isPending ? (
                <span className="material-symbols-outlined animate-spin text-base">
                  progress_activity
                </span>
              ) : (
                <span className="material-symbols-outlined text-lg">
                  play_arrow
                </span>
              )}
              Fix Top {maxIssues} Issues
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
  );
}

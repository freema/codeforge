import { useState, useMemo, type FormEvent } from "react";
import { useNavigate } from "react-router";
import { usePageTitle } from "../hooks/usePageTitle";
import { useWorkflows } from "../hooks/useWorkflows";
import { useKeys } from "../hooks/useKeys";
import { useRepositories } from "../hooks/useRepositories";
import {
  useSentryOrganizations,
  useSentryProjects,
} from "../hooks/useSentry";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../hooks/useApi";
import { useToast } from "../context/ToastContext";
import Select from "../components/Select";

const workflowIcons: Record<string, string> = {
  "sentry-fixer": "bug_report",
  "github-issue-fixer": "code",
  "gitlab-issue-fixer": "code",
  "knowledge-update": "menu_book",
};

const inputCls =
  "w-full rounded-lg border border-edge bg-surface px-3 py-2.5 text-sm text-fg font-mono placeholder-fg-4 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent transition-colors";

export default function WorkflowCreate() {
  usePageTitle("Create Workflow");
  const navigate = useNavigate();
  const { toast } = useToast();
  const api = useApi();
  const qc = useQueryClient();
  const { data: templates = [] } = useWorkflows();
  const { data: allKeys } = useKeys();

  const builtinTemplates = useMemo(
    () => templates.filter((t) => t.builtin),
    [templates],
  );

  const [selectedTemplate, setSelectedTemplate] = useState<string | null>(null);
  const [params, setParams] = useState<Record<string, string>>({});
  const [customName, setCustomName] = useState("");
  const [timeoutMinutes, setTimeoutMinutes] = useState(15);
  const [error, setError] = useState("");

  const template = builtinTemplates.find((t) => t.name === selectedTemplate);

  // Git keys for repo selection
  const gitKeys = useMemo(
    () =>
      allKeys?.filter(
        (k) => k.provider === "github" || k.provider === "gitlab",
      ) ?? [],
    [allKeys],
  );
  const selectedGitKey = params.provider_key || gitKeys?.[0]?.name || "";
  const { data: repos } = useRepositories(selectedGitKey || undefined);

  // Sentry cascade (for sentry-fixer template)
  const sentryKeys = useMemo(
    () => allKeys?.filter((k) => k.provider === "sentry") ?? [],
    [allKeys],
  );
  const effectiveSentryKey = params.key_name || (sentryKeys.length === 1 ? sentryKeys[0]!.name : "");
  const { data: sentryOrgs } = useSentryOrganizations(effectiveSentryKey || undefined);
  const effectiveSentryOrg = params.sentry_org || (sentryOrgs?.length === 1 ? sentryOrgs[0]!.slug : "");
  const sentryOrgRegion = sentryOrgs?.find((o) => o.slug === effectiveSentryOrg)?.region ?? "";
  const { data: sentryProjects } = useSentryProjects(
    effectiveSentryKey || undefined,
    effectiveSentryOrg || undefined,
    sentryOrgRegion || undefined,
  );
  const isSentryFixer = selectedTemplate === "sentry-fixer";

  const createConfig = useMutation({
    mutationFn: (req: { name: string; workflow: string; params: Record<string, string>; timeout_seconds?: number }) =>
      api.createWorkflowConfig(req),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["workflowConfigs"] });
      toast("success", "Workflow created");
      void navigate("/workflows");
    },
  });

  function updateParam(key: string, value: string) {
    setParams((prev) => ({ ...prev, [key]: value }));
  }

  function generateName(): string {
    if (customName.trim()) return customName.trim();
    if (!selectedTemplate) return "";
    const parts = [selectedTemplate];
    if (params.sentry_project) parts.push(params.sentry_project);
    else if (params.repo_url) {
      const match = params.repo_url.match(/\/([^/]+?)(?:\.git)?$/);
      if (match?.[1]) parts.push(match[1]);
    }
    else if (params.issue_number) parts.push(`issue-${params.issue_number}`);
    return parts.join("-");
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");

    if (!selectedTemplate) {
      setError("Select a workflow template");
      return;
    }

    const name = generateName();
    if (!name) {
      setError("Could not generate a name — enter one manually");
      return;
    }

    // Merge in auto-selected values that may not be in params state
    const finalParams = { ...params };
    if (isSentryFixer) {
      if (!finalParams.key_name && effectiveSentryKey) {
        finalParams.key_name = effectiveSentryKey;
      }
      if (!finalParams.sentry_org && sentryOrgs?.length === 1) {
        finalParams.sentry_org = sentryOrgs[0]!.slug;
      }
    }
    if (!finalParams.provider_key && gitKeys.length === 1) {
      finalParams.provider_key = gitKeys[0]!.name;
    }

    try {
      await createConfig.mutateAsync({
        name: name.toLowerCase().replace(/[^a-z0-9-]/g, "-"),
        workflow: selectedTemplate,
        params: finalParams,
        timeout_seconds: timeoutMinutes * 60,
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create workflow");
    }
  }

  // Step 1: Select template
  if (!selectedTemplate) {
    return (
      <div className="mx-auto max-w-3xl space-y-6">
        <div className="flex items-center gap-3">
          <button
            onClick={() => void navigate("/workflows")}
            className="flex items-center rounded-lg border border-edge p-2 text-fg-3 transition-colors hover:border-accent/30 hover:text-accent"
          >
            <span className="material-symbols-outlined text-lg">arrow_back</span>
          </button>
          <div>
            <h1 className="text-2xl font-bold tracking-tight text-fg">
              Create Workflow
            </h1>
            <p className="mt-0.5 text-sm text-fg-3">
              Pick a template to get started
            </p>
          </div>
        </div>

        <div className="grid gap-3">
          {builtinTemplates.map((t) => {
            const icon = workflowIcons[t.name] || "account_tree";
            return (
              <button
                key={t.name}
                type="button"
                onClick={() => {
                  setSelectedTemplate(t.name);
                  // Pre-fill defaults
                  const defaults: Record<string, string> = {};
                  for (const p of t.parameters) {
                    if (p.default) defaults[p.name] = p.default;
                  }
                  setParams(defaults);
                }}
                className="group flex items-start gap-4 rounded-xl border border-edge bg-surface-alt p-5 text-left transition-all hover:border-accent/40 hover:bg-accent/5"
              >
                <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-lg bg-surface border border-edge group-hover:border-accent/30">
                  <span className="material-symbols-outlined text-2xl text-fg-3 group-hover:text-accent">
                    {icon}
                  </span>
                </div>
                <div>
                  <p className="text-sm font-bold text-fg group-hover:text-accent">
                    {t.name}
                  </p>
                  <p className="mt-0.5 text-xs text-fg-4">{t.description}</p>
                  <div className="mt-2 flex items-center gap-2">
                    <span className="rounded-full border border-edge px-2 py-0.5 text-[10px] font-bold text-fg-4">
                      {t.steps.length} steps
                    </span>
                    <span className="rounded-full border border-edge px-2 py-0.5 text-[10px] font-bold text-fg-4">
                      {t.parameters.filter((p) => p.required).length} required params
                    </span>
                  </div>
                </div>
                <span className="material-symbols-outlined ml-auto mt-1 text-lg text-fg-4 group-hover:text-accent">
                  chevron_right
                </span>
              </button>
            );
          })}
        </div>
      </div>
    );
  }

  // Step 2: Configure parameters
  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <div className="flex items-center gap-3">
        <button
          onClick={() => {
            setSelectedTemplate(null);
            setParams({});
            setError("");
          }}
          className="flex items-center rounded-lg border border-edge p-2 text-fg-3 transition-colors hover:border-accent/30 hover:text-accent"
        >
          <span className="material-symbols-outlined text-lg">arrow_back</span>
        </button>
        <div>
          <h1 className="text-2xl font-bold tracking-tight text-fg">
            Configure Workflow
          </h1>
          <p className="mt-0.5 text-sm text-fg-3">
            Template:{" "}
            <span className="font-mono text-accent">{selectedTemplate}</span>
          </p>
        </div>
      </div>

      <form onSubmit={(e) => void handleSubmit(e)} className="space-y-6">
        {/* Name (optional override) */}
        <section className="rounded-xl border border-edge bg-surface/50 p-6 space-y-4">
          <h3 className="flex items-center gap-2 text-sm font-bold uppercase tracking-wider text-fg-2">
            <span className="material-symbols-outlined text-accent text-base">label</span>
            Name
          </h3>
          <input
            type="text"
            value={customName}
            onChange={(e) => setCustomName(e.target.value)}
            placeholder={`Auto: ${generateName() || "will be generated..."}`}
            className={inputCls}
          />
          <p className="text-[10px] text-fg-4">
            Leave empty to auto-generate from template + parameters.
          </p>
        </section>

        {/* Parameters */}
        <section className="rounded-xl border border-edge bg-surface/50 p-6 space-y-4">
          <h3 className="flex items-center gap-2 text-sm font-bold uppercase tracking-wider text-fg-2">
            <span className="material-symbols-outlined text-accent text-base">tune</span>
            Parameters
          </h3>

          {isSentryFixer ? (
            <SentryFixerParams
              params={params}
              updateParam={updateParam}
              sentryKeys={sentryKeys}
              effectiveSentryKey={effectiveSentryKey}
              sentryOrgs={sentryOrgs}
              sentryProjects={sentryProjects}
              gitKeys={gitKeys}
              repos={repos}
            />
          ) : template?.parameters.map((p) => {
            // Smart: provider_key with git keys selector
            if (p.name === "provider_key" && gitKeys.length > 0) {
              return (
                <div key={p.name}>
                  <label className="mb-1.5 block text-xs text-fg-3">
                    Provider Key
                    {p.required && <span className="ml-1 text-red-400">*</span>}
                  </label>
                  <div className="flex gap-2">
                    {gitKeys.map((k) => (
                      <button
                        key={k.name}
                        type="button"
                        onClick={() => updateParam("provider_key", k.name)}
                        className={`flex items-center gap-2 rounded-lg border px-4 py-2.5 text-sm font-medium transition-all ${
                          params.provider_key === k.name
                            ? "border-accent bg-accent/10 text-accent"
                            : "border-edge text-fg-3 hover:border-fg-4"
                        }`}
                      >
                        <span className="material-symbols-outlined text-lg">code</span>
                        {k.name}
                      </button>
                    ))}
                  </div>
                </div>
              );
            }

            // Smart: repo_url with repo selector
            if (p.name === "repo_url" && repos && repos.length > 0) {
              return (
                <div key={p.name}>
                  <label className="mb-1.5 block text-xs text-fg-3">
                    Repository
                    {p.required && <span className="ml-1 text-red-400">*</span>}
                  </label>
                  <Select
                    value={params.repo_url || ""}
                    onChange={(v) => updateParam("repo_url", v)}
                    placeholder="Select repository..."
                    options={repos.map((r) => ({
                      value: r.clone_url,
                      label: r.full_name,
                    }))}
                  />
                </div>
              );
            }

            // Smart: key_name for sentry/provider keys
            if (p.name === "key_name" && allKeys && allKeys.length > 0) {
              return (
                <div key={p.name}>
                  <label className="mb-1.5 block text-xs text-fg-3">
                    Auth Key
                    {p.required && <span className="ml-1 text-red-400">*</span>}
                  </label>
                  <Select
                    value={params.key_name || ""}
                    onChange={(v) => updateParam("key_name", v)}
                    placeholder="Select key..."
                    options={allKeys.map((k) => ({
                      value: k.name,
                      label: `${k.name} (${k.provider})`,
                    }))}
                  />
                </div>
              );
            }

            // Default: text input
            return (
              <div key={p.name}>
                <label className="mb-1.5 block text-xs text-fg-3">
                  {p.name.replace(/_/g, " ")}
                  {p.required && <span className="ml-1 text-red-400">*</span>}
                </label>
                <input
                  type="text"
                  value={params[p.name] ?? p.default ?? ""}
                  onChange={(e) => updateParam(p.name, e.target.value)}
                  placeholder={p.default ?? `Enter ${p.name}...`}
                  className={inputCls}
                />
              </div>
            );
          })}
        </section>

        {/* Timeout */}
        <section className="rounded-xl border border-edge bg-surface/50 p-6 space-y-4">
          <h3 className="flex items-center gap-2 text-sm font-bold uppercase tracking-wider text-fg-2">
            <span className="material-symbols-outlined text-accent text-base">timer</span>
            Timeout
          </h3>
          <div className="flex items-center gap-4">
            <input
              type="range"
              min={5}
              max={30}
              step={1}
              value={timeoutMinutes}
              onChange={(e) => setTimeoutMinutes(Number(e.target.value))}
              className="flex-1 accent-accent"
            />
            <span className="w-16 text-center font-mono text-sm font-bold text-accent">
              {timeoutMinutes} min
            </span>
          </div>
          <p className="text-[10px] text-fg-4">
            If the session exceeds this limit it will complete gracefully — you can still create a PR or continue with a follow-up instruction.
          </p>
        </section>

        {/* Error */}
        {error && (
          <div className="rounded-lg border border-red-900/50 bg-red-900/20 px-4 py-3 text-sm text-red-400">
            {error}
          </div>
        )}

        {/* Submit */}
        <div className="flex items-center gap-3">
          <button
            type="submit"
            disabled={createConfig.isPending}
            className="flex items-center gap-2 rounded-lg bg-accent px-6 py-2.5 text-sm font-bold text-page shadow-[0_0_15px_rgba(0,255,64,0.3)] transition-all hover:bg-accent-hover disabled:opacity-50"
          >
            {createConfig.isPending ? (
              <span className="material-symbols-outlined animate-spin text-base">progress_activity</span>
            ) : (
              <span className="material-symbols-outlined text-lg">save</span>
            )}
            Save Workflow
          </button>
          <button
            type="button"
            onClick={() => void navigate("/workflows")}
            className="rounded-lg border border-edge px-4 py-2.5 text-sm text-fg-3 transition-colors hover:text-fg-2"
          >
            Cancel
          </button>
        </div>
      </form>
    </div>
  );
}

// ── Sentry Fixer specific parameter form with cascade selectors ──

import type { ProviderKey, Repository, SentryOrganization, SentryProject } from "../types";

function SentryFixerParams({
  params,
  updateParam,
  sentryKeys,
  effectiveSentryKey,
  sentryOrgs,
  sentryProjects,
  gitKeys,
  repos,
}: {
  params: Record<string, string>;
  updateParam: (k: string, v: string) => void;
  sentryKeys: ProviderKey[];
  effectiveSentryKey: string;
  sentryOrgs: SentryOrganization[] | undefined;
  sentryProjects: SentryProject[] | undefined;
  gitKeys: ProviderKey[];
  repos: Repository[] | undefined;
}) {
  return (
    <>
      {/* Sentry Key */}
      <div>
        <label className="mb-1.5 block text-xs font-medium text-fg-3">Sentry Key</label>
        {sentryKeys.length === 1 ? (
          <div className="flex items-center gap-2 rounded-lg border border-accent/30 bg-accent/5 px-3 py-2.5 text-sm font-mono text-accent">
            <span className="material-symbols-outlined text-base">vpn_key</span>
            {sentryKeys[0]!.name}
          </div>
        ) : sentryKeys.length > 1 ? (
          <Select
            value={params.key_name || ""}
            onChange={(v) => {
              updateParam("key_name", v);
              updateParam("sentry_org", "");
              updateParam("sentry_project", "");
            }}
            placeholder="Select sentry key..."
            options={sentryKeys.map((k) => ({ value: k.name, label: k.name }))}
          />
        ) : (
          <p className="py-2 text-xs text-fg-4">No Sentry keys configured. Add one in Settings.</p>
        )}
      </div>

      {/* Organization */}
      {effectiveSentryKey && (
        <div>
          <label className="mb-1.5 block text-xs font-medium text-fg-3">Organization</label>
          {!sentryOrgs ? (
            <div className="flex items-center gap-2 py-2.5 text-xs text-fg-4">
              <span className="material-symbols-outlined animate-spin text-sm">progress_activity</span>
              Loading organizations...
            </div>
          ) : sentryOrgs.length === 1 ? (
            <div className="flex items-center gap-2 rounded-lg border border-accent/30 bg-accent/5 px-3 py-2.5 text-sm font-mono text-accent">
              <span className="material-symbols-outlined text-base">corporate_fare</span>
              {sentryOrgs[0]!.name}
              <span className="text-accent/50 text-xs">({sentryOrgs[0]!.slug})</span>
            </div>
          ) : sentryOrgs.length > 1 ? (
            <Select
              value={params.sentry_org || ""}
              onChange={(v) => {
                updateParam("sentry_org", v);
                updateParam("sentry_project", "");
              }}
              placeholder="Select organization..."
              options={sentryOrgs.map((o) => ({
                value: o.slug,
                label: `${o.name} (${o.slug})${o.region !== "us" ? ` [${o.region.toUpperCase()}]` : ""}`,
              }))}
            />
          ) : (
            <p className="py-2 text-xs text-fg-4">No organizations found.</p>
          )}
        </div>
      )}

      {/* Project */}
      {(params.sentry_org || (sentryOrgs?.length === 1)) && (
        <div>
          <label className="mb-1.5 block text-xs font-medium text-fg-3">Project</label>
          {!sentryProjects ? (
            <div className="flex items-center gap-2 py-2.5 text-xs text-fg-4">
              <span className="material-symbols-outlined animate-spin text-sm">progress_activity</span>
              Loading projects...
            </div>
          ) : sentryProjects.length > 0 ? (
            <Select
              value={params.sentry_project || ""}
              onChange={(v) => updateParam("sentry_project", v)}
              placeholder="Select project..."
              options={sentryProjects.map((p) => ({
                value: p.slug,
                label: `${p.name}${p.platform ? ` (${p.platform})` : ""}`,
              }))}
            />
          ) : (
            <p className="py-2 text-xs text-fg-4">No projects found.</p>
          )}
        </div>
      )}

      {/* Git Provider */}
      {gitKeys.length > 0 && (
        <div>
          <label className="mb-1.5 block text-xs font-medium text-fg-3">Git Provider</label>
          <div className="flex gap-2">
            {gitKeys.map((k) => (
              <button
                key={k.name}
                type="button"
                onClick={() => {
                  updateParam("provider_key", k.name);
                  updateParam("repo_url", "");
                }}
                className={`flex items-center gap-2 rounded-lg border px-4 py-2.5 text-sm font-medium transition-all ${
                  params.provider_key === k.name
                    ? "border-accent bg-accent/10 text-accent"
                    : "border-edge text-fg-3 hover:border-fg-4"
                }`}
              >
                <span className="material-symbols-outlined text-lg">code</span>
                {k.name}
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Repository */}
      {repos && repos.length > 0 && (
        <div>
          <label className="mb-1.5 block text-xs font-medium text-fg-3">Repository</label>
          <Select
            value={params.repo_url || ""}
            onChange={(v) => updateParam("repo_url", v)}
            placeholder="Select repository..."
            options={repos.map((r) => ({ value: r.clone_url, label: r.full_name }))}
          />
        </div>
      )}

      {/* Max issues */}
      <div>
        <label className="mb-1.5 block text-xs font-medium text-fg-3">Max issues to fix</label>
        <div className="flex items-center gap-4">
          <input
            type="range"
            min={1}
            max={20}
            value={params.max_issues || "5"}
            onChange={(e) => updateParam("max_issues", e.target.value)}
            className="flex-1 accent-accent"
          />
          <span className="w-8 text-center font-mono text-sm font-bold text-accent">
            {params.max_issues || "5"}
          </span>
        </div>
      </div>
    </>
  );
}

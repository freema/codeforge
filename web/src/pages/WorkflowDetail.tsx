import { useState, useMemo } from "react";
import { useParams, useNavigate } from "react-router";
import { usePageTitle } from "../hooks/usePageTitle";
import Select from "../components/Select";
import SentryFixerRunForm from "../components/SentryFixerRunForm";
import { useWorkflow } from "../hooks/useWorkflows";
import {
  useDeleteWorkflow,
  useRunWorkflow,
} from "../hooks/useWorkflowMutations";
import { useKeys } from "../hooks/useKeys";
import { useRepositories } from "../hooks/useRepositories";
import { useToast } from "../context/ToastContext";
import type { StepType } from "../types";

const stepTypeColors: Record<
  StepType,
  { color: string; bg: string; icon: string }
> = {
  fetch: {
    color: "text-cyan-400",
    bg: "bg-cyan-400/10",
    icon: "cloud_download",
  },
  session: {
    color: "text-yellow-400",
    bg: "bg-yellow-400/10",
    icon: "terminal",
  },
  action: { color: "text-purple-400", bg: "bg-purple-400/10", icon: "bolt" },
};

export default function WorkflowDetail() {
  usePageTitle("Workflow Detail");
  const { name } = useParams<{ name: string }>();
  const decodedName = name ? decodeURIComponent(name) : undefined;
  const navigate = useNavigate();
  const { toast } = useToast();
  const { data: workflow, isLoading } = useWorkflow(decodedName);
  const deleteWorkflow = useDeleteWorkflow();
  const runWorkflow = useRunWorkflow();
  const { data: allKeys } = useKeys();
  const gitKeys = useMemo(
    () =>
      allKeys?.filter(
        (k) => k.provider === "github" || k.provider === "gitlab",
      ),
    [allKeys],
  );

  const [showRun, setShowRun] = useState(false);
  const [params, setParams] = useState<Record<string, string>>({});
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [selectedKey, setSelectedKey] = useState("");

  // Smart: auto-select provider key for repo-related workflows
  const firstGitKey = gitKeys?.[0]?.name;
  const { data: repos } = useRepositories(selectedKey || firstGitKey);

  const isSentryFixer = decodedName === "sentry-fixer";

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-20">
        <span className="material-symbols-outlined animate-spin text-3xl text-accent/50">
          progress_activity
        </span>
      </div>
    );
  }

  if (!workflow) {
    return <p className="py-20 text-center text-fg-4">Workflow not found.</p>;
  }

  async function handleDelete() {
    if (!decodedName) return;
    await deleteWorkflow.mutateAsync(decodedName);
    toast("success", "Workflow deleted");
    void navigate("/workflows");
  }

  async function handleRun() {
    if (!decodedName) return;
    const allParams = { ...params };
    if (selectedKey && !allParams.provider_key) {
      allParams.provider_key = selectedKey;
    }
    const hasParams = Object.keys(allParams).length > 0;
    const run = await runWorkflow.mutateAsync({
      name: decodedName,
      params: hasParams ? allParams : undefined,
    });
    void navigate(`/sessions/${run.session_id}`);
  }

  function updateParam(key: string, value: string) {
    setParams((prev) => ({ ...prev, [key]: value }));
  }

  const inputCls =
    "w-full rounded-lg border border-edge bg-surface px-3 py-2.5 text-sm text-fg font-mono placeholder-fg-4 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent transition-colors";

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      {/* Header */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-2xl font-bold text-fg">{workflow.name}</h1>
            {workflow.builtin && (
              <span className="rounded-full border border-accent/20 bg-accent/10 px-3 py-0.5 text-xs font-bold text-accent">
                BUILT-IN
              </span>
            )}
          </div>
          {workflow.description && (
            <p className="mt-1 text-sm text-fg-3">{workflow.description}</p>
          )}
        </div>

        <div className="flex items-center gap-2">
          {!workflow.builtin && (
            <>
              {confirmDelete ? (
                <div className="flex items-center gap-2">
                  <span className="text-xs text-fg-3">Sure?</span>
                  <button
                    onClick={() => void handleDelete()}
                    disabled={deleteWorkflow.isPending}
                    className="rounded-lg border border-red-900/50 bg-red-900/20 px-3 py-1.5 text-xs font-medium text-red-400 disabled:opacity-50"
                  >
                    Delete
                  </button>
                  <button
                    onClick={() => setConfirmDelete(false)}
                    className="rounded-lg border border-edge px-3 py-1.5 text-xs text-fg-3"
                  >
                    Cancel
                  </button>
                </div>
              ) : (
                <button
                  onClick={() => setConfirmDelete(true)}
                  className="flex items-center gap-1.5 rounded-lg border border-red-900/50 bg-red-900/20 px-3 py-2 text-sm text-red-400 transition-colors hover:bg-red-900/40"
                >
                  <span className="material-symbols-outlined text-lg">
                    delete
                  </span>
                  Delete
                </button>
              )}
            </>
          )}
          <button
            onClick={() => setShowRun(!showRun)}
            className={`flex items-center gap-2 rounded-lg px-5 py-2 text-sm font-bold transition-all ${
              showRun
                ? "border border-edge bg-surface-alt text-fg-2"
                : "bg-accent text-page shadow-[0_0_15px_rgba(0,255,64,0.3)] hover:bg-accent-hover"
            }`}
          >
            <span className="material-symbols-outlined text-lg">
              {showRun ? "close" : "play_arrow"}
            </span>
            {showRun ? "Close" : "Run Workflow"}
          </button>
        </div>
      </div>

      {/* Run form -- only shown when toggled */}
      {showRun && (isSentryFixer ? <SentryFixerRunForm /> : (
        <div className="rounded-xl border border-edge bg-surface/50 p-6 space-y-4">
          <h3 className="flex items-center gap-2 text-sm font-bold uppercase tracking-wider text-fg-2">
            <span className="material-symbols-outlined text-accent text-base">
              tune
            </span>
            Run Parameters
          </h3>

          {/* Smart: Provider key selector */}
          {gitKeys &&
            gitKeys.length > 0 &&
            workflow.parameters.some(
              (p) => p.name === "provider_key" || p.name === "repo_url",
            ) && (
              <div>
                <label className="mb-2 block text-xs text-fg-3">
                  Provider Key
                </label>
                <div className="flex gap-2">
                  {gitKeys.map((k) => (
                    <button
                      key={k.name}
                      type="button"
                      onClick={() => {
                        setSelectedKey(k.name);
                        updateParam("provider_key", k.name);
                      }}
                      className={`flex items-center gap-2 rounded-lg border px-4 py-2.5 text-sm font-medium transition-all ${
                        (selectedKey || params.provider_key) === k.name
                          ? "border-accent bg-accent/10 text-accent"
                          : "border-edge text-fg-3 hover:border-fg-4"
                      }`}
                    >
                      <span className="material-symbols-outlined text-lg">
                        code
                      </span>
                      {k.name}
                    </button>
                  ))}
                </div>
              </div>
            )}

          {/* Smart: Repo URL selector if repos available */}
          {repos &&
            repos.length > 0 &&
            workflow.parameters.some((p) => p.name === "repo_url") && (
              <div>
                <label className="mb-2 block text-xs text-fg-3">
                  Repository
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
            )}

          {/* Other parameters */}
          {workflow.parameters
            .filter((p) => p.name !== "provider_key" && p.name !== "repo_url")
            .map((p) => (
              <div key={p.name}>
                <label className="mb-2 block text-xs text-fg-3">
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
            ))}

          {workflow.parameters.length === 0 && (
            <p className="text-sm text-fg-4">
              This workflow has no parameters.
            </p>
          )}

          <button
            onClick={() => void handleRun()}
            disabled={runWorkflow.isPending}
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
            Start Run
          </button>
        </div>
      ))}

      {/* Steps */}
      <div className="rounded-xl border border-edge bg-surface-alt overflow-hidden">
        <div className="flex items-center justify-between border-b border-edge bg-surface/70 px-6 py-4">
          <h3 className="flex items-center gap-2 font-bold text-fg">
            <span className="material-symbols-outlined text-sm text-accent">
              schema
            </span>
            Workflow Steps
          </h3>
          <span className="rounded-full bg-edge px-2 py-0.5 font-mono text-xs text-fg-3">
            {workflow.steps.length} step{workflow.steps.length !== 1 ? "s" : ""}
          </span>
        </div>
        <div className="p-4">
          <div className="flex flex-col gap-2">
            {workflow.steps.map((step, i) => {
              const tc = stepTypeColors[step.type] ?? {
                color: "text-fg-3",
                bg: "bg-surface",
                icon: "circle",
              };
              return (
                <div key={step.name}>
                  <div className="flex items-center gap-3 rounded-lg border border-edge bg-surface p-4 transition-colors hover:border-accent/30">
                    <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-surface-alt font-mono text-sm font-bold text-accent/60">
                      {i + 1}
                    </span>
                    <span className={`material-symbols-outlined ${tc.color}`}>
                      {tc.icon}
                    </span>
                    <div className="flex-1">
                      <span className="text-sm font-medium text-fg">
                        {step.name}
                      </span>
                    </div>
                    <span
                      className={`rounded-full border px-2.5 py-0.5 text-[10px] font-bold uppercase tracking-wider ${tc.bg} ${tc.color}`}
                      style={{
                        borderColor: "currentColor",
                        borderWidth: "1px",
                        opacity: 0.5,
                      }}
                    >
                      {step.type}
                    </span>
                  </div>
                  {i < workflow.steps.length - 1 && (
                    <div className="ml-[19px] flex h-4 items-center">
                      <div className="h-full w-px bg-edge" />
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </div>
      </div>
    </div>
  );
}

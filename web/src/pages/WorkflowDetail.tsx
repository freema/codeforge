import { useState, useMemo } from "react";
import { useParams, useNavigate, Link } from "react-router";
import { usePageTitle } from "../hooks/usePageTitle";
import Select from "../components/Select";
import SentryFixerRunForm from "../components/SentryFixerRunForm";
import { useWorkflow } from "../hooks/useWorkflows";
import { useWorkflowRuns } from "../hooks/useWorkflowRuns";
import {
  useDeleteWorkflow,
  useRunWorkflow,
  useCancelWorkflowRun,
  useCancelAllWorkflowRuns,
} from "../hooks/useWorkflowMutations";
import { useKeys } from "../hooks/useKeys";
import { useRepositories } from "../hooks/useRepositories";
import { useToast } from "../context/ToastContext";
import type { RunStatus, StepType } from "../types";

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

const runStatusColors: Record<
  RunStatus,
  { color: string; bg: string; border: string }
> = {
  pending: { color: "text-fg-3", bg: "bg-surface", border: "border-edge" },
  running: {
    color: "text-yellow-400",
    bg: "bg-yellow-400/10",
    border: "border-yellow-500/20",
  },
  completed: {
    color: "text-accent",
    bg: "bg-accent/10",
    border: "border-accent/20",
  },
  failed: {
    color: "text-red-400",
    bg: "bg-red-400/10",
    border: "border-red-500/20",
  },
  cancelled: {
    color: "text-orange-400",
    bg: "bg-orange-400/10",
    border: "border-orange-500/20",
  },
};

export default function WorkflowDetail() {
  usePageTitle("Workflow Detail");
  const { name } = useParams<{ name: string }>();
  const decodedName = name ? decodeURIComponent(name) : undefined;
  const navigate = useNavigate();
  const { toast } = useToast();
  const { data: workflow, isLoading } = useWorkflow(decodedName);
  const { data: runs = [] } = useWorkflowRuns(decodedName);
  const deleteWorkflow = useDeleteWorkflow();
  const runWorkflow = useRunWorkflow();
  const cancelRun = useCancelWorkflowRun();
  const cancelAllRuns = useCancelAllWorkflowRuns();
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
    void navigate(`/workflows/runs/${run.id}`);
  }

  function updateParam(key: string, value: string) {
    setParams((prev) => ({ ...prev, [key]: value }));
  }

  const recentRuns = [...runs]
    .sort(
      (a, b) =>
        new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
    )
    .slice(0, 10);

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

      {/* Run form — only shown when toggled */}
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

      {/* Recent Runs */}
      {recentRuns.length > 0 && (
        <div className="rounded-xl border border-edge bg-surface-alt overflow-hidden">
          <div className="flex items-center justify-between border-b border-edge bg-surface/70 px-6 py-4">
            <h3 className="flex items-center gap-2 font-bold text-fg">
              <span className="material-symbols-outlined text-sm text-accent">
                history
              </span>
              Recent Runs
            </h3>
            {recentRuns.some(
              (r) => r.status === "pending" || r.status === "running",
            ) && (
              <button
                onClick={() => {
                  cancelAllRuns.mutate(decodedName, {
                    onSuccess: (data) => toast("success", data.message),
                    onError: (err) =>
                      toast("error", `Cancel failed: ${err.message}`),
                  });
                }}
                disabled={cancelAllRuns.isPending}
                className="flex items-center gap-1.5 rounded-lg border border-red-900/50 bg-red-900/20 px-3 py-1.5 text-xs font-medium text-red-400 transition-colors hover:bg-red-900/40 disabled:opacity-50"
              >
                <span className="material-symbols-outlined text-sm">
                  stop_circle
                </span>
                {cancelAllRuns.isPending ? "Cancelling..." : "Cancel All"}
              </button>
            )}
          </div>
          <div className="divide-y divide-edge">
            {recentRuns.map((run) => {
              const sc = runStatusColors[run.status];
              const isRunActive =
                run.status === "pending" || run.status === "running";
              return (
                <div
                  key={run.id}
                  className="flex items-center gap-4 px-6 py-3 transition-colors hover:bg-white/5"
                >
                  <Link
                    to={`/workflows/runs/${run.id}`}
                    className="flex flex-1 items-center gap-4"
                  >
                    <span className="font-mono text-xs text-accent/60">
                      {run.id.slice(0, 8)}
                    </span>
                    <span
                      className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-xs font-bold uppercase ${sc.bg} ${sc.color} ${sc.border}`}
                    >
                      {run.status === "running" && (
                        <span className="size-1.5 animate-pulse rounded-full bg-current" />
                      )}
                      {run.status}
                    </span>
                    <span className="ml-auto font-mono text-xs text-fg-4">
                      {formatTimeAgo(run.created_at)}
                    </span>
                  </Link>
                  {isRunActive && (
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        cancelRun.mutate(run.id, {
                          onSuccess: () =>
                            toast("success", "Run cancellation requested"),
                          onError: (err) =>
                            toast("error", `Cancel failed: ${err.message}`),
                        });
                      }}
                      disabled={cancelRun.isPending}
                      className="flex items-center gap-1 rounded border border-red-900/50 bg-red-900/20 px-2 py-1 text-[10px] font-medium text-red-400 transition-colors hover:bg-red-900/40 disabled:opacity-50"
                    >
                      <span className="material-symbols-outlined text-xs">
                        stop
                      </span>
                      Cancel
                    </button>
                  )}
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}

function formatTimeAgo(dateStr: string): string {
  const seconds = Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000);
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

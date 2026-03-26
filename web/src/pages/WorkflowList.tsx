import { useMemo, useState } from "react";
import { useNavigate, Link, useSearchParams } from "react-router";
import { usePageTitle } from "../hooks/usePageTitle";
import { useWorkflowRuns } from "../hooks/useWorkflowRuns";
import {
  useCancelWorkflowRun,
  useCancelAllWorkflowRuns,
} from "../hooks/useWorkflowMutations";
import { useToast } from "../context/ToastContext";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../hooks/useApi";
import type { WorkflowRun, RunStatus } from "../types";

type Tab = "configs" | "runs";
const VALID_TABS = new Set<Tab>(["configs", "runs"]);

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

const workflowIcons: Record<string, string> = {
  "sentry-fixer": "bug_report",
  "github-issue-fixer": "code",
  "gitlab-issue-fixer": "code",
  "knowledge-update": "menu_book",
};

export default function WorkflowList() {
  usePageTitle("Workflows");
  const navigate = useNavigate();
  const { toast } = useToast();
  const api = useApi();
  const qc = useQueryClient();

  const { data: configs = [], isLoading: configsLoading } = useQuery({
    queryKey: ["workflowConfigs"],
    queryFn: () => api.listWorkflowConfigs(),
  });

  const {
    data: runs = [],
    isLoading: runsLoading,
  } = useWorkflowRuns();
  const cancelRun = useCancelWorkflowRun();
  const cancelAll = useCancelAllWorkflowRuns();

  const deleteConfig = useMutation({
    mutationFn: (id: number) => api.deleteWorkflowConfig(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["workflowConfigs"] }),
  });
  const runConfig = useMutation({
    mutationFn: (id: number) => api.runWorkflowConfig(id),
    onSuccess: (data) => {
      void navigate(`/workflows/runs/${data.run_id}`);
    },
  });

  const [searchParams, setSearchParams] = useSearchParams();
  const rawTab = searchParams.get("tab") as Tab | null;
  const tab: Tab = rawTab && VALID_TABS.has(rawTab) ? rawTab : "configs";

  function setTab(t: Tab) {
    setSearchParams(t === "configs" ? {} : { tab: t }, { replace: true });
  }
  const [search, setSearch] = useState("");
  const [confirmDelete, setConfirmDelete] = useState<number | null>(null);
  const [expandedConfig, setExpandedConfig] = useState<number | null>(null);

  const activeRuns = runs.filter(
    (r) => r.status === "pending" || r.status === "running",
  );

  const filteredConfigs = useMemo(() => {
    if (!search) return configs;
    const q = search.toLowerCase();
    return configs.filter(
      (c) =>
        c.name.toLowerCase().includes(q) ||
        c.workflow.toLowerCase().includes(q),
    );
  }, [configs, search]);

  const filteredRuns = useMemo(() => {
    const sorted = [...runs].sort(
      (a, b) =>
        new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
    );
    if (!search) return sorted;
    const q = search.toLowerCase();
    return sorted.filter(
      (r) =>
        r.id.toLowerCase().includes(q) ||
        r.workflow_name.toLowerCase().includes(q) ||
        r.status.toLowerCase().includes(q),
    );
  }, [runs, search]);

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      {/* Header */}
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold tracking-tight text-fg">
            Workflows
          </h1>
          <p className="mt-1 text-sm text-fg-3">
            Saved workflow configurations
          </p>
        </div>
        <div className="flex items-center gap-2">
          {tab === "runs" && activeRuns.length > 0 && (
            <button
              onClick={() => {
                cancelAll.mutate(undefined, {
                  onSuccess: (data) => toast("success", data.message),
                  onError: (err) =>
                    toast("error", `Cancel failed: ${err.message}`),
                });
              }}
              disabled={cancelAll.isPending}
              className="flex h-10 items-center gap-1.5 rounded-lg border border-red-900/50 bg-red-900/20 px-4 text-sm font-medium text-red-400 transition-colors hover:bg-red-900/40 disabled:opacity-50"
            >
              <span className="material-symbols-outlined text-lg">
                stop_circle
              </span>
              {cancelAll.isPending ? "Cancelling..." : "Cancel All"}
            </button>
          )}
          <button
            onClick={() => void navigate("/workflows/new")}
            className="flex h-10 items-center gap-2 rounded-lg bg-accent px-5 text-sm font-bold text-page shadow-[0_0_15px_rgba(0,255,64,0.3)] transition-all hover:bg-accent-hover"
          >
            <span className="material-symbols-outlined text-lg">add</span>
            Create Workflow
          </button>
        </div>
      </div>

      {/* Tabs */}
      <div className="flex gap-1 rounded-lg border border-edge bg-surface p-1">
        {[
          { key: "configs" as const, label: "My Workflows", icon: "account_tree" },
          {
            key: "runs" as const,
            label: "Runs",
            icon: "history",
            count: activeRuns.length,
          },
        ].map((t) => (
          <button
            key={t.key}
            onClick={() => {
              setTab(t.key);
              setSearch("");
            }}
            className={`flex flex-1 items-center justify-center gap-2 rounded-md px-4 py-2.5 text-sm font-medium transition-all ${
              tab === t.key
                ? "bg-accent/15 text-accent border border-accent/30"
                : "text-fg-3 border border-transparent hover:text-fg-2"
            }`}
          >
            <span className="material-symbols-outlined text-lg">{t.icon}</span>
            {t.label}
            {t.count !== undefined && t.count > 0 && (
              <span className="rounded-full bg-yellow-400/20 px-1.5 py-0.5 text-[10px] font-bold text-yellow-400">
                {t.count}
              </span>
            )}
          </button>
        ))}
      </div>

      {/* Search */}
      <div className="group relative">
        <div className="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3 text-fg-4 transition-colors group-focus-within:text-accent">
          <span className="material-symbols-outlined">search</span>
        </div>
        <input
          type="text"
          placeholder={
            tab === "configs"
              ? "> Search workflows..."
              : "> Search runs..."
          }
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="h-12 w-full rounded-lg border border-edge bg-surface-alt pl-10 pr-4 font-mono text-sm text-fg placeholder-fg-4 transition-all focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent"
        />
      </div>

      {/* Content */}
      {tab === "configs" ? (
        <>
          {!configsLoading && configs.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-20">
              <span className="material-symbols-outlined mb-4 text-5xl text-slate-700">
                account_tree
              </span>
              <p className="mb-1 text-lg font-medium text-fg-3">
                No workflows yet
              </p>
              <p className="mb-4 text-sm text-fg-4">
                Create your first workflow by picking a template and configuring it.
              </p>
              <button
                onClick={() => void navigate("/workflows/new")}
                className="flex items-center gap-2 rounded-lg bg-accent px-5 py-2.5 text-sm font-bold text-page shadow-[0_0_15px_rgba(0,255,64,0.3)] transition-all hover:bg-accent-hover"
              >
                <span className="material-symbols-outlined text-lg">add</span>
                Create Workflow
              </button>
            </div>
          ) : filteredConfigs.length === 0 ? (
            <p className="py-12 text-center text-sm text-fg-4">
              No workflows match your search.
            </p>
          ) : (
            <div className="flex flex-col gap-3">
              {filteredConfigs.map((cfg) => {
                const icon = workflowIcons[cfg.workflow] || "account_tree";
                return (
                  <div
                    key={cfg.id}
                    className="group rounded-xl border border-edge bg-surface-alt p-4 transition-colors hover:border-accent/30"
                  >
                    <div className="flex items-center justify-between gap-4">
                      <div className="flex items-center gap-3 min-w-0">
                        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-surface border border-edge">
                          <span className="material-symbols-outlined text-xl text-accent/60 group-hover:text-accent">
                            {icon}
                          </span>
                        </div>
                        <div className="min-w-0">
                          <div className="flex items-center gap-2">
                            <span className="font-medium text-fg">{cfg.name}</span>
                            <span className="rounded-full border border-edge bg-surface px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider text-fg-3">
                              {cfg.workflow}
                            </span>
                          </div>
                          <div className="mt-0.5 flex items-center gap-2 text-xs text-fg-4">
                            {Object.entries(cfg.params)
                              .filter(([, v]) => v)
                              .slice(0, 3)
                              .map(([k, v]) => (
                                <span key={k} className="font-mono">
                                  {k}={v.length > 20 ? v.slice(0, 20) + "..." : v}
                                </span>
                              ))}
                            {Object.keys(cfg.params).length > 3 && (
                              <span>+{Object.keys(cfg.params).length - 3} more</span>
                            )}
                          </div>
                        </div>
                      </div>
                      <div className="flex items-center gap-2 shrink-0">
                        <button
                          onClick={() => setExpandedConfig(expandedConfig === cfg.id ? null : cfg.id)}
                          className={`flex items-center gap-1 rounded-lg border px-3 py-2 text-xs font-medium transition-colors ${
                            expandedConfig === cfg.id
                              ? "border-accent/30 text-accent"
                              : "border-edge text-fg-3 hover:border-accent/30 hover:text-accent"
                          }`}
                        >
                          <span className="material-symbols-outlined text-sm">
                            {expandedConfig === cfg.id ? "expand_less" : "expand_more"}
                          </span>
                          Detail
                        </button>
                        <button
                          onClick={() => {
                            runConfig.mutate(cfg.id, {
                              onError: (err) =>
                                toast("error", `Run failed: ${err.message}`),
                            });
                          }}
                          disabled={runConfig.isPending}
                          className="flex items-center gap-1.5 rounded-lg bg-accent px-4 py-2 text-xs font-bold text-page transition-all hover:bg-accent-hover disabled:opacity-50"
                        >
                          <span className="material-symbols-outlined text-sm">
                            play_arrow
                          </span>
                          Run
                        </button>
                        {confirmDelete === cfg.id ? (
                          <span className="flex items-center gap-2">
                            <button
                              onClick={() => {
                                deleteConfig.mutate(cfg.id, {
                                  onSuccess: () => {
                                    toast("success", "Workflow deleted");
                                    setConfirmDelete(null);
                                  },
                                });
                              }}
                              className="rounded-lg border border-red-900/50 bg-red-900/20 px-3 py-2 text-xs font-medium text-red-400"
                            >
                              Confirm
                            </button>
                            <button
                              onClick={() => setConfirmDelete(null)}
                              className="text-xs text-fg-3"
                            >
                              Cancel
                            </button>
                          </span>
                        ) : (
                          <button
                            onClick={() => setConfirmDelete(cfg.id)}
                            className="rounded-md border border-edge p-1.5 text-fg-4 transition-colors hover:border-red-900/50 hover:text-red-400"
                          >
                            <span className="material-symbols-outlined text-base">
                              delete
                            </span>
                          </button>
                        )}
                      </div>
                    </div>

                    {/* Expanded detail */}
                    {expandedConfig === cfg.id && (
                      <div className="mt-3 border-t border-edge pt-3">
                        <div className="grid grid-cols-2 gap-x-6 gap-y-1.5">
                          {Object.entries(cfg.params)
                            .filter(([, v]) => v)
                            .map(([k, v]) => (
                              <div key={k} className="flex items-baseline gap-2 text-xs">
                                <span className="font-medium text-fg-3">{k}</span>
                                <span className="font-mono text-fg truncate" title={v}>
                                  {v}
                                </span>
                              </div>
                            ))}
                        </div>
                        <div className="mt-2 text-[10px] text-fg-4">
                          Template: <span className="font-mono text-fg-3">{cfg.workflow}</span>
                          {cfg.timeout_seconds ? (
                            <> &middot; Timeout: <span className="font-mono text-fg-3">{Math.round(cfg.timeout_seconds / 60)}min</span></>
                          ) : null}
                          {cfg.created_at && (
                            <> &middot; Created: {new Date(cfg.created_at).toLocaleDateString()}</>
                          )}
                        </div>
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </>
      ) : (
        <>
          {!runsLoading && runs.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-20">
              <span className="material-symbols-outlined mb-4 text-5xl text-slate-700">
                history
              </span>
              <p className="mb-1 text-lg font-medium text-fg-3">
                No workflow runs yet
              </p>
              <p className="text-sm text-fg-4">
                Runs will appear here once a workflow is executed.
              </p>
            </div>
          ) : filteredRuns.length === 0 ? (
            <p className="py-12 text-center text-sm text-fg-4">
              No runs match your search.
            </p>
          ) : (
            <div className="overflow-hidden rounded-lg border border-edge">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-edge bg-surface text-left">
                    <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-fg-4">
                      Run ID
                    </th>
                    <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-fg-4">
                      Status
                    </th>
                    <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-fg-4">
                      Workflow
                    </th>
                    <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-fg-4">
                      Error
                    </th>
                    <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-fg-4 text-right">
                      Time
                    </th>
                    <th className="w-10"></th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-edge">
                  {filteredRuns.map((run) => (
                    <RunRow
                      key={run.id}
                      run={run}
                      onCancel={() => {
                        cancelRun.mutate(run.id, {
                          onSuccess: () =>
                            toast("success", "Run cancellation requested"),
                          onError: (err) =>
                            toast("error", `Cancel failed: ${err.message}`),
                        });
                      }}
                      cancelPending={cancelRun.isPending}
                    />
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}
    </div>
  );
}

function RunRow({
  run,
  onCancel,
  cancelPending,
}: {
  run: WorkflowRun;
  onCancel: () => void;
  cancelPending: boolean;
}) {
  const sc = runStatusColors[run.status];
  const isActive = run.status === "pending" || run.status === "running";

  return (
    <tr className="group bg-surface-alt transition-colors hover:bg-accent/5">
      <td className="px-4 py-3">
        <Link
          to={`/workflows/runs/${run.id}`}
          className="font-mono text-xs text-accent/60 hover:text-accent transition-colors"
        >
          {run.id.slice(0, 8)}
        </Link>
      </td>
      <td className="px-4 py-3">
        <span
          className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-xs font-bold uppercase ${sc.bg} ${sc.color} ${sc.border}`}
        >
          {run.status === "running" && (
            <span className="size-1.5 animate-pulse rounded-full bg-current" />
          )}
          {run.status}
        </span>
      </td>
      <td className="px-4 py-3">
        <span className="text-sm text-fg-3">{run.workflow_name}</span>
      </td>
      <td className="px-4 py-3">
        {run.error && (
          <span className="truncate text-xs text-red-400" title={run.error}>
            {run.error.length > 50 ? run.error.slice(0, 50) + "..." : run.error}
          </span>
        )}
      </td>
      <td className="px-4 py-3 text-right">
        <span className="font-mono text-xs text-fg-4">
          {formatTimeAgo(run.created_at)}
        </span>
      </td>
      <td className="px-4 py-3">
        <div className="flex items-center gap-2">
          <Link
            to={`/workflows/runs/${run.id}`}
            className="flex items-center gap-1 rounded-lg border border-edge px-3 py-1.5 text-xs font-medium text-fg-3 transition-colors hover:border-accent/30 hover:text-accent"
          >
            <span className="material-symbols-outlined text-sm">visibility</span>
            Detail
          </Link>
          {isActive && (
            <button
              onClick={(e) => {
                e.stopPropagation();
                onCancel();
              }}
              disabled={cancelPending}
              className="flex items-center rounded border border-red-900/50 bg-red-900/20 p-1.5 text-red-400 transition-colors hover:bg-red-900/40 disabled:opacity-50"
              title="Cancel run"
            >
              <span className="material-symbols-outlined text-sm">stop</span>
            </button>
          )}
        </div>
      </td>
    </tr>
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

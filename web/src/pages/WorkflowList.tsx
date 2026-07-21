import { useMemo, useState } from "react";
import { useNavigate } from "react-router";
import {
  BookOpen,
  Bug,
  ChevronDown,
  ChevronUp,
  Code,
  Network,
  Play,
  Plus,
  Search,
  Trash2,
  type LucideIcon,
} from "lucide-react";
import { usePageTitle } from "../hooks/usePageTitle";
import { useToast } from "../context/ToastContext";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../hooks/useApi";

const workflowIcons: Record<string, LucideIcon> = {
  "sentry-fixer": Bug,
  "github-issue-fixer": Code,
  "gitlab-issue-fixer": Code,
  "knowledge-update": BookOpen,
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

  const deleteConfig = useMutation({
    mutationFn: (id: number) => api.deleteWorkflowConfig(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["workflowConfigs"] }),
  });
  const runConfig = useMutation({
    mutationFn: (id: number) => api.runWorkflowConfig(id),
    onSuccess: (data) => {
      void navigate(`/sessions/${data.session_id}`);
    },
  });

  const [search, setSearch] = useState("");
  const [confirmDelete, setConfirmDelete] = useState<number | null>(null);
  const [expandedConfig, setExpandedConfig] = useState<number | null>(null);

  const filteredConfigs = useMemo(() => {
    if (!search) return configs;
    const q = search.toLowerCase();
    return configs.filter(
      (c) =>
        c.name.toLowerCase().includes(q) ||
        c.workflow.toLowerCase().includes(q),
    );
  }, [configs, search]);

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      {/* Header */}
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="eyebrow mb-1">Automation</p>
          <h2 className="font-expanded text-2xl font-extrabold tracking-tight text-fg">
            Workflows
          </h2>
        </div>
        <button
          onClick={() => void navigate("/workflows/new")}
          className="flex items-center gap-2 rounded-md bg-accent px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-accent-hover"
        >
          <Plus className="size-4" />
          New workflow
        </button>
      </div>

      {/* Search */}
      <div className="relative">
        <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-fg-4" />
        <input
          type="text"
          placeholder="Search workflows"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="w-full rounded-md border border-edge bg-input py-2 pr-3 pl-9 text-sm text-fg placeholder-fg-4 transition-colors focus:border-accent focus:outline-none"
        />
      </div>

      {/* Content */}
      {!configsLoading && configs.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-md border border-edge bg-surface py-16 text-center">
          <Network className="mb-3 size-6 text-fg-4" strokeWidth={1.75} />
          <p className="mb-4 text-sm text-fg-3">
            No workflows yet. Create one from a template.
          </p>
          <button
            onClick={() => void navigate("/workflows/new")}
            className="flex items-center gap-2 rounded-md bg-accent px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-accent-hover"
          >
            <Plus className="size-4" />
            New workflow
          </button>
        </div>
      ) : filteredConfigs.length === 0 ? (
        <p className="py-12 text-center text-sm text-fg-4">
          No workflows match your search.
        </p>
      ) : (
        <div className="flex flex-col gap-3">
          {filteredConfigs.map((cfg) => {
            const Icon = workflowIcons[cfg.workflow] ?? Network;
            return (
              <div
                key={cfg.id}
                className="rounded-md border border-edge bg-surface p-4"
              >
                <div className="flex items-center justify-between gap-4">
                  <div className="flex min-w-0 items-center gap-3">
                    <div className="flex size-10 shrink-0 items-center justify-center rounded-md border border-edge bg-surface-alt">
                      <Icon className="size-5 text-fg-3" strokeWidth={1.75} />
                    </div>
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="font-mono text-sm font-medium text-fg">
                          {cfg.name}
                        </span>
                        <span className="rounded-[4px] border border-edge bg-surface-alt px-1.5 py-0.5 font-mono text-[10px] tracking-wider text-fg-3 uppercase">
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
                          <span>
                            +{Object.keys(cfg.params).length - 3} more
                          </span>
                        )}
                      </div>
                    </div>
                  </div>
                  <div className="flex shrink-0 items-center gap-2">
                    <button
                      onClick={() =>
                        setExpandedConfig(
                          expandedConfig === cfg.id ? null : cfg.id,
                        )
                      }
                      className={`flex items-center gap-1 rounded-md border px-3 py-2 text-xs font-medium transition-colors ${
                        expandedConfig === cfg.id
                          ? "border-accent-muted bg-accent-soft text-accent"
                          : "border-edge bg-surface text-fg-2 hover:border-fg-4 hover:text-fg"
                      }`}
                    >
                      {expandedConfig === cfg.id ? (
                        <ChevronUp className="size-4" />
                      ) : (
                        <ChevronDown className="size-4" />
                      )}
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
                      className="flex items-center gap-1.5 rounded-md bg-accent px-3 py-2 text-xs font-semibold text-white transition-colors hover:bg-accent-hover disabled:opacity-50"
                    >
                      <Play className="size-4" />
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
                          className="rounded-md border border-danger/30 bg-surface px-3 py-2 text-xs font-medium text-danger transition-colors hover:bg-danger/10"
                        >
                          Confirm
                        </button>
                        <button
                          onClick={() => setConfirmDelete(null)}
                          className="text-xs text-fg-3 transition-colors hover:text-fg"
                        >
                          Cancel
                        </button>
                      </span>
                    ) : (
                      <button
                        onClick={() => setConfirmDelete(cfg.id)}
                        className="rounded-md p-2 text-fg-3 transition-colors hover:bg-danger/10 hover:text-danger"
                        title="Delete workflow"
                      >
                        <Trash2 className="size-4" />
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
                          <div
                            key={k}
                            className="flex items-baseline gap-2 text-xs"
                          >
                            <span className="font-medium text-fg-3">{k}</span>
                            <span
                              className="truncate font-mono text-fg"
                              title={v}
                            >
                              {v}
                            </span>
                          </div>
                        ))}
                    </div>
                    <div className="mt-2 text-xs text-fg-4">
                      Template:{" "}
                      <span className="font-mono text-fg-3">
                        {cfg.workflow}
                      </span>
                      {cfg.timeout_seconds ? (
                        <>
                          {" "}
                          &middot; Timeout:{" "}
                          <span className="font-mono text-fg-3">
                            {Math.round(cfg.timeout_seconds / 60)} min
                          </span>
                        </>
                      ) : null}
                      {cfg.created_at && (
                        <>
                          {" "}
                          &middot; Created:{" "}
                          <span className="font-mono text-fg-3">
                            {new Date(cfg.created_at).toLocaleDateString()}
                          </span>
                        </>
                      )}
                    </div>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

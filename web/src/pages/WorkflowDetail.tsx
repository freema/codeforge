import { useState, useMemo } from "react";
import { useParams, useNavigate } from "react-router";
import {
  Circle,
  CloudDownload,
  Code,
  Loader2,
  Play,
  SquareTerminal,
  Trash2,
  X,
  Zap,
  type LucideIcon,
} from "lucide-react";
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

/* Step-type chips: session is the hot AI work (ember), fetch and
   action are cold machine processes (steel). */
const stepTypeColors: Record<
  StepType,
  { chip: string; iconColor: string; icon: LucideIcon }
> = {
  fetch: {
    icon: CloudDownload,
    iconColor: "text-info",
    chip: "border-info/30 bg-info/10 text-info",
  },
  session: {
    icon: SquareTerminal,
    iconColor: "text-accent",
    chip: "border-accent-muted bg-accent-soft text-accent",
  },
  action: {
    icon: Zap,
    iconColor: "text-info",
    chip: "border-info/30 bg-info/10 text-info",
  },
};

export default function WorkflowDetail() {
  usePageTitle("Workflow detail");
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
        <Loader2 className="size-6 animate-spin text-fg-4" />
      </div>
    );
  }

  if (!workflow) {
    return (
      <p className="py-20 text-center text-sm text-fg-4">Workflow not found.</p>
    );
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
    "w-full rounded-md border border-edge bg-input px-3 py-2 font-mono text-sm text-fg placeholder-fg-4 transition-colors focus:border-accent focus:outline-none";

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      {/* Header */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p className="eyebrow mb-1">Workflows</p>
          <div className="flex items-center gap-3">
            <h2 className="font-expanded text-2xl font-extrabold tracking-tight text-fg">
              {workflow.name}
            </h2>
            {workflow.builtin && (
              <span className="rounded-[4px] border border-edge bg-surface-alt px-1.5 py-0.5 font-mono text-[10px] tracking-wider text-fg-3 uppercase">
                Built-in
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
                  <span className="text-xs text-fg-3">
                    Delete this workflow?
                  </span>
                  <button
                    onClick={() => void handleDelete()}
                    disabled={deleteWorkflow.isPending}
                    className="rounded-md border border-danger/30 bg-surface px-3 py-1.5 text-xs font-medium text-danger transition-colors hover:bg-danger/10 disabled:opacity-50"
                  >
                    Delete
                  </button>
                  <button
                    onClick={() => setConfirmDelete(false)}
                    className="rounded-md border border-edge bg-surface px-3 py-1.5 text-xs font-medium text-fg-2 transition-colors hover:border-fg-4 hover:text-fg"
                  >
                    Cancel
                  </button>
                </div>
              ) : (
                <button
                  onClick={() => setConfirmDelete(true)}
                  className="flex items-center gap-1.5 rounded-md border border-danger/30 bg-surface px-3 py-2 text-sm font-medium text-danger transition-colors hover:bg-danger/10"
                >
                  <Trash2 className="size-4" />
                  Delete
                </button>
              )}
            </>
          )}
          <button
            onClick={() => setShowRun(!showRun)}
            className={`flex items-center gap-2 rounded-md px-4 py-2 text-sm transition-colors ${
              showRun
                ? "border border-edge bg-surface font-medium text-fg-2 hover:border-fg-4 hover:text-fg"
                : "bg-accent font-semibold text-white hover:bg-accent-hover"
            }`}
          >
            {showRun ? <X className="size-4" /> : <Play className="size-4" />}
            {showRun ? "Close" : "Run workflow"}
          </button>
        </div>
      </div>

      {/* Run form -- only shown when toggled */}
      {showRun &&
        (isSentryFixer ? (
          <SentryFixerRunForm />
        ) : (
          <div className="overflow-hidden rounded-md border border-edge bg-surface">
            <div className="border-b border-edge px-5 py-3.5">
              <span className="eyebrow">Run parameters</span>
            </div>
            <div className="space-y-4 p-5">
              {/* Smart: Provider key selector */}
              {gitKeys &&
                gitKeys.length > 0 &&
                workflow.parameters.some(
                  (p) => p.name === "provider_key" || p.name === "repo_url",
                ) && (
                  <div>
                    <label className="mb-1.5 block text-xs font-medium text-fg-3">
                      Provider key
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
                          className={`flex items-center gap-2 rounded-md border px-4 py-2 font-mono text-sm transition-colors ${
                            (selectedKey || params.provider_key) === k.name
                              ? "border-accent-muted bg-accent-soft text-accent"
                              : "border-edge bg-surface text-fg-2 hover:border-fg-4 hover:text-fg"
                          }`}
                        >
                          <Code className="size-4" />
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
                    <label className="mb-1.5 block text-xs font-medium text-fg-3">
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
                .filter(
                  (p) => p.name !== "provider_key" && p.name !== "repo_url",
                )
                .map((p) => (
                  <div key={p.name}>
                    <label className="mb-1.5 block text-xs font-medium text-fg-3">
                      {p.name.replace(/_/g, " ")}
                      {p.required && (
                        <span className="ml-1 text-danger">*</span>
                      )}
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
                className="flex items-center gap-2 rounded-md bg-accent px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-accent-hover disabled:opacity-50"
              >
                {runWorkflow.isPending ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <Play className="size-4" />
                )}
                Start run
              </button>
            </div>
          </div>
        ))}

      {/* Steps */}
      <div className="overflow-hidden rounded-md border border-edge bg-surface">
        <div className="flex items-center justify-between border-b border-edge px-5 py-3.5">
          <span className="eyebrow">Workflow steps</span>
          <span className="rounded-[4px] border border-edge bg-surface-alt px-1.5 py-0.5 font-mono text-xs text-fg-3">
            {workflow.steps.length} step{workflow.steps.length !== 1 ? "s" : ""}
          </span>
        </div>
        <div className="p-4">
          <div className="flex flex-col gap-2">
            {workflow.steps.map((step, i) => {
              const tc = stepTypeColors[step.type] ?? {
                icon: Circle,
                iconColor: "text-fg-3",
                chip: "border-edge bg-surface-alt text-fg-3",
              };
              const StepIcon = tc.icon;
              return (
                <div key={step.name}>
                  <div className="flex items-center gap-3 rounded-md border border-edge bg-surface-alt p-4">
                    <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-surface font-mono text-sm font-semibold text-fg-3">
                      {i + 1}
                    </span>
                    <StepIcon className={`size-4 shrink-0 ${tc.iconColor}`} />
                    <div className="flex-1">
                      <span className="font-mono text-sm text-fg">
                        {step.name}
                      </span>
                    </div>
                    <span
                      className={`rounded-[4px] border px-1.5 py-0.5 font-mono text-[10px] tracking-wider uppercase ${tc.chip}`}
                    >
                      {step.type}
                    </span>
                  </div>
                  {i < workflow.steps.length - 1 && (
                    <div className="ml-[33px] flex h-4 items-center">
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

import { useMemo, useState } from "react";
import { Link, useNavigate } from "react-router";
import {
  BookOpen,
  Bug,
  CalendarClock,
  ChevronDown,
  ChevronUp,
  Layers,
  Loader2,
  Play,
  Plus,
  Search,
  SearchCode,
  Trash2,
  X,
  type LucideIcon,
} from "lucide-react";
import { usePageTitle } from "../hooks/usePageTitle";
import { useToast } from "../context/ToastContext";
import { useBlueprints } from "../hooks/useBlueprints";
import {
  useDeleteBlueprint,
  useRunBlueprint,
} from "../hooks/useBlueprintMutations";
import SentryFixerRunForm from "../components/SentryFixerRunForm";
import type { Blueprint } from "../types";

const blueprintIcons: Record<string, LucideIcon> = {
  "sentry-fixer": Bug,
  "knowledge-update": BookOpen,
  "repo-review": SearchCode,
};

const inputCls =
  "w-full rounded-md border border-edge bg-input px-3 py-2 font-mono text-sm text-fg placeholder-fg-4 transition-colors focus:border-accent focus:outline-none";

export default function Blueprints() {
  usePageTitle("Blueprints");
  const navigate = useNavigate();
  const { data: blueprints = [], isLoading } = useBlueprints();

  const [search, setSearch] = useState("");

  const filtered = useMemo(() => {
    if (!search) return blueprints;
    const q = search.toLowerCase();
    return blueprints.filter(
      (b) =>
        b.name.toLowerCase().includes(q) ||
        b.description.toLowerCase().includes(q),
    );
  }, [blueprints, search]);

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      {/* Header */}
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="eyebrow mb-1">Automation</p>
          <h2 className="font-expanded text-2xl font-extrabold tracking-tight text-fg">
            Blueprints
          </h2>
        </div>
        <button
          onClick={() => void navigate("/blueprints/new")}
          className="flex items-center gap-2 rounded-md bg-accent px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-accent-hover"
        >
          <Plus className="size-4" />
          New blueprint
        </button>
      </div>

      {/* Search */}
      <div className="relative">
        <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-fg-4" />
        <input
          type="text"
          placeholder="Search blueprints"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="w-full rounded-md border border-edge bg-input py-2 pr-3 pl-9 text-sm text-fg placeholder-fg-4 transition-colors focus:border-accent focus:outline-none"
        />
      </div>

      {/* Content */}
      {isLoading ? (
        <div className="flex items-center justify-center py-20">
          <Loader2 className="size-6 animate-spin text-fg-4" />
        </div>
      ) : blueprints.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-md border border-edge bg-surface py-16 text-center">
          <Layers className="mb-3 size-6 text-fg-4" strokeWidth={1.75} />
          <p className="mb-4 text-sm text-fg-3">
            No blueprints yet. Create one from a template.
          </p>
          <button
            onClick={() => void navigate("/blueprints/new")}
            className="flex items-center gap-2 rounded-md bg-accent px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-accent-hover"
          >
            <Plus className="size-4" />
            New blueprint
          </button>
        </div>
      ) : filtered.length === 0 ? (
        <p className="py-12 text-center text-sm text-fg-4">
          No blueprints match your search.
        </p>
      ) : (
        <div className="flex flex-col gap-3">
          {filtered.map((b) => (
            <BlueprintCard key={b.id} blueprint={b} />
          ))}
        </div>
      )}
    </div>
  );
}

function BlueprintCard({ blueprint }: { blueprint: Blueprint }) {
  const { toast } = useToast();
  const deleteBlueprint = useDeleteBlueprint();

  const [expanded, setExpanded] = useState(false);
  const [showRun, setShowRun] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);

  const Icon = blueprintIcons[blueprint.name] ?? Layers;

  function handleDelete() {
    deleteBlueprint.mutate(blueprint.id, {
      onSuccess: () => {
        toast("success", "Blueprint deleted");
        setConfirmDelete(false);
      },
      onError: (err) => {
        toast("error", `Delete failed: ${err.message}`);
        setConfirmDelete(false);
      },
    });
  }

  const req = blueprint.request;

  return (
    <div className="rounded-md border border-edge bg-surface p-4">
      <div className="flex items-center justify-between gap-4">
        <div className="flex min-w-0 items-center gap-3">
          <div className="flex size-10 shrink-0 items-center justify-center rounded-md border border-edge bg-surface-alt">
            <Icon className="size-5 text-fg-3" strokeWidth={1.75} />
          </div>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-mono text-sm font-medium text-fg">
                {blueprint.name}
              </span>
              {blueprint.builtin && (
                <span className="rounded-[4px] border border-edge bg-surface-alt px-1.5 py-0.5 font-mono text-[10px] tracking-wider text-fg-3 uppercase">
                  Built-in
                </span>
              )}
            </div>
            {blueprint.description && (
              <p className="mt-0.5 truncate text-xs text-fg-3">
                {blueprint.description}
              </p>
            )}
            {blueprint.parameters.length > 0 && (
              <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
                {blueprint.parameters.map((p) => (
                  <span
                    key={p.name}
                    className="rounded-[4px] border border-edge bg-surface-alt px-1.5 py-0.5 font-mono text-[10px] text-fg-3"
                    title={
                      p.required ? "Required parameter" : "Optional parameter"
                    }
                  >
                    {p.name}
                    {p.default ? `=${p.default}` : ""}
                    {p.required && <span className="text-danger">*</span>}
                  </span>
                ))}
              </div>
            )}
          </div>
        </div>

        <div className="flex shrink-0 items-center gap-2">
          <button
            onClick={() => setExpanded(!expanded)}
            className={`flex items-center gap-1 rounded-md border px-3 py-2 text-xs font-medium transition-colors ${
              expanded
                ? "border-accent-muted bg-accent-soft text-accent"
                : "border-edge bg-surface text-fg-2 hover:border-fg-4 hover:text-fg"
            }`}
          >
            {expanded ? (
              <ChevronUp className="size-4" />
            ) : (
              <ChevronDown className="size-4" />
            )}
            Detail
          </button>
          <Link
            to={`/schedules?blueprint=${blueprint.id}`}
            className="flex items-center gap-1.5 rounded-md border border-edge bg-surface px-3 py-2 text-xs font-medium text-fg-2 transition-colors hover:border-fg-4 hover:text-fg"
            title="Create a recurring schedule from this blueprint"
          >
            <CalendarClock className="size-4" />
            Schedule
          </Link>
          <button
            onClick={() => setShowRun(!showRun)}
            className={`flex items-center gap-1.5 rounded-md px-3 py-2 text-xs transition-colors ${
              showRun
                ? "border border-edge bg-surface font-medium text-fg-2 hover:border-fg-4 hover:text-fg"
                : "bg-accent font-semibold text-white hover:bg-accent-hover"
            }`}
          >
            {showRun ? <X className="size-4" /> : <Play className="size-4" />}
            {showRun ? "Close" : "Run"}
          </button>
          {!blueprint.builtin &&
            (confirmDelete ? (
              <span className="flex items-center gap-2">
                <button
                  onClick={handleDelete}
                  disabled={deleteBlueprint.isPending}
                  className="rounded-md border border-danger/30 bg-surface px-3 py-2 text-xs font-medium text-danger transition-colors hover:bg-danger/10 disabled:opacity-50"
                >
                  Confirm
                </button>
                <button
                  onClick={() => setConfirmDelete(false)}
                  className="text-xs text-fg-3 transition-colors hover:text-fg"
                >
                  Cancel
                </button>
              </span>
            ) : (
              <button
                onClick={() => setConfirmDelete(true)}
                className="rounded-md p-2 text-fg-3 transition-colors hover:bg-danger/10 hover:text-danger"
                title="Delete blueprint"
              >
                <Trash2 className="size-4" />
              </button>
            ))}
        </div>
      </div>

      {/* Expanded detail — request template summary */}
      {expanded && (
        <div className="mt-3 border-t border-edge pt-3">
          <div className="grid grid-cols-1 gap-x-6 gap-y-1.5 sm:grid-cols-2">
            <DetailRow label="repo_url" value={req.repo_url} />
            <DetailRow label="session_type" value={req.session_type} />
            <DetailRow label="cli" value={req.config?.cli} />
            <DetailRow label="provider_key" value={req.provider_key} />
          </div>
          <div className="mt-2 text-xs text-fg-4">
            {blueprint.parameters.length} parameter
            {blueprint.parameters.length !== 1 ? "s" : ""}
            {blueprint.created_at && (
              <>
                {" "}
                &middot; Created:{" "}
                <span className="font-mono text-fg-3">
                  {new Date(blueprint.created_at).toLocaleDateString()}
                </span>
              </>
            )}
          </div>
        </div>
      )}

      {/* Inline run panel */}
      {showRun && (
        <div className="mt-3 border-t border-edge pt-3">
          {blueprint.builtin && blueprint.name === "sentry-fixer" ? (
            <SentryFixerRunForm blueprintId={blueprint.id} />
          ) : (
            <RunPanel blueprint={blueprint} />
          )}
        </div>
      )}
    </div>
  );
}

function DetailRow({ label, value }: { label: string; value?: string }) {
  if (!value) return null;
  return (
    <div className="flex items-baseline gap-2 text-xs">
      <span className="shrink-0 font-medium text-fg-3">{label}</span>
      <span className="truncate font-mono text-fg" title={value}>
        {value}
      </span>
    </div>
  );
}

function RunPanel({ blueprint }: { blueprint: Blueprint }) {
  const navigate = useNavigate();
  const { toast } = useToast();
  const runBlueprint = useRunBlueprint();

  const [params, setParams] = useState<Record<string, string>>(() => {
    const defaults: Record<string, string> = {};
    for (const p of blueprint.parameters) {
      if (p.default) defaults[p.name] = p.default;
    }
    return defaults;
  });
  const [promptOverride, setPromptOverride] = useState("");

  function updateParam(key: string, value: string) {
    setParams((prev) => ({ ...prev, [key]: value }));
  }

  async function handleRun() {
    // Send only non-empty values so backend defaults and required-parameter
    // validation behave as declared.
    const filled = Object.fromEntries(
      Object.entries(params).filter(([, v]) => v.trim() !== ""),
    );
    try {
      const created = await runBlueprint.mutateAsync({
        id: blueprint.id,
        ...(Object.keys(filled).length > 0 ? { params: filled } : {}),
        ...(promptOverride.trim() ? { prompt: promptOverride.trim() } : {}),
      });
      toast("success", "Session started");
      void navigate(`/sessions/${created.id}`);
    } catch (err) {
      toast(
        "error",
        `Run failed: ${err instanceof Error ? err.message : String(err)}`,
      );
    }
  }

  const missingRequired = blueprint.parameters.some(
    (p) => p.required && !(params[p.name] ?? "").trim(),
  );

  return (
    <div className="space-y-4">
      {blueprint.parameters.length > 0 ? (
        blueprint.parameters.map((p) => (
          <div key={p.name}>
            <label className="mb-1.5 block text-xs font-medium text-fg-3">
              {p.name.replace(/_/g, " ")}
              {p.required && <span className="ml-1 text-danger">*</span>}
            </label>
            <input
              type="text"
              value={params[p.name] ?? ""}
              onChange={(e) => updateParam(p.name, e.target.value)}
              placeholder={p.default ?? `Enter ${p.name}...`}
              className={inputCls}
            />
          </div>
        ))
      ) : (
        <p className="text-xs text-fg-4">
          This blueprint has no parameters — it runs its saved request as-is.
        </p>
      )}

      <div>
        <label className="mb-1.5 block text-xs font-medium text-fg-3">
          Prompt override (optional)
        </label>
        <textarea
          value={promptOverride}
          onChange={(e) => setPromptOverride(e.target.value)}
          placeholder="Replace the blueprint's prompt for this run…"
          rows={2}
          className="w-full resize-y rounded-md border border-edge bg-input px-3 py-2 text-sm text-fg placeholder-fg-4 transition-colors focus:border-accent focus:outline-none"
        />
      </div>

      <button
        onClick={() => void handleRun()}
        disabled={runBlueprint.isPending || missingRequired}
        className="flex items-center gap-2 rounded-md bg-accent px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-accent-hover disabled:opacity-50"
      >
        {runBlueprint.isPending ? (
          <Loader2 className="size-4 animate-spin" />
        ) : (
          <Play className="size-4" />
        )}
        Start run
      </button>
    </div>
  );
}

import { useEffect, useMemo, useState, type FormEvent } from "react";
import { Link, useNavigate } from "react-router";
import {
  CalendarClock,
  ChevronDown,
  CircleAlert,
  History,
  Loader2,
  Play,
  Plus,
  Trash2,
  TriangleAlert,
} from "lucide-react";
import { usePageTitle } from "../hooks/usePageTitle";
import { useToast } from "../context/ToastContext";
import {
  useSchedules,
  useScheduleRuns,
  useCreateSchedule,
  useUpdateSchedule,
  useDeleteSchedule,
  useRunSchedule,
} from "../hooks/useSchedules";
import { useCLIs } from "../hooks/useCLIs";
import { formatTimeAgo } from "../lib/formatters";
import type { Schedule, ScheduleRunStatus } from "../types";

const inputCls =
  "w-full rounded-md border border-edge bg-input px-3 py-2 text-sm text-fg placeholder-fg-4 transition-colors focus:border-accent focus:outline-none";

const DEFAULT_CRON = "*/15 * * * *";

const CRON_PRESETS = [
  { value: DEFAULT_CRON, label: "Every 15 min" },
  { value: "0 * * * *", label: "Hourly" },
  { value: "0 3 * * *", label: "Daily 03:00" },
  { value: "0 3 * * 0", label: "Weekly Sun 03:00" },
  { value: "custom", label: "Custom" },
];

/* Run-history status → semantic tint (ok = fired/completed, danger = failed,
   muted = skipped) */
const RUN_STATUS_STYLES: Record<ScheduleRunStatus, string> = {
  fired: "border-ok/25 bg-ok/10 text-ok",
  session_completed: "border-ok/25 bg-ok/10 text-ok",
  fire_failed: "border-danger/25 bg-danger/10 text-danger",
  session_failed: "border-danger/25 bg-danger/10 text-danger",
  skipped_overlap: "border-edge bg-surface text-fg-4",
};

function extractRepoName(repoUrl: string): string {
  return repoUrl
    .replace(/^https?:\/\//, "")
    .replace(/\.git$/, "")
    .split("/")
    .slice(-2)
    .join("/");
}

export default function Schedules() {
  usePageTitle("Schedules");
  const { data: schedules, isLoading, isError } = useSchedules();

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      <div>
        <p className="eyebrow mb-1">Automation</p>
        <h2 className="font-expanded text-2xl font-extrabold tracking-tight text-fg">
          Schedules
        </h2>
      </div>

      {isLoading ? (
        <LoadingSkeleton />
      ) : isError ? (
        <div className="flex flex-col items-center rounded-md border border-danger/30 bg-danger/10 p-8 text-center">
          <CircleAlert className="mb-3 size-6 text-danger" />
          <p className="text-sm text-danger">Failed to load schedules.</p>
        </div>
      ) : schedules && schedules.length > 0 ? (
        <div className="flex flex-col gap-3">
          {schedules.map((s) => (
            <ScheduleCard key={s.id} schedule={s} />
          ))}
        </div>
      ) : (
        <div className="flex flex-col items-center justify-center rounded-md border border-edge bg-surface py-16 text-center">
          <CalendarClock className="mb-3 size-6 text-fg-4" strokeWidth={1.75} />
          <p className="text-sm text-fg-3">
            No schedules yet. Create one below.
          </p>
        </div>
      )}

      {!isError && <AddScheduleForm />}
    </div>
  );
}

function EnabledToggle({ schedule }: { schedule: Schedule }) {
  const updateSchedule = useUpdateSchedule();
  const { toast } = useToast();

  function handleToggle() {
    updateSchedule.mutate(
      { id: schedule.id, req: { enabled: !schedule.enabled } },
      {
        onError: (err) => toast("error", `Update failed: ${err.message}`),
      },
    );
  }

  return (
    <button
      type="button"
      role="switch"
      aria-checked={schedule.enabled}
      onClick={handleToggle}
      disabled={updateSchedule.isPending}
      title={schedule.enabled ? "Disable schedule" : "Enable schedule"}
      className={`relative h-6 w-11 shrink-0 rounded-full border transition-colors disabled:opacity-50 ${
        schedule.enabled
          ? "border-accent-muted bg-accent-soft"
          : "border-edge bg-surface-alt"
      }`}
    >
      <span
        className={`absolute top-1 size-4 rounded-full transition-all ${
          schedule.enabled ? "left-6 bg-accent" : "left-1 bg-fg-4"
        }`}
      />
    </button>
  );
}

function ScheduleCard({ schedule }: { schedule: Schedule }) {
  const navigate = useNavigate();
  const { toast } = useToast();
  const runSchedule = useRunSchedule();
  const deleteSchedule = useDeleteSchedule();
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [showHistory, setShowHistory] = useState(false);

  function handleRun() {
    runSchedule.mutate(schedule.id, {
      onSuccess: (data) => {
        toast("success", "Run started — opening session");
        void navigate(`/sessions/${data.session_id}`);
      },
      onError: (err) => toast("error", `Run failed: ${err.message}`),
    });
  }

  function handleDelete() {
    deleteSchedule.mutate(schedule.id, {
      onSuccess: () => {
        toast("success", "Schedule deleted");
        setConfirmDelete(false);
      },
      onError: (err) => toast("error", `Delete failed: ${err.message}`),
    });
  }

  const nextRun =
    schedule.enabled && schedule.next_run_at
      ? new Date(schedule.next_run_at).toLocaleString()
      : "—";

  return (
    <div className="rounded-md border border-edge bg-surface p-4">
      <div className="flex items-center justify-between gap-4">
        <div className="flex min-w-0 items-center gap-3">
          <div className="flex size-10 shrink-0 items-center justify-center rounded-md border border-edge bg-surface-alt">
            <CalendarClock
              className={`size-5 ${schedule.enabled ? "text-accent" : "text-fg-4"}`}
              strokeWidth={1.75}
            />
          </div>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-medium text-fg">{schedule.name}</span>
              <span className="rounded-[4px] border border-edge bg-surface-alt px-1.5 py-0.5 font-mono text-xs text-fg-2">
                {schedule.cron}
              </span>
              {schedule.timezone && (
                <span className="rounded-[4px] border border-edge bg-surface-alt px-1.5 py-0.5 font-mono text-xs text-fg-3">
                  {schedule.timezone}
                </span>
              )}
              {schedule.consecutive_failures > 0 && (
                <span
                  className="flex items-center gap-1 rounded-[4px] border border-warn/25 bg-warn/10 px-1.5 py-0.5 font-mono text-[10px] tracking-wider text-warn uppercase"
                  title="Consecutive failures"
                >
                  <TriangleAlert className="size-3" />
                  {schedule.consecutive_failures}{" "}
                  {schedule.consecutive_failures === 1 ? "failure" : "failures"}
                </span>
              )}
            </div>
            <p className="mt-0.5 text-xs text-fg-4">
              <span className="font-mono">
                {extractRepoName(schedule.session_request.repo_url)}
              </span>
              <span className="mx-1.5">·</span>
              {schedule.enabled && schedule.next_run_at && (
                <span className="mr-1.5 inline-block size-1.5 animate-soft-pulse rounded-full bg-ok align-middle" />
              )}
              Next run: <span className="font-mono">{nextRun}</span>
              <span className="mx-1.5">·</span>
              Last run:{" "}
              {schedule.last_run_at ? (
                schedule.last_session_id ? (
                  <Link
                    to={`/sessions/${schedule.last_session_id}`}
                    className="text-accent transition-colors hover:text-accent-hover"
                  >
                    {formatTimeAgo(schedule.last_run_at)}
                  </Link>
                ) : (
                  formatTimeAgo(schedule.last_run_at)
                )
              ) : (
                "never"
              )}
            </p>
          </div>
        </div>

        <div className="flex shrink-0 items-center gap-2">
          <EnabledToggle schedule={schedule} />
          <button
            onClick={handleRun}
            disabled={runSchedule.isPending}
            className="flex items-center gap-1.5 rounded-md bg-accent px-3 py-2 text-xs font-semibold text-white transition-colors hover:bg-accent-hover disabled:opacity-50"
          >
            <Play className="size-4" />
            Run now
          </button>
          {confirmDelete ? (
            <span className="flex items-center gap-2">
              <button
                onClick={handleDelete}
                disabled={deleteSchedule.isPending}
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
              title="Delete schedule"
            >
              <Trash2 className="size-4" />
            </button>
          )}
        </div>
      </div>

      {schedule.disabled_reason && (
        <div className="mt-3 flex items-center gap-2 rounded-md border border-danger/25 bg-danger/10 px-3 py-2 text-xs text-danger">
          <CircleAlert className="size-3.5 shrink-0" />
          <span>Disabled: {schedule.disabled_reason}</span>
        </div>
      )}

      <button
        type="button"
        onClick={() => setShowHistory((v) => !v)}
        className="mt-3 flex items-center gap-1.5 text-xs font-medium text-fg-3 transition-colors hover:text-fg"
      >
        <History className="size-3.5" />
        Run history
        <ChevronDown
          className={`size-3.5 ${showHistory ? "rotate-180" : ""}`}
        />
      </button>
      {showHistory && <RunHistory scheduleId={schedule.id} />}
    </div>
  );
}

function RunHistory({ scheduleId }: { scheduleId: string }) {
  const { data: runs, isLoading, isError } = useScheduleRuns(scheduleId);

  if (isLoading) {
    return (
      <div className="mt-2 flex items-center gap-2 rounded-md border border-edge bg-surface-alt px-3 py-2.5 text-xs text-fg-4">
        <Loader2 className="size-3.5 animate-spin" />
        Loading run history…
      </div>
    );
  }
  if (isError) {
    return (
      <div className="mt-2 rounded-md border border-danger/25 bg-danger/10 px-3 py-2.5 text-xs text-danger">
        Failed to load run history.
      </div>
    );
  }
  if (!runs || runs.length === 0) {
    return (
      <div className="mt-2 rounded-md border border-edge bg-surface-alt px-3 py-2.5 text-xs text-fg-4">
        No runs recorded yet.
      </div>
    );
  }
  return (
    <div className="mt-2 divide-y divide-edge overflow-hidden rounded-md border border-edge bg-surface-alt">
      {runs.map((run) => (
        <div key={run.id} className="flex items-center gap-3 px-3 py-2">
          <span
            className="w-16 shrink-0 font-mono text-xs text-fg-4"
            title={new Date(run.created_at).toLocaleString()}
          >
            {formatTimeAgo(run.created_at)}
          </span>
          <span className="shrink-0 rounded-[4px] border border-edge bg-surface px-1.5 py-0.5 font-mono text-[10px] tracking-wider text-fg-3 uppercase">
            {run.trigger}
          </span>
          <span
            className={`shrink-0 rounded-[4px] border px-1.5 py-0.5 font-mono text-[10px] tracking-wider uppercase ${RUN_STATUS_STYLES[run.status]}`}
          >
            {run.status.replace(/_/g, " ")}
          </span>
          {run.session_id && (
            <Link
              to={`/sessions/${run.session_id}`}
              className="shrink-0 font-mono text-xs text-accent transition-colors hover:text-accent-hover"
            >
              {run.session_id.slice(0, 8)}
            </Link>
          )}
          {run.error && (
            <span
              className="min-w-0 flex-1 truncate text-xs text-danger"
              title={run.error}
            >
              {run.error}
            </span>
          )}
        </div>
      ))}
    </div>
  );
}

function AddScheduleForm() {
  const { toast } = useToast();
  const createSchedule = useCreateSchedule();
  const { data: clis } = useCLIs();

  const [name, setName] = useState("");
  const [cronPreset, setCronPreset] = useState(DEFAULT_CRON);
  const [customCron, setCustomCron] = useState("");
  const [repoUrl, setRepoUrl] = useState("");
  const [prompt, setPrompt] = useState("");
  const [cli, setCli] = useState("");
  const [providerKey, setProviderKey] = useState("");
  const [timezone, setTimezone] = useState("");

  const availableClis = useMemo(
    () => clis?.filter((c) => c.available) ?? [],
    [clis],
  );

  // Auto-select default CLI once the list loads (same pattern as NewSession)
  useEffect(() => {
    if (availableClis.length > 0 && !cli) {
      const defaultCli = availableClis.find((c) => c.is_default);
      setCli(defaultCli?.name ?? availableClis[0]!.name);
    }
  }, [availableClis, cli]);

  async function handleCreate(e: FormEvent) {
    e.preventDefault();
    const cron = cronPreset === "custom" ? customCron.trim() : cronPreset;
    try {
      await createSchedule.mutateAsync({
        name,
        cron,
        ...(timezone.trim() ? { timezone: timezone.trim() } : {}),
        session_request: {
          repo_url: repoUrl,
          prompt,
          ...(providerKey.trim() ? { provider_key: providerKey.trim() } : {}),
          ...(cli ? { config: { cli } } : {}),
        },
      });
      toast("success", "Schedule created");
      setName("");
      setCronPreset(DEFAULT_CRON);
      setCustomCron("");
      setRepoUrl("");
      setPrompt("");
      setCli("");
      setProviderKey("");
      setTimezone("");
    } catch (err) {
      toast(
        "error",
        `Create failed: ${err instanceof Error ? err.message : String(err)}`,
      );
    }
  }

  return (
    <form
      onSubmit={(e) => void handleCreate(e)}
      className="overflow-hidden rounded-md border border-edge bg-surface"
    >
      <div className="border-b border-edge px-5 py-3.5">
        <span className="eyebrow">New schedule</span>
      </div>
      <div className="p-5">
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Name"
            required
            className={inputCls}
          />
          <select
            value={cronPreset}
            onChange={(e) => setCronPreset(e.target.value)}
            className={inputCls}
          >
            {CRON_PRESETS.map((p) => (
              <option key={p.value} value={p.value}>
                {p.label}
              </option>
            ))}
          </select>
          {cronPreset === "custom" && (
            <input
              type="text"
              value={customCron}
              onChange={(e) => setCustomCron(e.target.value)}
              placeholder="Cron expression (e.g. 0 6 * * 1-5)"
              required
              className={`${inputCls} font-mono sm:col-span-2`}
            />
          )}
          <input
            type="text"
            value={repoUrl}
            onChange={(e) => setRepoUrl(e.target.value)}
            placeholder="Repository URL"
            required
            className={`${inputCls} font-mono sm:col-span-2`}
          />
          <textarea
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder="Prompt"
            required
            rows={3}
            className={`${inputCls} resize-y sm:col-span-2`}
          />
          <select
            value={cli}
            onChange={(e) => setCli(e.target.value)}
            className={`${inputCls} font-mono`}
          >
            {availableClis.map((c) => (
              <option key={c.name} value={c.name}>
                {c.name + (c.is_default ? " (default)" : "")}
              </option>
            ))}
          </select>
          <input
            type="text"
            value={providerKey}
            onChange={(e) => setProviderKey(e.target.value)}
            placeholder="Provider key name (optional)"
            className={`${inputCls} font-mono`}
          />
          <input
            type="text"
            value={timezone}
            onChange={(e) => setTimezone(e.target.value)}
            placeholder="Europe/Prague — optional"
            title="Timezone for cron evaluation (IANA name)"
            className={`${inputCls} font-mono`}
          />
        </div>
        <button
          type="submit"
          disabled={createSchedule.isPending}
          className="mt-4 flex items-center gap-2 rounded-md bg-accent px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-accent-hover disabled:opacity-50"
        >
          {createSchedule.isPending ? (
            <Loader2 className="size-4 animate-spin" />
          ) : (
            <Plus className="size-4" />
          )}
          Create schedule
        </button>
      </div>
    </form>
  );
}

function LoadingSkeleton() {
  return (
    <div className="space-y-3">
      {[1, 2, 3].map((i) => (
        <div key={i} className="h-16 animate-pulse rounded-md bg-surface-alt" />
      ))}
    </div>
  );
}

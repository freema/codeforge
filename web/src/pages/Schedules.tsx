import { useState, type FormEvent } from "react";
import { Link, useNavigate } from "react-router";
import {
  CalendarClock,
  CircleAlert,
  Loader2,
  Play,
  Plus,
  Trash2,
} from "lucide-react";
import { usePageTitle } from "../hooks/usePageTitle";
import { useToast } from "../context/ToastContext";
import {
  useSchedules,
  useCreateSchedule,
  useUpdateSchedule,
  useDeleteSchedule,
  useRunSchedule,
} from "../hooks/useSchedules";
import { formatTimeAgo } from "../lib/formatters";
import type { Schedule } from "../types";

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

const CLI_OPTIONS = [
  { value: "claude-code", label: "Claude Code" },
  { value: "codex", label: "Codex" },
  { value: "cursor", label: "Cursor" },
];

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
            <div className="flex items-center gap-2">
              <span className="font-medium text-fg">{schedule.name}</span>
              <span className="rounded-[4px] border border-edge bg-surface-alt px-1.5 py-0.5 font-mono text-xs text-fg-2">
                {schedule.cron}
              </span>
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
    </div>
  );
}

function AddScheduleForm() {
  const { toast } = useToast();
  const createSchedule = useCreateSchedule();

  const [name, setName] = useState("");
  const [cronPreset, setCronPreset] = useState(DEFAULT_CRON);
  const [customCron, setCustomCron] = useState("");
  const [repoUrl, setRepoUrl] = useState("");
  const [prompt, setPrompt] = useState("");
  const [cli, setCli] = useState("claude-code");
  const [providerKey, setProviderKey] = useState("");

  async function handleCreate(e: FormEvent) {
    e.preventDefault();
    const cron = cronPreset === "custom" ? customCron.trim() : cronPreset;
    try {
      await createSchedule.mutateAsync({
        name,
        cron,
        session_request: {
          repo_url: repoUrl,
          prompt,
          ...(providerKey.trim() ? { provider_key: providerKey.trim() } : {}),
          config: { cli },
        },
      });
      toast("success", "Schedule created");
      setName("");
      setCronPreset(DEFAULT_CRON);
      setCustomCron("");
      setRepoUrl("");
      setPrompt("");
      setCli("claude-code");
      setProviderKey("");
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
            className={inputCls}
          >
            {CLI_OPTIONS.map((c) => (
              <option key={c.value} value={c.value}>
                {c.label}
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

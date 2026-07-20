import { useState, type FormEvent } from "react";
import { Link, useNavigate } from "react-router";
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
  "w-full rounded-lg border border-edge bg-surface px-3 py-2.5 text-sm text-fg font-mono placeholder-fg-4 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent transition-colors";

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
        <h1 className="text-3xl font-bold tracking-tight text-fg">Schedules</h1>
        <p className="mt-1 text-sm text-fg-3">
          Recurring sessions triggered on a cron schedule
        </p>
      </div>

      {isLoading ? (
        <LoadingSkeleton />
      ) : isError ? (
        <div className="rounded-xl border border-red-900/50 bg-red-900/10 p-8 text-center">
          <span className="material-symbols-outlined mb-2 text-3xl text-red-400">
            error
          </span>
          <p className="text-sm text-red-400">Failed to load schedules.</p>
        </div>
      ) : schedules && schedules.length > 0 ? (
        <div className="flex flex-col gap-3">
          {schedules.map((s) => (
            <ScheduleCard key={s.id} schedule={s} />
          ))}
        </div>
      ) : (
        <p className="py-8 text-center text-sm text-fg-4">
          No schedules configured.
        </p>
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
          ? "border-accent/50 bg-accent/20"
          : "border-edge bg-surface"
      }`}
    >
      <span
        className={`absolute top-1 h-4 w-4 rounded-full transition-all ${
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
    <div className="rounded-xl border border-edge bg-surface-alt p-4">
      <div className="flex items-center justify-between gap-4">
        <div className="flex items-center gap-3 min-w-0">
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border border-edge bg-surface">
            <span className="material-symbols-outlined text-accent/60">
              schedule
            </span>
          </div>
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <span className="font-medium text-fg">{schedule.name}</span>
              <span className="rounded-full border border-edge bg-surface px-2 py-0.5 font-mono text-[10px] text-fg-3">
                {schedule.cron}
              </span>
            </div>
            <p className="mt-0.5 text-xs text-fg-4">
              <span className="font-mono">
                {extractRepoName(schedule.session_request.repo_url)}
              </span>
              <span className="mx-1.5 text-fg-4">·</span>
              Next run: {nextRun}
              <span className="mx-1.5 text-fg-4">·</span>
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

        <div className="flex items-center gap-2 shrink-0">
          <EnabledToggle schedule={schedule} />
          <button
            onClick={handleRun}
            disabled={runSchedule.isPending}
            className="flex items-center gap-1.5 rounded-lg bg-accent px-4 py-2 text-xs font-bold text-page transition-all hover:bg-accent-hover disabled:opacity-50"
          >
            <span className="material-symbols-outlined text-sm">
              play_arrow
            </span>
            Run now
          </button>
          {confirmDelete ? (
            <span className="flex items-center gap-2">
              <button
                onClick={handleDelete}
                disabled={deleteSchedule.isPending}
                className="rounded-lg border border-red-900/50 bg-red-900/20 px-3 py-2 text-xs font-medium text-red-400"
              >
                Confirm
              </button>
              <button
                onClick={() => setConfirmDelete(false)}
                className="text-xs text-fg-3"
              >
                Cancel
              </button>
            </span>
          ) : (
            <button
              onClick={() => setConfirmDelete(true)}
              className="rounded-md border border-edge p-1.5 text-fg-4 transition-colors hover:border-red-900/50 hover:text-red-400"
              title="Delete schedule"
            >
              <span className="material-symbols-outlined text-base">
                delete
              </span>
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
      className="rounded-xl border border-edge bg-surface/50 p-5"
    >
      <h3 className="mb-4 flex items-center gap-2 text-sm font-bold uppercase tracking-wider text-fg-2">
        <span className="material-symbols-outlined text-accent text-base">
          add_circle
        </span>
        Add Schedule
      </h3>
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
            className={`${inputCls} sm:col-span-2`}
          />
        )}
        <input
          type="text"
          value={repoUrl}
          onChange={(e) => setRepoUrl(e.target.value)}
          placeholder="Repo URL"
          required
          className={`${inputCls} sm:col-span-2`}
        />
        <textarea
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          placeholder="Prompt"
          required
          rows={3}
          className={`${inputCls} sm:col-span-2 resize-y`}
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
          className={inputCls}
        />
      </div>
      <button
        type="submit"
        disabled={createSchedule.isPending}
        className="mt-4 flex items-center gap-2 rounded-lg bg-accent px-5 py-2 text-sm font-bold text-page transition-all hover:bg-accent-hover disabled:opacity-50"
      >
        {createSchedule.isPending ? (
          <span className="material-symbols-outlined animate-spin text-base">
            progress_activity
          </span>
        ) : (
          <span className="material-symbols-outlined text-lg">add</span>
        )}
        Create Schedule
      </button>
    </form>
  );
}

function LoadingSkeleton() {
  return (
    <div className="space-y-3">
      {[1, 2, 3].map((i) => (
        <div key={i} className="h-16 animate-pulse rounded-xl bg-surface-alt" />
      ))}
    </div>
  );
}

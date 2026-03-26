import { useMemo } from "react";
import { useNavigate, Link } from "react-router";
import { usePageTitle } from "../hooks/usePageTitle";
import { useSessions } from "../hooks/useSessions";
import StatusBadge from "../components/StatusBadge";

const PIE_COLORS: Record<string, string> = {
  completed: "#00ff40",
  failed: "#ef4444",
  running: "#3b82f6",
  pending: "#eab308",
  cloning: "#06b6d4",
  awaiting_instruction: "#a855f7",
  reviewing: "#8b5cf6",
  creating_pr: "#14b8a6",
  pr_created: "#14b8a6",
  cancelling: "#f97316",
};

const ACTIVE_STATUSES = new Set([
  "pending",
  "cloning",
  "running",
  "reviewing",
  "creating_pr",
  "cancelling",
]);

export default function Dashboard() {
  usePageTitle("Dashboard");
  const navigate = useNavigate();
  const { data: sessions } = useSessions();

  const running = useMemo(
    () => sessions?.filter((t) => ACTIVE_STATUSES.has(t.status)).length ?? 0,
    [sessions],
  );
  const completed = useMemo(
    () => sessions?.filter((t) => t.status === "completed").length ?? 0,
    [sessions],
  );
  const failed = useMemo(
    () => sessions?.filter((t) => t.status === "failed").length ?? 0,
    [sessions],
  );
  const total = sessions?.length ?? 0;

  const sessionsByStatus = useMemo(() => {
    if (!sessions || sessions.length === 0) return [];
    const counts: Record<string, number> = {};
    for (const t of sessions) {
      counts[t.status] = (counts[t.status] || 0) + 1;
    }
    return Object.entries(counts)
      .map(([name, value]) => ({ name, value }))
      .sort((a, b) => b.value - a.value);
  }, [sessions]);

  const recentSessions = useMemo(() => {
    if (!sessions) return [];
    return [...sessions]
      .sort(
        (a, b) =>
          new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
      )
      .slice(0, 8);
  }, [sessions]);

  const sessionCount = sessions?.length ?? 0;

  // Compute pie chart conic gradient
  const pieGradient = useMemo(() => {
    if (sessionsByStatus.length === 0) return "";
    const total = sessionsByStatus.reduce((sum, s) => sum + s.value, 0);
    let acc = 0;
    const stops = sessionsByStatus.map((s) => {
      const start = acc;
      acc += (s.value / total) * 100;
      const color = PIE_COLORS[s.name] || "#6b7280";
      return `${color} ${start}% ${acc}%`;
    });
    return `conic-gradient(${stops.join(", ")})`;
  }, [sessionsByStatus]);

  return (
    <div className="mx-auto max-w-[1600px] space-y-6">
      {/* Header */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h2 className="text-3xl font-bold tracking-tight text-fg">
            Dashboard Overview
          </h2>
        </div>
        <button
          onClick={() => void navigate("/sessions/new")}
          className="group flex items-center justify-center gap-2 rounded-lg bg-accent px-6 py-3 font-bold text-page shadow-[0_0_20px_rgba(0,255,64,0.3)]  transition-all duration-300 hover:shadow-[0_0_30px_rgba(0,255,64,0.5)] hover:bg-accent-bold"
        >
          <span className="material-symbols-outlined text-xl transition-transform group-hover:rotate-90">
            add
          </span>
          <span className="tracking-wide">NEW SESSION</span>
        </button>
      </div>

      {/* Stat cards */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard
          label="Running"
          value={running}
          icon="play_circle"
          iconColor="text-blue-400/50 group-hover:text-blue-400"
          valueColor="group-hover:text-blue-400"
        />
        <StatCard
          label="Completed"
          value={completed}
          icon="check_circle"
          iconColor="text-accent/50 group-hover:text-accent"
          valueColor="group-hover:text-accent"
        />
        <StatCard
          label="Failed"
          value={failed}
          icon="error"
          iconColor="text-red-400/50 group-hover:text-red-400"
          valueColor="group-hover:text-red-400"
        />
        <StatCard
          label="Total"
          value={total}
          icon="summarize"
          iconColor="text-purple-400/50 group-hover:text-purple-400"
          valueColor="group-hover:text-purple-400"
        />
      </div>

      {/* Main content grid */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
        {/* Recent Sessions - 2 cols */}
        <div className="flex flex-col overflow-hidden rounded-xl border border-edge bg-surface-alt lg:col-span-2">
          <div className="flex items-center justify-between border-b border-edge bg-surface/70 px-6 py-4">
            <h3 className="flex items-center gap-2 font-bold text-fg">
              <span className="material-symbols-outlined text-sm text-accent">
                history
              </span>
              Recent Sessions
            </h3>
            <Link
              to="/sessions"
              className="flex items-center gap-1 text-xs font-medium text-accent/70 transition-colors hover:text-accent"
            >
              View all
              <span className="material-symbols-outlined text-sm">
                chevron_right
              </span>
            </Link>
          </div>
          <div className="flex-1">
            {recentSessions.length > 0 ? (
              <div className="divide-y divide-edge">
                {recentSessions.map((t) => (
                  <Link
                    key={t.id}
                    to={`/sessions/${t.id}`}
                    className="flex items-center gap-4 px-6 py-3 transition-colors hover:bg-white/5"
                  >
                    <span className="shrink-0 font-mono text-xs text-accent/60">
                      {t.id.slice(0, 8)}
                    </span>
                    <p className="min-w-0 flex-1 truncate text-sm text-fg-2">
                      {t.prompt}
                    </p>
                    <span className="hidden shrink-0 font-mono text-xs text-fg-4 sm:block">
                      {extractRepoName(t.repo_url)}
                    </span>
                    <StatusBadge status={t.status} />
                    <span className="shrink-0 text-xs text-fg-4">
                      {formatTimeAgo(t.created_at)}
                    </span>
                  </Link>
                ))}
              </div>
            ) : (
              <div className="flex flex-col items-center justify-center py-16 text-center">
                <span className="material-symbols-outlined mb-2 text-3xl text-fg-4">
                  task
                </span>
                <p className="text-sm text-fg-4">No sessions yet</p>
              </div>
            )}
          </div>
        </div>

        {/* Sessions by Status - 1 col */}
        <div className="flex flex-col overflow-hidden rounded-xl border border-edge bg-surface-alt">
          <div className="flex items-center justify-between border-b border-edge bg-surface/70 px-6 py-4">
            <h3 className="flex items-center gap-2 font-bold text-fg">
              <span className="material-symbols-outlined text-sm text-accent">
                donut_large
              </span>
              Sessions by Status
            </h3>
          </div>
          <div className="flex flex-1 flex-col items-center justify-center p-6">
            {sessionsByStatus.length > 0 ? (
              <>
                <div
                  className="relative mb-6 h-48 w-48 rounded-full"
                  style={{ background: pieGradient }}
                >
                  <div className="absolute inset-4 flex flex-col items-center justify-center rounded-full bg-surface-alt">
                    <span className="text-3xl font-bold tracking-tight text-fg">
                      {sessionCount.toLocaleString()}
                    </span>
                    <span className="font-mono text-xs text-fg-3">
                      TOTAL SESSIONS
                    </span>
                  </div>
                </div>
                <div className="w-full space-y-3">
                  {sessionsByStatus.map((s) => (
                    <div
                      key={s.name}
                      className="flex items-center justify-between text-sm"
                    >
                      <div className="flex items-center gap-2">
                        <div
                          className="h-3 w-3 rounded-sm"
                          style={{
                            backgroundColor: PIE_COLORS[s.name] || "#6b7280",
                            boxShadow:
                              s.name === "completed"
                                ? "0 0 5px rgba(0,255,64,0.5)"
                                : undefined,
                          }}
                        />
                        <span className="text-fg-2 capitalize">
                          {s.name.replace("_", " ")}
                        </span>
                      </div>
                      <span className="font-mono text-fg">
                        {s.value} (
                        {sessionCount > 0
                          ? Math.round((s.value / sessionCount) * 100)
                          : 0}
                        %)
                      </span>
                    </div>
                  ))}
                </div>
              </>
            ) : (
              <div className="text-center">
                <span className="material-symbols-outlined mb-2 text-3xl text-slate-700">
                  donut_large
                </span>
                <p className="text-sm text-fg-4">No session data yet</p>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

function formatTimeAgo(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const seconds = Math.floor(diff / 1000);
  if (seconds < 60) return "now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h`;
  const days = Math.floor(hours / 24);
  return `${days}d`;
}

function extractRepoName(repoUrl: string): string {
  return repoUrl
    .replace(/^https?:\/\//, "")
    .replace(/\.git$/, "")
    .split("/")
    .slice(-2)
    .join("/");
}

function StatCard({
  label,
  value,
  icon,
  iconColor,
  valueColor,
}: {
  label: string;
  value: number;
  icon: string;
  iconColor: string;
  valueColor: string;
}) {
  return (
    <div className="group flex flex-col justify-between gap-2 rounded-xl border border-edge bg-surface-alt p-5 transition-all duration-300 hover:border-accent hover:shadow-[0_0_15px_rgba(0,255,64,0.2)]">
      <div className="flex items-center justify-between">
        <p className="text-sm font-medium text-fg-3">{label}</p>
        <span
          className={`material-symbols-outlined transition-colors ${iconColor}`}
        >
          {icon}
        </span>
      </div>
      <div className="flex items-end justify-between">
        <p
          className={`font-mono text-3xl font-bold tracking-tight text-fg transition-colors ${valueColor}`}
        >
          {value.toLocaleString()}
        </p>
      </div>
    </div>
  );
}

import { useMemo } from "react";
import { useNavigate, Link } from "react-router";
import {
  Plus,
  Flame,
  CircleCheck,
  CircleX,
  Layers,
  ChevronRight,
  SquareTerminal,
  type LucideIcon,
} from "lucide-react";
import { usePageTitle } from "../hooks/usePageTitle";
import { useSessions } from "../hooks/useSessions";
import StatusBadge from "../components/StatusBadge";
import { formatTimeAgo } from "../lib/formatters";

/* Status → semantic token (heat semantics, see DESIGN.md) */
const STATUS_COLORS: Record<string, string> = {
  completed: "var(--th-ok)",
  failed: "var(--th-danger)",
  running: "var(--th-accent)",
  pending: "var(--th-warn)",
  cloning: "var(--th-info)",
  awaiting_instruction: "var(--th-accent)",
  reviewing: "var(--th-info)",
  creating_pr: "var(--th-info)",
  pr_created: "var(--th-info)",
  cancelling: "var(--th-warn)",
  canceled: "var(--th-fg-4)",
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

  // Compute donut chart conic gradient from semantic tokens
  const pieGradient = useMemo(() => {
    if (sessionsByStatus.length === 0) return "";
    const total = sessionsByStatus.reduce((sum, s) => sum + s.value, 0);
    let acc = 0;
    const stops = sessionsByStatus.map((s) => {
      const start = acc;
      acc += (s.value / total) * 100;
      const color = STATUS_COLORS[s.name] || "var(--th-fg-4)";
      return `${color} ${start}% ${acc}%`;
    });
    return `conic-gradient(${stops.join(", ")})`;
  }, [sessionsByStatus]);

  return (
    <div className="mx-auto max-w-[1600px] space-y-6">
      {/* Header */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p className="eyebrow mb-1">Operations</p>
          <h2 className="font-expanded text-2xl font-extrabold tracking-tight text-fg">
            Dashboard
          </h2>
        </div>
        <button
          onClick={() => void navigate("/sessions/new")}
          className="flex items-center justify-center gap-2 rounded-md bg-accent px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-accent-hover"
        >
          <Plus className="size-4" />
          New session
        </button>
      </div>

      {/* Stat cards */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard label="Active" value={running} icon={Flame} live />
        <StatCard label="Completed" value={completed} icon={CircleCheck} />
        <StatCard label="Failed" value={failed} icon={CircleX} />
        <StatCard label="Total" value={total} icon={Layers} />
      </div>

      {/* Main content grid */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
        {/* Recent sessions — 2 cols */}
        <div className="flex flex-col overflow-hidden rounded-md border border-edge bg-surface lg:col-span-2">
          <div className="flex items-center justify-between border-b border-edge px-5 py-3.5">
            <span className="eyebrow">Recent sessions</span>
            <Link
              to="/sessions"
              className="flex items-center gap-1 text-xs font-medium text-fg-3 transition-colors hover:text-accent"
            >
              View all
              <ChevronRight className="size-3.5" />
            </Link>
          </div>
          <div className="flex-1">
            {recentSessions.length > 0 ? (
              <div className="divide-y divide-edge">
                {recentSessions.map((t) => (
                  <Link
                    key={t.id}
                    to={`/sessions/${t.id}`}
                    className="flex items-center gap-4 px-5 py-3 transition-colors hover:bg-surface-alt"
                  >
                    <span className="shrink-0 font-mono text-xs text-fg-4">
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
                <SquareTerminal className="mb-3 size-6 text-fg-4" />
                <p className="text-sm text-fg-3">
                  No sessions yet. Start one to see it here.
                </p>
              </div>
            )}
          </div>
        </div>

        {/* By status — 1 col */}
        <div className="flex flex-col overflow-hidden rounded-md border border-edge bg-surface">
          <div className="border-b border-edge px-5 py-3.5">
            <span className="eyebrow">By status</span>
          </div>
          <div className="flex flex-1 flex-col items-center justify-center p-6">
            {sessionsByStatus.length > 0 ? (
              <>
                <div
                  className="relative mb-6 size-44 rounded-full"
                  style={{ background: pieGradient }}
                >
                  <div className="absolute inset-3 flex flex-col items-center justify-center rounded-full bg-surface">
                    <span className="font-mono text-3xl font-semibold tracking-tight text-fg">
                      {sessionCount.toLocaleString()}
                    </span>
                    <span className="eyebrow">Sessions</span>
                  </div>
                </div>
                <div className="w-full space-y-2.5">
                  {sessionsByStatus.map((s) => (
                    <div
                      key={s.name}
                      className="flex items-center justify-between text-sm"
                    >
                      <div className="flex items-center gap-2">
                        <span
                          className="size-2 rounded-full"
                          style={{
                            backgroundColor:
                              STATUS_COLORS[s.name] || "var(--th-fg-4)",
                          }}
                        />
                        <span className="text-fg-2 capitalize">
                          {s.name.replace(/_/g, " ")}
                        </span>
                      </div>
                      <span className="font-mono text-xs text-fg-3">
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
              <div className="flex flex-col items-center text-center">
                <Layers className="mb-3 size-6 text-fg-4" />
                <p className="text-sm text-fg-3">No session data yet</p>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
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
  icon: Icon,
  live = false,
}: {
  label: string;
  value: number;
  icon: LucideIcon;
  live?: boolean;
}) {
  const isHot = live && value > 0;
  return (
    <div className="flex flex-col justify-between gap-3 rounded-md border border-edge bg-surface p-5">
      <div className="flex items-center justify-between">
        <p className="text-sm font-medium text-fg-3">{label}</p>
        <Icon className={`size-4 ${isHot ? "text-accent" : "text-fg-4"}`} />
      </div>
      <div className="flex items-center gap-2.5">
        <p className="font-mono text-3xl font-semibold tracking-tight text-fg">
          {value.toLocaleString()}
        </p>
        {isHot && (
          <span className="animate-ember size-2 rounded-full bg-accent" />
        )}
      </div>
    </div>
  );
}

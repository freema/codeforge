import { useState, useMemo } from "react";
import { useNavigate } from "react-router";
import {
  Ban,
  CircleCheck,
  CircleX,
  FileDiff,
  FolderGit2,
  GitMerge,
  Hourglass,
  List,
  MessageSquare,
  Play,
  Plus,
  RefreshCw,
  Search,
  SquareTerminal,
  type LucideIcon,
} from "lucide-react";
import { usePageTitle } from "../hooks/usePageTitle";
import { useSessions } from "../hooks/useSessions";
import StatusBadge from "../components/StatusBadge";
import { formatTimeAgo } from "../lib/formatters";
import type { Session, SessionStatus } from "../types";

const STATUS_FILTERS: {
  label: string;
  value: SessionStatus | "all";
  icon: LucideIcon;
}[] = [
  { label: "All", value: "all", icon: List },
  { label: "Queued", value: "pending", icon: Hourglass },
  { label: "Running", value: "running", icon: Play },
  { label: "Completed", value: "completed", icon: CircleCheck },
  { label: "Failed", value: "failed", icon: CircleX },
  { label: "Canceled", value: "canceled", icon: Ban },
  { label: "Awaiting", value: "awaiting_instruction", icon: MessageSquare },
  { label: "PR created", value: "pr_created", icon: GitMerge },
];

export default function SessionList() {
  usePageTitle("Sessions");
  const navigate = useNavigate();
  const { data: sessions = [], isLoading, refetch } = useSessions();
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<SessionStatus | "all">(
    "all",
  );

  const sortedSessions = useMemo(() => {
    return [...sessions].sort(
      (a, b) =>
        new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
    );
  }, [sessions]);

  const filteredSessions = useMemo(() => {
    return sortedSessions.filter((t) => {
      if (statusFilter !== "all" && t.status !== statusFilter) return false;
      if (search) {
        const q = search.toLowerCase();
        return (
          t.repo_url.toLowerCase().includes(q) ||
          t.prompt.toLowerCase().includes(q) ||
          t.id.toLowerCase().includes(q)
        );
      }
      return true;
    });
  }, [sortedSessions, statusFilter, search]);

  const statusCounts = useMemo(() => {
    const counts: Record<string, number> = {};
    for (const t of sessions) {
      counts[t.status] = (counts[t.status] || 0) + 1;
    }
    return counts;
  }, [sessions]);

  return (
    <div className="mx-auto max-w-6xl space-y-6">
      {/* Header */}
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="eyebrow mb-1">Operations</p>
          <h2 className="font-expanded text-2xl font-extrabold tracking-tight text-fg">
            Sessions
          </h2>
        </div>
        <div className="flex gap-3">
          <button
            onClick={() => void refetch()}
            className="flex items-center gap-2 rounded-md border border-edge bg-surface px-4 py-2 text-sm font-medium text-fg-2 transition-colors hover:border-fg-4 hover:text-fg"
          >
            <RefreshCw
              className={`size-4 ${isLoading ? "animate-spin" : ""}`}
            />
            Refresh
          </button>
          <button
            onClick={() => void navigate("/sessions/new")}
            className="flex items-center gap-2 rounded-md bg-accent px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-accent-hover"
          >
            <Plus className="size-4" />
            New session
          </button>
        </div>
      </div>

      {/* Search and filters */}
      <div className="flex flex-col gap-3 md:flex-row">
        <div className="relative flex-1">
          <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-fg-4" />
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by ID, repo, or prompt"
            className="w-full rounded-md border border-edge bg-input py-2 pr-3 pl-9 font-mono text-sm text-fg placeholder-fg-4 transition-colors focus:border-accent focus:outline-none"
          />
        </div>
        <div className="flex gap-2 overflow-x-auto pb-1 md:pb-0">
          {STATUS_FILTERS.map((f) => {
            const count =
              f.value === "all"
                ? sessions.length
                : (statusCounts[f.value] ?? 0);
            const active = statusFilter === f.value;
            const Icon = f.icon;
            return (
              <button
                key={f.value}
                onClick={() => setStatusFilter(f.value)}
                className={`flex items-center gap-1.5 rounded-md border px-3 py-2 font-mono text-xs whitespace-nowrap transition-colors ${
                  active
                    ? "border-accent-muted bg-accent-soft text-accent"
                    : "border-edge bg-surface text-fg-3 hover:text-fg"
                }`}
              >
                <Icon className="size-3.5" />
                {f.label}
                {count > 0 && f.value !== "all" && (
                  <span
                    className={`rounded-[4px] px-1.5 py-0.5 text-[10px] ${
                      active
                        ? "bg-accent/15 text-accent"
                        : "bg-surface-alt text-fg-3"
                    }`}
                  >
                    {count}
                  </span>
                )}
              </button>
            );
          })}
        </div>
      </div>

      {/* Session list */}
      {!isLoading && sessions.length === 0 ? (
        <EmptyState onNew={() => void navigate("/sessions/new")} />
      ) : filteredSessions.length === 0 ? (
        <p className="py-12 text-center text-sm text-fg-3">
          No sessions match your filters.
        </p>
      ) : (
        <div className="flex flex-col gap-3">
          {filteredSessions.map((session) => (
            <SessionRow
              key={session.id}
              session={session}
              onClick={() => void navigate(`/sessions/${session.id}`)}
            />
          ))}
        </div>
      )}

      {/* Pagination info */}
      {filteredSessions.length > 0 && (
        <div className="flex items-center justify-between border-t border-edge pt-6">
          <span className="text-xs text-fg-4">
            Showing {filteredSessions.length} of {sessions.length} sessions
          </span>
        </div>
      )}
    </div>
  );
}

function SessionRow({
  session,
  onClick,
}: {
  session: Session;
  onClick: () => void;
}) {
  const repoShort = session.repo_url
    .replace(/^https?:\/\//, "")
    .replace(/\.git$/, "");

  const isRunning =
    session.status === "running" || session.status === "cloning";
  const isFailed = session.status === "failed";

  return (
    <button
      onClick={onClick}
      className="relative flex w-full cursor-pointer flex-col gap-4 overflow-hidden rounded-md border border-edge bg-surface p-5 text-left transition-colors hover:border-fg-4 md:flex-row"
    >
      {/* Left status rail */}
      {isRunning && (
        <span className="absolute inset-y-0 left-0 w-0.5 bg-accent" />
      )}
      {isFailed && (
        <span className="absolute inset-y-0 left-0 w-0.5 bg-danger" />
      )}

      {/* Session ID & timing */}
      <div className="flex min-w-[140px] items-center gap-3 md:flex-col md:items-start md:gap-1">
        <span className="font-mono text-sm text-fg-4">
          {session.id.slice(0, 8)}
        </span>
        <span className="text-xs text-fg-4">
          {formatTimeAgo(session.created_at)}
        </span>
      </div>

      {/* Content */}
      <div className="flex min-w-0 flex-1 flex-col gap-1.5">
        <div className="flex items-center gap-1.5 font-mono text-xs text-fg-3">
          <FolderGit2 className="size-3.5 shrink-0 text-fg-4" />
          <span className="truncate">{repoShort}</span>
        </div>
        <p className="text-sm text-fg-2">
          {session.prompt.length > 150
            ? session.prompt.slice(0, 150) + "..."
            : session.prompt}
        </p>
        {session.error && (
          <p className="truncate font-mono text-xs text-danger">
            {session.error.slice(0, 100)}
          </p>
        )}
      </div>

      {/* Status + meta */}
      <div className="flex min-w-[120px] items-end justify-between gap-2 md:flex-col md:items-end md:justify-center">
        <StatusBadge status={session.status} />
        <div className="flex items-center gap-2">
          {session.session_type && (
            <span className="rounded-[4px] border border-edge bg-surface-alt px-1.5 py-0.5 font-mono text-[10px] tracking-wider text-fg-3 uppercase">
              {session.session_type}
            </span>
          )}
          {session.changes_summary &&
            (session.changes_summary.files_modified > 0 ||
              session.changes_summary.files_created > 0 ||
              session.changes_summary.files_deleted > 0) && (
              <DiffStats
                diffStats={session.changes_summary.diff_stats}
                filesCount={
                  session.changes_summary.files_modified +
                  session.changes_summary.files_created +
                  session.changes_summary.files_deleted
                }
              />
            )}
          {session.pr_url && (
            <span className="flex items-center gap-1 font-mono text-[10px] text-info">
              <GitMerge className="size-3" />
              PR
            </span>
          )}
        </div>
      </div>
    </button>
  );
}

function DiffStats({
  diffStats,
  filesCount,
}: {
  diffStats?: string;
  filesCount: number;
}) {
  if (diffStats) {
    const match = diffStats.match(/\+(\d+)\s+-(\d+)/);
    if (match && (match[1] !== "0" || match[2] !== "0")) {
      return (
        <span className="flex items-center gap-1.5 font-mono text-xs">
          <FileDiff className="size-3.5 text-fg-4" />
          <span className="text-ok">+{match[1]}</span>
          <span className="text-danger">-{match[2]}</span>
        </span>
      );
    }
  }
  if (filesCount > 0) {
    return (
      <span className="flex items-center gap-1 font-mono text-xs text-fg-4">
        <FileDiff className="size-3.5" />
        {filesCount} files
      </span>
    );
  }
  return null;
}

function EmptyState({ onNew }: { onNew: () => void }) {
  return (
    <div className="flex flex-col items-center justify-center py-20 text-center">
      <SquareTerminal className="mb-3 size-6 text-fg-4" strokeWidth={1.75} />
      <p className="mb-6 text-sm text-fg-3">
        No sessions yet. Create one to get started.
      </p>
      <button
        onClick={onNew}
        className="flex items-center gap-2 rounded-md bg-accent px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-accent-hover"
      >
        <Plus className="size-4" />
        New session
      </button>
    </div>
  );
}

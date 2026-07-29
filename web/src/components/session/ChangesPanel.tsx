import { Loader2, TriangleAlert } from "lucide-react";
import { useSessionDiff } from "../../hooks/useSessionDiff";
import { ApiError } from "../../lib/api";
import type { FileDiff } from "../../types";

const STATUS_STYLES: Record<
  FileDiff["status"],
  { letter: string; cls: string }
> = {
  added: { letter: "A", cls: "text-ok" },
  modified: { letter: "M", cls: "text-info" },
  deleted: { letter: "D", cls: "text-danger" },
  renamed: { letter: "R", cls: "text-warn" },
};

/** Uncommitted workspace changes: file list + unified diff. */
export function ChangesPanel({ sessionId }: { sessionId: string }) {
  const { data, isLoading, error } = useSessionDiff(sessionId);

  if (isLoading) {
    return (
      <div className="mt-3 flex items-center gap-3 rounded-md border border-edge bg-surface px-3 py-2.5 text-fg-4">
        <Loader2 className="size-4 animate-spin" />
        <span className="text-xs">Loading changes…</span>
      </div>
    );
  }

  if (error instanceof ApiError && error.status === 404) {
    return (
      <p className="mt-3 px-2 font-mono text-xs text-fg-4">
        Workspace expired — changes are no longer available.
      </p>
    );
  }

  if (error) {
    return (
      <p className="mt-3 px-2 font-mono text-xs text-danger">
        Failed to load changes:{" "}
        {error instanceof Error ? error.message : "unknown error"}
      </p>
    );
  }

  if (!data) return null;

  const files = data.files ?? [];

  return (
    <div className="mt-3 overflow-hidden rounded-md border border-edge bg-surface">
      {/* Summary header */}
      <div className="flex items-center gap-3 border-b border-edge px-3 py-2">
        <span className="eyebrow">Changes</span>
        <span className="font-mono text-xs text-fg-3">
          {files.length} file{files.length === 1 ? "" : "s"}
        </span>
        <span className="font-mono text-xs">
          <span className="text-ok">+{data.total_additions}</span>{" "}
          <span className="text-danger">-{data.total_deletions}</span>
        </span>
      </div>

      {data.truncated && (
        <div className="flex items-center gap-2 border-b border-edge bg-warn/10 px-3 py-1.5">
          <TriangleAlert className="size-3.5 shrink-0 text-warn" />
          <span className="text-xs text-warn">
            Diff truncated — showing the first part only.
          </span>
        </div>
      )}

      {files.length === 0 ? (
        <p className="px-3 py-4 text-center text-xs text-fg-4">
          No uncommitted changes in the workspace.
        </p>
      ) : (
        <div className="max-h-48 overflow-y-auto border-b border-edge">
          {files.map((f) => {
            const style = STATUS_STYLES[f.status] ?? {
              letter: "?",
              cls: "text-fg-3",
            };
            return (
              <div
                key={f.path}
                className="flex items-center gap-2 border-b border-edge px-3 py-1.5 font-mono text-xs last:border-b-0"
              >
                <span
                  className={`w-4 shrink-0 text-center font-semibold ${style.cls}`}
                  title={f.status}
                >
                  {style.letter}
                </span>
                <span className="min-w-0 flex-1 truncate text-fg-2">
                  {f.path}
                </span>
                <span className="shrink-0 text-ok">+{f.additions}</span>
                <span className="shrink-0 text-danger">-{f.deletions}</span>
              </div>
            );
          })}
        </div>
      )}

      {data.diff && <DiffView diff={data.diff} />}
    </div>
  );
}

type DiffLineKind = "file" | "meta" | "hunk" | "add" | "del" | "context";

function classifyLine(line: string): DiffLineKind {
  if (line.startsWith("diff --git")) return "file";
  if (
    line.startsWith("index ") ||
    line.startsWith("+++ ") ||
    line.startsWith("--- ") ||
    line.startsWith("new file") ||
    line.startsWith("deleted file") ||
    line.startsWith("old mode") ||
    line.startsWith("new mode") ||
    line.startsWith("rename ") ||
    line.startsWith("similarity ") ||
    line.startsWith("Binary ") ||
    line.startsWith("\\ No newline")
  ) {
    return "meta";
  }
  if (line.startsWith("@@")) return "hunk";
  if (line.startsWith("+")) return "add";
  if (line.startsWith("-")) return "del";
  return "context";
}

const LINE_CLASSES: Record<DiffLineKind, string> = {
  file: "mt-2 border-t border-edge pt-2 font-semibold text-fg-2 first:mt-0 first:border-t-0 first:pt-0",
  meta: "text-fg-4",
  hunk: "bg-info/10 text-info",
  add: "bg-ok/10 text-ok",
  del: "bg-danger/10 text-danger",
  context: "text-fg-4",
};

/** Unified diff, colored per hunk with the same palette as DiffContent. */
function DiffView({ diff }: { diff: string }) {
  const lines = diff.replace(/\n$/, "").split("\n");

  return (
    <div className="max-h-[28rem] overflow-auto bg-surface-alt">
      <div className="w-max min-w-full p-3 font-mono text-[11px] leading-relaxed">
        {lines.map((line, i) => (
          <div
            key={i}
            className={`-mx-1 whitespace-pre px-1 ${LINE_CLASSES[classifyLine(line)]}`}
          >
            {line || " "}
          </div>
        ))}
      </div>
    </div>
  );
}

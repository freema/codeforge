import { useState } from "react";
import StatusBadge from "../StatusBadge";
import type { SessionStatus } from "../../types";

export function PromptCard({ prompt, ts }: { prompt: string; ts: string }) {
  const time = new Date(ts).toLocaleTimeString("en-US", {
    hour12: false,
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });

  return (
    <div className="mb-1 flex items-start gap-2 rounded-lg border border-blue-500/20 bg-blue-500/5 px-3 py-2">
      <span className="w-14 shrink-0 font-mono text-[10px] text-fg-4 pt-0.5">
        {time}
      </span>
      <span className="material-symbols-outlined text-[14px] text-blue-400 mt-0.5">
        person
      </span>
      <div className="min-w-0 flex-1">
        <span className="text-[10px] font-bold uppercase tracking-wider text-blue-400">
          You
        </span>
        <p className="mt-0.5 text-xs text-fg whitespace-pre-wrap">{prompt}</p>
      </div>
    </div>
  );
}

export function IterationsHistory({
  iterations,
}: {
  iterations: {
    number: number;
    prompt: string;
    result?: string;
    status: SessionStatus;
  }[];
}) {
  const [expanded, setExpanded] = useState(false);
  const past = iterations.slice(0, -1);

  return (
    <div className="mt-2">
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left transition-colors hover:bg-surface/30"
      >
        <span className="material-symbols-outlined text-[14px] text-fg-4">
          history
        </span>
        <span className="text-xs font-bold text-fg-3">
          {past.length} previous iteration{past.length > 1 ? "s" : ""}
        </span>
        <span
          className="material-symbols-outlined ml-auto text-[14px] text-fg-4 transition-transform"
          style={{ transform: expanded ? "rotate(180deg)" : "none" }}
        >
          expand_more
        </span>
      </button>
      {expanded && (
        <div className="ml-4 mt-1 space-y-2 border-l-2 border-edge pl-3">
          {past.map((iter) => (
            <div
              key={iter.number}
              className="rounded-lg border border-accent/10 bg-surface/30 p-2.5"
            >
              <div className="mb-1 flex items-center justify-between">
                <span className="text-[10px] font-bold text-accent/70">
                  #{iter.number}
                </span>
                <StatusBadge status={iter.status} />
              </div>
              <p className="text-xs text-fg-2">{iter.prompt}</p>
              {iter.result && (
                <p className="mt-1 truncate text-xs text-fg-4">{iter.result}</p>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

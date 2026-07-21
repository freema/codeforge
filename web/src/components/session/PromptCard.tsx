import { useState } from "react";
import { ChevronDown, History, User } from "lucide-react";
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
    <div className="mb-1 flex items-start gap-2 rounded-md border border-info/30 bg-info/10 px-3 py-2">
      <span className="w-14 shrink-0 pt-0.5 font-mono text-[10px] text-fg-4">
        {time}
      </span>
      <User className="mt-0.5 size-3.5 shrink-0 text-info" />
      <div className="min-w-0 flex-1">
        <span className="font-mono text-[10px] font-medium tracking-wider text-info uppercase">
          You
        </span>
        <p className="mt-0.5 text-xs whitespace-pre-wrap text-fg">{prompt}</p>
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
        className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left transition-colors hover:bg-surface-alt"
      >
        <History className="size-3.5 shrink-0 text-fg-4" />
        <span className="text-xs font-medium text-fg-3">
          {past.length} previous iteration{past.length > 1 ? "s" : ""}
        </span>
        <ChevronDown
          className="ml-auto size-3.5 shrink-0 text-fg-4 transition-transform"
          style={{ transform: expanded ? "rotate(180deg)" : "none" }}
        />
      </button>
      {expanded && (
        <div className="mt-1 ml-4 space-y-2 border-l-2 border-edge pl-3">
          {past.map((iter) => (
            <div
              key={iter.number}
              className="rounded-md border border-edge bg-surface-alt p-2.5"
            >
              <div className="mb-1 flex items-center justify-between">
                <span className="font-mono text-[10px] text-fg-3">
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

import { useState } from "react";

export function ThinkingBlock({
  ts,
  content,
}: {
  ts: string;
  content: string;
}) {
  const [expanded, setExpanded] = useState(false);
  const preview =
    content.length > 100 ? content.slice(0, 100) + "..." : content;

  return (
    <div className="mb-0.5">
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex w-full items-start gap-2 rounded px-2 py-1 text-left transition-colors hover:bg-surface/30"
      >
        <span className="w-14 shrink-0 font-mono text-[10px] text-fg-4 pt-0.5">
          {ts}
        </span>
        <span className="material-symbols-outlined text-[14px] text-fg-4 mt-0.5 italic">
          psychology
        </span>
        <span className="flex-1 text-[11px] text-fg-4 italic truncate">
          {expanded ? "" : preview}
        </span>
        <span
          className="material-symbols-outlined text-[12px] text-fg-4"
          style={{ transform: expanded ? "rotate(180deg)" : "none" }}
        >
          expand_more
        </span>
      </button>
      {expanded && (
        <div className="ml-[72px] mr-2 mb-1 rounded border-l-2 border-edge bg-surface/20 px-3 py-2 overflow-x-auto">
          <pre className="font-mono text-[11px] text-fg-4 italic whitespace-pre-wrap break-words leading-relaxed">
            {content}
          </pre>
        </div>
      )}
    </div>
  );
}

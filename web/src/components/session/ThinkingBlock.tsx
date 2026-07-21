import { useState } from "react";
import { Brain, ChevronDown } from "lucide-react";

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
        className="flex w-full items-start gap-2 rounded-md px-2 py-1 text-left transition-colors hover:bg-surface-alt"
      >
        <span className="w-14 shrink-0 pt-0.5 font-mono text-[10px] text-fg-4">
          {ts}
        </span>
        <Brain className="mt-0.5 size-3.5 shrink-0 text-info/70" />
        <span className="flex-1 truncate font-mono text-[11px] text-fg-4 italic">
          {expanded ? "Thinking" : preview}
        </span>
        <ChevronDown
          className="size-3.5 shrink-0 text-fg-4 transition-transform"
          style={{ transform: expanded ? "rotate(180deg)" : "none" }}
        />
      </button>
      {expanded && (
        <div className="mr-2 mb-1 ml-[72px] overflow-x-auto rounded-md border-l-2 border-info/30 bg-surface-alt px-3 py-2">
          <pre className="font-mono text-[11px] leading-relaxed whitespace-pre-wrap break-words text-fg-3 italic">
            {content}
          </pre>
        </div>
      )}
    </div>
  );
}

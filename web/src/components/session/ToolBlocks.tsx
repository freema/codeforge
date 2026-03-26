import { useState } from "react";
import { getToolDisplay } from "../../lib/streamFormatters";

export function ToolUseBlock({
  ts,
  toolName,
  toolInput,
  content,
  defaultExpanded = false,
}: {
  ts: string;
  toolName: string;
  toolInput?: Record<string, unknown>;
  content: string;
  defaultExpanded?: boolean;
}) {
  const [expanded, setExpanded] = useState(defaultExpanded);

  const isMCP = toolName.startsWith("mcp__");
  const { icon, label, detail } = getToolDisplay(toolName, toolInput, content);
  const hasContent = content.length > 0;
  const isEdit =
    toolName.toLowerCase() === "edit" ||
    toolName.toLowerCase().includes("edit");
  const mcpColor = isMCP ? "text-cyan-400" : "text-purple-400";

  return (
    <div className="mt-1 mb-0.5">
      <button
        onClick={() => hasContent && setExpanded(!expanded)}
        className={`flex w-full items-center gap-2 rounded px-2 py-1.5 text-left transition-colors ${hasContent ? (isMCP ? "hover:bg-cyan-500/5" : "hover:bg-purple-500/5") + " cursor-pointer" : "cursor-default"}`}
      >
        <span className="w-14 shrink-0 font-mono text-[10px] text-fg-4">
          {ts}
        </span>
        <span className={`material-symbols-outlined text-[14px] ${mcpColor}`}>
          {icon}
        </span>
        <div className="flex-1 min-w-0 flex items-center gap-2">
          {isMCP && (
            <span className="rounded border border-cyan-500/30 bg-cyan-500/10 px-1.5 py-0.5 text-[9px] font-bold uppercase tracking-wider text-cyan-400">
              MCP
            </span>
          )}
          <span className={`text-xs font-bold ${mcpColor}`}>{label}</span>
          {detail && (
            <span className="truncate text-xs text-fg-3 font-mono">
              {detail}
            </span>
          )}
        </div>
        {hasContent && (
          <span
            className="material-symbols-outlined text-[14px] text-fg-4 transition-transform"
            style={{ transform: expanded ? "rotate(180deg)" : "none" }}
          >
            expand_more
          </span>
        )}
      </button>
      {expanded && content && (
        <div
          className={`ml-[72px] mr-2 mb-1 rounded-b border-l-2 overflow-x-auto ${isMCP ? "border-cyan-500/30 bg-cyan-500/5" : "border-purple-500/30 bg-purple-500/5"}`}
        >
          {isEdit ? (
            <DiffContent content={content} />
          ) : (
            <pre className="p-3 text-[11px] font-mono text-fg-3 whitespace-pre-wrap break-words leading-relaxed">
              {content}
            </pre>
          )}
        </div>
      )}
    </div>
  );
}

function DiffContent({ content }: { content: string }) {
  const lines = content.split("\n");
  return (
    <div className="p-3 text-[11px] font-mono leading-relaxed">
      {lines.map((line, i) => {
        if (line.startsWith("+ ")) {
          return (
            <div
              key={i}
              className="text-accent/90 bg-accent/5 px-1 -mx-1 rounded-sm"
            >
              {line}
            </div>
          );
        }
        if (line.startsWith("- ")) {
          return (
            <div
              key={i}
              className="text-red-400/90 bg-red-400/5 px-1 -mx-1 rounded-sm"
            >
              {line}
            </div>
          );
        }
        return (
          <div key={i} className="text-fg-4">
            {line}
          </div>
        );
      })}
    </div>
  );
}

export function ToolResultBlock({ content }: { content: string }) {
  const [expanded, setExpanded] = useState(false);

  if (!content || content === "(tool output too large to display)") return null;

  const lineCount = content.split("\n").length;

  // Short results (few lines): show inline, no expand needed
  if (lineCount <= 4) {
    return (
      <div className="ml-[72px] mr-2 mb-1 rounded border-l-2 border-fg-4/20 bg-surface/30 px-3 py-1.5">
        <pre className="font-mono text-[11px] text-fg-4 whitespace-pre-wrap break-words leading-relaxed">
          {content}
        </pre>
      </div>
    );
  }

  // Long results: collapsed by default, expandable
  const preview = content.split("\n").slice(0, 3).join("\n");

  return (
    <div className="ml-[72px] mr-2 mb-1 rounded border-l-2 border-fg-4/20 bg-surface/30 overflow-hidden">
      <pre className="px-3 py-1.5 font-mono text-[11px] text-fg-4 whitespace-pre-wrap break-words leading-relaxed">
        {expanded ? content : preview}
      </pre>
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex w-full items-center gap-1 border-t border-edge/50 px-3 py-1 text-[10px] text-purple-400 hover:bg-purple-500/5 transition-colors"
      >
        <span className="material-symbols-outlined text-xs">
          {expanded ? "expand_less" : "expand_more"}
        </span>
        {expanded ? "Collapse" : `Show all (${lineCount} lines)`}
      </button>
    </div>
  );
}

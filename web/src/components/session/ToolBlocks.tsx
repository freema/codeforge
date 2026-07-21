import { useState } from "react";
import {
  ChevronDown,
  ChevronUp,
  FilePen,
  FileText,
  FolderOpen,
  ListChecks,
  Search,
  Server,
  SquarePen,
  SquareTerminal,
  Wrench,
  type LucideIcon,
} from "lucide-react";
import { getToolDisplay } from "../../lib/streamFormatters";

/* getToolDisplay returns legacy icon-name strings — map them to lucide */
const TOOL_ICONS: Record<string, LucideIcon> = {
  description: FileText,
  checklist: ListChecks,
  edit_document: FilePen,
  edit: SquarePen,
  terminal: SquareTerminal,
  search: Search,
  folder_open: FolderOpen,
  dns: Server,
  build: Wrench,
};

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
  const Icon = TOOL_ICONS[icon] ?? Wrench;
  const hasContent = content.length > 0;
  const isEdit =
    toolName.toLowerCase() === "edit" ||
    toolName.toLowerCase().includes("edit");

  return (
    <div className="mt-1 mb-0.5">
      <button
        onClick={() => hasContent && setExpanded(!expanded)}
        className={`flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left transition-colors ${hasContent ? "cursor-pointer hover:bg-surface-alt" : "cursor-default"}`}
      >
        <span className="w-14 shrink-0 font-mono text-[10px] text-fg-4">
          {ts}
        </span>
        <Icon className="size-3.5 shrink-0 text-fg-3" />
        <div className="flex min-w-0 flex-1 items-center gap-2">
          {isMCP && (
            <span className="rounded-[4px] border border-info/30 bg-info/10 px-1.5 py-0.5 font-mono text-[9px] font-medium tracking-wider text-info uppercase">
              MCP
            </span>
          )}
          <span className="font-mono text-xs font-medium text-fg-2">
            {label}
          </span>
          {detail && (
            <span className="truncate font-mono text-xs text-fg-3">
              {detail}
            </span>
          )}
        </div>
        {hasContent && (
          <ChevronDown
            className="size-3.5 shrink-0 text-fg-4 transition-transform"
            style={{ transform: expanded ? "rotate(180deg)" : "none" }}
          />
        )}
      </button>
      {expanded && content && (
        <div
          className={`mr-2 mb-1 ml-[72px] overflow-x-auto rounded-md border-l-2 bg-surface-alt ${isMCP ? "border-info/30" : "border-edge"}`}
        >
          {isEdit ? (
            <DiffContent content={content} />
          ) : (
            <pre className="p-3 font-mono text-[11px] leading-relaxed whitespace-pre-wrap break-words text-fg-3">
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
    <div className="p-3 font-mono text-[11px] leading-relaxed">
      {lines.map((line, i) => {
        if (line.startsWith("+ ")) {
          return (
            <div key={i} className="-mx-1 rounded-sm bg-ok/10 px-1 text-ok">
              {line}
            </div>
          );
        }
        if (line.startsWith("- ")) {
          return (
            <div
              key={i}
              className="-mx-1 rounded-sm bg-danger/10 px-1 text-danger"
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
      <div className="mr-2 mb-1 ml-[72px] rounded-md border-l-2 border-edge bg-surface-alt px-3 py-1.5">
        <pre className="font-mono text-[11px] leading-relaxed whitespace-pre-wrap break-words text-fg-4">
          {content}
        </pre>
      </div>
    );
  }

  // Long results: collapsed by default, expandable
  const preview = content.split("\n").slice(0, 3).join("\n");

  return (
    <div className="mr-2 mb-1 ml-[72px] overflow-hidden rounded-md border-l-2 border-edge bg-surface-alt">
      <pre className="px-3 py-1.5 font-mono text-[11px] leading-relaxed whitespace-pre-wrap break-words text-fg-4">
        {expanded ? content : preview}
      </pre>
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex w-full items-center gap-1 border-t border-edge px-3 py-1 font-mono text-[10px] text-fg-3 transition-colors hover:bg-surface hover:text-fg"
      >
        {expanded ? (
          <ChevronUp className="size-3" />
        ) : (
          <ChevronDown className="size-3" />
        )}
        {expanded ? "Collapse" : `Show all (${lineCount} lines)`}
      </button>
    </div>
  );
}

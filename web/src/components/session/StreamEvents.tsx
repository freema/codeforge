import { useMemo } from "react";
import {
  Circle,
  CircleAlert,
  CircleCheck,
  GitCommitHorizontal,
  Info,
  ListChecks,
  Loader2,
  SquareTerminal,
  User,
} from "lucide-react";
import MarkdownText from "../MarkdownText";
import { ToolUseBlock, ToolResultBlock } from "./ToolBlocks";
import { ThinkingBlock } from "./ThinkingBlock";
import {
  getContent,
  formatSystemEvent,
  formatStreamSystemEvent,
  formatToolExpandedContent,
  extractToolName,
  extractToolFromRaw,
  extractToolResultContent,
} from "../../lib/streamFormatters";
import type { StreamEvent } from "../../types";

/** Helper: extract TodoWrite todos from a stream event, or null */
function extractTodoWrite(
  event: StreamEvent,
): Array<{ content: string; status: string }> | null {
  const data = event.data as Record<string, unknown> | string;
  if (typeof data !== "object" || data === null) return null;
  if (data.type !== "tool_use") return null;
  const raw = data.raw as Record<string, unknown> | undefined;
  if (!raw) return null;
  const message = raw.message as Record<string, unknown> | undefined;
  const contentArr = (message?.content ?? raw.content) as
    | Array<Record<string, unknown>>
    | undefined;
  if (!contentArr || !Array.isArray(contentArr)) return null;
  const block = contentArr.find((c) => c.name === "TodoWrite");
  if (!block) return null;
  const input = block.input as Record<string, unknown> | undefined;
  return (input?.todos as Array<{ content: string; status: string }>) ?? null;
}

export function StreamEvents({
  events,
  isActive,
}: {
  events: StreamEvent[];
  isActive: boolean;
}) {
  // Find all TodoWrite event indices and compute latest plan state
  const planData = useMemo(() => {
    const indices: number[] = [];
    let latestTodos: Array<{ content: string; status: string }> | null = null;

    for (let i = 0; i < events.length; i++) {
      const todos = extractTodoWrite(events[i]!);
      if (todos) {
        indices.push(i);
        latestTodos = todos;
      }
    }

    return {
      todoWriteIndices: new Set(indices),
      latestTodos,
      firstIndex: indices[0] ?? -1,
    };
  }, [events]);

  return (
    <>
      {events.map((event, i) => {
        // TodoWrite events: show the plan block at the FIRST occurrence, skip the rest
        if (planData.todoWriteIndices.has(i)) {
          if (i === planData.firstIndex && planData.latestTodos) {
            return (
              <PlanProgressBlock
                key={i}
                todos={planData.latestTodos}
                isActive={isActive}
              />
            );
          }
          return null; // skip subsequent TodoWrite events
        }
        return <TerminalEvent key={i} event={event} />;
      })}
    </>
  );
}

function PlanProgressBlock({
  todos,
  isActive,
}: {
  todos: Array<{ content: string; status: string }>;
  isActive: boolean;
}) {
  const completed = todos.filter((t) => t.status === "completed").length;
  const inProgress = todos.filter((t) => t.status === "in_progress").length;
  const total = todos.length;
  const pct = total > 0 ? Math.round((completed / total) * 100) : 0;

  return (
    <div className="my-2 overflow-hidden rounded-md border border-edge bg-surface-alt">
      {/* Header with progress */}
      <div className="flex items-center gap-3 border-b border-edge px-3 py-2">
        <ListChecks className="size-4 shrink-0 text-accent" />
        <span className="font-mono text-[10px] font-medium tracking-[0.14em] text-fg-2 uppercase">
          Plan
        </span>
        <span className="ml-auto font-mono text-[10px] text-fg-3">
          {completed}/{total}
        </span>
        {/* Mini progress bar */}
        <div className="h-1.5 w-20 overflow-hidden rounded-full bg-edge">
          <div
            className="h-full rounded-full bg-accent transition-all duration-500"
            style={{ width: `${pct}%` }}
          />
        </div>
        {pct === 100 && <CircleCheck className="size-3.5 shrink-0 text-ok" />}
        {pct < 100 && isActive && inProgress > 0 && (
          <Loader2 className="size-3 shrink-0 animate-spin text-accent" />
        )}
      </div>

      {/* Todo items */}
      <div className="px-3 py-1.5">
        {todos.map((todo, i) => {
          const isCompleted = todo.status === "completed";
          const isInProg = todo.status === "in_progress";
          return (
            <div
              key={i}
              className={`flex items-center gap-2 py-1 ${isCompleted ? "text-fg-4" : isInProg ? "text-fg" : "text-fg-3"}`}
            >
              {isCompleted ? (
                <CircleCheck className="size-3.5 shrink-0 text-ok/70" />
              ) : isInProg ? (
                <Loader2 className="size-3.5 shrink-0 animate-spin text-accent" />
              ) : (
                <Circle className="size-3.5 shrink-0 text-fg-4/60" />
              )}
              <span
                className={`text-xs ${isCompleted ? "line-through opacity-60" : ""}`}
              >
                {todo.content}
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}

function TerminalEvent({ event }: { event: StreamEvent }) {
  const data = event.data as Record<string, unknown> | string;
  const dataType =
    typeof data === "object" && data !== null ? (data.type as string) : null;
  const content = typeof data === "string" ? data : getContent(data);
  const raw =
    typeof data === "object" && data !== null
      ? (data.raw as Record<string, unknown> | undefined)
      : undefined;

  const ts = event.ts
    ? new Date(event.ts).toLocaleTimeString("en-US", {
        hour12: false,
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
      })
    : "";

  // User instruction — follow-up prompt from the user
  if (event.type === "system" && event.event === "user_instruction") {
    const obj = typeof data === "object" && data !== null ? data : {};
    const prompt = typeof obj.prompt === "string" ? obj.prompt : "";
    if (!prompt) return null;
    return (
      <div className="mt-3 mb-1 flex items-start gap-2 rounded-md border border-info/30 bg-info/10 px-3 py-2">
        <span className="w-14 shrink-0 pt-0.5 font-mono text-[10px] text-fg-4">
          {ts}
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

  // System-level events (clone, cli_started, review_started, etc.)
  if (event.type === "system") {
    const msg = formatSystemEvent(event.event, data);
    if (!msg) return null;
    return (
      <div className="flex items-start gap-2 rounded-md px-2 py-1.5">
        <span className="w-14 shrink-0 pt-0.5 font-mono text-[10px] text-fg-4">
          {ts}
        </span>
        <Info className="mt-0.5 size-3.5 shrink-0 text-fg-4" />
        <span className="font-mono text-xs text-fg-3">{msg}</span>
      </div>
    );
  }

  // Git events
  if (event.type === "git") {
    const msg = formatSystemEvent(event.event, data);
    if (!msg) return null;
    return (
      <div className="flex items-start gap-2 rounded-md px-2 py-1.5">
        <span className="w-14 shrink-0 pt-0.5 font-mono text-[10px] text-fg-4">
          {ts}
        </span>
        <GitCommitHorizontal className="mt-0.5 size-3.5 shrink-0 text-fg-4" />
        <span className="font-mono text-xs text-fg-3">{msg}</span>
      </div>
    );
  }

  // Result event — just a status marker
  if (event.type === "result") {
    return (
      <div className="mt-3 flex items-center gap-2 rounded-md bg-ok/10 px-2 py-1.5">
        <span className="w-14 shrink-0 pt-0.5 font-mono text-[10px] text-fg-4">
          {ts}
        </span>
        <CircleCheck className="size-3.5 shrink-0 text-ok" />
        <span className="text-xs font-medium text-ok">Session completed</span>
      </div>
    );
  }

  // Stream events — the main agent output
  if (event.type === "stream") {
    // Tool use — agent calling a tool
    if (dataType === "tool_use") {
      const toolInfo = extractToolFromRaw(raw);
      const toolName = toolInfo.name ?? extractToolName(content);
      const toolInput = toolInfo.input;
      const toolContent = formatToolExpandedContent(toolName, toolInput);
      const n = toolName.toLowerCase();
      const showExpanded =
        n === "write" ||
        (n.includes("write") && n !== "todowrite") ||
        n === "edit" ||
        n.includes("edit") ||
        n === "multiedit" ||
        n.includes("multiedit");
      return (
        <ToolUseBlock
          ts={ts}
          toolName={toolName}
          toolInput={toolInput}
          content={toolContent}
          defaultExpanded={showExpanded}
        />
      );
    }

    // Tool result — response from tool
    if (dataType === "tool_result") {
      // Codex command_execution: render as self-contained Bash block
      const itemType = raw?.item as Record<string, unknown> | undefined;
      if (itemType?.type === "command_execution") {
        const cmd = (itemType.command as string) ?? content;
        const exitCode = itemType.exit_code as number | undefined;
        const shortCmd = cmd
          .replace(/^\/bin\/sh\s+-lc\s+/, "")
          .replace(/^"(.*)"$/, "$1")
          .replace(/^'(.*)'$/, "$1");
        return (
          <div className="flex items-center gap-2 rounded-md px-2 py-1.5">
            <span className="w-14 shrink-0 font-mono text-[10px] text-fg-4">
              {ts}
            </span>
            <SquareTerminal className="size-3.5 shrink-0 text-fg-3" />
            <span className="font-mono text-xs font-medium text-fg-2">
              Bash
            </span>
            <span className="min-w-0 flex-1 truncate font-mono text-xs text-fg-3">
              {shortCmd}
            </span>
            {exitCode != null && (
              <span
                className={`font-mono text-[10px] ${exitCode === 0 ? "text-ok/70" : "text-danger"}`}
              >
                exit {exitCode}
              </span>
            )}
          </div>
        );
      }
      const resultText = extractToolResultContent(raw, content);
      return <ToolResultBlock content={resultText} />;
    }

    // Thinking — agent's reasoning. Older events may have empty content
    // (redacted blocks / pre-fix normalizer) — nothing to show for those.
    if (dataType === "thinking") {
      if (!content || !content.trim()) return null;
      return <ThinkingBlock ts={ts} content={content} />;
    }

    // Error
    if (dataType === "error") {
      return (
        <div className="flex items-start gap-2 rounded-md bg-danger/10 px-2 py-1.5">
          <span className="w-14 shrink-0 pt-0.5 font-mono text-[10px] text-fg-4">
            {ts}
          </span>
          <CircleAlert className="mt-0.5 size-3.5 shrink-0 text-danger" />
          <span className="font-mono text-xs whitespace-pre-wrap break-words text-danger">
            {content}
          </span>
        </div>
      );
    }

    // Result — session result with optional metadata (cost, tokens, turns)
    if (dataType === "result") {
      const usage = raw?.usage as Record<string, unknown> | undefined;
      const cost = raw?.total_cost_usd as number | undefined;
      const inputTokens = usage?.input_tokens as number | undefined;
      const outputTokens = usage?.output_tokens as number | undefined;
      const numTurns = raw?.num_turns as number | undefined;
      const durationMs = raw?.duration_ms as number | undefined;
      const hasMeta =
        cost != null ||
        inputTokens != null ||
        numTurns != null ||
        durationMs != null;

      // Don't show text content — it was already shown in the preceding assistant
      // text event. Only render if we have metadata (cost, tokens, etc.).
      if (!hasMeta) return null;

      return (
        <div className="flex items-start gap-2 px-2 py-1">
          <span className="w-14 shrink-0 pt-0.5 font-mono text-[10px] text-fg-4">
            {ts}
          </span>
          <div className="min-w-0 flex-1">
            {hasMeta && (
              <div className="mt-1 flex flex-wrap items-center gap-1.5">
                {cost != null && (
                  <span className="inline-flex items-center gap-0.5 rounded-[4px] border border-edge bg-surface-alt px-1.5 py-0.5 font-mono text-[10px] text-fg-3">
                    ${cost.toFixed(3)}
                  </span>
                )}
                {inputTokens != null && outputTokens != null && (
                  <span className="inline-flex items-center gap-0.5 rounded-[4px] border border-edge bg-surface-alt px-1.5 py-0.5 font-mono text-[10px] text-fg-3">
                    {inputTokens > 1000
                      ? `${(inputTokens / 1000).toFixed(1)}k`
                      : inputTokens}
                    →
                    {outputTokens > 1000
                      ? `${(outputTokens / 1000).toFixed(1)}k`
                      : outputTokens}{" "}
                    tokens
                  </span>
                )}
                {numTurns != null && (
                  <span className="inline-flex items-center gap-0.5 rounded-[4px] border border-edge bg-surface-alt px-1.5 py-0.5 font-mono text-[10px] text-fg-3">
                    {numTurns} {numTurns === 1 ? "turn" : "turns"}
                  </span>
                )}
                {durationMs != null && (
                  <span className="inline-flex items-center gap-0.5 rounded-[4px] border border-edge bg-surface-alt px-1.5 py-0.5 font-mono text-[10px] text-fg-3">
                    {durationMs > 60000
                      ? `${(durationMs / 60000).toFixed(1)}m`
                      : `${(durationMs / 1000).toFixed(1)}s`}
                  </span>
                )}
              </div>
            )}
          </div>
        </div>
      );
    }

    // Text — agent speaking
    if (dataType === "text") {
      if (!content) return null;
      return (
        <div className="flex items-start gap-2 px-2 py-1">
          <span className="w-14 shrink-0 pt-0.5 font-mono text-[10px] text-fg-4">
            {ts}
          </span>
          <div className="min-w-0 flex-1">
            <MarkdownText text={content} className="text-xs" />
          </div>
        </div>
      );
    }

    // System message from stream (init, config, etc.)
    if (dataType === "system") {
      const msg = formatStreamSystemEvent(data as Record<string, unknown>);
      if (!msg) return null;
      return (
        <div className="flex items-start gap-2 rounded-md px-2 py-1.5">
          <span className="w-14 shrink-0 pt-0.5 font-mono text-[10px] text-fg-4">
            {ts}
          </span>
          <Info className="mt-0.5 size-3.5 shrink-0 text-fg-4" />
          <span className="font-mono text-xs text-fg-3">{msg}</span>
        </div>
      );
    }

    // Default / unknown stream sub-type — skip if content looks like raw JSON
    if (content.startsWith("{") && content.length > 200) return null;
    return (
      <div className="flex items-start gap-2 px-2 py-0.5">
        <span className="w-14 shrink-0 pt-0.5 font-mono text-[10px] text-fg-4">
          {ts}
        </span>
        <span className="min-w-0 flex-1 font-mono text-xs whitespace-pre-wrap break-words text-fg-3">
          {content}
        </span>
      </div>
    );
  }

  // Fallback for any other event type — skip raw JSON dumps
  if (content.startsWith("{") && content.length > 200) return null;
  return (
    <div className="flex items-start gap-2 px-2 py-0.5">
      <span className="w-14 shrink-0 pt-0.5 font-mono text-[10px] text-fg-4">
        {ts}
      </span>
      <span className="mt-0.5 font-mono text-[10px] font-medium text-fg-4 uppercase">
        [{event.type}]
      </span>
      <span className="min-w-0 flex-1 font-mono text-xs whitespace-pre-wrap break-words text-fg-3">
        {content}
      </span>
    </div>
  );
}

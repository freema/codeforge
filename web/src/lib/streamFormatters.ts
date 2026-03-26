export function getContent(data: Record<string, unknown> | null): string {
  if (!data) return "";
  if (typeof data.content === "string") return data.content;
  if (typeof data.message === "string") return data.message;
  if (typeof data.text === "string") return data.text;
  if (typeof data.result === "string") return data.result;
  if (typeof data.error === "string") return data.error;
  return "";
}

/** Known noisy system events with no useful content */
const HIDDEN_SYSTEM_EVENTS = new Set(["user", "heartbeat", "ping", "ack"]);

export function formatSystemEvent(
  eventName: string,
  data: unknown,
): string | null {
  if (HIDDEN_SYSTEM_EVENTS.has(eventName)) return null;

  const obj =
    typeof data === "object" && data !== null
      ? (data as Record<string, unknown>)
      : {};
  switch (eventName) {
    case "user_instruction":
      return null; // rendered separately with special styling
    case "cli_started":
      return `Agent started (${obj.cli ?? "cli"}, iteration ${obj.iteration ?? "?"})`;
    case "clone_started":
      return `Cloning ${obj.repo_url ?? "repository"}...`;
    case "clone_completed":
      return `Clone complete → ${obj.work_dir ?? "workspace"}`;
    case "task_timeout":
      return `Session timed out after ${obj.timeout_seconds ?? "?"}s`;
    case "task_cancelled":
      return "Session cancelled";
    case "task_failed":
      return `Session failed: ${obj.error ?? "unknown error"}`;
    case "review_started":
      return "Code review started...";
    case "review_completed": {
      const verdict = obj.verdict as string | undefined;
      const score = obj.score as number | undefined;
      return `Review complete: ${verdict ?? "?"} (score: ${score ?? "?"}/10, ${obj.issues_count ?? 0} issues)`;
    }
    default: {
      const msg = getContent(obj);
      if (msg.startsWith("{") || msg.startsWith("[")) return null;
      return msg || null;
    }
  }
}

/** Known noisy stream subtypes to hide */
const HIDDEN_STREAM_SUBTYPES = new Set([
  "user",
  "heartbeat",
  "ping",
  "ack",
  "result",
  // Codex-specific internal events
  "thread.started",
  "thread.completed",
  "turn.started",
  "turn.completed",
  "item.completed",
  "item.started",
  "item.created",
  "item.updated",
]);

/** Format stream system events (init, config, etc.) into readable text */
export function formatStreamSystemEvent(
  data: Record<string, unknown>,
): string | null {
  const raw = data.raw as Record<string, unknown> | undefined;
  const cli = data.cli as string | undefined;
  const subtype =
    (raw?.subtype as string | undefined) ?? (raw?.type as string | undefined);

  if (subtype && HIDDEN_STREAM_SUBTYPES.has(subtype)) return null;

  if (subtype === "init" || subtype === "system") {
    const model = raw?.model as string | undefined;
    const cwd = raw?.cwd as string | undefined;
    const parts = [`Agent started`];
    if (cli) parts[0] = `${cli} started`;
    if (model) parts.push(`model: ${model}`);
    if (cwd) {
      const short = cwd.split("/").slice(-2).join("/");
      parts.push(`in ${short}`);
    }
    return parts.join(" · ");
  }

  if (subtype === "config" || subtype === "settings") {
    return null; // hide config noise
  }

  if (subtype === "api_retry") {
    const attempt = raw?.attempt as number | undefined;
    const maxRetries = raw?.max_retries as number | undefined;
    const delaySec = raw?.retry_delay_ms
      ? ((raw.retry_delay_ms as number) / 1000).toFixed(0)
      : "?";
    const attemptStr =
      attempt != null && maxRetries != null
        ? `${attempt}/${maxRetries}`
        : attempt != null
          ? `${attempt}`
          : "?";
    return `API retry (attempt ${attemptStr}, waiting ${delaySec}s)...`;
  }

  if (subtype === "compact_boundary") {
    return "Context compacted (conversation history trimmed)";
  }

  // For other subtypes, try to extract a readable message
  if (raw) {
    if (typeof raw.message === "string") return raw.message;
    if (typeof raw.text === "string") return raw.text;
  }

  // If we have a subtype, show it as a label
  if (subtype) return subtype.replace(/_/g, " ");

  return null; // hide unrecognizable system events
}

/** Format tool expanded content based on tool type — not raw JSON */
export function formatToolExpandedContent(
  toolName: string,
  toolInput?: Record<string, unknown>,
): string {
  if (!toolInput) return "";
  const name = toolName.toLowerCase();

  // TodoWrite: show the todo items as a checklist
  if (name === "todowrite") {
    const todos = toolInput.todos as
      | Array<{ content: string; status: string }>
      | undefined;
    if (todos && todos.length > 0) {
      return todos
        .map((t) => {
          const icon =
            t.status === "completed"
              ? "[x]"
              : t.status === "in_progress"
                ? "[~]"
                : "[ ]";
          return `${icon} ${t.content}`;
        })
        .join("\n");
    }
    return "";
  }

  // Write: show file content being written
  if (name === "write" || name.includes("write")) {
    const fileContent = toolInput.content as string | undefined;
    if (fileContent) {
      const preview =
        fileContent.length > 1000
          ? fileContent.slice(0, 1000) + "\n..."
          : fileContent;
      return preview;
    }
    return "";
  }

  // Edit: show old → new
  if (name === "edit" || name.includes("edit")) {
    const oldStr = toolInput.old_string as string | undefined;
    const newStr = toolInput.new_string as string | undefined;
    if (oldStr && newStr) {
      const lines: string[] = [];
      for (const line of oldStr.split("\n")) lines.push(`- ${line}`);
      for (const line of newStr.split("\n")) lines.push(`+ ${line}`);
      return lines.join("\n");
    }
    return "";
  }

  // Bash: show full command (header may truncate it)
  if (name === "bash" || name.includes("bash") || name.includes("shell")) {
    const cmd = toolInput.command as string | undefined;
    return cmd ?? "";
  }

  // Read/Grep/Glob/LS: detail is already in header, nothing extra needed
  if (
    name === "read" ||
    name.includes("read") ||
    name === "grep" ||
    name.includes("grep") ||
    name.includes("search") ||
    name === "glob" ||
    name.includes("glob") ||
    name.includes("find") ||
    name === "ls" ||
    name === "listdir"
  ) {
    return "";
  }

  // Fallback: show input params but truncate long values
  const cleaned: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(toolInput)) {
    if (typeof value === "string" && value.length > 200) {
      cleaned[key] = value.slice(0, 200) + "...";
    } else {
      cleaned[key] = value;
    }
  }
  return JSON.stringify(cleaned, null, 2);
}

export function extractToolName(content: string): string {
  // Try to extract tool name from content like "Running: Read file ..." or "Tool: Bash ..."
  const match = content.match(/^(?:Running|Tool|Using):\s*(\w+)/i);
  if (match?.[1]) return match[1];
  // Codex format: "tool_name({...})" or just "tool_name"
  const codexMatch = content.match(/^(\w+)(?:\(|$)/);
  return codexMatch?.[1] ?? "Tool";
}

/** Extract tool name + input from the nested raw API response */
export function extractToolFromRaw(raw: Record<string, unknown> | undefined): {
  name?: string;
  input?: Record<string, unknown>;
} {
  if (!raw) return {};

  // Direct: raw.name, raw.input (simple format)
  if (typeof raw.name === "string") {
    return {
      name: raw.name,
      input: raw.input as Record<string, unknown> | undefined,
    };
  }

  // Nested in raw.message.content[0] (full API response format)
  const message = raw.message as Record<string, unknown> | undefined;
  if (message) {
    const msgContent = message.content as
      | Array<Record<string, unknown>>
      | undefined;
    if (msgContent && msgContent.length > 0) {
      const first = msgContent[0]!;
      if (typeof first.name === "string") {
        return {
          name: first.name,
          input: first.input as Record<string, unknown> | undefined,
        };
      }
    }
  }

  // Nested in raw.content[0] (alternate format)
  const content = raw.content as Array<Record<string, unknown>> | undefined;
  if (content && content.length > 0) {
    const first = content[0]!;
    if (typeof first.name === "string") {
      return {
        name: first.name,
        input: first.input as Record<string, unknown> | undefined,
      };
    }
  }

  // Codex format: raw = { type: "item.completed", item: { type: "function_call", name: "read_file", arguments: "{...}" } }
  const item = raw.item as Record<string, unknown> | undefined;
  if (item && typeof item.name === "string") {
    let input: Record<string, unknown> | undefined;
    if (typeof item.arguments === "string" && item.arguments) {
      try {
        input = JSON.parse(item.arguments as string) as Record<string, unknown>;
      } catch {
        /* ignore */
      }
    }
    return { name: item.name, input };
  }

  return {};
}

/** Extract meaningful text from tool_result, avoiding full JSON dump */
export function extractToolResultContent(
  raw: Record<string, unknown> | undefined,
  fallback: string,
): string {
  if (raw) {
    // Direct content string
    if (typeof raw.content === "string") return raw.content;
    if (typeof raw.text === "string") return raw.text;
    if (typeof raw.result === "string") return raw.result;
    if (typeof raw.output === "string") return raw.output;

    // Nested in raw.message.content[0].text
    const message = raw.message as Record<string, unknown> | undefined;
    if (message) {
      const msgContent = message.content as
        | Array<Record<string, unknown>>
        | undefined;
      if (msgContent && msgContent.length > 0) {
        const texts = msgContent
          .map((c) =>
            typeof c.text === "string"
              ? c.text
              : typeof c.content === "string"
                ? c.content
                : null,
          )
          .filter(Boolean);
        if (texts.length > 0) return texts.join("\n");
      }
    }

    // Nested in raw.content array
    const content = raw.content as Array<Record<string, unknown>> | undefined;
    if (Array.isArray(content) && content.length > 0) {
      const texts = content
        .map((c) => (typeof c.text === "string" ? c.text : null))
        .filter(Boolean);
      if (texts.length > 0) return texts.join("\n");
    }
  }

  // If fallback looks like a full JSON dump, don't show it
  if (fallback.startsWith("{") && fallback.length > 300) {
    return "(tool output too large to display)";
  }

  return fallback;
}

export function getToolDisplay(
  toolName: string,
  toolInput?: Record<string, unknown>,
  content?: string,
): { icon: string; label: string; detail: string } {
  const name = toolName.toLowerCase();
  const filePath =
    (toolInput?.file_path as string | undefined) ??
    (toolInput?.path as string | undefined);
  const command = toolInput?.command as string | undefined;

  if (name === "read" || name.includes("read")) {
    return { icon: "description", label: "Read", detail: filePath ?? "" };
  }
  if (name === "todowrite") {
    const todos = toolInput?.todos as
      | Array<{ content: string; status: string }>
      | undefined;
    const count = todos?.length ?? 0;
    return {
      icon: "checklist",
      label: "Plan",
      detail: count > 0 ? `${count} items` : "",
    };
  }
  if (name === "write" || name.includes("write")) {
    const fileContent = toolInput?.content as string | undefined;
    const lineCount = fileContent ? fileContent.split("\n").length : 0;
    const suffix = lineCount > 0 ? ` (${lineCount} lines)` : "";
    return {
      icon: "edit_document",
      label: "Write",
      detail: (filePath ?? "") + suffix,
    };
  }
  if (name === "edit" || name.includes("edit")) {
    const oldStr = toolInput?.old_string as string | undefined;
    const newStr = toolInput?.new_string as string | undefined;
    const oldLines = oldStr ? oldStr.split("\n").length : 0;
    const newLines = newStr ? newStr.split("\n").length : 0;
    const suffix = oldStr ? ` (-${oldLines} +${newLines})` : "";
    return { icon: "edit", label: "Edit", detail: (filePath ?? "") + suffix };
  }
  if (name === "bash" || name.includes("bash") || name.includes("shell")) {
    const cmd = command
      ? command.length > 80
        ? command.slice(0, 80) + "..."
        : command
      : "";
    return { icon: "terminal", label: "Bash", detail: cmd };
  }
  if (name === "grep" || name.includes("grep") || name.includes("search")) {
    const pattern = toolInput?.pattern as string | undefined;
    return { icon: "search", label: "Search", detail: pattern ?? "" };
  }
  if (name === "glob" || name.includes("glob") || name.includes("find")) {
    const pattern = toolInput?.pattern as string | undefined;
    return { icon: "folder_open", label: "Glob", detail: pattern ?? "" };
  }
  if (name === "ls" || name === "listdir") {
    const path = (toolInput?.path as string | undefined) ?? filePath ?? "";
    return { icon: "folder_open", label: "LS", detail: path };
  }
  // MCP tools: mcp__servername__toolname
  if (toolName.startsWith("mcp__")) {
    const parts = toolName.split("__");
    const serverName = parts[1] ?? "";
    const mcpToolName = parts.slice(2).join("__") || toolName;
    return { icon: "dns", label: mcpToolName, detail: serverName };
  }

  // Fallback: show tool name, but never raw JSON as detail
  const fallbackDetail =
    content &&
    content.length <= 100 &&
    !content.startsWith("{") &&
    !content.startsWith("[")
      ? content
      : "";
  return { icon: "build", label: toolName, detail: fallbackDetail };
}

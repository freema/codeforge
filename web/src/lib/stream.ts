import type { StreamEvent, SessionStatus, RunStatus } from "../types";

interface StreamHandlers {
  onEvent: (event: StreamEvent) => void;
  onConnected: (data: { session_id: string; status: SessionStatus }) => void;
  onDone: (data: { status: SessionStatus }) => void;
  onError: (error: Error) => void;
}

function parseSSE(chunk: string): { event?: string; data?: string }[] {
  const messages: { event?: string; data?: string }[] = [];
  let currentEvent: string | undefined;
  let currentData = "";

  for (const line of chunk.split("\n")) {
    if (line.startsWith(": ")) continue; // keepalive comment
    if (line.startsWith("event: ")) {
      currentEvent = line.slice(7).trim();
    } else if (line.startsWith("data: ")) {
      currentData += line.slice(6);
    } else if (line === "") {
      if (currentData) {
        messages.push({ event: currentEvent, data: currentData });
      }
      currentEvent = undefined;
      currentData = "";
    }
  }

  return messages;
}

export function connectToSessionStream(
  serverUrl: string,
  sessionId: string,
  token: string,
  handlers: StreamHandlers,
): AbortController {
  const controller = new AbortController();

  const connect = async () => {
    try {
      const res = await fetch(
        `${serverUrl}/api/v1/sessions/${sessionId}/stream`,
        {
          headers: { Authorization: `Bearer ${token}` },
          signal: controller.signal,
        },
      );

      if (!res.ok || !res.body) {
        handlers.onError(new Error(`Stream error: ${res.status}`));
        return;
      }

      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const parts = buffer.split("\n\n");
        buffer = parts.pop() ?? "";

        for (const part of parts) {
          const messages = parseSSE(part + "\n\n");
          for (const msg of messages) {
            if (!msg.data) continue;

            try {
              const parsed = JSON.parse(msg.data) as Record<string, unknown>;

              if (msg.event === "connected") {
                handlers.onConnected(
                  parsed as unknown as {
                    session_id: string;
                    status: SessionStatus;
                  },
                );
              } else if (msg.event === "done") {
                handlers.onDone(parsed as unknown as { status: SessionStatus });
              } else {
                handlers.onEvent({
                  type: (parsed.type as StreamEvent["type"]) ?? "stream",
                  event: (parsed.event as string) ?? msg.event ?? "message",
                  data: parsed.data ?? parsed,
                  ts: (parsed.ts as string) ?? new Date().toISOString(),
                });
              }
            } catch {
              // Non-JSON data, treat as raw stream text
              handlers.onEvent({
                type: "stream",
                event: msg.event ?? "message",
                data: msg.data,
                ts: new Date().toISOString(),
              });
            }
          }
        }
      }
    } catch (err) {
      if (controller.signal.aborted) return;
      handlers.onError(err instanceof Error ? err : new Error("Stream failed"));
    }
  };

  void connect();
  return controller;
}

interface WorkflowStreamHandlers {
  onEvent: (event: StreamEvent) => void;
  onConnected: (data: { run_id: string; status: RunStatus }) => void;
  onDone: (data: { status: RunStatus }) => void;
  onError: (error: Error) => void;
}

export function connectToWorkflowRunStream(
  serverUrl: string,
  runId: string,
  token: string,
  handlers: WorkflowStreamHandlers,
): AbortController {
  const controller = new AbortController();

  const connect = async () => {
    try {
      const res = await fetch(
        `${serverUrl}/api/v1/workflow-runs/${runId}/stream`,
        {
          headers: { Authorization: `Bearer ${token}` },
          signal: controller.signal,
        },
      );

      if (!res.ok || !res.body) {
        handlers.onError(new Error(`Stream error: ${res.status}`));
        return;
      }

      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const parts = buffer.split("\n\n");
        buffer = parts.pop() ?? "";

        for (const part of parts) {
          const messages = parseSSE(part + "\n\n");
          for (const msg of messages) {
            if (!msg.data) continue;

            try {
              const parsed = JSON.parse(msg.data) as Record<string, unknown>;

              if (msg.event === "connected") {
                handlers.onConnected(
                  parsed as unknown as {
                    run_id: string;
                    status: RunStatus;
                  },
                );
              } else if (msg.event === "done") {
                handlers.onDone(parsed as unknown as { status: RunStatus });
              } else {
                handlers.onEvent({
                  type: (parsed.type as StreamEvent["type"]) ?? "stream",
                  event: (parsed.event as string) ?? msg.event ?? "message",
                  data: parsed.data ?? parsed,
                  ts: (parsed.ts as string) ?? new Date().toISOString(),
                });
              }
            } catch {
              handlers.onEvent({
                type: "stream",
                event: msg.event ?? "message",
                data: msg.data,
                ts: new Date().toISOString(),
              });
            }
          }
        }
      }
    } catch (err) {
      if (controller.signal.aborted) return;
      handlers.onError(err instanceof Error ? err : new Error("Stream failed"));
    }
  };

  void connect();
  return controller;
}

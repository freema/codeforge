export type StreamEventType = "system" | "git" | "cli" | "stream" | "result";

export interface StreamEvent {
  type: StreamEventType;
  event: string;
  data: unknown;
  ts: string;
}

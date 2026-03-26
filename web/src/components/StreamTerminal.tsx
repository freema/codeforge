import { useEffect, useRef, useState } from "react";
import { ArrowDownToLine } from "lucide-react";
import type { StreamEvent } from "../types";

const typeColors: Record<string, string> = {
  system: "text-accent",
  git: "text-cyan-400",
  cli: "text-fg-3",
  stream: "text-fg",
  result: "text-green-400 font-semibold",
};

export default function StreamTerminal({ events }: { events: StreamEvent[] }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [autoScroll, setAutoScroll] = useState(true);

  useEffect(() => {
    if (autoScroll && containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
  }, [events, autoScroll]);

  function handleScroll() {
    const el = containerRef.current;
    if (!el) return;
    const isAtBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
    setAutoScroll(isAtBottom);
  }

  return (
    <div className="relative rounded-lg border border-edge bg-page">
      <div className="flex items-center justify-between border-b border-edge px-4 py-2">
        <span className="text-xs font-medium text-fg-3">Live Stream</span>
        <button
          onClick={() => {
            setAutoScroll(true);
            if (containerRef.current) {
              containerRef.current.scrollTop =
                containerRef.current.scrollHeight;
            }
          }}
          className={`flex items-center gap-1 rounded px-2 py-0.5 text-xs transition-colors ${
            autoScroll
              ? "bg-accent-soft text-accent"
              : "text-fg-3 hover:text-fg-2"
          }`}
        >
          <ArrowDownToLine className="h-3 w-3" />
          Auto
        </button>
      </div>

      <div
        ref={containerRef}
        onScroll={handleScroll}
        className="h-96 overflow-y-auto p-4 font-mono text-sm"
      >
        {events.length === 0 ? (
          <p className="text-fg-4">Waiting for stream...</p>
        ) : (
          events.map((event, i) => (
            <div key={i} className="leading-relaxed">
              <span className="text-fg-4">[{event.type}]</span>{" "}
              <span className={typeColors[event.type] ?? "text-fg-2"}>
                {formatEventData(event.data)}
              </span>
            </div>
          ))
        )}
      </div>
    </div>
  );
}

function formatEventData(data: unknown): string {
  if (typeof data === "string") return data;
  if (typeof data === "object" && data !== null) {
    const obj = data as Record<string, unknown>;
    if ("message" in obj && typeof obj.message === "string") return obj.message;
    if ("text" in obj && typeof obj.text === "string") return obj.text;
    if ("content" in obj && typeof obj.content === "string") return obj.content;
    return JSON.stringify(data);
  }
  return String(data);
}

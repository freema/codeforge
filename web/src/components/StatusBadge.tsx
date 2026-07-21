import type { SessionStatus } from "../types";

/* Heat semantics: running is hot (ember, the one glow in the app),
   cold machine processes are steel, waiting is amber, success is
   tempered green, failure is rust, canceled is cooled ash. */
const statusConfig: Record<
  SessionStatus,
  {
    label: string;
    tone: string;
    dot?: "ember" | "pulse";
  }
> = {
  pending: {
    label: "Queued",
    tone: "border-warn/30 bg-warn/10 text-warn",
  },
  cloning: {
    label: "Cloning",
    tone: "border-info/30 bg-info/10 text-info",
    dot: "pulse",
  },
  running: {
    label: "Running",
    tone: "border-accent-muted bg-accent-soft text-accent",
    dot: "ember",
  },
  completed: {
    label: "Done",
    tone: "border-ok/30 bg-ok/10 text-ok",
  },
  failed: {
    label: "Failed",
    tone: "border-danger/30 bg-danger/10 text-danger",
  },
  awaiting_instruction: {
    label: "Awaiting input",
    tone: "border-accent-muted bg-accent-soft text-accent",
  },
  reviewing: {
    label: "Reviewing",
    tone: "border-info/30 bg-info/10 text-info",
    dot: "pulse",
  },
  creating_pr: {
    label: "Creating PR",
    tone: "border-info/30 bg-info/10 text-info",
    dot: "pulse",
  },
  pr_created: {
    label: "PR created",
    tone: "border-info/30 bg-info/10 text-info",
  },
  cancelling: {
    label: "Canceling",
    tone: "border-warn/30 bg-warn/10 text-warn",
    dot: "pulse",
  },
  canceled: {
    label: "Canceled",
    tone: "border-edge bg-surface-alt text-fg-3",
  },
};

export default function StatusBadge({ status }: { status: SessionStatus }) {
  const config = statusConfig[status] ?? statusConfig.pending;

  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-[4px] border px-2 py-0.5 font-mono text-[10px] font-medium tracking-[0.08em] uppercase ${config.tone}`}
    >
      <span
        className={`size-1.5 rounded-full bg-current ${
          config.dot === "ember"
            ? "animate-ember"
            : config.dot === "pulse"
              ? "animate-soft-pulse"
              : ""
        }`}
      />
      {config.label}
    </span>
  );
}

import type { SessionStatus } from "../types";

const statusConfig: Record<
  SessionStatus,
  {
    label: string;
    color: string;
    bg: string;
    border: string;
    icon: string;
    animated?: boolean;
  }
> = {
  pending: {
    label: "QUEUED",
    color: "text-yellow-500",
    bg: "bg-yellow-500/10",
    border: "border-yellow-500/20",
    icon: "pending",
  },
  cloning: {
    label: "CLONING",
    color: "text-emerald-400",
    bg: "bg-emerald-400/10",
    border: "border-emerald-400/20",
    icon: "download",
    animated: true,
  },
  running: {
    label: "RUNNING",
    color: "text-accent",
    bg: "bg-accent/10",
    border: "border-accent/20",
    icon: "terminal",
    animated: true,
  },
  completed: {
    label: "DONE",
    color: "text-fg-3",
    bg: "bg-surface",
    border: "border-edge",
    icon: "check",
  },
  failed: {
    label: "FAILED",
    color: "text-red-500",
    bg: "bg-red-500/10",
    border: "border-red-500/20",
    icon: "close",
  },
  awaiting_instruction: {
    label: "AWAITING",
    color: "text-purple-400",
    bg: "bg-purple-400/10",
    border: "border-purple-400/20",
    icon: "chat_bubble",
  },
  reviewing: {
    label: "REVIEWING",
    color: "text-blue-400",
    bg: "bg-blue-400/10",
    border: "border-blue-400/20",
    icon: "rate_review",
    animated: true,
  },
  creating_pr: {
    label: "CREATING PR",
    color: "text-teal-400",
    bg: "bg-teal-400/10",
    border: "border-teal-400/20",
    icon: "call_merge",
    animated: true,
  },
  pr_created: {
    label: "PR CREATED",
    color: "text-teal-400",
    bg: "bg-teal-400/10",
    border: "border-teal-400/20",
    icon: "call_merge",
  },
  cancelling: {
    label: "CANCELLING",
    color: "text-orange-400",
    bg: "bg-orange-400/10",
    border: "border-orange-400/20",
    icon: "cancel",
  },
};

export default function StatusBadge({ status }: { status: SessionStatus }) {
  const config = statusConfig[status] ?? statusConfig.pending;

  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-full px-3 py-0.5 text-xs font-bold uppercase tracking-wider ${config.bg} ${config.border} ${config.color} border`}
    >
      {config.animated && (
        <span className={`size-1.5 rounded-full bg-current animate-pulse`} />
      )}
      {!config.animated && (
        <span className="material-symbols-outlined text-[14px]">
          {config.icon}
        </span>
      )}
      {config.label}
    </span>
  );
}

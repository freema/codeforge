import { useState, useRef, useEffect } from "react";
import { useParams, Link } from "react-router";
import { Loader2 } from "lucide-react";
import { useSession } from "../hooks/useSession";
import { useSessionStream } from "../hooks/useSessionStream";
import {
  useCancelSession,
  useInstructSession,
  useCreatePR,
  usePushToPR,
  useReviewSession,
  usePostReviewComments,
  usePRStatus,
} from "../hooks/useSessionMutations";
import StatusBadge from "../components/StatusBadge";
import MarkdownText from "../components/MarkdownText";
import { usePageTitle } from "../hooks/usePageTitle";
import { useToast } from "../context/ToastContext";
import {
  PromptCard,
  IterationsHistory,
} from "../components/session/PromptCard";
import { StreamEvents } from "../components/session/StreamEvents";
import { ReviewResultCard } from "../components/session/ReviewResultCard";
import { formatDuration, formatChangesSummary } from "../lib/formatters";
import type { SessionStatus } from "../types";

const ACTIVE_STATUSES: SessionStatus[] = [
  "pending",
  "cloning",
  "running",
  "reviewing",
  "creating_pr",
  "cancelling",
];

export default function SessionDetail() {
  usePageTitle("Session Detail");
  const { id } = useParams<{ id: string }>();
  const { data: session, isLoading } = useSession(id, "iterations");
  const stream = useSessionStream(id);
  const cancelSession = useCancelSession();
  const instructSession = useInstructSession();
  const createPR = useCreatePR();
  const pushToPR = usePushToPR();
  const reviewSession = useReviewSession();
  const postReviewComments = usePostReviewComments();
  const { data: prStatus } = usePRStatus(id, !!session?.pr_url);
  const { toast } = useToast();

  const [instructPrompt, setInstructPrompt] = useState("");
  const [pushed, setPushed] = useState(false);
  const terminalRef = useRef<HTMLDivElement>(null);

  // Auto-scroll terminal to bottom on new events
  useEffect(() => {
    if (terminalRef.current) {
      terminalRef.current.scrollTop = terminalRef.current.scrollHeight;
    }
  }, [stream.events.length]);

  if (isLoading) {
    return (
      <div className="flex h-full items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-accent/50" />
      </div>
    );
  }

  if (!session) {
    return <p className="py-20 text-center text-fg-4">Session not found.</p>;
  }

  const isActive = ACTIVE_STATUSES.includes(session.status);
  const isPlan = session.session_type === "plan";
  const canCancel =
    session.status === "running" ||
    session.status === "cloning" ||
    session.status === "reviewing";
  const canInstruct =
    session.status === "completed" || session.status === "awaiting_instruction" || session.status === "pr_created";
  const hasChanges =
    session.changes_summary &&
    (session.changes_summary.files_modified > 0 ||
      session.changes_summary.files_created > 0 ||
      session.changes_summary.files_deleted > 0);
  const canCreatePR =
    (session.status === "completed" || session.status === "pr_created") && !isPlan && hasChanges;
  const isPrReview = session.session_type === "pr_review";
  const isReview = session.session_type === "review";
  const canReview = session.status === "completed" && !isPlan && !isPrReview && !isReview;
  const canPostComments =
    isPrReview &&
    session.status === "completed" &&
    !!session.review_result;

  const repoShort = session.repo_url
    .replace(/^https?:\/\//, "")
    .replace(/\.git$/, "");

  async function handleCancel() {
    if (!id) return;
    try {
      await cancelSession.mutateAsync(id);
      toast("info", "Session cancellation requested");
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to cancel");
    }
  }

  async function handleInstruct() {
    if (!id || !instructPrompt.trim()) return;
    try {
      await instructSession.mutateAsync({ id, prompt: instructPrompt });
      setInstructPrompt("");
      setPushed(false);
      stream.reconnect();
    } catch (err) {
      toast(
        "error",
        err instanceof Error ? err.message : "Failed to send instruction",
      );
    }
  }

  async function handleCreatePR() {
    if (!id) return;
    try {
      await createPR.mutateAsync({ id });
      toast("success", "PR created");
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "PR creation failed");
    }
  }

  async function handlePushToPR() {
    if (!id) return;
    try {
      await pushToPR.mutateAsync(id);
      setPushed(true);
      toast("success", "Changes pushed to PR");
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Push failed");
    }
  }

  async function handlePostComments() {
    if (!id) return;
    toast("info", "Posting review comments...");
    try {
      await postReviewComments.mutateAsync(id);
      toast("success", "Review comments posted!");
    } catch (err) {
      toast(
        "error",
        err instanceof Error ? err.message : "Failed to post comments",
      );
    }
  }

  async function handleReview() {
    if (!id) return;
    toast("info", "Starting code review...");
    stream.reconnect();
    try {
      await reviewSession.mutateAsync({ id });
      toast("success", "Code review completed!");
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Review failed");
    }
  }

  const totalTokens = session.usage
    ? session.usage.input_tokens + session.usage.output_tokens
    : 0;

  return (
    <div className="-m-6 lg:-m-10 flex h-[calc(100vh-4rem)] flex-col overflow-hidden">
      {/* Header bar — session info + stats + actions */}
      <div className="flex flex-wrap items-center gap-x-6 gap-y-2 border-b border-accent/20 bg-surface/80 px-6 py-3 backdrop-blur-sm z-10">
        {/* Left: ID + status + session type + repo */}
        <div className="flex items-center gap-3">
          <span className="rounded border border-accent/30 px-1.5 py-0.5 font-mono text-xs text-accent/70">
            {session.id.slice(0, 8)}
          </span>
          <StatusBadge status={session.status} />
          {session.session_type && (
            <span className="rounded border border-fg-4/30 bg-surface px-2 py-0.5 font-mono text-[10px] font-bold uppercase tracking-wider text-fg-3">
              {session.session_type}
            </span>
          )}
          <span className="hidden sm:flex items-center gap-1.5 font-mono text-xs text-fg-3">
            <span className="material-symbols-outlined text-sm">folder</span>
            {repoShort}
          </span>
        </div>

        {/* Center: stats */}
        <div className="flex items-center gap-4 font-mono text-xs text-fg-3">
          <span title="Tokens">
            <span className="text-fg-4">tok</span>{" "}
            <span className="text-fg">
              {totalTokens > 0 ? totalTokens.toLocaleString() : "\u2014"}
            </span>
          </span>
          <span title="Iteration">
            <span className="text-fg-4">iter</span>{" "}
            <span className="text-fg">{session.iteration}</span>
          </span>
          <span title="Duration">
            <span className="text-fg-4">dur</span>{" "}
            <span className="text-fg">
              {session.usage
                ? formatDuration(session.usage.duration_seconds)
                : "\u2014"}
            </span>
          </span>
          {hasChanges && (
            <span title="Changes" className="flex items-center gap-1">
              <span className="material-symbols-outlined text-sm text-cyan-400">
                difference
              </span>
              <span className="text-fg">
                {session.changes_summary!.diff_stats ||
                  formatChangesSummary(session.changes_summary!)}
              </span>
            </span>
          )}
          {session.workflow_run_id && (
            <Link
              to={`/workflows/runs/${session.workflow_run_id}`}
              className="flex items-center gap-1 text-purple-400 transition-colors hover:text-purple-300"
              title="Workflow run"
              onClick={(e) => e.stopPropagation()}
            >
              <span className="material-symbols-outlined text-sm">
                account_tree
              </span>
              Workflow
            </Link>
          )}
          {session.pr_url && (
            <Link
              to={session.pr_url}
              target="_blank"
              className="flex items-center gap-1 text-teal-500 transition-colors hover:text-teal-400"
              title="Pull Request"
              onClick={(e) => e.stopPropagation()}
            >
              <span className="material-symbols-outlined text-sm">
                call_merge
              </span>
              PR{session.pr_number ? ` #${session.pr_number}` : ""}
            </Link>
          )}
        </div>

        {/* Right: live/events indicator */}
        <div className="ml-auto flex items-center gap-2">
          {isActive && (
            <div className="flex items-center gap-1.5">
              <div className="size-2 animate-pulse rounded-full bg-accent" />
              <span className="font-mono text-xs uppercase tracking-widest text-accent">
                Live
              </span>
            </div>
          )}
          {!isActive && stream.events.length > 0 && (
            <span className="font-mono text-xs text-fg-4">
              {stream.events.length} events
            </span>
          )}
          {stream.error && (
            <button
              onClick={stream.reconnect}
              className="font-mono text-xs text-red-400 hover:text-red-300 underline"
            >
              Reconnect
            </button>
          )}
        </div>
      </div>

      {/* Error banner */}
      {session.error && (
        <div className="flex items-center gap-2 border-b border-red-900/30 bg-red-900/10 px-6 py-2">
          <span className="material-symbols-outlined text-sm text-red-400">
            error
          </span>
          <span className="font-mono text-xs text-red-300">
            {session.error}
          </span>
        </div>
      )}

      {/* Terminal — full width */}
      <div
        ref={terminalRef}
        className="flex-1 overflow-y-auto overflow-x-hidden p-4 text-sm"
      >
        {/* Prompt card — always first */}
        <PromptCard prompt={session.prompt} ts={session.created_at} />

        {/* Previous iterations */}
        {session.iterations && session.iterations.length > 1 && (
          <IterationsHistory iterations={session.iterations} />
        )}

        {/* Stream events */}
        {stream.events.length === 0 && isActive ? (
          <div className="flex items-center gap-3 text-fg-4 p-2 mt-2">
            <Loader2 className="h-4 w-4 animate-spin text-accent/50" />
            <span className="italic">Connecting to stream...</span>
          </div>
        ) : stream.events.length > 0 ? (
          <div className="mt-2 space-y-0.5">
            <StreamEvents events={stream.events} isActive={isActive} />
            {isActive && (
              <div className="mt-3 flex items-center gap-2 px-2 text-accent/50">
                <span
                  className="inline-block w-2 animate-pulse bg-accent/60"
                  style={{ height: "14px" }}
                />
                <span className="text-xs italic">Agent working...</span>
              </div>
            )}
          </div>
        ) : null}

        {/* Post-completion entries */}
        {!isActive && (
          <>
            {session.review_result && (
              <div className="mt-3">
                <ReviewResultCard review={session.review_result} />
              </div>
            )}
            {session.result &&
              !stream.events.some((e) => e.type === "result") && (
                <div className="mt-3 rounded-lg border border-accent/20 bg-accent/5 px-3 py-2">
                  <div className="flex items-center gap-2 mb-1">
                    <span className="material-symbols-outlined text-sm text-accent">
                      check_circle
                    </span>
                    <span className="text-xs font-bold uppercase tracking-wider text-accent">
                      Result
                    </span>
                  </div>
                  <MarkdownText text={session.result} className="text-xs" />
                </div>
              )}
          </>
        )}
      </div>

      {/* Bottom bar */}
      <div className="border-t border-edge bg-surface px-4 py-3">
        {/* Input row */}
        {canInstruct && (
          <div className="relative mb-2">
            <textarea
              value={instructPrompt}
              onChange={(e) => setInstructPrompt(e.target.value)}
              placeholder="Reply..."
              className="w-full resize-none rounded-lg border border-edge bg-surface-alt py-2 pl-3 pr-10 font-mono text-sm text-fg placeholder-fg-4 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent"
              rows={1}
              onKeyDown={(e) => {
                if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
                  void handleInstruct();
                }
              }}
            />
            <button
              onClick={() => void handleInstruct()}
              disabled={instructSession.isPending || !instructPrompt.trim()}
              className="absolute right-2 top-1/2 -translate-y-1/2 flex h-7 w-7 items-center justify-center rounded-md bg-accent text-page transition-opacity disabled:opacity-30"
            >
              {instructSession.isPending ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <span className="material-symbols-outlined text-base">
                  arrow_upward
                </span>
              )}
            </button>
          </div>
        )}

        {/* Actions row */}
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-2">
            {canCancel && (
              <button
                onClick={() => void handleCancel()}
                disabled={cancelSession.isPending}
                className="flex items-center gap-1.5 rounded-lg border border-red-900/50 bg-red-900/20 px-3 py-1.5 text-xs font-medium text-red-400 transition-colors hover:bg-red-900/30 disabled:opacity-50"
              >
                <span className="material-symbols-outlined text-sm">
                  stop_circle
                </span>
                {cancelSession.isPending ? "Cancelling..." : "Cancel"}
              </button>
            )}
            {canReview && (
              <button
                onClick={() => void handleReview()}
                disabled={reviewSession.isPending}
                className="flex items-center gap-1.5 rounded-lg border border-edge bg-surface px-3 py-1.5 text-xs font-medium text-fg-3 transition-colors hover:border-accent/30 hover:text-accent disabled:opacity-50"
              >
                {reviewSession.isPending ? (
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                ) : (
                  <span className="material-symbols-outlined text-sm">
                    rate_review
                  </span>
                )}
                {reviewSession.isPending ? "Reviewing..." : "Review"}
              </button>
            )}
            {canPostComments && (
              <button
                onClick={() => void handlePostComments()}
                disabled={postReviewComments.isPending}
                className="flex items-center gap-1.5 rounded-lg border border-accent/30 bg-accent/10 px-3 py-1.5 text-xs font-medium text-accent transition-colors hover:bg-accent/20 disabled:opacity-50"
              >
                {postReviewComments.isPending ? (
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                ) : (
                  <span className="material-symbols-outlined text-sm">
                    comment
                  </span>
                )}
                {postReviewComments.isPending
                  ? "Posting..."
                  : "Post Comments to MR"}
              </button>
            )}
            {/* PR Actions */}
            {canCreatePR && !session.pr_url && hasChanges && (
              <button
                onClick={() => void handleCreatePR()}
                disabled={createPR.isPending}
                className="flex items-center gap-1.5 rounded-lg border border-accent/30 bg-accent/10 px-3 py-1.5 text-xs font-medium text-accent transition-colors hover:bg-accent/20 disabled:opacity-50"
              >
                {createPR.isPending ? (
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                ) : (
                  <span className="material-symbols-outlined text-sm">call_merge</span>
                )}
                {createPR.isPending ? "Creating..." : "Create PR"}
              </button>
            )}
            {session.pr_url && hasChanges && !pushed && (!prStatus || prStatus.state === "open") && (
              <button
                onClick={() => void handlePushToPR()}
                disabled={pushToPR.isPending}
                className="flex items-center gap-1.5 rounded-lg border border-accent/30 bg-accent/10 px-3 py-1.5 text-xs font-medium text-accent transition-colors hover:bg-accent/20 disabled:opacity-50"
              >
                {pushToPR.isPending ? (
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                ) : (
                  <span className="material-symbols-outlined text-sm">upload</span>
                )}
                {pushToPR.isPending ? "Pushing..." : "Push to PR"}
              </button>
            )}
            {session.pr_url && hasChanges && prStatus && (prStatus.state === "merged" || prStatus.state === "closed") && (
              <button
                onClick={() => void handleCreatePR()}
                disabled={createPR.isPending}
                className="flex items-center gap-1.5 rounded-lg border border-accent/30 bg-accent/10 px-3 py-1.5 text-xs font-medium text-accent transition-colors hover:bg-accent/20 disabled:opacity-50"
              >
                {createPR.isPending ? (
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                ) : (
                  <span className="material-symbols-outlined text-sm">call_merge</span>
                )}
                {createPR.isPending ? "Creating..." : "Create New PR"}
              </button>
            )}
            {session.pr_url && (
              <Link
                to={session.pr_url}
                target="_blank"
                className="flex items-center gap-1.5 rounded-lg border border-edge bg-surface px-3 py-1.5 text-xs font-medium text-fg-3 transition-colors hover:border-accent/30 hover:text-accent"
              >
                <span className="material-symbols-outlined text-sm">open_in_new</span>
                View PR
                {prStatus && (
                  <span className={`ml-1 rounded-full px-1.5 py-0.5 text-[10px] font-bold uppercase ${
                    prStatus.state === "merged"
                      ? "bg-purple-500/20 text-purple-400"
                      : prStatus.state === "closed"
                        ? "bg-red-500/20 text-red-400"
                        : "bg-accent/20 text-accent"
                  }`}>
                    {prStatus.state}
                  </span>
                )}
              </Link>
            )}
          </div>

          <div className="ml-auto flex items-center gap-3 text-[10px] text-fg-4">
            {canInstruct && <span>Cmd+Enter to send</span>}
            {!canInstruct && !isActive && session.status === "completed" && (
              <span className="text-accent/60">Completed</span>
            )}
            {session.status === "failed" && (
              <span className="text-red-400/60">Failed</span>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

import { useState, useRef, useEffect } from "react";
import { useParams, Link } from "react-router";
import {
  ArrowUp,
  CircleAlert,
  CircleCheck,
  CircleStop,
  ExternalLink,
  FileDiff,
  FolderGit2,
  GitPullRequest,
  Loader2,
  MessageSquare,
  SearchCheck,
  Upload,
} from "lucide-react";
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
        <Loader2 className="size-8 animate-spin text-accent" />
      </div>
    );
  }

  if (!session) {
    return <p className="py-20 text-center text-fg-4">Session not found.</p>;
  }

  const isActive = ACTIVE_STATUSES.includes(session.status);
  const isPlan = session.session_type === "plan";
  const canCancel =
    session.status === "pending" ||
    session.status === "running" ||
    session.status === "cloning" ||
    session.status === "reviewing";
  const canInstruct =
    session.status === "completed" ||
    session.status === "awaiting_instruction" ||
    session.status === "pr_created";
  const hasChanges =
    session.changes_summary &&
    (session.changes_summary.files_modified > 0 ||
      session.changes_summary.files_created > 0 ||
      session.changes_summary.files_deleted > 0);
  const canCreatePR =
    (session.status === "completed" || session.status === "pr_created") &&
    !isPlan &&
    hasChanges;
  const isPrReview = session.session_type === "pr_review";
  const isReview = session.session_type === "review";
  const canReview =
    session.status === "completed" && !isPlan && !isPrReview && !isReview;
  const canPostComments =
    isPrReview && session.status === "completed" && !!session.review_result;

  const repoShort = session.repo_url
    .replace(/^https?:\/\//, "")
    .replace(/\.git$/, "");
  const repoName = repoShort.split("/").pop() || "Session";

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
      <div className="z-10 flex flex-wrap items-center gap-x-6 gap-y-2 border-b border-edge bg-surface px-6 py-3">
        {/* Left: ID + title + status + session type */}
        <div className="flex min-w-0 items-center gap-3">
          <div className="min-w-0">
            <p className="font-mono text-[11px] tracking-[0.14em] text-fg-3 uppercase">
              {session.id.slice(0, 8)}
            </p>
            <h2 className="truncate font-expanded text-lg leading-tight font-extrabold tracking-tight text-fg">
              {repoName}
            </h2>
          </div>
          <StatusBadge status={session.status} />
          {session.session_type && (
            <span className="rounded-[4px] border border-edge bg-surface-alt px-1.5 py-0.5 font-mono text-[10px] tracking-wider text-fg-3 uppercase">
              {session.session_type}
            </span>
          )}
        </div>

        {/* Center: stats */}
        <div className="flex items-center gap-4 font-mono text-xs text-fg-3">
          <span
            className="hidden items-center gap-1.5 sm:flex"
            title="Repository"
          >
            <FolderGit2 className="size-3.5 text-fg-4" />
            {repoShort}
          </span>
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
              <FileDiff className="size-3.5 text-info" />
              <span className="text-fg">
                {session.changes_summary!.diff_stats ||
                  formatChangesSummary(session.changes_summary!)}
              </span>
            </span>
          )}
          {session.pr_url && (
            <Link
              to={session.pr_url}
              target="_blank"
              className="flex items-center gap-1 text-info transition-colors hover:text-accent"
              title="Pull request"
              onClick={(e) => e.stopPropagation()}
            >
              <GitPullRequest className="size-3.5" />
              PR{session.pr_number ? ` #${session.pr_number}` : ""}
            </Link>
          )}
        </div>

        {/* Right: live/events indicator */}
        <div className="ml-auto flex items-center gap-2">
          {isActive && (
            <div className="flex items-center gap-1.5">
              <div className="animate-ember size-2 rounded-full bg-accent" />
              <span className="font-mono text-[10px] tracking-[0.14em] text-accent uppercase">
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
              className="font-mono text-xs text-danger underline transition-colors hover:text-danger/80"
            >
              Reconnect
            </button>
          )}
        </div>
      </div>

      {/* Error banner */}
      {session.error && (
        <div className="flex items-center gap-2 border-b border-danger/30 bg-danger/10 px-6 py-2">
          <CircleAlert className="size-4 shrink-0 text-danger" />
          <span className="font-mono text-xs text-danger">{session.error}</span>
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
          <div className="mt-2 flex items-center gap-3 p-2 text-fg-4">
            <Loader2 className="size-4 animate-spin text-fg-4" />
            <span className="text-xs">Connecting to stream…</span>
          </div>
        ) : stream.events.length > 0 ? (
          <div className="mt-2 space-y-0.5">
            <StreamEvents events={stream.events} isActive={isActive} />
            {isActive && (
              <div className="mt-3 flex items-center gap-2 px-2 text-fg-4">
                <span className="animate-soft-pulse inline-block h-3.5 w-2 bg-accent" />
                <span className="text-xs">Agent working…</span>
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
                <div className="mt-3 rounded-md border border-ok/25 bg-ok/10 px-3 py-2">
                  <div className="mb-1 flex items-center gap-2">
                    <CircleCheck className="size-4 text-ok" />
                    <span className="font-mono text-[10px] font-medium tracking-wider text-ok uppercase">
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
              placeholder="Reply…"
              className="w-full resize-none rounded-md border border-edge bg-input py-2 pr-10 pl-3 text-sm text-fg placeholder-fg-4 focus:border-accent focus:outline-none"
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
              className="absolute top-1/2 right-2 flex size-7 -translate-y-1/2 items-center justify-center rounded-md bg-accent text-white transition-colors hover:bg-accent-hover disabled:opacity-30"
            >
              {instructSession.isPending ? (
                <Loader2 className="size-3.5 animate-spin" />
              ) : (
                <ArrowUp className="size-4" />
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
                className="flex items-center gap-2 rounded-md border border-danger/30 bg-surface px-4 py-2 text-sm font-medium text-danger transition-colors hover:bg-danger/10 disabled:opacity-50"
              >
                <CircleStop className="size-4" />
                {cancelSession.isPending ? "Canceling…" : "Cancel"}
              </button>
            )}
            {canReview && (
              <button
                onClick={() => void handleReview()}
                disabled={reviewSession.isPending}
                className="flex items-center gap-2 rounded-md border border-edge bg-surface px-4 py-2 text-sm font-medium text-fg-2 transition-colors hover:border-fg-4 hover:text-fg disabled:opacity-50"
              >
                {reviewSession.isPending ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <SearchCheck className="size-4" />
                )}
                {reviewSession.isPending ? "Reviewing…" : "Review"}
              </button>
            )}
            {canPostComments && (
              <button
                onClick={() => void handlePostComments()}
                disabled={postReviewComments.isPending}
                className="flex items-center gap-2 rounded-md bg-accent px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-accent-hover disabled:opacity-50"
              >
                {postReviewComments.isPending ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <MessageSquare className="size-4" />
                )}
                {postReviewComments.isPending
                  ? "Posting…"
                  : "Post comments to MR"}
              </button>
            )}
            {/* PR Actions */}
            {canCreatePR && !session.pr_url && hasChanges && (
              <button
                onClick={() => void handleCreatePR()}
                disabled={createPR.isPending}
                className="flex items-center gap-2 rounded-md bg-accent px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-accent-hover disabled:opacity-50"
              >
                {createPR.isPending ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <GitPullRequest className="size-4" />
                )}
                {createPR.isPending ? "Creating…" : "Create PR"}
              </button>
            )}
            {session.pr_url &&
              hasChanges &&
              !pushed &&
              (!prStatus || prStatus.state === "open") && (
                <button
                  onClick={() => void handlePushToPR()}
                  disabled={pushToPR.isPending}
                  className="flex items-center gap-2 rounded-md bg-accent px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-accent-hover disabled:opacity-50"
                >
                  {pushToPR.isPending ? (
                    <Loader2 className="size-4 animate-spin" />
                  ) : (
                    <Upload className="size-4" />
                  )}
                  {pushToPR.isPending ? "Pushing…" : "Push to PR"}
                </button>
              )}
            {session.pr_url &&
              hasChanges &&
              prStatus &&
              (prStatus.state === "merged" || prStatus.state === "closed") && (
                <button
                  onClick={() => void handleCreatePR()}
                  disabled={createPR.isPending}
                  className="flex items-center gap-2 rounded-md bg-accent px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-accent-hover disabled:opacity-50"
                >
                  {createPR.isPending ? (
                    <Loader2 className="size-4 animate-spin" />
                  ) : (
                    <GitPullRequest className="size-4" />
                  )}
                  {createPR.isPending ? "Creating…" : "Create new PR"}
                </button>
              )}
            {session.pr_url && (
              <Link
                to={session.pr_url}
                target="_blank"
                className="flex items-center gap-2 rounded-md border border-edge bg-surface px-4 py-2 text-sm font-medium text-fg-2 transition-colors hover:border-fg-4 hover:text-fg"
              >
                <ExternalLink className="size-4" />
                View PR
                {prStatus && (
                  <span
                    className={`ml-1 rounded-[4px] border px-1.5 py-0.5 font-mono text-[10px] font-medium uppercase ${
                      prStatus.state === "merged"
                        ? "border-info/30 bg-info/10 text-info"
                        : prStatus.state === "closed"
                          ? "border-danger/30 bg-danger/10 text-danger"
                          : "border-ok/30 bg-ok/10 text-ok"
                    }`}
                  >
                    {prStatus.state}
                  </span>
                )}
              </Link>
            )}
          </div>

          <div className="ml-auto flex items-center gap-3 text-[10px] text-fg-4">
            {canInstruct && <span>Cmd+Enter to send</span>}
            {!canInstruct && !isActive && session.status === "completed" && (
              <span className="text-ok/70">Completed</span>
            )}
            {session.status === "failed" && (
              <span className="text-danger/70">Failed</span>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

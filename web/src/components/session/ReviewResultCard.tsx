import type { ReviewResult } from "../../types";

export function ReviewResultCard({ review }: { review: ReviewResult }) {
  const verdictColors = {
    approve: "text-accent border-accent/30 bg-accent/10",
    request_changes: "text-yellow-400 border-yellow-500/30 bg-yellow-500/10",
    comment: "text-blue-400 border-blue-500/30 bg-blue-500/10",
  };

  const severityColors = {
    critical: "text-red-400 bg-red-900/20 border-red-900/30",
    major: "text-orange-400 bg-orange-900/20 border-orange-900/30",
    minor: "text-yellow-400 bg-yellow-900/20 border-yellow-900/30",
    suggestion: "text-blue-400 bg-blue-900/20 border-blue-900/30",
  };

  return (
    <div>
      <h3 className="mb-3 flex items-center gap-2 text-sm font-bold uppercase tracking-wider text-fg-2">
        <span className="material-symbols-outlined text-blue-400 text-base">
          rate_review
        </span>
        Code Review
      </h3>

      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <span
            className={`rounded-full border px-3 py-1 text-xs font-bold uppercase ${verdictColors[review.verdict]}`}
          >
            {review.verdict.replace("_", " ")}
          </span>
          <span className="font-mono text-sm text-fg">
            Score: <span className="text-accent font-bold">{review.score}</span>
            /10
          </span>
        </div>

        <p className="text-sm text-fg-2">{review.summary}</p>

        {review.issues && review.issues.length > 0 && (
          <div className="space-y-2">
            {review.issues.map((issue, i) => (
              <div
                key={i}
                className={`rounded-lg border p-3 ${severityColors[issue.severity]}`}
              >
                <div className="flex items-center gap-2 mb-1">
                  <span className="text-xs font-bold uppercase">
                    {issue.severity}
                  </span>
                  <span className="font-mono text-xs text-fg-3">
                    {issue.file}
                    {issue.line ? `:${issue.line}` : ""}
                  </span>
                </div>
                <p className="text-sm">{issue.description}</p>
                {issue.suggestion && (
                  <p className="mt-1 text-xs text-fg-3">
                    Suggestion: {issue.suggestion}
                  </p>
                )}
              </div>
            ))}
          </div>
        )}

        <p className="text-xs text-fg-4">
          Reviewed by {review.reviewed_by} in{" "}
          {review.duration_seconds.toFixed(1)}s
        </p>
      </div>
    </div>
  );
}

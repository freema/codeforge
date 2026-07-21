import type { ReviewResult } from "../../types";

export function ReviewResultCard({ review }: { review: ReviewResult }) {
  const verdictColors = {
    approve: "border-ok/30 bg-ok/10 text-ok",
    request_changes: "border-warn/30 bg-warn/10 text-warn",
    comment: "border-info/30 bg-info/10 text-info",
  };

  const severityColors = {
    critical: "border-danger/30 bg-danger/10 text-danger",
    major: "border-warn/30 bg-warn/10 text-warn",
    minor: "border-warn/30 bg-warn/10 text-warn",
    suggestion: "border-info/30 bg-info/10 text-info",
  };

  const scoreColor =
    review.score >= 8
      ? "text-ok"
      : review.score >= 5
        ? "text-warn"
        : "text-danger";

  return (
    <div className="overflow-hidden rounded-md border border-edge bg-surface">
      <div className="border-b border-edge px-4 py-3">
        <span className="eyebrow">Code review</span>
      </div>

      <div className="space-y-3 p-4">
        <div className="flex items-center justify-between">
          <span
            className={`inline-flex items-center rounded-[4px] border px-2 py-0.5 font-mono text-[10px] font-medium tracking-[0.08em] uppercase ${verdictColors[review.verdict]}`}
          >
            {review.verdict.replace("_", " ")}
          </span>
          <span className="font-mono text-sm text-fg-2">
            Score:{" "}
            <span className={`font-semibold ${scoreColor}`}>
              {review.score}
            </span>
            /10
          </span>
        </div>

        <p className="text-sm text-fg-2">{review.summary}</p>

        {review.issues && review.issues.length > 0 && (
          <div className="space-y-2">
            {review.issues.map((issue, i) => (
              <div
                key={i}
                className="rounded-md border border-edge bg-surface-alt p-3"
              >
                <div className="mb-1 flex items-center gap-2">
                  <span
                    className={`inline-flex items-center rounded-[4px] border px-1.5 py-0.5 font-mono text-[10px] font-medium tracking-[0.08em] uppercase ${severityColors[issue.severity]}`}
                  >
                    {issue.severity}
                  </span>
                  <span className="font-mono text-xs text-fg-3">
                    {issue.file}
                    {issue.line ? `:${issue.line}` : ""}
                  </span>
                </div>
                <p className="text-sm text-fg-2">{issue.description}</p>
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

export function formatDuration(seconds: number): string {
  const m = Math.floor(seconds / 60);
  const s = Math.round(seconds % 60);
  if (m > 0)
    return `${String(m).padStart(2, "0")}m ${String(s).padStart(2, "0")}s`;
  return `${s}s`;
}

export function formatTimeAgo(dateStr: string): string {
  const seconds = Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000);
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

export function formatChangesSummary(c: {
  files_modified: number;
  files_created: number;
  files_deleted: number;
}): string {
  const parts: string[] = [];
  if (c.files_modified > 0) parts.push(`${c.files_modified} mod`);
  if (c.files_created > 0) parts.push(`${c.files_created} new`);
  if (c.files_deleted > 0) parts.push(`${c.files_deleted} del`);
  return parts.join(", ");
}

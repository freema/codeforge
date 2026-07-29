import { useMemo, useState } from "react";
import { ChevronsUpDown, Globe, Loader2, Lock, Search } from "lucide-react";
import type { Repository } from "../types";

export default function RepoSelector({
  repos,
  selected,
  loading,
  onSelect,
  autoFocusSearch = true,
}: {
  repos: Repository[];
  selected: Repository | null;
  loading: boolean;
  onSelect: (repo: Repository) => void;
  autoFocusSearch?: boolean;
}) {
  const [search, setSearch] = useState("");
  const [open, setOpen] = useState(!selected);

  const filtered = useMemo(() => {
    if (!search) return repos.slice(0, 20);
    const q = search.toLowerCase();
    return repos.filter(
      (r) =>
        r.full_name.toLowerCase().includes(q) ||
        r.description?.toLowerCase().includes(q),
    );
  }, [repos, search]);

  if (!open && selected) {
    return (
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="flex w-full items-center justify-between rounded-md border border-edge bg-input p-3 text-left transition-colors hover:border-fg-4"
      >
        <span className="font-mono text-sm text-fg">{selected.full_name}</span>
        <ChevronsUpDown className="size-4 text-fg-3" />
      </button>
    );
  }

  return (
    <div className="space-y-2">
      <div className="relative">
        <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-fg-4" />
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search repositories…"
          className="w-full rounded-md border border-edge bg-input py-2 pr-3 pl-9 font-mono text-sm text-fg placeholder-fg-4 transition-colors focus:border-accent focus:outline-none"
          autoFocus={autoFocusSearch}
        />
      </div>

      {loading ? (
        <div className="flex items-center justify-center py-8">
          <Loader2 className="size-5 animate-spin text-accent" />
        </div>
      ) : (
        <div className="max-h-64 overflow-y-auto rounded-md border border-edge">
          {filtered.length === 0 ? (
            <p className="p-4 text-center text-sm text-fg-4">
              No repositories found
            </p>
          ) : (
            filtered.map((repo) => (
              <button
                key={repo.full_name}
                type="button"
                onClick={() => {
                  onSelect(repo);
                  setOpen(false);
                  setSearch("");
                }}
                className={`flex w-full items-center gap-3 border-b border-edge p-3 text-left transition-colors last:border-b-0 hover:bg-surface-alt ${
                  selected?.full_name === repo.full_name ? "bg-accent-soft" : ""
                }`}
              >
                {repo.private ? (
                  <Lock className="size-4 shrink-0 text-fg-4" />
                ) : (
                  <Globe className="size-4 shrink-0 text-fg-4" />
                )}
                <div className="flex-1 overflow-hidden">
                  <p className="truncate font-mono text-sm text-fg">
                    {repo.full_name}
                  </p>
                  {repo.description && (
                    <p className="truncate text-xs text-fg-4">
                      {repo.description}
                    </p>
                  )}
                </div>
                <span className="shrink-0 rounded-[4px] border border-edge bg-surface-alt px-1.5 py-0.5 font-mono text-[10px] text-fg-3">
                  {repo.default_branch}
                </span>
              </button>
            ))
          )}
        </div>
      )}
    </div>
  );
}

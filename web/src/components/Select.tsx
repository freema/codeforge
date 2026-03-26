import { useState, useRef, useEffect, useMemo } from "react";

export interface SelectOption {
  value: string;
  label: string;
}

interface SelectProps {
  value: string;
  onChange: (value: string) => void;
  options: SelectOption[];
  placeholder?: string;
}

export default function Select({
  value,
  onChange,
  options,
  placeholder,
}: SelectProps) {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const ref = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const selected = options.find((o) => o.value === value);
  const displayLabel = selected?.label ?? placeholder ?? "Select...";

  const filtered = useMemo(() => {
    if (!search) return options;
    const q = search.toLowerCase();
    return options.filter((o) => o.label.toLowerCase().includes(q));
  }, [options, search]);

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
        setSearch("");
      }
    }
    if (open) {
      document.addEventListener("mousedown", handleClick);
      return () => document.removeEventListener("mousedown", handleClick);
    }
  }, [open]);

  useEffect(() => {
    if (open && inputRef.current) {
      inputRef.current.focus();
    }
  }, [open]);

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="flex w-full items-center justify-between rounded-lg border border-edge bg-surface px-3 py-2.5 text-left text-sm font-mono transition-colors hover:border-fg-4 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent"
      >
        <span className={selected ? "text-fg" : "text-fg-4"}>
          {displayLabel}
        </span>
        <span
          className="material-symbols-outlined text-base text-fg-4 transition-transform"
          style={{ transform: open ? "rotate(180deg)" : "none" }}
        >
          expand_more
        </span>
      </button>

      {open && (
        <div className="absolute left-0 right-0 top-full z-50 mt-1 rounded-lg border border-edge bg-surface-alt shadow-lg shadow-black/40 backdrop-blur-xl">
          {options.length > 5 && (
            <div className="border-b border-edge p-1.5">
              <input
                ref={inputRef}
                type="text"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="Search..."
                className="w-full rounded-md border border-edge bg-surface px-2.5 py-1.5 text-xs text-fg font-mono placeholder-fg-4 focus:border-accent focus:outline-none"
              />
            </div>
          )}
          <div className="max-h-64 overflow-y-auto">
            {filtered.length === 0 ? (
              <div className="px-3 py-3 text-center text-xs text-fg-4">
                No results
              </div>
            ) : (
              filtered.map((opt) => (
                <button
                  key={opt.value}
                  type="button"
                  onClick={() => {
                    onChange(opt.value);
                    setOpen(false);
                    setSearch("");
                  }}
                  className={`flex w-full items-center gap-2 px-3 py-2.5 text-left text-sm font-mono transition-colors first:rounded-t-lg last:rounded-b-lg ${
                    opt.value === value
                      ? "bg-accent/10 text-accent"
                      : "text-fg hover:bg-accent/5"
                  }`}
                >
                  {opt.label}
                </button>
              ))
            )}
          </div>
        </div>
      )}
    </div>
  );
}

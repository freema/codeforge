import {
  useState,
  useRef,
  useEffect,
  useMemo,
  useLayoutEffect,
  useCallback,
} from "react";
import { createPortal } from "react-dom";
import { ChevronDown } from "lucide-react";

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

interface PanelPosition {
  left: number;
  width: number;
  top?: number;
  bottom?: number;
}

// Estimated max panel height: search bar + max-h-64 option list + chrome.
const PANEL_MAX_HEIGHT = 320;

export default function Select({
  value,
  onChange,
  options,
  placeholder,
}: SelectProps) {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const [position, setPosition] = useState<PanelPosition | null>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const selected = options.find((o) => o.value === value);
  const displayLabel = selected?.label ?? placeholder ?? "Select...";

  const filtered = useMemo(() => {
    if (!search) return options;
    const q = search.toLowerCase();
    return options.filter((o) => o.label.toLowerCase().includes(q));
  }, [options, search]);

  // The panel renders in a portal with position:fixed so no overflow-hidden
  // ancestor can clip it; the position derives from the trigger rect and
  // flips above the trigger when the viewport space below is too small.
  const updatePosition = useCallback(() => {
    const rect = triggerRef.current?.getBoundingClientRect();
    if (!rect) return;
    const spaceBelow = window.innerHeight - rect.bottom;
    const openUp = spaceBelow < PANEL_MAX_HEIGHT && rect.top > spaceBelow;
    setPosition({
      left: rect.left,
      width: rect.width,
      ...(openUp
        ? { bottom: window.innerHeight - rect.top + 4 }
        : { top: rect.bottom + 4 }),
    });
  }, []);

  useLayoutEffect(() => {
    if (open) updatePosition();
  }, [open, updatePosition]);

  useEffect(() => {
    if (!open) return;
    function handleClick(e: MouseEvent) {
      const t = e.target as Node;
      if (triggerRef.current?.contains(t) || panelRef.current?.contains(t)) {
        return;
      }
      setOpen(false);
      setSearch("");
    }
    window.addEventListener("resize", updatePosition);
    // capture: true catches scrolling in any ancestor container, not just window
    window.addEventListener("scroll", updatePosition, true);
    document.addEventListener("mousedown", handleClick);
    return () => {
      window.removeEventListener("resize", updatePosition);
      window.removeEventListener("scroll", updatePosition, true);
      document.removeEventListener("mousedown", handleClick);
    };
  }, [open, updatePosition]);

  useEffect(() => {
    if (open && inputRef.current) {
      inputRef.current.focus();
    }
  }, [open]);

  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        onClick={() => setOpen(!open)}
        className="flex w-full items-center justify-between rounded-md border border-edge bg-input px-3 py-2.5 text-left font-mono text-sm transition-colors hover:border-fg-4 focus:border-accent focus:outline-none"
      >
        <span className={selected ? "text-fg" : "text-fg-4"}>
          {displayLabel}
        </span>
        <ChevronDown
          className={`size-4 text-fg-4 transition-transform ${open ? "rotate-180" : ""}`}
        />
      </button>

      {open &&
        position &&
        createPortal(
          <div
            ref={panelRef}
            style={{
              position: "fixed",
              left: position.left,
              width: position.width,
              top: position.top,
              bottom: position.bottom,
            }}
            className="z-50 rounded-md border border-edge bg-surface shadow-lg shadow-black/20"
          >
            {options.length > 5 && (
              <div className="border-b border-edge p-1.5">
                <input
                  ref={inputRef}
                  type="text"
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  placeholder="Search…"
                  className="w-full rounded-md border border-edge bg-input px-2.5 py-1.5 font-mono text-xs text-fg placeholder-fg-4 focus:border-accent focus:outline-none"
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
                    className={`flex w-full items-center gap-2 px-3 py-2.5 text-left font-mono text-sm transition-colors first:rounded-t-md last:rounded-b-md ${
                      opt.value === value
                        ? "bg-accent-soft text-accent"
                        : "text-fg hover:bg-surface-alt"
                    }`}
                  >
                    {opt.label}
                  </button>
                ))
              )}
            </div>
          </div>,
          document.body,
        )}
    </>
  );
}

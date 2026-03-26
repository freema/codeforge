import {
  createContext,
  useContext,
  useState,
  useCallback,
  type ReactNode,
} from "react";
import { X, CheckCircle, AlertCircle, Info } from "lucide-react";

type ToastType = "success" | "error" | "info";

interface Toast {
  id: number;
  type: ToastType;
  message: string;
}

interface ToastContextValue {
  toast: (type: ToastType, message: string) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

let nextId = 0;

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const toast = useCallback((type: ToastType, message: string) => {
    const id = nextId++;
    setToasts((prev) => [...prev, { id, type, message }]);
    globalThis.setTimeout(() => {
      setToasts((prev) => prev.filter((t) => t.id !== id));
    }, 5000);
  }, []);

  const dismiss = useCallback((id: number) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  return (
    <ToastContext.Provider value={{ toast }}>
      {children}

      {/* Toast container */}
      <div className="fixed bottom-4 right-4 z-50 space-y-2">
        {toasts.map((t) => (
          <ToastItem key={t.id} toast={t} onDismiss={() => dismiss(t.id)} />
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast(): ToastContextValue {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error("useToast must be used within ToastProvider");
  return ctx;
}

const iconMap = {
  success: CheckCircle,
  error: AlertCircle,
  info: Info,
};

const colorMap = {
  success: "border-green-800 bg-green-500/10 text-green-400",
  error: "border-red-800 bg-red-500/10 text-red-400",
  info: "border-emerald-800 bg-emerald-500/10 text-emerald-400",
};

function ToastItem({
  toast,
  onDismiss,
}: {
  toast: Toast;
  onDismiss: () => void;
}) {
  const Icon = iconMap[toast.type];

  return (
    <div
      className={`flex items-center gap-3 rounded-lg border px-4 py-3 shadow-lg ${colorMap[toast.type]}`}
    >
      <Icon className="h-4 w-4 shrink-0" />
      <p className="text-sm">{toast.message}</p>
      <button
        onClick={onDismiss}
        className="ml-2 shrink-0 opacity-60 hover:opacity-100"
      >
        <X className="h-3.5 w-3.5" />
      </button>
    </div>
  );
}

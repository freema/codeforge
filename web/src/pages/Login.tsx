import { useState, useEffect, type FormEvent } from "react";
import { useNavigate } from "react-router";
import { Loader2, Sun, Moon, Anvil, KeyRound } from "lucide-react";
import { useAuth } from "../context/AuthContext";
import { useTheme } from "../context/ThemeContext";

export default function Login() {
  const { login } = useAuth();
  const { theme, toggle } = useTheme();
  const navigate = useNavigate();

  const [token, setToken] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [mounted, setMounted] = useState(false);

  useEffect(() => {
    setMounted(true);
  }, []);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);

    try {
      await login(token);
      void navigate("/");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Connection failed");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="relative flex min-h-screen items-center justify-center overflow-hidden bg-page">
      {/* Distant furnace — faint warm light at the bottom, dark mode only */}
      <div
        className="pointer-events-none fixed inset-x-0 bottom-0 hidden h-1/2 dark:block"
        style={{
          background:
            "radial-gradient(ellipse 70% 100% at 50% 100%, rgba(255,109,46,0.07), transparent 70%)",
        }}
      />

      {/* Theme toggle */}
      <button
        onClick={toggle}
        className="absolute top-4 right-4 z-20 rounded-md p-2 text-fg-3 transition-colors hover:bg-surface-alt hover:text-fg"
        title={`Switch to ${theme === "dark" ? "light" : "dark"} mode`}
      >
        {theme === "dark" ? (
          <Sun className="size-4" />
        ) : (
          <Moon className="size-4" />
        )}
      </button>

      <main
        className={`relative z-10 w-full max-w-sm p-6 transition-all duration-700 ${
          mounted ? "translate-y-0 opacity-100" : "translate-y-3 opacity-0"
        }`}
      >
        {/* Mark + wordmark */}
        <div className="mb-8">
          <div className="mb-5 inline-flex size-12 items-center justify-center rounded-md border border-accent-muted bg-accent-soft">
            <Anvil className="size-6 text-accent" strokeWidth={1.75} />
          </div>
          <h1 className="font-expanded text-4xl font-black tracking-tight text-fg">
            CodeForge
          </h1>
          <p className="mt-2 text-sm text-fg-3">AI sessions over your repos</p>
        </div>

        {/* Molten divider */}
        <div className="relative mb-8 h-px overflow-hidden bg-edge">
          <span className="animate-molten-sweep absolute inset-y-0 w-1/4 bg-gradient-to-r from-transparent via-accent to-transparent" />
        </div>

        {error && (
          <div className="mb-5 rounded-md border border-danger/30 bg-danger/10 px-3 py-2.5 text-sm text-danger">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="space-y-5">
          <div>
            <label
              htmlFor="access-key"
              className="mb-2 block text-sm font-medium text-fg-2"
            >
              Access key
            </label>
            <div className="relative">
              <KeyRound className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-fg-4" />
              <input
                id="access-key"
                type="password"
                value={token}
                onChange={(e) => setToken(e.target.value)}
                placeholder="Paste your key"
                required
                autoFocus
                className="w-full rounded-md border border-edge bg-input py-2.5 pr-3 pl-9 font-mono text-sm text-fg placeholder-fg-4 transition-colors focus:border-accent focus:outline-none"
              />
            </div>
          </div>

          <button
            type="submit"
            disabled={loading}
            className="flex w-full items-center justify-center gap-2 rounded-md bg-accent px-4 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-accent-hover disabled:opacity-50"
          >
            {loading ? (
              <>
                <Loader2 className="size-4 animate-spin" />
                Signing in…
              </>
            ) : (
              "Sign in"
            )}
          </button>
        </form>

        <p className="mt-10 font-mono text-[11px] text-fg-4">
          Tomas Grasl · tomasgrasl.cz
        </p>
      </main>
    </div>
  );
}

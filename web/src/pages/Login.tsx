import {
  useState,
  useEffect,
  useRef,
  useCallback,
  type FormEvent,
} from "react";
import { useNavigate } from "react-router";
import { Loader2, Sun, Moon } from "lucide-react";
import { useAuth } from "../context/AuthContext";
import { useTheme } from "../context/ThemeContext";

const MATRIX_CHARS =
  "01アイウエオカキクケコサシスセソタチツテトナニヌネノハヒフヘホマミムメモヤユヨラリルレロワヲン";

function useMatrixCanvas(canvasRef: React.RefObject<HTMLCanvasElement | null>) {
  const dropsRef = useRef<number[]>([]);
  const rafRef = useRef<number>(0);
  const lastFrameRef = useRef<number>(0);

  const draw = useCallback(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const now = performance.now();
    if (now - lastFrameRef.current < 50) {
      rafRef.current = requestAnimationFrame(draw);
      return;
    }
    lastFrameRef.current = now;

    ctx.fillStyle = "rgba(15, 35, 20, 0.05)";
    ctx.fillRect(0, 0, canvas.width, canvas.height);

    ctx.fillStyle = "#00ff40";
    ctx.font = "15px monospace";

    const drops = dropsRef.current;
    for (let i = 0; i < drops.length; i++) {
      const text =
        MATRIX_CHARS[Math.floor(Math.random() * MATRIX_CHARS.length)] ?? "0";
      ctx.fillText(text, i * 20, (drops[i] ?? 0) * 20);
      if ((drops[i] ?? 0) * 20 > canvas.height && Math.random() > 0.975) {
        drops[i] = 0;
      }
      drops[i] = (drops[i] ?? 0) + 1;
    }

    rafRef.current = requestAnimationFrame(draw);
  }, [canvasRef]);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    const resize = () => {
      canvas.width = window.innerWidth;
      canvas.height = window.innerHeight;
      const columns = Math.floor(canvas.width / 20);
      dropsRef.current = Array.from({ length: columns }, () => 1);
    };

    resize();
    window.addEventListener("resize", resize);
    rafRef.current = requestAnimationFrame(draw);

    return () => {
      window.removeEventListener("resize", resize);
      cancelAnimationFrame(rafRef.current);
    };
  }, [canvasRef, draw]);
}

export default function Login() {
  const { login } = useAuth();
  const { theme, toggle } = useTheme();
  const navigate = useNavigate();
  const canvasRef = useRef<HTMLCanvasElement>(null);

  const [token, setToken] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [mounted, setMounted] = useState(false);

  useMatrixCanvas(canvasRef);

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
    <div className="relative flex min-h-screen items-center justify-center overflow-hidden bg-[#f5f8f6] selection:bg-[#00ff40] selection:text-[#0f2314] dark:bg-[#0f2314]">
      {/* Matrix rain canvas — dark mode only */}
      <canvas
        ref={canvasRef}
        className="pointer-events-none fixed inset-0 z-0 hidden opacity-15 dark:block"
      />

      {/* Dark gradient overlay */}
      <div className="fixed inset-0 z-[1] hidden bg-gradient-to-b from-[#0f2314]/90 via-[#0f2314]/80 to-[#0f2314]/90 dark:block" />

      {/* Light mode background */}
      <div className="fixed inset-0 z-[1] bg-[#f5f8f6] dark:hidden" />

      {/* CRT scanline overlay — dark mode only */}
      <div className="crt-overlay pointer-events-none fixed inset-0 z-[10] hidden dark:block" />

      {/* Scanline bar sweep — dark mode only */}
      <div
        className="animate-scanline pointer-events-none fixed inset-x-0 top-0 z-[10] hidden h-[100px] bg-gradient-to-b from-transparent via-[#00ff40]/10 to-transparent opacity-10 dark:block"
        style={{ animationDuration: "6s" }}
      />

      {/* Theme toggle */}
      <button
        onClick={toggle}
        className="absolute top-4 right-4 z-20 rounded-lg border border-edge bg-surface p-2 text-fg-3 transition-colors hover:text-fg"
        title={`Switch to ${theme === "dark" ? "light" : "dark"} mode`}
      >
        {theme === "dark" ? (
          <Sun className="h-4 w-4" />
        ) : (
          <Moon className="h-4 w-4" />
        )}
      </button>

      {/* Main container */}
      <main
        className={`relative z-20 w-full max-w-md p-6 transition-all duration-700 ${
          mounted ? "translate-y-0 opacity-100" : "translate-y-4 opacity-0"
        }`}
      >
        {/* Cyberpunk card */}
        <div className="group relative">
          {/* Glow effect behind card */}
          <div className="absolute -inset-1 rounded-lg bg-[#00ff40]/20 opacity-25 blur transition duration-1000 group-hover:opacity-40 group-hover:duration-200 dark:block hidden" />

          {/* Card body */}
          <div className="cyber-card neon-border relative flex flex-col items-center border border-[#00ff40]/30 bg-slate-100/90 p-10 shadow-2xl backdrop-blur-xl dark:bg-black/80">
            {/* Corner brackets (SVGs) */}
            <svg
              className="absolute top-0 left-0 h-8 w-8 text-[#00ff40] opacity-80"
              fill="none"
              viewBox="0 0 32 32"
            >
              <path d="M1 31V1H31" stroke="currentColor" strokeWidth="2" />
            </svg>
            <svg
              className="absolute top-0 right-0 h-8 w-8 rotate-90 text-[#00ff40] opacity-80"
              fill="none"
              viewBox="0 0 32 32"
            >
              <path d="M1 31V1H31" stroke="currentColor" strokeWidth="2" />
            </svg>
            <svg
              className="absolute right-0 bottom-0 h-8 w-8 rotate-180 text-[#00ff40] opacity-80"
              fill="none"
              viewBox="0 0 32 32"
            >
              <path d="M1 31V1H31" stroke="currentColor" strokeWidth="2" />
            </svg>
            <svg
              className="absolute bottom-0 left-0 h-8 w-8 -rotate-90 text-[#00ff40] opacity-80"
              fill="none"
              viewBox="0 0 32 32"
            >
              <path d="M1 31V1H31" stroke="currentColor" strokeWidth="2" />
            </svg>

            {/* Header */}
            <div className="mb-10 w-full text-center">
              <div className="mb-4 inline-flex items-center justify-center">
                <span className="material-symbols-outlined animate-pulse text-5xl text-[#00ff40]">
                  terminal
                </span>
              </div>
              <h1 className="neon-text mb-2 font-mono text-5xl font-bold tracking-tight text-slate-900 dark:text-white">
                CodeForge
              </h1>
              <div className="flex items-center justify-center gap-2 font-mono text-xs tracking-[0.2em] text-[#00ff40]/80">
                <span className="h-2 w-2 animate-pulse rounded-full bg-[#00ff40]" />
                REMOTE AI TASK RUNNER
              </div>
            </div>

            {/* Terminal form */}
            <div className="w-full space-y-8">
              {/* Error display */}
              {error && (
                <div className="rounded border border-red-300 bg-red-50 px-3 py-2 font-mono text-xs text-red-600 dark:border-red-900/50 dark:bg-red-500/5 dark:text-red-400">
                  <span className="text-red-400 dark:text-red-500/60">
                    ERR{" "}
                  </span>
                  {error}
                </div>
              )}

              <form onSubmit={handleSubmit} className="space-y-8">
                {/* Input group */}
                <div className="group/input relative">
                  <label className="mb-2 block pl-1 font-mono text-xs uppercase tracking-widest text-slate-500 dark:text-[#00ff40]">
                    &gt; Enter_Access_Key:
                  </label>
                  <div className="relative flex items-center border-b-2 border-slate-300 bg-slate-200/50 transition-colors duration-300 focus-within:border-[#00ff40] dark:border-[#00ff40]/40 dark:bg-black/20 dark:focus-within:border-[#00ff40]">
                    <span className="material-symbols-outlined pl-2 text-sm text-slate-400 dark:text-[#00ff40]/50">
                      vpn_key
                    </span>
                    <input
                      type="password"
                      value={token}
                      onChange={(e) => setToken(e.target.value)}
                      placeholder="********"
                      required
                      className="w-full border-none bg-transparent px-3 py-3 font-mono tracking-widest text-slate-900 caret-slate-900 placeholder-slate-400 focus:ring-0 focus:outline-none dark:text-[#00ff40] dark:caret-[#00ff40] dark:placeholder-[#00ff40]/30"
                    />
                  </div>
                </div>

                {/* Action button */}
                <button
                  type="submit"
                  disabled={loading}
                  className="group/btn relative flex w-full items-center justify-between overflow-hidden rounded-lg border border-slate-300 bg-slate-200/50 px-6 py-4 font-mono font-bold uppercase tracking-widest text-slate-700 transition-all duration-300 hover:bg-slate-300/50 hover:shadow-lg disabled:opacity-40 dark:border-[#00ff40] dark:bg-[#00ff40]/10 dark:text-[#00ff40] dark:hover:bg-[#00ff40]/20 dark:hover:shadow-[0_0_20px_rgba(0,255,64,0.4)]"
                >
                  {loading ? (
                    <span className="flex items-center gap-2">
                      <Loader2 className="h-4 w-4 animate-spin" />
                      Connecting...
                    </span>
                  ) : (
                    <>
                      <span>Authenticate</span>
                      <span className="transition-transform duration-300 group-hover/btn:translate-x-1">
                        &gt;
                      </span>
                    </>
                  )}
                  {/* Shimmer effect inside button */}
                  <div className="absolute inset-0 h-full w-1/2 -translate-x-full skew-x-12 bg-gradient-to-r from-transparent via-[#00ff40]/20 to-transparent group-hover/btn:animate-[shimmer_1s_infinite]" />
                </button>
              </form>

              {/* Footer info */}
              <div className="mt-6 text-center">
                <p className="font-mono text-[10px] text-slate-400 dark:text-slate-500">
                  created by Tomas Grasl{" "}
                  <span className="text-slate-500 dark:text-slate-600">
                    &lt;tomasgrasl.cz&gt;
                  </span>
                </p>
              </div>
            </div>
          </div>
        </div>

        {/* Bottom abstract decoration */}
        <div className="absolute -bottom-12 left-1/2 h-1 w-3/4 -translate-x-1/2 bg-gradient-to-r from-transparent via-[#00ff40]/30 to-transparent" />
        <div className="absolute -bottom-12 left-1/2 h-16 w-1/2 -translate-x-1/2 rounded-t-full bg-[#00ff40]/5 blur-xl" />
      </main>
    </div>
  );
}

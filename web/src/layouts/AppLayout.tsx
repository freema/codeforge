import { useState } from "react";
import { Outlet, NavLink, useNavigate } from "react-router";
import { useAuth } from "../context/AuthContext";
import { useTheme } from "../context/ThemeContext";
import { useHealth } from "../hooks/useHealth";

const navItems = [
  { to: "/", label: "Dashboard", icon: "dashboard", end: true },
  { to: "/sessions", label: "Sessions", icon: "task", end: false },
  { to: "/workflows", label: "Workflows", icon: "account_tree", end: false },
  { to: "/settings", label: "Settings", icon: "settings", end: false },
];

export default function AppLayout() {
  const { logout } = useAuth();
  const { theme, toggle } = useTheme();
  const navigate = useNavigate();
  const { data: health } = useHealth();
  const [sidebarOpen, setSidebarOpen] = useState(false);

  function handleLogout() {
    logout();
    void navigate("/login");
  }

  return (
    <div className="relative flex h-screen w-full overflow-hidden bg-page">
      {/* Grid background */}
      <div
        className="pointer-events-none absolute inset-0 z-0 opacity-20"
        style={{
          backgroundImage:
            "linear-gradient(to right, var(--th-grid-line) 1px, transparent 1px), linear-gradient(to bottom, var(--th-grid-line) 1px, transparent 1px)",
          backgroundSize: "40px 40px",
          maskImage:
            "linear-gradient(to bottom, rgba(0,0,0,1) 80%, rgba(0,0,0,0))",
          WebkitMaskImage:
            "linear-gradient(to bottom, rgba(0,0,0,1) 80%, rgba(0,0,0,0))",
        }}
      />

      {/* Mobile overlay */}
      {sidebarOpen && (
        <div
          className="fixed inset-0 z-20 bg-page/80 backdrop-blur-sm lg:hidden"
          onClick={() => setSidebarOpen(false)}
        />
      )}

      {/* Sidebar */}
      <aside
        className={`fixed inset-y-0 left-0 z-30 flex w-72 flex-col border-r border-edge backdrop-blur-xl transition-transform lg:static lg:translate-x-0 ${
          sidebarOpen ? "translate-x-0" : "-translate-x-full"
        }`}
        style={{ background: "var(--th-surface-glass)" }}
      >
        <div className="flex h-full flex-col justify-between p-4">
          {/* Top section */}
          <div className="flex flex-col gap-6">
            {/* Logo */}
            <div className="flex items-center gap-3 px-2 py-2">
              <div className="relative flex h-10 w-10 items-center justify-center rounded-lg border border-accent/30 bg-surface-alt shadow-[0_0_15px_rgba(0,255,64,0.15)]">
                <span className="material-symbols-outlined text-2xl text-accent">
                  terminal
                </span>
              </div>
              <div className="flex flex-col">
                <h1
                  className="text-lg font-bold tracking-tight text-fg"
                  style={{ textShadow: "0 0 10px rgba(0,255,64,0.5)" }}
                >
                  CodeForge
                </h1>
                {health?.version && (
                  <p className="font-mono text-xs text-accent/70">
                    {health.version}
                  </p>
                )}
              </div>
            </div>

            {/* Navigation */}
            <nav className="flex flex-col gap-2">
              {navItems.map(({ to, label, icon, end }) => (
                <NavLink
                  key={to}
                  to={to}
                  end={end}
                  onClick={() => setSidebarOpen(false)}
                  className={({ isActive }) =>
                    `group flex items-center gap-3 rounded-lg px-3 py-3 transition-all duration-200 ${
                      isActive
                        ? "border border-accent/20 bg-accent/10 text-accent"
                        : "text-fg-3 hover:bg-surface-alt hover:text-fg"
                    }`
                  }
                >
                  <span className="material-symbols-outlined transition-colors group-hover:text-accent">
                    {icon}
                  </span>
                  <p className="text-sm font-medium tracking-wide">{label}</p>
                </NavLink>
              ))}
            </nav>
          </div>

          {/* Bottom section */}
          <div className="flex flex-col gap-4">
            {/* User actions */}
            <div className="flex items-center gap-2 rounded-xl border border-edge bg-surface-alt p-3">
              <button
                onClick={toggle}
                className="flex items-center justify-center rounded-lg p-2 text-fg-3 transition-colors hover:bg-edge hover:text-fg"
                title={`Switch to ${theme === "dark" ? "light" : "dark"} mode`}
              >
                <span className="material-symbols-outlined text-lg">
                  {theme === "dark" ? "light_mode" : "dark_mode"}
                </span>
              </button>
              <div className="flex-1" />
              <button
                onClick={handleLogout}
                className="flex items-center gap-2 rounded-lg border border-red-900/50 bg-red-900/10 px-4 py-2 font-mono text-xs font-bold text-red-400 transition-all hover:bg-red-900/20 hover:text-red-300"
              >
                <span className="material-symbols-outlined text-sm">
                  logout
                </span>
                LOGOUT
              </button>
            </div>
          </div>
        </div>
      </aside>

      {/* Main area */}
      <main className="relative z-10 flex h-full flex-1 flex-col overflow-y-auto">
        {/* Top header bar */}
        <header
          className="sticky top-0 z-30 flex h-16 items-center justify-between border-b border-edge px-6 py-3 backdrop-blur-md"
          style={{ background: "var(--th-surface-glass)" }}
        >
          {/* Mobile menu */}
          <div className="flex items-center gap-4 lg:hidden">
            <button
              onClick={() => setSidebarOpen(!sidebarOpen)}
              className="text-fg-3 hover:text-accent"
            >
              <span className="material-symbols-outlined">
                {sidebarOpen ? "close" : "menu"}
              </span>
            </button>
            <span className="text-lg font-bold text-fg">CodeForge</span>
          </div>

          <div className="hidden lg:block" />

          <div className="ml-auto flex items-center gap-4">
            <button className="relative rounded-full p-2 text-fg-3 transition-colors hover:bg-surface-alt hover:text-fg">
              <span className="material-symbols-outlined">notifications</span>
            </button>
          </div>
        </header>

        {/* Content */}
        <div className="flex-1 p-6 lg:p-10">
          <Outlet />
        </div>
      </main>
    </div>
  );
}

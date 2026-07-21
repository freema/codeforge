import { useState } from "react";
import { Outlet, NavLink, useNavigate } from "react-router";
import {
  Anvil,
  LayoutDashboard,
  SquareTerminal,
  Network,
  CalendarClock,
  Settings,
  ShieldCheck,
  Gauge,
  Building2,
  Sun,
  Moon,
  LogOut,
  Menu,
  X,
  type LucideIcon,
} from "lucide-react";
import { useAuth } from "../context/AuthContext";
import { useTheme } from "../context/ThemeContext";
import { useHealth } from "../hooks/useHealth";

interface NavItem {
  to: string;
  label: string;
  icon: LucideIcon;
  end: boolean;
}

const operatorNavItems: NavItem[] = [
  { to: "/", label: "Dashboard", icon: LayoutDashboard, end: true },
  { to: "/sessions", label: "Sessions", icon: SquareTerminal, end: false },
  { to: "/workflows", label: "Workflows", icon: Network, end: false },
  { to: "/schedules", label: "Schedules", icon: CalendarClock, end: false },
  { to: "/settings", label: "Settings", icon: Settings, end: false },
  { to: "/admin", label: "Admin", icon: ShieldCheck, end: false },
];

const tenantNavItems: NavItem[] = [
  { to: "/", label: "Dashboard", icon: LayoutDashboard, end: true },
  { to: "/sessions", label: "Sessions", icon: SquareTerminal, end: false },
  { to: "/usage", label: "Usage", icon: Gauge, end: false },
];

export default function AppLayout() {
  const { logout, role, tenantName, tier } = useAuth();
  const { theme, toggle } = useTheme();
  const navigate = useNavigate();
  const { data: health } = useHealth();
  const [sidebarOpen, setSidebarOpen] = useState(false);

  const navItems = role === "tenant" ? tenantNavItems : operatorNavItems;

  function handleLogout() {
    logout();
    void navigate("/login");
  }

  return (
    <div className="relative flex h-screen w-full overflow-hidden bg-page">
      {/* Mobile overlay */}
      {sidebarOpen && (
        <div
          className="fixed inset-0 z-20 bg-page/70 backdrop-blur-sm lg:hidden"
          onClick={() => setSidebarOpen(false)}
        />
      )}

      {/* Sidebar */}
      <aside
        className={`fixed inset-y-0 left-0 z-30 flex w-64 flex-col border-r border-edge bg-surface transition-transform lg:static lg:translate-x-0 ${
          sidebarOpen ? "translate-x-0" : "-translate-x-full"
        }`}
      >
        <div className="flex h-full flex-col justify-between p-4">
          {/* Top section */}
          <div className="flex flex-col gap-7">
            {/* Mark + wordmark */}
            <div className="flex items-center gap-3 px-2 pt-1">
              <div className="flex size-9 items-center justify-center rounded-md border border-accent-muted bg-accent-soft">
                <Anvil className="size-5 text-accent" strokeWidth={1.75} />
              </div>
              <div className="flex flex-col">
                <span className="font-expanded text-base leading-tight font-extrabold tracking-tight text-fg">
                  CodeForge
                </span>
                {health?.version && (
                  <span className="font-mono text-[10px] text-fg-4">
                    {health.version}
                  </span>
                )}
              </div>
            </div>

            {/* Navigation */}
            <nav className="flex flex-col gap-1">
              {navItems.map(({ to, label, icon: Icon, end }) => (
                <NavLink
                  key={to}
                  to={to}
                  end={end}
                  onClick={() => setSidebarOpen(false)}
                  className={({ isActive }) =>
                    `relative flex items-center gap-3 rounded-md px-3 py-2.5 text-sm transition-colors ${
                      isActive
                        ? "bg-surface-alt font-semibold text-fg"
                        : "font-medium text-fg-3 hover:bg-surface-alt hover:text-fg"
                    }`
                  }
                >
                  {({ isActive }) => (
                    <>
                      {/* Hot rail — the ember marks where you are */}
                      {isActive && (
                        <span className="absolute inset-y-2 left-0 w-0.5 rounded-full bg-accent" />
                      )}
                      <Icon
                        className={`size-4.5 ${isActive ? "text-accent" : ""}`}
                      />
                      {label}
                    </>
                  )}
                </NavLink>
              ))}
            </nav>
          </div>

          {/* Bottom section */}
          <div className="flex flex-col gap-3">
            {/* Tenant identity */}
            {role === "tenant" && tenantName && (
              <div className="flex items-center gap-2 px-3">
                <Building2 className="size-3.5 shrink-0 text-fg-4" />
                <span className="min-w-0 truncate text-xs text-fg-3">
                  {tenantName}
                </span>
                {tier && (
                  <span className="shrink-0 rounded-[4px] border border-edge bg-surface-alt px-1.5 py-0.5 font-mono text-[10px] tracking-wider text-fg-3 uppercase">
                    {tier}
                  </span>
                )}
              </div>
            )}

            {/* User actions */}
            <div className="flex items-center gap-1 border-t border-edge px-1 pt-3">
              <button
                onClick={toggle}
                className="rounded-md p-2 text-fg-3 transition-colors hover:bg-surface-alt hover:text-fg"
                title={`Switch to ${theme === "dark" ? "light" : "dark"} mode`}
              >
                {theme === "dark" ? (
                  <Sun className="size-4" />
                ) : (
                  <Moon className="size-4" />
                )}
              </button>
              <div className="flex-1" />
              <button
                onClick={handleLogout}
                className="flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium text-fg-3 transition-colors hover:bg-surface-alt hover:text-fg"
              >
                <LogOut className="size-4" />
                Log out
              </button>
            </div>
          </div>
        </div>
      </aside>

      {/* Main area */}
      <main className="relative z-10 flex h-full flex-1 flex-col overflow-y-auto">
        {/* Mobile top bar */}
        <header className="sticky top-0 z-30 flex h-14 items-center gap-4 border-b border-edge bg-surface-glass px-4 backdrop-blur-md lg:hidden">
          <button
            onClick={() => setSidebarOpen(!sidebarOpen)}
            className="rounded-md p-2 text-fg-3 hover:text-fg"
          >
            {sidebarOpen ? (
              <X className="size-5" />
            ) : (
              <Menu className="size-5" />
            )}
          </button>
          <span className="font-expanded text-base font-extrabold text-fg">
            CodeForge
          </span>
        </header>

        {/* Content */}
        <div className="flex-1 p-6 lg:p-10">
          <Outlet />
        </div>
      </main>
    </div>
  );
}

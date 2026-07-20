import { useState } from "react";
import { Link } from "react-router";
import { usePageTitle } from "../hooks/usePageTitle";
import { useMe, useMyUsage } from "../hooks/useMe";
import type { MyUsage } from "../types";

type Period = "24h" | "7d" | "30d";

const PERIODS: Period[] = ["24h", "7d", "30d"];

const TIER_BADGE: Record<string, string> = {
  free: "border-edge bg-surface text-fg-3",
  pro: "border-cyan-500/30 bg-cyan-500/10 text-cyan-400",
  enterprise: "border-purple-500/30 bg-purple-500/10 text-purple-400",
};

function TierBadge({ tier }: { tier: string }) {
  return (
    <span
      className={`rounded-full border px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider ${
        TIER_BADGE[tier] ?? "border-edge bg-surface text-fg-3"
      }`}
    >
      {tier}
    </span>
  );
}

function formatCost(value: number): string {
  if (value > 0 && value < 0.01) return `$${value.toFixed(4)}`;
  return `$${value.toFixed(2)}`;
}

export default function Usage() {
  usePageTitle("Usage");
  const [period, setPeriod] = useState<Period>("7d");
  const { data: me, isLoading: meLoading, isError: meError } = useMe();
  const isTenant = me?.role === "tenant";
  const {
    data: usage,
    isLoading: usageLoading,
    isError: usageError,
  } = useMyUsage(period, isTenant);

  if (meLoading) {
    return (
      <div className="flex items-center justify-center py-20">
        <span className="material-symbols-outlined animate-spin text-3xl text-accent/50">
          progress_activity
        </span>
      </div>
    );
  }

  if (meError) {
    return (
      <div className="mx-auto max-w-4xl">
        <div className="rounded-xl border border-red-900/50 bg-red-900/10 p-8 text-center">
          <span className="material-symbols-outlined mb-2 text-3xl text-red-400">
            error
          </span>
          <p className="text-sm text-red-400">
            Failed to load account information.
          </p>
        </div>
      </div>
    );
  }

  if (!isTenant) {
    return (
      <div className="mx-auto max-w-4xl space-y-6">
        <div>
          <h1 className="text-3xl font-bold tracking-tight text-fg">Usage</h1>
          <p className="mt-1 text-sm text-fg-3">
            Session usage and account limits
          </p>
        </div>
        <div className="flex flex-col items-center rounded-xl border border-edge bg-surface-alt p-10 text-center">
          <span className="material-symbols-outlined mb-3 text-3xl text-fg-4">
            monitoring
          </span>
          <p className="text-sm text-fg-2">
            Usage is available only for tenant accounts.
          </p>
          <p className="mt-1 text-xs text-fg-4">
            As an operator, you can inspect per-tenant usage in the Admin
            section.
          </p>
          <Link
            to="/admin"
            className="mt-4 flex items-center gap-1.5 rounded-lg border border-edge px-4 py-2 text-xs font-medium text-fg-3 transition-colors hover:border-accent/30 hover:text-accent"
          >
            <span className="material-symbols-outlined text-sm">
              admin_panel_settings
            </span>
            Go to Admin
          </Link>
        </div>
      </div>
    );
  }

  const tenant = me?.tenant;

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      {/* Header */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight text-fg">Usage</h1>
          <p className="mt-1 text-sm text-fg-3">
            Session usage and account limits
          </p>
        </div>
        {tenant && (
          <div className="flex items-center gap-2">
            <span className="material-symbols-outlined text-accent/60">
              apartment
            </span>
            <span className="font-medium text-fg">{tenant.name}</span>
            <TierBadge tier={tenant.tier} />
          </div>
        )}
      </div>

      {/* Period selector */}
      <div className="flex items-center justify-between">
        <h3 className="flex items-center gap-1.5 text-xs font-bold uppercase tracking-wider text-fg-2">
          <span className="material-symbols-outlined text-sm text-accent">
            monitoring
          </span>
          Usage
        </h3>
        <div className="flex gap-1 rounded-lg border border-edge bg-surface p-1">
          {PERIODS.map((p) => (
            <button
              key={p}
              type="button"
              onClick={() => setPeriod(p)}
              className={`rounded-md px-3 py-1 text-xs font-bold uppercase tracking-wider transition-colors ${
                period === p ? "bg-accent text-page" : "text-fg-3 hover:text-fg"
              }`}
            >
              {p}
            </button>
          ))}
        </div>
      </div>

      {usageError ? (
        <div className="rounded-xl border border-red-900/50 bg-red-900/10 p-8 text-center">
          <span className="material-symbols-outlined mb-2 text-3xl text-red-400">
            error
          </span>
          <p className="text-sm text-red-400">Failed to load usage data.</p>
        </div>
      ) : usageLoading || !usage ? (
        <div className="space-y-4">
          <div className="h-24 animate-pulse rounded-xl bg-surface-alt" />
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            {[1, 2, 3, 4].map((i) => (
              <div
                key={i}
                className="h-20 animate-pulse rounded-xl bg-surface-alt"
              />
            ))}
          </div>
          <div className="h-40 animate-pulse rounded-xl bg-surface-alt" />
        </div>
      ) : (
        <UsageContent usage={usage} />
      )}
    </div>
  );
}

function UsageContent({ usage }: { usage: MyUsage }) {
  const { sessions_today, summary, limits } = usage;
  const unlimited = limits.max_sessions_per_day === -1;
  const pct = unlimited
    ? 0
    : Math.min(
        100,
        limits.max_sessions_per_day > 0
          ? (sessions_today / limits.max_sessions_per_day) * 100
          : 100,
      );
  const barColor =
    pct >= 100 ? "bg-red-400" : pct >= 80 ? "bg-amber-400" : "bg-accent";

  const clis = limits.allowed_clis
    .split(",")
    .map((c) => c.trim())
    .filter(Boolean);
  const models = (limits.allowed_models ?? "")
    .split(",")
    .map((m) => m.trim())
    .filter(Boolean);

  return (
    <>
      {/* Sessions today */}
      <div className="rounded-xl border border-edge bg-surface-alt p-5">
        <div className="flex items-center justify-between">
          <p className="text-sm font-medium text-fg-3">Sessions today</p>
          <span className="material-symbols-outlined text-accent/50">
            today
          </span>
        </div>
        <p className="mt-2 font-mono text-3xl font-bold tracking-tight text-fg">
          {sessions_today.toLocaleString()}
          <span className="ml-1 text-sm font-medium text-fg-4">
            / {unlimited ? "unlimited" : limits.max_sessions_per_day}
          </span>
        </p>
        {!unlimited && (
          <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-surface">
            <div
              className={`h-full rounded-full transition-all ${barColor}`}
              style={{ width: `${pct}%` }}
            />
          </div>
        )}
      </div>

      {/* Period totals */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        {[
          {
            label: "Sessions",
            value: summary.total_sessions.toLocaleString(),
          },
          {
            label: "Input tokens",
            value: summary.total_input_tokens.toLocaleString(),
          },
          {
            label: "Output tokens",
            value: summary.total_output_tokens.toLocaleString(),
          },
          { label: "Cost", value: formatCost(summary.total_cost_usd) },
        ].map(({ label, value }) => (
          <div
            key={label}
            className="rounded-xl border border-edge bg-surface-alt px-4 py-3"
          >
            <p className="text-[10px] font-bold uppercase tracking-wider text-fg-4">
              {label}
            </p>
            <p className="mt-1 font-mono text-lg text-fg">{value}</p>
          </div>
        ))}
      </div>

      {/* Limits */}
      <div className="rounded-xl border border-edge bg-surface-alt p-5">
        <h3 className="mb-4 flex items-center gap-2 text-sm font-bold uppercase tracking-wider text-fg-2">
          <span className="material-symbols-outlined text-accent text-base">
            speed
          </span>
          Limits
        </h3>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
          <div className="rounded-lg border border-edge bg-surface px-3 py-2">
            <p className="text-[10px] font-bold uppercase tracking-wider text-fg-4">
              Tier
            </p>
            <div className="mt-1">
              <TierBadge tier={limits.tier} />
            </div>
          </div>
          <div className="rounded-lg border border-edge bg-surface px-3 py-2">
            <p className="text-[10px] font-bold uppercase tracking-wider text-fg-4">
              Concurrent sessions
            </p>
            <p className="mt-0.5 font-mono text-sm text-fg">
              {limits.max_concurrent_sessions}
            </p>
          </div>
          <div className="rounded-lg border border-edge bg-surface px-3 py-2">
            <p className="text-[10px] font-bold uppercase tracking-wider text-fg-4">
              Budget / session
            </p>
            <p className="mt-0.5 font-mono text-sm text-fg">
              {formatCost(limits.max_budget_usd_per_session)}
            </p>
          </div>
        </div>
        <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div className="rounded-lg border border-edge bg-surface px-3 py-2">
            <p className="text-[10px] font-bold uppercase tracking-wider text-fg-4">
              Allowed CLIs
            </p>
            <div className="mt-1.5 flex flex-wrap gap-1.5">
              {clis.length > 0 ? (
                clis.map((cli) => (
                  <span
                    key={cli}
                    className="rounded-full border border-edge bg-surface-alt px-2 py-0.5 font-mono text-[10px] text-fg-2"
                  >
                    {cli}
                  </span>
                ))
              ) : (
                <span className="text-xs text-fg-4">none</span>
              )}
            </div>
          </div>
          <div className="rounded-lg border border-edge bg-surface px-3 py-2">
            <p className="text-[10px] font-bold uppercase tracking-wider text-fg-4">
              Allowed models
            </p>
            <div className="mt-1.5 flex flex-wrap gap-1.5">
              {models.length > 0 ? (
                models.map((model) => (
                  <span
                    key={model}
                    className="rounded-full border border-edge bg-surface-alt px-2 py-0.5 font-mono text-[10px] text-fg-2"
                  >
                    {model}
                  </span>
                ))
              ) : (
                <span className="text-xs text-fg-3">all models</span>
              )}
            </div>
          </div>
        </div>
      </div>
    </>
  );
}

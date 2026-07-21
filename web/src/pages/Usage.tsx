import { useState } from "react";
import { Link } from "react-router";
import {
  Building2,
  CalendarDays,
  ChartLine,
  CircleAlert,
  Loader2,
  ShieldCheck,
} from "lucide-react";
import { usePageTitle } from "../hooks/usePageTitle";
import { useMe, useMyUsage } from "../hooks/useMe";
import type { MyUsage } from "../types";

type Period = "24h" | "7d" | "30d";

const PERIODS: Period[] = ["24h", "7d", "30d"];

function TierBadge({ tier }: { tier: string }) {
  return (
    <span className="rounded-[4px] border border-edge bg-surface-alt px-1.5 py-0.5 font-mono text-[10px] tracking-wider text-fg-3 uppercase">
      {tier}
    </span>
  );
}

function formatCost(value: number): string {
  if (value > 0 && value < 0.01) return `$${value.toFixed(4)}`;
  return `$${value.toFixed(2)}`;
}

// allowed_clis/allowed_models arrive either as a JSON array string
// ('["claude-code"]') or a plain comma-separated list.
function parseList(raw: string): string[] {
  const s = raw.trim();
  if (s.startsWith("[")) {
    try {
      const arr: unknown = JSON.parse(s);
      if (Array.isArray(arr)) return arr.map(String).filter(Boolean);
    } catch {
      // fall through to comma parsing
    }
  }
  return s
    .split(",")
    .map((c) => c.trim())
    .filter(Boolean);
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
        <Loader2 className="size-6 animate-spin text-fg-3" />
      </div>
    );
  }

  if (meError) {
    return (
      <div className="mx-auto max-w-4xl">
        <div className="flex flex-col items-center rounded-md border border-danger/30 bg-danger/10 p-8 text-center">
          <CircleAlert className="mb-2 size-6 text-danger" />
          <p className="text-sm text-danger">
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
          <p className="eyebrow mb-1">Account</p>
          <h2 className="font-expanded text-2xl font-extrabold tracking-tight text-fg">
            Usage
          </h2>
        </div>
        <div className="flex flex-col items-center rounded-md border border-edge bg-surface p-10 text-center">
          <ChartLine className="mb-3 size-6 text-fg-4" strokeWidth={1.75} />
          <p className="text-sm text-fg-2">
            Usage is available only for tenant accounts.
          </p>
          <p className="mt-1 text-xs text-fg-4">
            As an operator, you can inspect per-tenant usage in the Admin
            section.
          </p>
          <Link
            to="/admin"
            className="mt-4 flex items-center gap-1.5 rounded-md border border-edge bg-surface px-4 py-2 text-xs font-medium text-fg-2 transition-colors hover:border-fg-4 hover:text-fg"
          >
            <ShieldCheck className="size-3.5" />
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
      <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p className="eyebrow mb-1">Account</p>
          <h2 className="font-expanded text-2xl font-extrabold tracking-tight text-fg">
            Usage
          </h2>
        </div>
        {tenant && (
          <div className="flex items-center gap-2">
            <Building2 className="size-4 text-fg-3" />
            <span className="text-sm font-medium text-fg">{tenant.name}</span>
            <TierBadge tier={tenant.tier} />
          </div>
        )}
      </div>

      {/* Period selector */}
      <div className="flex items-center justify-between">
        <span className="eyebrow">Usage</span>
        <div className="flex gap-1 rounded-md border border-edge bg-surface-alt p-1">
          {PERIODS.map((p) => (
            <button
              key={p}
              type="button"
              onClick={() => setPeriod(p)}
              className={`rounded-[4px] px-3 py-1 font-mono text-[10px] tracking-wider uppercase transition-colors ${
                period === p
                  ? "bg-accent text-white"
                  : "text-fg-3 hover:text-fg"
              }`}
            >
              {p}
            </button>
          ))}
        </div>
      </div>

      {usageError ? (
        <div className="flex flex-col items-center rounded-md border border-danger/30 bg-danger/10 p-8 text-center">
          <CircleAlert className="mb-2 size-6 text-danger" />
          <p className="text-sm text-danger">Failed to load usage data.</p>
        </div>
      ) : usageLoading || !usage ? (
        <div className="space-y-4">
          <div className="h-24 animate-pulse rounded-md bg-surface-alt" />
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            {[1, 2, 3, 4].map((i) => (
              <div
                key={i}
                className="h-20 animate-pulse rounded-md bg-surface-alt"
              />
            ))}
          </div>
          <div className="h-40 animate-pulse rounded-md bg-surface-alt" />
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
    pct >= 100 ? "bg-danger" : pct >= 80 ? "bg-warn" : "bg-accent";

  const clis = parseList(limits.allowed_clis);
  const models = parseList(limits.allowed_models ?? "");

  return (
    <>
      {/* Sessions today */}
      <div className="rounded-md border border-edge bg-surface p-5">
        <div className="flex items-center justify-between">
          <p className="text-sm font-medium text-fg-3">Sessions today</p>
          <CalendarDays className="size-4 text-fg-4" />
        </div>
        <p className="mt-2 font-mono text-3xl font-semibold tracking-tight text-fg">
          {sessions_today.toLocaleString()}
          <span className="ml-1 text-sm font-medium text-fg-4">
            / {unlimited ? "unlimited" : limits.max_sessions_per_day}
          </span>
        </p>
        {!unlimited && (
          <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-surface-alt">
            <div
              className={`h-full rounded-full ${barColor}`}
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
            className="rounded-md border border-edge bg-surface px-4 py-3"
          >
            <p className="text-[11px] font-medium text-fg-3">{label}</p>
            <p className="mt-1 font-mono text-lg text-fg">{value}</p>
          </div>
        ))}
      </div>

      {/* Limits */}
      <div className="overflow-hidden rounded-md border border-edge bg-surface">
        <div className="border-b border-edge px-5 py-3.5">
          <span className="eyebrow">Limits</span>
        </div>
        <div className="p-5">
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
            <div className="rounded-md border border-edge bg-surface-alt px-3 py-2">
              <p className="text-[11px] font-medium text-fg-3">Tier</p>
              <div className="mt-1">
                <TierBadge tier={limits.tier} />
              </div>
            </div>
            <div className="rounded-md border border-edge bg-surface-alt px-3 py-2">
              <p className="text-[11px] font-medium text-fg-3">
                Concurrent sessions
              </p>
              <p className="mt-0.5 font-mono text-sm text-fg">
                {limits.max_concurrent_sessions}
              </p>
            </div>
            <div className="rounded-md border border-edge bg-surface-alt px-3 py-2">
              <p className="text-[11px] font-medium text-fg-3">
                Budget / session
              </p>
              <p className="mt-0.5 font-mono text-sm text-fg">
                {formatCost(limits.max_budget_usd_per_session)}
              </p>
            </div>
          </div>
          <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div className="rounded-md border border-edge bg-surface-alt px-3 py-2">
              <p className="text-[11px] font-medium text-fg-3">Allowed CLIs</p>
              <div className="mt-1.5 flex flex-wrap gap-1.5">
                {clis.length > 0 ? (
                  clis.map((cli) => (
                    <span
                      key={cli}
                      className="rounded-[4px] border border-edge bg-surface px-1.5 py-0.5 font-mono text-[10px] text-fg-2"
                    >
                      {cli}
                    </span>
                  ))
                ) : (
                  <span className="text-xs text-fg-4">none</span>
                )}
              </div>
            </div>
            <div className="rounded-md border border-edge bg-surface-alt px-3 py-2">
              <p className="text-[11px] font-medium text-fg-3">
                Allowed models
              </p>
              <div className="mt-1.5 flex flex-wrap gap-1.5">
                {models.length > 0 ? (
                  models.map((model) => (
                    <span
                      key={model}
                      className="rounded-[4px] border border-edge bg-surface px-1.5 py-0.5 font-mono text-[10px] text-fg-2"
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
      </div>
    </>
  );
}

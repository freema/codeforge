import { useState, type FormEvent } from "react";
import { useSearchParams } from "react-router";
import { usePageTitle } from "../hooks/usePageTitle";
import {
  useTenants,
  useTenantUsage,
  useCreateTenant,
  useUpdateTenant,
  useDeleteTenant,
  useKeyPool,
  useAddKeyPoolEntry,
  useDeleteKeyPoolEntry,
} from "../hooks/useTenants";
import { formatTimeAgo } from "../lib/formatters";
import type { CreateTenantResult, Tenant, TenantTier } from "../types";

type Tab = "tenants" | "keypool";

const tabs: { id: Tab; label: string; icon: string }[] = [
  { id: "tenants", label: "Tenants", icon: "group" },
  { id: "keypool", label: "Key Pool", icon: "key" },
];

const VALID_TABS = new Set<Tab>(["tenants", "keypool"]);

export default function Admin() {
  usePageTitle("Admin");
  const [searchParams, setSearchParams] = useSearchParams();
  const rawTab = searchParams.get("tab") as Tab | null;
  const activeTab: Tab = rawTab && VALID_TABS.has(rawTab) ? rawTab : "tenants";

  function setActiveTab(tab: Tab) {
    setSearchParams(tab === "tenants" ? {} : { tab }, { replace: true });
  }

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      <div>
        <h1 className="text-3xl font-bold tracking-tight text-fg">Admin</h1>
        <p className="mt-1 text-sm text-fg-3">
          Manage tenants and the operator key pool
        </p>
      </div>

      {/* Tabs */}
      <div className="flex gap-2 border-b border-edge pb-px">
        {tabs.map(({ id, label, icon }) => (
          <button
            key={id}
            onClick={() => setActiveTab(id)}
            className={`flex items-center gap-2 border-b-2 px-4 py-3 text-sm font-medium transition-colors ${
              activeTab === id
                ? "border-accent text-accent"
                : "border-transparent text-fg-3 hover:text-fg"
            }`}
          >
            <span className="material-symbols-outlined text-lg">{icon}</span>
            {label}
          </button>
        ))}
      </div>

      {activeTab === "tenants" && <TenantsTab />}
      {activeTab === "keypool" && <KeyPoolTab />}
    </div>
  );
}

const inputCls =
  "w-full rounded-lg border border-edge bg-surface px-3 py-2.5 text-sm text-fg font-mono placeholder-fg-4 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent transition-colors";

const TIERS: { value: TenantTier; label: string }[] = [
  { value: "free", label: "Free" },
  { value: "pro", label: "Pro" },
  { value: "enterprise", label: "Enterprise" },
];

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

function CreatedTokenPanel({
  result,
  onDismiss,
}: {
  result: CreateTenantResult;
  onDismiss: () => void;
}) {
  const [copied, setCopied] = useState(false);

  async function handleCopy() {
    await navigator.clipboard.writeText(result.api_token);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }

  return (
    <div className="rounded-xl border border-accent/30 bg-accent/5 p-5">
      <div className="flex items-center justify-between">
        <h3 className="flex items-center gap-2 text-sm font-bold uppercase tracking-wider text-fg-2">
          <span className="material-symbols-outlined text-accent text-base">
            check_circle
          </span>
          Tenant &quot;{result.tenant.name}&quot; created
        </h3>
        <button
          onClick={onDismiss}
          className="rounded-md p-1 text-fg-4 transition-colors hover:text-fg"
          title="Dismiss"
        >
          <span className="material-symbols-outlined text-base">close</span>
        </button>
      </div>
      <p className="mt-2 text-xs text-amber-400">
        This API token is shown only once. Copy it now — it cannot be retrieved
        later.
      </p>
      <div className="mt-3 flex items-center gap-2">
        <code className="flex-1 overflow-x-auto rounded-lg border border-edge bg-surface px-3 py-2.5 font-mono text-xs text-fg">
          {result.api_token}
        </code>
        <button
          onClick={() => void handleCopy()}
          className="flex shrink-0 items-center gap-1.5 rounded-lg border border-edge px-3 py-2 text-xs font-medium text-fg-3 transition-colors hover:border-accent/30 hover:text-accent"
        >
          <span className="material-symbols-outlined text-sm">
            {copied ? "check" : "content_copy"}
          </span>
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
    </div>
  );
}

function TenantUsagePanel({ tenantId }: { tenantId: string }) {
  const [period, setPeriod] = useState<"24h" | "7d" | "30d">("7d");
  const { data: usage, isLoading } = useTenantUsage(tenantId, period);

  return (
    <div className="mt-4 border-t border-edge pt-4">
      <div className="mb-3 flex items-center justify-between">
        <h4 className="flex items-center gap-1.5 text-xs font-bold uppercase tracking-wider text-fg-2">
          <span className="material-symbols-outlined text-sm text-accent">
            monitoring
          </span>
          Usage
        </h4>
        <div className="flex gap-1 rounded-lg border border-edge bg-surface p-1">
          {(["24h", "7d", "30d"] as const).map((p) => (
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

      {isLoading || !usage ? (
        <div className="h-14 animate-pulse rounded-lg bg-surface" />
      ) : (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          {[
            { label: "Sessions", value: usage.total_sessions.toLocaleString() },
            {
              label: "Input tokens",
              value: usage.total_input_tokens.toLocaleString(),
            },
            {
              label: "Output tokens",
              value: usage.total_output_tokens.toLocaleString(),
            },
            { label: "Cost", value: formatCost(usage.total_cost_usd) },
          ].map(({ label, value }) => (
            <div
              key={label}
              className="rounded-lg border border-edge bg-surface px-3 py-2"
            >
              <p className="text-[10px] font-bold uppercase tracking-wider text-fg-4">
                {label}
              </p>
              <p className="mt-0.5 font-mono text-sm text-fg">{value}</p>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function TenantCard({ tenant }: { tenant: Tenant }) {
  const updateTenant = useUpdateTenant();
  const deleteTenant = useDeleteTenant();

  const [showUsage, setShowUsage] = useState(false);
  const [editing, setEditing] = useState(false);
  const [editName, setEditName] = useState(tenant.name);
  const [editTier, setEditTier] = useState(tenant.tier);
  const [confirmDelete, setConfirmDelete] = useState(false);

  function startEdit() {
    setEditName(tenant.name);
    setEditTier(tenant.tier);
    setEditing(true);
  }

  async function handleSave(e: FormEvent) {
    e.preventDefault();
    await updateTenant.mutateAsync({
      id: tenant.id,
      req: { name: editName, tier: editTier },
    });
    setEditing(false);
  }

  async function handleDelete() {
    await deleteTenant.mutateAsync(tenant.id);
    setConfirmDelete(false);
  }

  return (
    <div className="rounded-xl border border-edge bg-surface-alt p-4">
      <div className="flex items-center justify-between gap-4">
        <div className="flex items-center gap-3 min-w-0">
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border border-edge bg-surface">
            <span className="material-symbols-outlined text-accent/60">
              apartment
            </span>
          </div>
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <span className="font-medium text-fg">{tenant.name}</span>
              <span className="font-mono text-[10px] text-fg-4">
                {tenant.slug}
              </span>
              <TierBadge tier={tenant.tier} />
            </div>
            <p className="mt-0.5 text-xs text-fg-4">
              Created {formatTimeAgo(tenant.created_at)}
              <span className="mx-1.5 text-fg-4">·</span>
              {tenant.max_sessions_per_day} sessions/day
              <span className="mx-1.5 text-fg-4">·</span>
              {tenant.max_concurrent_sessions} concurrent
              <span className="mx-1.5 text-fg-4">·</span>
              {formatCost(tenant.max_budget_usd_per_session)}/session
            </p>
          </div>
        </div>

        <div className="flex items-center gap-2 shrink-0">
          <button
            onClick={() => setShowUsage((v) => !v)}
            className={`flex items-center gap-1.5 rounded-lg border px-3 py-2 text-xs font-medium transition-colors ${
              showUsage
                ? "border-accent/30 text-accent"
                : "border-edge text-fg-3 hover:border-accent/30 hover:text-accent"
            }`}
          >
            <span className="material-symbols-outlined text-sm">
              monitoring
            </span>
            Usage
          </button>
          <button
            onClick={startEdit}
            className="rounded-md border border-edge p-1.5 text-fg-4 transition-colors hover:border-accent/30 hover:text-accent"
            title="Edit tenant"
          >
            <span className="material-symbols-outlined text-base">edit</span>
          </button>
          {confirmDelete ? (
            <span className="flex items-center gap-2">
              <button
                onClick={() => void handleDelete()}
                disabled={deleteTenant.isPending}
                className="rounded-lg border border-red-900/50 bg-red-900/20 px-3 py-2 text-xs font-medium text-red-400"
              >
                Confirm
              </button>
              <button
                onClick={() => setConfirmDelete(false)}
                className="text-xs text-fg-3"
              >
                Cancel
              </button>
            </span>
          ) : (
            <button
              onClick={() => setConfirmDelete(true)}
              className="rounded-md border border-edge p-1.5 text-fg-4 transition-colors hover:border-red-900/50 hover:text-red-400"
              title="Delete tenant"
            >
              <span className="material-symbols-outlined text-base">
                delete
              </span>
            </button>
          )}
        </div>
      </div>

      {editing && (
        <form
          onSubmit={(e) => void handleSave(e)}
          className="mt-4 border-t border-edge pt-4"
        >
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <input
              type="text"
              value={editName}
              onChange={(e) => setEditName(e.target.value)}
              placeholder="Name"
              required
              className={inputCls}
            />
            <select
              value={editTier}
              onChange={(e) => setEditTier(e.target.value)}
              className={inputCls}
            >
              {TIERS.map((t) => (
                <option key={t.value} value={t.value}>
                  {t.label}
                </option>
              ))}
            </select>
          </div>
          <div className="mt-3 flex items-center gap-2">
            <button
              type="submit"
              disabled={updateTenant.isPending}
              className="flex items-center gap-2 rounded-lg bg-accent px-4 py-2 text-xs font-bold text-page transition-all hover:bg-accent-hover disabled:opacity-50"
            >
              {updateTenant.isPending ? (
                <span className="material-symbols-outlined animate-spin text-sm">
                  progress_activity
                </span>
              ) : (
                <span className="material-symbols-outlined text-sm">save</span>
              )}
              Save
            </button>
            <button
              type="button"
              onClick={() => setEditing(false)}
              className="text-xs text-fg-3 transition-colors hover:text-fg"
            >
              Cancel
            </button>
          </div>
        </form>
      )}

      {showUsage && <TenantUsagePanel tenantId={tenant.id} />}
    </div>
  );
}

function TenantsTab() {
  const { data: tenants, isLoading } = useTenants();
  const createTenant = useCreateTenant();

  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [tier, setTier] = useState<TenantTier>("free");
  const [createdResult, setCreatedResult] = useState<CreateTenantResult | null>(
    null,
  );

  async function handleCreate(e: FormEvent) {
    e.preventDefault();
    const result = await createTenant.mutateAsync({ name, slug, tier });
    setCreatedResult(result);
    setName("");
    setSlug("");
    setTier("free");
  }

  return (
    <div className="space-y-6">
      {createdResult && (
        <CreatedTokenPanel
          result={createdResult}
          onDismiss={() => setCreatedResult(null)}
        />
      )}

      {isLoading ? (
        <LoadingSkeleton />
      ) : tenants && tenants.length > 0 ? (
        <div className="flex flex-col gap-3">
          {tenants.map((t) => (
            <TenantCard key={t.id} tenant={t} />
          ))}
        </div>
      ) : (
        <p className="py-8 text-center text-sm text-fg-4">
          No tenants configured.
        </p>
      )}

      {/* ── Create Tenant ── */}
      <form
        onSubmit={(e) => void handleCreate(e)}
        className="rounded-xl border border-edge bg-surface/50 p-5"
      >
        <h3 className="mb-4 flex items-center gap-2 text-sm font-bold uppercase tracking-wider text-fg-2">
          <span className="material-symbols-outlined text-accent text-base">
            add_circle
          </span>
          Add Tenant
        </h3>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Name"
            required
            className={inputCls}
          />
          <input
            type="text"
            value={slug}
            onChange={(e) => setSlug(e.target.value)}
            placeholder="Slug (e.g. acme-corp)"
            required
            className={inputCls}
          />
          <select
            value={tier}
            onChange={(e) => setTier(e.target.value as TenantTier)}
            className={inputCls}
          >
            {TIERS.map((t) => (
              <option key={t.value} value={t.value}>
                {t.label}
              </option>
            ))}
          </select>
        </div>
        <button
          type="submit"
          disabled={createTenant.isPending}
          className="mt-4 flex items-center gap-2 rounded-lg bg-accent px-5 py-2 text-sm font-bold text-page transition-all hover:bg-accent-hover disabled:opacity-50"
        >
          {createTenant.isPending ? (
            <span className="material-symbols-outlined animate-spin text-base">
              progress_activity
            </span>
          ) : (
            <span className="material-symbols-outlined text-lg">add</span>
          )}
          Create Tenant
        </button>
      </form>
    </div>
  );
}

const POOL_PROVIDERS = [
  { value: "anthropic", label: "Anthropic" },
  { value: "openai", label: "OpenAI" },
] as const;

function KeyPoolTab() {
  const { data: keys, isLoading } = useKeyPool();
  const addEntry = useAddKeyPoolEntry();
  const deleteEntry = useDeleteKeyPoolEntry();

  const [provider, setProvider] = useState("anthropic");
  const [token, setToken] = useState("");
  const [weight, setWeight] = useState("");
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null);

  async function handleAdd(e: FormEvent) {
    e.preventDefault();
    const parsedWeight = weight.trim() ? Number(weight) : undefined;
    await addEntry.mutateAsync({
      provider,
      token,
      weight:
        parsedWeight && Number.isFinite(parsedWeight) && parsedWeight > 0
          ? parsedWeight
          : undefined,
    });
    setToken("");
    setWeight("");
  }

  async function handleDelete(id: string) {
    await deleteEntry.mutateAsync(id);
    setConfirmDelete(null);
  }

  return (
    <div className="space-y-6">
      {isLoading ? (
        <LoadingSkeleton />
      ) : keys && keys.length > 0 ? (
        <div className="flex flex-col gap-3">
          {keys.map((k) => (
            <div
              key={k.id}
              className="flex items-center justify-between gap-4 rounded-xl border border-edge bg-surface-alt p-4"
            >
              <div className="flex items-center gap-3 min-w-0">
                <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border border-edge bg-surface">
                  <span className="material-symbols-outlined text-accent/60">
                    key
                  </span>
                </div>
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="rounded-full border border-edge bg-surface px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider text-fg-3">
                      {k.provider}
                    </span>
                    <span
                      className={`rounded-full border px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider ${
                        k.active
                          ? "border-accent/30 bg-accent/10 text-accent"
                          : "border-red-500/30 bg-red-500/10 text-red-400"
                      }`}
                    >
                      {k.active ? "Active" : "Inactive"}
                    </span>
                    <span className="font-mono text-[10px] text-fg-4">
                      {k.id.slice(0, 12)}...
                    </span>
                  </div>
                  <p className="mt-0.5 text-xs text-fg-4">
                    Weight {k.weight}
                    <span className="mx-1.5 text-fg-4">·</span>
                    Added {formatTimeAgo(k.created_at)}
                  </p>
                </div>
              </div>

              <div className="shrink-0">
                {confirmDelete === k.id ? (
                  <span className="flex items-center gap-2">
                    <button
                      onClick={() => void handleDelete(k.id)}
                      disabled={deleteEntry.isPending}
                      className="rounded-lg border border-red-900/50 bg-red-900/20 px-3 py-2 text-xs font-medium text-red-400"
                    >
                      Confirm
                    </button>
                    <button
                      onClick={() => setConfirmDelete(null)}
                      className="text-xs text-fg-3"
                    >
                      Cancel
                    </button>
                  </span>
                ) : (
                  <button
                    onClick={() => setConfirmDelete(k.id)}
                    className="rounded-md border border-edge p-1.5 text-fg-4 transition-colors hover:border-red-900/50 hover:text-red-400"
                    title="Delete key"
                  >
                    <span className="material-symbols-outlined text-base">
                      delete
                    </span>
                  </button>
                )}
              </div>
            </div>
          ))}
        </div>
      ) : (
        <p className="py-8 text-center text-sm text-fg-4">
          No keys in the pool.
        </p>
      )}

      {/* ── Add Key ── */}
      <form
        onSubmit={(e) => void handleAdd(e)}
        className="rounded-xl border border-edge bg-surface/50 p-5"
      >
        <h3 className="mb-4 flex items-center gap-2 text-sm font-bold uppercase tracking-wider text-fg-2">
          <span className="material-symbols-outlined text-accent text-base">
            add_circle
          </span>
          Add Pool Key
        </h3>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
          <select
            value={provider}
            onChange={(e) => setProvider(e.target.value)}
            className={inputCls}
          >
            {POOL_PROVIDERS.map((p) => (
              <option key={p.value} value={p.value}>
                {p.label}
              </option>
            ))}
          </select>
          <input
            type="password"
            value={token}
            onChange={(e) => setToken(e.target.value)}
            placeholder="API Key"
            required
            className={inputCls}
          />
          <input
            type="number"
            min={1}
            value={weight}
            onChange={(e) => setWeight(e.target.value)}
            placeholder="Weight (default: 1)"
            className={inputCls}
          />
        </div>
        <button
          type="submit"
          disabled={addEntry.isPending}
          className="mt-4 flex items-center gap-2 rounded-lg bg-accent px-5 py-2 text-sm font-bold text-page transition-all hover:bg-accent-hover disabled:opacity-50"
        >
          {addEntry.isPending ? (
            <span className="material-symbols-outlined animate-spin text-base">
              progress_activity
            </span>
          ) : (
            <span className="material-symbols-outlined text-lg">add</span>
          )}
          Add Key
        </button>
      </form>
    </div>
  );
}

function LoadingSkeleton() {
  return (
    <div className="space-y-3">
      {[1, 2, 3].map((i) => (
        <div key={i} className="h-16 animate-pulse rounded-xl bg-surface-alt" />
      ))}
    </div>
  );
}

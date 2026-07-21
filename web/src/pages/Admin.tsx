import { useState, type FormEvent } from "react";
import { useSearchParams } from "react-router";
import {
  Building2,
  ChartLine,
  Check,
  CircleCheck,
  Copy,
  KeyRound,
  Loader2,
  Plus,
  Save,
  SquarePen,
  Trash2,
  Users,
  X,
  type LucideIcon,
} from "lucide-react";
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

const tabs: { id: Tab; label: string; icon: LucideIcon }[] = [
  { id: "tenants", label: "Tenants", icon: Users },
  { id: "keypool", label: "Key pool", icon: KeyRound },
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
        <p className="eyebrow mb-1">Administration</p>
        <h2 className="font-expanded text-2xl font-extrabold tracking-tight text-fg">
          Admin
        </h2>
      </div>

      {/* Tabs */}
      <div className="flex gap-2 border-b border-edge pb-px">
        {tabs.map(({ id, label, icon: Icon }) => (
          <button
            key={id}
            onClick={() => setActiveTab(id)}
            className={`flex items-center gap-2 border-b-2 px-4 py-3 text-sm font-medium transition-colors ${
              activeTab === id
                ? "border-accent text-fg"
                : "border-transparent text-fg-3 hover:text-fg"
            }`}
          >
            <Icon className="size-4" />
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
  "w-full rounded-md border border-edge bg-input px-3 py-2 text-sm text-fg placeholder-fg-4 transition-colors focus:border-accent focus:outline-none";

const monoInputCls = `${inputCls} font-mono`;

const TIERS: { value: TenantTier; label: string }[] = [
  { value: "free", label: "Free" },
  { value: "pro", label: "Pro" },
  { value: "enterprise", label: "Enterprise" },
];

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
    <div className="rounded-md border border-ok/30 bg-ok/10 p-5">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <CircleCheck className="size-4 text-ok" />
          <h3 className="text-sm font-semibold text-fg">
            Tenant &quot;{result.tenant.name}&quot; created
          </h3>
        </div>
        <button
          onClick={onDismiss}
          className="rounded-md p-1.5 text-fg-4 transition-colors hover:bg-surface-alt hover:text-fg"
          title="Dismiss"
        >
          <X className="size-4" />
        </button>
      </div>
      <p className="mt-2 text-xs text-warn">
        This API token is shown only once. Copy it now — it cannot be retrieved
        later.
      </p>
      <div className="mt-3 flex items-center gap-2">
        <code className="flex-1 overflow-x-auto rounded-md border border-edge bg-input px-3 py-2.5 font-mono text-xs text-fg">
          {result.api_token}
        </code>
        <button
          onClick={() => void handleCopy()}
          className="flex shrink-0 items-center gap-1.5 rounded-md border border-edge bg-surface px-3 py-2 text-xs font-medium text-fg-2 transition-colors hover:border-fg-4 hover:text-fg"
        >
          {copied ? (
            <Check className="size-3.5 text-ok" />
          ) : (
            <Copy className="size-3.5" />
          )}
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
        <span className="eyebrow">Usage</span>
        <div className="flex gap-1 rounded-md border border-edge bg-surface-alt p-1">
          {(["24h", "7d", "30d"] as const).map((p) => (
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

      {isLoading || !usage ? (
        <div className="h-14 animate-pulse rounded-md bg-surface-alt" />
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
              className="rounded-md border border-edge bg-surface-alt px-3 py-2"
            >
              <p className="text-[11px] font-medium text-fg-3">{label}</p>
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
    <div className="px-5 py-4">
      <div className="flex items-center justify-between gap-4">
        <div className="flex min-w-0 items-center gap-3">
          <div className="flex size-9 shrink-0 items-center justify-center rounded-md border border-edge bg-surface-alt">
            <Building2 className="size-4 text-fg-3" />
          </div>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-sm font-medium text-fg">{tenant.name}</span>
              <span className="font-mono text-[10px] text-fg-4">
                {tenant.slug}
              </span>
              <TierBadge tier={tenant.tier} />
            </div>
            <p className="mt-0.5 text-xs text-fg-4">
              Created {formatTimeAgo(tenant.created_at)}
              <span className="mx-1.5 text-fg-4">·</span>
              <span className="font-mono">
                {tenant.max_sessions_per_day}
              </span>{" "}
              sessions/day
              <span className="mx-1.5 text-fg-4">·</span>
              <span className="font-mono">
                {tenant.max_concurrent_sessions}
              </span>{" "}
              concurrent
              <span className="mx-1.5 text-fg-4">·</span>
              <span className="font-mono">
                {formatCost(tenant.max_budget_usd_per_session)}
              </span>
              /session
            </p>
          </div>
        </div>

        <div className="flex shrink-0 items-center gap-2">
          <button
            onClick={() => setShowUsage((v) => !v)}
            className={`flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs font-medium transition-colors ${
              showUsage
                ? "border-accent-muted bg-accent-soft text-accent"
                : "border-edge bg-surface text-fg-2 hover:border-fg-4 hover:text-fg"
            }`}
          >
            <ChartLine className="size-3.5" />
            Usage
          </button>
          <button
            onClick={startEdit}
            className="rounded-md p-2 text-fg-3 transition-colors hover:bg-surface-alt hover:text-fg"
            title="Edit tenant"
          >
            <SquarePen className="size-4" />
          </button>
          {confirmDelete ? (
            <span className="flex items-center gap-2">
              <button
                onClick={() => void handleDelete()}
                disabled={deleteTenant.isPending}
                className="rounded-md border border-danger/30 px-3 py-1.5 text-xs font-medium text-danger transition-colors hover:bg-danger/10"
              >
                Confirm
              </button>
              <button
                onClick={() => setConfirmDelete(false)}
                className="text-xs text-fg-3 transition-colors hover:text-fg"
              >
                Cancel
              </button>
            </span>
          ) : (
            <button
              onClick={() => setConfirmDelete(true)}
              className="rounded-md p-2 text-fg-4 transition-colors hover:bg-danger/10 hover:text-danger"
              title="Delete tenant"
            >
              <Trash2 className="size-4" />
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
              className="flex items-center gap-1.5 rounded-md bg-accent px-4 py-2 text-xs font-semibold text-white transition-colors hover:bg-accent-hover disabled:opacity-50"
            >
              {updateTenant.isPending ? (
                <Loader2 className="size-3.5 animate-spin" />
              ) : (
                <Save className="size-3.5" />
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
        <div className="overflow-hidden rounded-md border border-edge bg-surface">
          <div className="border-b border-edge px-5 py-3.5">
            <span className="eyebrow">Tenants</span>
          </div>
          <div className="divide-y divide-edge">
            {tenants.map((t) => (
              <TenantCard key={t.id} tenant={t} />
            ))}
          </div>
        </div>
      ) : (
        <div className="flex flex-col items-center rounded-md border border-edge bg-surface py-12 text-center">
          <Users className="mb-3 size-6 text-fg-4" strokeWidth={1.75} />
          <p className="text-sm text-fg-3">No tenants configured.</p>
        </div>
      )}

      {/* ── Create tenant ── */}
      <form
        onSubmit={(e) => void handleCreate(e)}
        className="overflow-hidden rounded-md border border-edge bg-surface"
      >
        <div className="border-b border-edge px-5 py-3.5">
          <span className="eyebrow">Add tenant</span>
        </div>
        <div className="p-5">
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
              className={monoInputCls}
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
            className="mt-4 flex items-center gap-2 rounded-md bg-accent px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-accent-hover disabled:opacity-50"
          >
            {createTenant.isPending ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <Plus className="size-4" />
            )}
            Create tenant
          </button>
        </div>
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
        <div className="overflow-hidden rounded-md border border-edge bg-surface">
          <div className="border-b border-edge px-5 py-3.5">
            <span className="eyebrow">Key pool</span>
          </div>
          <div className="divide-y divide-edge">
            {keys.map((k) => (
              <div
                key={k.id}
                className="flex items-center justify-between gap-4 px-5 py-4"
              >
                <div className="flex min-w-0 items-center gap-3">
                  <div className="flex size-9 shrink-0 items-center justify-center rounded-md border border-edge bg-surface-alt">
                    <KeyRound className="size-4 text-fg-3" />
                  </div>
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="rounded-[4px] border border-edge px-1.5 py-0.5 font-mono text-[10px] tracking-wider text-fg-3 uppercase">
                        {k.provider}
                      </span>
                      <span
                        className={`rounded-[4px] border px-1.5 py-0.5 font-mono text-[10px] tracking-wider uppercase ${
                          k.active
                            ? "border-ok/30 bg-ok/10 text-ok"
                            : "border-danger/30 bg-danger/10 text-danger"
                        }`}
                      >
                        {k.active ? "Active" : "Inactive"}
                      </span>
                      <span className="font-mono text-[10px] text-fg-4">
                        {k.id.slice(0, 12)}...
                      </span>
                    </div>
                    <p className="mt-0.5 text-xs text-fg-4">
                      Weight <span className="font-mono">{k.weight}</span>
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
                        className="rounded-md border border-danger/30 px-3 py-1.5 text-xs font-medium text-danger transition-colors hover:bg-danger/10"
                      >
                        Confirm
                      </button>
                      <button
                        onClick={() => setConfirmDelete(null)}
                        className="text-xs text-fg-3 transition-colors hover:text-fg"
                      >
                        Cancel
                      </button>
                    </span>
                  ) : (
                    <button
                      onClick={() => setConfirmDelete(k.id)}
                      className="rounded-md p-2 text-fg-4 transition-colors hover:bg-danger/10 hover:text-danger"
                      title="Delete key"
                    >
                      <Trash2 className="size-4" />
                    </button>
                  )}
                </div>
              </div>
            ))}
          </div>
        </div>
      ) : (
        <div className="flex flex-col items-center rounded-md border border-edge bg-surface py-12 text-center">
          <KeyRound className="mb-3 size-6 text-fg-4" strokeWidth={1.75} />
          <p className="text-sm text-fg-3">No keys in the pool.</p>
        </div>
      )}

      {/* ── Add key ── */}
      <form
        onSubmit={(e) => void handleAdd(e)}
        className="overflow-hidden rounded-md border border-edge bg-surface"
      >
        <div className="border-b border-edge px-5 py-3.5">
          <span className="eyebrow">Add pool key</span>
        </div>
        <div className="p-5">
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
              placeholder="API key"
              required
              className={monoInputCls}
            />
            <input
              type="number"
              min={1}
              value={weight}
              onChange={(e) => setWeight(e.target.value)}
              placeholder="Weight (default: 1)"
              className={monoInputCls}
            />
          </div>
          <button
            type="submit"
            disabled={addEntry.isPending}
            className="mt-4 flex items-center gap-2 rounded-md bg-accent px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-accent-hover disabled:opacity-50"
          >
            {addEntry.isPending ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <Plus className="size-4" />
            )}
            Add key
          </button>
        </div>
      </form>
    </div>
  );
}

function LoadingSkeleton() {
  return (
    <div className="space-y-3">
      {[1, 2, 3].map((i) => (
        <div key={i} className="h-16 animate-pulse rounded-md bg-surface-alt" />
      ))}
    </div>
  );
}

import { useState, useMemo, type FormEvent } from "react";
import { useSearchParams } from "react-router";
import {
  AppWindow,
  ArrowLeft,
  Bot,
  Bug,
  CircleCheck,
  ClipboardCheck,
  Cloud,
  Folder,
  FolderOpen,
  GitCommitHorizontal,
  Github,
  Gitlab,
  Globe,
  KeyRound,
  Loader2,
  Lock,
  Plus,
  Puzzle,
  Server,
  ShieldCheck,
  ShieldX,
  Terminal,
  Trash2,
  TriangleAlert,
  X,
  type LucideIcon,
} from "lucide-react";
import { usePageTitle } from "../hooks/usePageTitle";
import {
  useKeys,
  useCreateKey,
  useDeleteKey,
  useVerifyKey,
} from "../hooks/useKeys";
import { formatTimeAgo } from "../lib/formatters";
import type { KeyVerifyResult, MCPServer } from "../types";
import type { ToolDefinition } from "../types/session";
import {
  useMCPServers,
  useCreateMCPServer,
  useDeleteMCPServer,
} from "../hooks/useMCPServers";
import { useToolsCatalog } from "../hooks/useTools";
import { useWorkspaces, useDeleteWorkspace } from "../hooks/useWorkspaces";
type Tab = "keys" | "ai" | "mcp" | "workspaces";

const tabs: { id: Tab; label: string; icon: LucideIcon }[] = [
  { id: "keys", label: "Provider keys", icon: KeyRound },
  { id: "ai", label: "AI providers", icon: Bot },
  { id: "mcp", label: "MCP servers", icon: Server },
  { id: "workspaces", label: "Workspaces", icon: FolderOpen },
];

const VALID_TABS = new Set<Tab>(["keys", "ai", "mcp", "workspaces"]);

export default function Settings() {
  usePageTitle("Settings");
  const [searchParams, setSearchParams] = useSearchParams();
  const rawTab = searchParams.get("tab") as Tab | null;
  const activeTab: Tab = rawTab && VALID_TABS.has(rawTab) ? rawTab : "keys";

  function setActiveTab(tab: Tab) {
    setSearchParams(tab === "keys" ? {} : { tab }, { replace: true });
  }

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      <div>
        <p className="eyebrow mb-1">Configuration</p>
        <h2 className="font-expanded text-2xl font-extrabold tracking-tight text-fg">
          Settings
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
            {id === "ai" ? (
              <AnthropicLogo className="size-4" />
            ) : (
              <Icon className="size-4" />
            )}
            {label}
          </button>
        ))}
      </div>

      {activeTab === "keys" && <KeysTab />}
      {activeTab === "ai" && <AIProvidersTab />}
      {activeTab === "mcp" && <MCPTab />}
      {activeTab === "workspaces" && <WorkspacesTab />}
    </div>
  );
}

const inputCls =
  "w-full rounded-md border border-edge bg-input px-3 py-2 font-mono text-sm text-fg placeholder-fg-4 transition-colors focus:border-accent focus:outline-none";

const KEY_PROVIDERS = [
  {
    value: "github",
    label: "GitHub",
    icon: Github,
    description: "GitHub personal access token",
  },
  {
    value: "gitlab",
    label: "GitLab",
    icon: Gitlab,
    description: "GitLab personal access token",
  },
  {
    value: "sentry",
    label: "Sentry",
    icon: Bug,
    description: "Sentry authentication token",
  },
] as const;

function AnthropicLogo({ className = "" }: { className?: string }) {
  return (
    <svg viewBox="0 0 256 176" fill="currentColor" className={className}>
      <path d="m149.508 0 71.836 175.548h-45.381l-71.836-175.548z" />
      <path d="M106.492 0H62.674L0 153.079h44.57l17.14-43.207h50.376L94.946 78.986l-9.905-25.398L106.492 0Z" />
    </svg>
  );
}

function OpenAILogo({ className = "" }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" className={className}>
      <path d="M22.282 9.821a5.985 5.985 0 0 0-.516-4.91 6.046 6.046 0 0 0-6.51-2.9A6.065 6.065 0 0 0 4.981 4.18a5.998 5.998 0 0 0-3.992 2.9 6.04 6.04 0 0 0 .743 7.097 5.98 5.98 0 0 0 .51 4.911 6.051 6.051 0 0 0 6.515 2.9A5.985 5.985 0 0 0 13.26 24a6.056 6.056 0 0 0 5.772-4.206 5.99 5.99 0 0 0 3.997-2.9 6.056 6.056 0 0 0-.747-7.073ZM13.26 22.43a4.476 4.476 0 0 1-2.876-1.04l.141-.081 4.779-2.758a.795.795 0 0 0 .392-.681v-6.737l2.02 1.168a.071.071 0 0 1 .038.052v5.583a4.504 4.504 0 0 1-4.494 4.494ZM3.6 18.304a4.47 4.47 0 0 1-.535-3.014l.142.085 4.783 2.759a.771.771 0 0 0 .78 0l5.843-3.369v2.332a.08.08 0 0 1-.033.062L9.74 19.95a4.5 4.5 0 0 1-6.14-1.646ZM2.34 7.896a4.485 4.485 0 0 1 2.366-1.973V11.6a.766.766 0 0 0 .388.676l5.815 3.355-2.02 1.168a.076.076 0 0 1-.071 0l-4.83-2.786A4.504 4.504 0 0 1 2.34 7.872v.024Zm16.597 3.855-5.833-3.387L15.119 7.2a.076.076 0 0 1 .071 0l4.83 2.791a4.494 4.494 0 0 1-.676 8.105v-5.678a.79.79 0 0 0-.407-.667Zm2.01-3.023-.141-.085-4.774-2.782a.776.776 0 0 0-.785 0L9.409 9.23V6.897a.066.066 0 0 1 .028-.061l4.83-2.787a4.5 4.5 0 0 1 6.68 4.66v.018ZM8.318 12.861l-2.02-1.164a.076.076 0 0 1-.038-.057V6.075a4.5 4.5 0 0 1 7.375-3.453l-.142.08L8.704 5.46a.795.795 0 0 0-.392.68l-.004 6.721h.01Zm1.096-2.365L12 8.893l2.59 1.494v2.998L12 14.88l-2.586-1.494v-2.89Z" />
    </svg>
  );
}

const AI_PROVIDERS = [
  {
    value: "anthropic",
    label: "Anthropic",
    description: "API key for Claude models (claude-code CLI)",
    placeholder: "sk-ant-api03-...",
    docsHint: "Get your key at console.anthropic.com",
  },
  {
    value: "openai",
    label: "OpenAI",
    description: "API key for GPT/o-series models (codex CLI)",
    placeholder: "sk-...",
    docsHint: "Get your key at platform.openai.com",
  },
] as const;

function AIProviderIcon({
  provider,
  className = "",
}: {
  provider: string;
  className?: string;
}) {
  if (provider === "anthropic") return <AnthropicLogo className={className} />;
  if (provider === "openai") return <OpenAILogo className={className} />;
  return <Bot className={className} />;
}

function KeysTab() {
  const { data: keys, isLoading } = useKeys();
  const createKey = useCreateKey();
  const deleteKey = useDeleteKey();
  const verifyKey = useVerifyKey();

  const [formMode, setFormMode] = useState<"select" | "form">("select");
  const [name, setName] = useState("");
  const [provider, setProvider] = useState("github");
  const [token, setToken] = useState("");
  const [scope, setScope] = useState("");
  const [baseUrl, setBaseUrl] = useState("");
  const [validationError, setValidationError] = useState("");
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null);
  const [verifyResults, setVerifyResults] = useState<
    Record<string, KeyVerifyResult>
  >({});
  const [verifying, setVerifying] = useState<string | null>(null);

  function selectProvider(value: string) {
    setProvider(value);
    setValidationError("");
    setFormMode("form");
  }

  function goBack() {
    setName("");
    setToken("");
    setScope("");
    setBaseUrl("");
    setValidationError("");
    setFormMode("select");
  }

  async function handleAdd(e: FormEvent) {
    e.preventDefault();
    setValidationError("");

    if (baseUrl.trim() && !/^https?:\/\/.+/.test(baseUrl.trim())) {
      setValidationError("Base URL must start with http:// or https://");
      return;
    }

    await createKey.mutateAsync({
      name,
      provider,
      token,
      scope: scope || undefined,
      base_url: baseUrl.trim() || undefined,
    });
    setName("");
    setToken("");
    setScope("");
    setBaseUrl("");
    setFormMode("select");
  }

  async function handleDelete(keyName: string) {
    await deleteKey.mutateAsync(keyName);
    setConfirmDelete(null);
    setVerifyResults((prev) => {
      const next = { ...prev };
      delete next[keyName];
      return next;
    });
  }

  async function handleVerify(keyName: string) {
    setVerifying(keyName);
    try {
      const result = await verifyKey.mutateAsync(keyName);
      setVerifyResults((prev) => ({ ...prev, [keyName]: result }));
    } catch {
      setVerifyResults((prev) => ({
        ...prev,
        [keyName]: {
          name: keyName,
          provider: "",
          valid: false,
          error: "Connection failed",
        },
      }));
    } finally {
      setVerifying(null);
    }
  }

  return (
    <div className="space-y-6">
      {isLoading ? (
        <LoadingSkeleton />
      ) : keys && keys.length > 0 ? (
        <div className="overflow-hidden rounded-md border border-edge bg-surface">
          <div className="border-b border-edge px-5 py-3.5">
            <span className="eyebrow">Provider keys</span>
          </div>
          <div className="divide-y divide-edge">
            {keys.map((k) => {
              const vr = verifyResults[k.name];
              const isVerifying = verifying === k.name;

              return (
                <div
                  key={k.name}
                  className="flex items-center justify-between gap-4 px-5 py-4"
                >
                  <div className="flex min-w-0 items-center gap-3">
                    <div
                      className={`flex size-9 shrink-0 items-center justify-center rounded-md border ${
                        vr
                          ? vr.valid
                            ? "border-ok/30 bg-ok/10"
                            : "border-danger/30 bg-danger/10"
                          : "border-edge bg-surface-alt"
                      }`}
                    >
                      {vr ? (
                        vr.valid ? (
                          <ShieldCheck className="size-4 text-ok" />
                        ) : (
                          <ShieldX className="size-4 text-danger" />
                        )
                      ) : (
                        <KeyRound className="size-4 text-fg-3" />
                      )}
                    </div>
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="font-mono text-sm font-medium text-fg">
                          {k.name}
                        </span>
                        <span className="rounded-[4px] border border-edge px-1.5 py-0.5 font-mono text-[10px] tracking-wider text-fg-3 uppercase">
                          {k.provider}
                        </span>
                        {k.source === "env" && (
                          <span className="rounded-[4px] border border-warn/30 bg-warn/10 px-1.5 py-0.5 font-mono text-[10px] tracking-wider text-warn uppercase">
                            env
                          </span>
                        )}
                        {k.base_url && (
                          <span className="font-mono text-[10px] text-fg-4">
                            {k.base_url}
                          </span>
                        )}
                        {k.scope && (
                          <span className="font-mono text-xs text-fg-4">
                            {k.scope}
                          </span>
                        )}
                      </div>
                      {vr && vr.valid && vr.username && (
                        <div className="mt-1 flex items-center gap-1.5 text-xs text-fg-3">
                          <CircleCheck className="size-3.5 text-ok" />
                          <span className="font-medium text-fg-2">
                            {vr.username}
                          </span>
                          {vr.email && (
                            <>
                              <span className="text-fg-4">·</span>
                              <span>{vr.email}</span>
                            </>
                          )}
                        </div>
                      )}
                      {vr && vr.valid && vr.scopes && (
                        <div className="mt-1 flex flex-wrap gap-1">
                          {vr.scopes.split(",").map((s) => (
                            <span
                              key={s.trim()}
                              className="rounded-[4px] border border-ok/30 bg-ok/10 px-1.5 py-0.5 font-mono text-[10px] text-ok"
                            >
                              {s.trim()}
                            </span>
                          ))}
                        </div>
                      )}
                      {vr && !vr.valid && (
                        <p className="mt-1 text-xs text-danger">
                          {vr.error || "Token is invalid or expired"}
                        </p>
                      )}
                    </div>
                  </div>

                  <div className="flex shrink-0 items-center gap-2">
                    <button
                      onClick={() => void handleVerify(k.name)}
                      disabled={isVerifying}
                      className="flex items-center gap-1.5 rounded-md border border-edge bg-surface px-3 py-1.5 text-xs font-medium text-fg-2 transition-colors hover:border-fg-4 hover:text-fg disabled:opacity-50"
                    >
                      {isVerifying ? (
                        <Loader2 className="size-3.5 animate-spin" />
                      ) : (
                        <ShieldCheck className="size-3.5" />
                      )}
                      {isVerifying ? "Checking" : vr ? "Re-check" : "Verify"}
                    </button>

                    {k.source === "env" ? (
                      <span
                        className="cursor-not-allowed rounded-md p-2 text-fg-4 opacity-40"
                        title="Environment keys cannot be deleted"
                      >
                        <Lock className="size-4" />
                      </span>
                    ) : confirmDelete === k.name ? (
                      <span className="flex items-center gap-2">
                        <button
                          onClick={() => void handleDelete(k.name)}
                          disabled={deleteKey.isPending}
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
                        onClick={() => setConfirmDelete(k.name)}
                        className="rounded-md p-2 text-fg-4 transition-colors hover:bg-danger/10 hover:text-danger"
                      >
                        <Trash2 className="size-4" />
                      </button>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      ) : (
        <div className="flex flex-col items-center rounded-md border border-edge bg-surface py-12 text-center">
          <KeyRound className="mb-3 size-6 text-fg-4" strokeWidth={1.75} />
          <p className="text-sm text-fg-3">No provider keys configured.</p>
        </div>
      )}

      {/* ── Provider picker ── */}
      {formMode === "select" && (
        <div className="overflow-hidden rounded-md border border-edge bg-surface">
          <div className="border-b border-edge px-5 py-3.5">
            <span className="eyebrow">Add provider key</span>
          </div>
          <div className="grid grid-cols-3 gap-3 p-5">
            {KEY_PROVIDERS.map((p) => (
              <button
                key={p.value}
                type="button"
                onClick={() => selectProvider(p.value)}
                className="group flex flex-col items-start gap-2 rounded-md border border-edge bg-surface-alt p-4 text-left transition-colors hover:border-accent-muted hover:bg-accent-soft"
              >
                <p.icon className="size-5 text-fg-3 transition-colors group-hover:text-accent" />
                <div>
                  <p className="text-sm font-medium text-fg">{p.label}</p>
                  <p className="mt-0.5 text-xs text-fg-4">{p.description}</p>
                </div>
              </button>
            ))}
          </div>
        </div>
      )}

      {/* ── Key form ── */}
      {formMode === "form" && (
        <form
          onSubmit={(e) => void handleAdd(e)}
          className="overflow-hidden rounded-md border border-edge bg-surface"
        >
          <div className="flex items-center justify-between border-b border-edge px-5 py-3.5">
            <span className="eyebrow">
              {KEY_PROVIDERS.find((p) => p.value === provider)?.label} key
            </span>
            <button
              type="button"
              onClick={goBack}
              className="flex items-center gap-1 text-xs font-medium text-fg-3 transition-colors hover:text-fg"
            >
              <ArrowLeft className="size-3.5" />
              Back
            </button>
          </div>

          <div className="p-5">
            <div className="space-y-3">
              <input
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Name"
                required
                className={inputCls}
              />
              <input
                type="password"
                value={token}
                onChange={(e) => setToken(e.target.value)}
                placeholder="Token"
                required
                className={inputCls}
              />
              <input
                type="url"
                value={baseUrl}
                onChange={(e) => setBaseUrl(e.target.value)}
                placeholder={`Base URL for self-hosted (e.g. https://${provider === "sentry" ? "sentry" : provider === "gitlab" ? "gitlab" : "github"}.example.com)`}
                className={inputCls}
              />
              <input
                type="text"
                value={scope}
                onChange={(e) => setScope(e.target.value)}
                placeholder="Scope (optional)"
                className={inputCls}
              />
            </div>

            {validationError && (
              <p className="mt-3 text-xs text-danger">{validationError}</p>
            )}

            <button
              type="submit"
              disabled={createKey.isPending}
              className="mt-4 flex items-center gap-2 rounded-md bg-accent px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-accent-hover disabled:opacity-50"
            >
              {createKey.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Plus className="size-4" />
              )}
              Add key
            </button>
          </div>
        </form>
      )}
    </div>
  );
}

function AIProvidersTab() {
  const { data: allKeys, isLoading } = useKeys();
  const createKey = useCreateKey();
  const deleteKey = useDeleteKey();
  const verifyKey = useVerifyKey();

  const aiKeys = useMemo(
    () =>
      allKeys?.filter(
        (k) => k.provider === "anthropic" || k.provider === "openai",
      ) ?? [],
    [allKeys],
  );

  const [formMode, setFormMode] = useState<"select" | "form">("select");
  const [provider, setProvider] = useState("anthropic");
  const [name, setName] = useState("");
  const [token, setToken] = useState("");
  const [validationError, setValidationError] = useState("");
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null);
  const [verifyResults, setVerifyResults] = useState<
    Record<string, KeyVerifyResult>
  >({});
  const [verifying, setVerifying] = useState<string | null>(null);

  function selectProvider(value: string) {
    setProvider(value);
    setName(value); // default name = provider name
    setValidationError("");
    setFormMode("form");
  }

  function goBack() {
    setName("");
    setToken("");
    setValidationError("");
    setFormMode("select");
  }

  async function handleAdd(e: FormEvent) {
    e.preventDefault();
    setValidationError("");

    await createKey.mutateAsync({
      name,
      provider,
      token,
    });
    setName("");
    setToken("");
    setFormMode("select");
  }

  async function handleDelete(keyName: string) {
    await deleteKey.mutateAsync(keyName);
    setConfirmDelete(null);
    setVerifyResults((prev) => {
      const next = { ...prev };
      delete next[keyName];
      return next;
    });
  }

  async function handleVerify(keyName: string) {
    setVerifying(keyName);
    try {
      const result = await verifyKey.mutateAsync(keyName);
      setVerifyResults((prev) => ({ ...prev, [keyName]: result }));
    } catch {
      setVerifyResults((prev) => ({
        ...prev,
        [keyName]: {
          name: keyName,
          provider: "",
          valid: false,
          error: "Connection failed",
        },
      }));
    } finally {
      setVerifying(null);
    }
  }

  const CLI_FOR_PROVIDER: Record<string, string> = {
    anthropic: "claude-code",
    openai: "codex",
  };

  return (
    <div className="space-y-6">
      {/* Warning if no AI keys */}
      {!isLoading && aiKeys.length === 0 && (
        <div className="flex items-start gap-3 rounded-md border border-warn/30 bg-warn/10 p-4">
          <TriangleAlert className="mt-0.5 size-4 shrink-0 text-warn" />
          <div>
            <p className="text-sm font-medium text-warn">
              No AI provider keys configured
            </p>
            <p className="mt-1 text-xs text-fg-3">
              Sessions will fail unless{" "}
              <code className="rounded-[4px] border border-edge bg-surface-alt px-1 py-0.5 font-mono text-fg-2">
                ANTHROPIC_API_KEY
              </code>{" "}
              or{" "}
              <code className="rounded-[4px] border border-edge bg-surface-alt px-1 py-0.5 font-mono text-fg-2">
                OPENAI_API_KEY
              </code>{" "}
              is set as an environment variable on the server.
            </p>
          </div>
        </div>
      )}

      {/* Existing AI keys */}
      {isLoading ? (
        <LoadingSkeleton />
      ) : aiKeys.length > 0 ? (
        <div className="overflow-hidden rounded-md border border-edge bg-surface">
          <div className="border-b border-edge px-5 py-3.5">
            <span className="eyebrow">AI provider keys</span>
          </div>
          <div className="divide-y divide-edge">
            {aiKeys.map((k) => {
              const vr = verifyResults[k.name];
              const isVerifying = verifying === k.name;
              const cliName = CLI_FOR_PROVIDER[k.provider] ?? k.provider;

              return (
                <div
                  key={k.name}
                  className="flex items-center justify-between gap-4 px-5 py-4"
                >
                  <div className="flex min-w-0 items-center gap-3">
                    <div
                      className={`flex size-9 shrink-0 items-center justify-center rounded-md border ${
                        vr
                          ? vr.valid
                            ? "border-ok/30 bg-ok/10"
                            : "border-danger/30 bg-danger/10"
                          : "border-edge bg-surface-alt"
                      }`}
                    >
                      {vr ? (
                        vr.valid ? (
                          <ShieldCheck className="size-4 text-ok" />
                        ) : (
                          <ShieldX className="size-4 text-danger" />
                        )
                      ) : (
                        <AIProviderIcon
                          provider={k.provider}
                          className="size-4 text-fg-3"
                        />
                      )}
                    </div>
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="font-mono text-sm font-medium text-fg">
                          {k.name}
                        </span>
                        <span className="rounded-[4px] border border-edge px-1.5 py-0.5 font-mono text-[10px] tracking-wider text-fg-3 uppercase">
                          {k.provider}
                        </span>
                        {k.source === "env" && (
                          <span className="rounded-[4px] border border-warn/30 bg-warn/10 px-1.5 py-0.5 font-mono text-[10px] tracking-wider text-warn uppercase">
                            env
                          </span>
                        )}
                      </div>
                      <p className="mt-0.5 text-xs text-fg-4">
                        Used by:{" "}
                        <span className="font-mono text-fg-3">{cliName}</span>
                      </p>
                      {vr && vr.valid && (
                        <div className="mt-1 flex items-center gap-1.5 text-xs text-fg-3">
                          <CircleCheck className="size-3.5 text-ok" />
                          <span className="font-medium text-fg-2">
                            {vr.scopes || "Valid"}
                          </span>
                        </div>
                      )}
                      {vr && !vr.valid && (
                        <p className="mt-1 text-xs text-danger">
                          {vr.error || "Token is invalid or expired"}
                        </p>
                      )}
                    </div>
                  </div>

                  <div className="flex shrink-0 items-center gap-2">
                    <button
                      onClick={() => void handleVerify(k.name)}
                      disabled={isVerifying}
                      className="flex items-center gap-1.5 rounded-md border border-edge bg-surface px-3 py-1.5 text-xs font-medium text-fg-2 transition-colors hover:border-fg-4 hover:text-fg disabled:opacity-50"
                    >
                      {isVerifying ? (
                        <Loader2 className="size-3.5 animate-spin" />
                      ) : (
                        <ShieldCheck className="size-3.5" />
                      )}
                      {isVerifying ? "Checking" : vr ? "Re-check" : "Verify"}
                    </button>

                    {k.source === "env" ? (
                      <span
                        className="cursor-not-allowed rounded-md p-2 text-fg-4 opacity-40"
                        title="Environment keys cannot be deleted"
                      >
                        <Lock className="size-4" />
                      </span>
                    ) : confirmDelete === k.name ? (
                      <span className="flex items-center gap-2">
                        <button
                          onClick={() => void handleDelete(k.name)}
                          disabled={deleteKey.isPending}
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
                        onClick={() => setConfirmDelete(k.name)}
                        className="rounded-md p-2 text-fg-4 transition-colors hover:bg-danger/10 hover:text-danger"
                      >
                        <Trash2 className="size-4" />
                      </button>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      ) : null}

      {/* Provider picker */}
      {formMode === "select" && (
        <div className="overflow-hidden rounded-md border border-edge bg-surface">
          <div className="border-b border-edge px-5 py-3.5">
            <span className="eyebrow">Add AI provider key</span>
          </div>
          <div className="grid grid-cols-2 gap-3 p-5">
            {AI_PROVIDERS.map((p) => {
              const alreadyHas = aiKeys.some((k) => k.provider === p.value);
              return (
                <button
                  key={p.value}
                  type="button"
                  disabled={alreadyHas}
                  onClick={() => selectProvider(p.value)}
                  className={`group flex flex-col items-start gap-2 rounded-md border p-4 text-left transition-colors ${
                    alreadyHas
                      ? "cursor-default border-ok/25 bg-ok/10 opacity-70"
                      : "border-edge bg-surface-alt hover:border-accent-muted hover:bg-accent-soft"
                  }`}
                >
                  <div className="flex w-full items-center justify-between">
                    <AIProviderIcon
                      provider={p.value}
                      className={`size-5 ${alreadyHas ? "text-ok" : "text-fg-3 transition-colors group-hover:text-accent"}`}
                    />
                    {alreadyHas && <CircleCheck className="size-3.5 text-ok" />}
                  </div>
                  <div>
                    <p className="text-sm font-medium text-fg">{p.label}</p>
                    <p className="mt-0.5 text-xs text-fg-4">{p.description}</p>
                  </div>
                  <span className="text-[10px] text-fg-4">{p.docsHint}</span>
                </button>
              );
            })}
          </div>
        </div>
      )}

      {/* Key form */}
      {formMode === "form" && (
        <form
          onSubmit={(e) => void handleAdd(e)}
          className="overflow-hidden rounded-md border border-edge bg-surface"
        >
          <div className="flex items-center justify-between border-b border-edge px-5 py-3.5">
            <span className="eyebrow">
              {AI_PROVIDERS.find((p) => p.value === provider)?.label} key
            </span>
            <button
              type="button"
              onClick={goBack}
              className="flex items-center gap-1 text-xs font-medium text-fg-3 transition-colors hover:text-fg"
            >
              <ArrowLeft className="size-3.5" />
              Back
            </button>
          </div>

          <div className="p-5">
            <div className="space-y-3">
              <input
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Name"
                required
                className={inputCls}
              />
              <input
                type="password"
                value={token}
                onChange={(e) => setToken(e.target.value)}
                placeholder={
                  AI_PROVIDERS.find((p) => p.value === provider)?.placeholder ??
                  "API key"
                }
                required
                className={inputCls}
              />
            </div>

            {validationError && (
              <p className="mt-3 text-xs text-danger">{validationError}</p>
            )}

            <button
              type="submit"
              disabled={createKey.isPending}
              className="mt-4 flex items-center gap-2 rounded-md bg-accent px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-accent-hover disabled:opacity-50"
            >
              {createKey.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Plus className="size-4" />
              )}
              Add key
            </button>
          </div>
        </form>
      )}
    </div>
  );
}

const NAME_PATTERN = /^[a-zA-Z0-9_-]+$/;

function MCPServerCard({
  server,
  onDelete,
  isDeleting,
}: {
  server: MCPServer;
  onDelete: (name: string) => void;
  isDeleting: boolean;
}) {
  const [confirmDelete, setConfirmDelete] = useState(false);
  const transport = server.transport ?? "stdio";
  const envCount = server.env ? Object.keys(server.env).length : 0;
  const headerCount = server.headers ? Object.keys(server.headers).length : 0;

  function handleDelete() {
    onDelete(server.name);
    setConfirmDelete(false);
  }

  return (
    <div className="flex items-center justify-between gap-4 px-5 py-4">
      <div className="flex min-w-0 items-center gap-3">
        <div className="flex size-9 shrink-0 items-center justify-center rounded-md border border-edge bg-surface-alt">
          {transport === "http" ? (
            <Cloud className="size-4 text-fg-3" />
          ) : (
            <Server className="size-4 text-fg-3" />
          )}
        </div>
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span className="font-mono text-sm font-medium text-fg">
              {server.name}
            </span>
            <span
              className={`rounded-[4px] border px-1.5 py-0.5 font-mono text-[10px] tracking-wider uppercase ${
                transport === "http"
                  ? "border-info/30 bg-info/10 text-info"
                  : "border-edge bg-surface-alt text-fg-3"
              }`}
            >
              {transport}
            </span>
          </div>
          <p className="mt-0.5 truncate font-mono text-xs text-fg-4">
            {transport === "http"
              ? server.url
              : [server.command, server.package].filter(Boolean).join(" ")}
          </p>
          <div className="mt-1 flex flex-wrap items-center gap-2">
            {envCount > 0 && (
              <span className="flex items-center gap-1 text-[10px] text-fg-4">
                <KeyRound className="size-3" />
                {envCount} env var{envCount !== 1 && "s"}
                <span className="ml-1 font-mono text-fg-4/60">
                  ({Object.keys(server.env!).join(", ")} = {"•••••"})
                </span>
              </span>
            )}
            {headerCount > 0 && (
              <span className="flex items-center gap-1 text-[10px] text-fg-4">
                <Globe className="size-3" />
                {headerCount} header{headerCount !== 1 && "s"}
                <span className="ml-1 font-mono text-fg-4/60">
                  ({Object.keys(server.headers!).join(", ")} = {"•••••"})
                </span>
              </span>
            )}
            {server.created_at && (
              <span className="text-[10px] text-fg-4">
                {formatTimeAgo(server.created_at)}
              </span>
            )}
          </div>
        </div>
      </div>

      <div className="shrink-0">
        {confirmDelete ? (
          <div className="flex items-center gap-2">
            <button
              onClick={handleDelete}
              disabled={isDeleting}
              className="rounded-md border border-danger/30 px-3 py-1.5 text-xs font-medium text-danger transition-colors hover:bg-danger/10"
            >
              Delete
            </button>
            <button
              onClick={() => setConfirmDelete(false)}
              className="text-xs text-fg-3 transition-colors hover:text-fg"
            >
              Cancel
            </button>
          </div>
        ) : (
          <button
            onClick={() => setConfirmDelete(true)}
            className="rounded-md p-2 text-fg-4 transition-colors hover:bg-danger/10 hover:text-danger"
          >
            <Trash2 className="size-4" />
          </button>
        )}
      </div>
    </div>
  );
}

function KeyValueEditor({
  entries,
  onChange,
  keyPlaceholder = "Key",
  valuePlaceholder = "Value",
}: {
  entries: [string, string][];
  onChange: (entries: [string, string][]) => void;
  keyPlaceholder?: string;
  valuePlaceholder?: string;
}) {
  function addRow() {
    onChange([...entries, ["", ""]]);
  }

  function updateRow(index: number, field: 0 | 1, value: string) {
    const next = entries.map((e, i) =>
      i === index
        ? ((field === 0 ? [value, e[1]] : [e[0], value]) as [string, string])
        : e,
    );
    onChange(next);
  }

  function removeRow(index: number) {
    onChange(entries.filter((_, i) => i !== index));
  }

  return (
    <div className="space-y-2">
      {entries.map(([key, val], i) => (
        <div key={i} className="flex items-center gap-2">
          <input
            type="text"
            value={key}
            onChange={(e) => updateRow(i, 0, e.target.value)}
            placeholder={keyPlaceholder}
            className={inputCls}
          />
          <input
            type="password"
            value={val}
            onChange={(e) => updateRow(i, 1, e.target.value)}
            placeholder={valuePlaceholder}
            className={inputCls}
          />
          <button
            type="button"
            onClick={() => removeRow(i)}
            className="shrink-0 rounded-md p-2 text-fg-4 transition-colors hover:bg-danger/10 hover:text-danger"
          >
            <X className="size-4" />
          </button>
        </div>
      ))}
      <button
        type="button"
        onClick={addRow}
        className="flex items-center gap-1 text-xs font-medium text-fg-3 transition-colors hover:text-fg"
      >
        <Plus className="size-3.5" />
        Add
      </button>
    </div>
  );
}

const TOOL_ICONS: Record<string, LucideIcon> = {
  sentry: Bug,
  jira: ClipboardCheck,
  git: GitCommitHorizontal,
  github: Github,
  playwright: AppWindow,
};

// Extracts provider_key values from a tool's required_config fields.
function getToolProviderKeys(tool: ToolDefinition): string[] {
  return (tool.required_config ?? [])
    .map((f) => f.provider_key)
    .filter((p): p is string => !!p);
}

function MCPTab() {
  const { data: servers, isLoading } = useMCPServers();
  const { data: catalog, isLoading: catalogLoading } = useToolsCatalog();
  const { data: allKeys } = useKeys();
  const createServer = useCreateMCPServer();
  const deleteServer = useDeleteMCPServer();

  // Providers that have a key configured (db or env)
  const configuredProviders = useMemo(
    () => new Set(allKeys?.map((k) => k.provider) ?? []),
    [allKeys],
  );

  const [formMode, setFormMode] = useState<"select" | "preset" | "custom">(
    "select",
  );
  const [selectedTool, setSelectedTool] = useState<ToolDefinition | null>(null);
  const [presetEnv, setPresetEnv] = useState<Record<string, string>>({});
  const [presetUrl, setPresetUrl] = useState("");

  // Custom form state
  const [transport, setTransport] = useState<"stdio" | "http">("stdio");
  const [name, setName] = useState("");
  const [pkg, setPkg] = useState("");
  const [command, setCommand] = useState("");
  const [args, setArgs] = useState("");
  const [url, setUrl] = useState("");
  const [envEntries, setEnvEntries] = useState<[string, string][]>([]);
  const [headerEntries, setHeaderEntries] = useState<[string, string][]>([]);
  const [validationError, setValidationError] = useState("");

  const mcpTools = useMemo(
    () =>
      catalog?.filter((t: ToolDefinition) => t.type === "mcp" && t.builtin) ??
      [],
    [catalog],
  );

  const serverNames = useMemo(
    () => new Set(servers?.map((s) => s.name) ?? []),
    [servers],
  );

  function resetForm() {
    setName("");
    setPkg("");
    setCommand("");
    setArgs("");
    setUrl("");
    setEnvEntries([]);
    setHeaderEntries([]);
    setValidationError("");
    setPresetEnv({});
    setPresetUrl("");
    setSelectedTool(null);
  }

  function goBack() {
    resetForm();
    setFormMode("select");
  }

  function selectPreset(tool: ToolDefinition) {
    setSelectedTool(tool);
    setPresetEnv({});
    setPresetUrl(tool.mcp_url ?? "");
    setValidationError("");
    setFormMode("preset");
  }

  async function handlePresetSubmit(e: FormEvent) {
    e.preventDefault();
    if (!selectedTool) return;
    setValidationError("");

    if (serverNames.has(selectedTool.name)) {
      setValidationError(`Server "${selectedTool.name}" already exists`);
      return;
    }

    const isHttp = selectedTool.mcp_transport === "http";

    if (isHttp && !presetUrl.trim()) {
      setValidationError("URL is required");
      return;
    }

    // For HTTP: config fields become headers; for stdio: they become env vars
    const configEntries = selectedTool.required_config?.length
      ? Object.fromEntries(
          selectedTool.required_config
            .filter((f) => f.env_var && presetEnv[f.name]?.trim())
            .map((f) => [f.env_var!, presetEnv[f.name] as string]),
        )
      : undefined;
    const hasConfig = configEntries && Object.keys(configEntries).length > 0;

    await createServer.mutateAsync({
      name: selectedTool.name,
      transport: isHttp ? "http" : "stdio",
      ...(isHttp
        ? {
            url: presetUrl.trim(),
            headers: hasConfig ? configEntries : undefined,
          }
        : {
            package: selectedTool.mcp_package || undefined,
            command: selectedTool.mcp_command || undefined,
            env: hasConfig ? configEntries : undefined,
          }),
    });
    resetForm();
    setFormMode("select");
  }

  async function handleCustomSubmit(e: FormEvent) {
    e.preventDefault();
    setValidationError("");

    if (!NAME_PATTERN.test(name)) {
      setValidationError(
        "Name must contain only letters, numbers, hyphens, and underscores",
      );
      return;
    }

    if (transport === "stdio" && !pkg.trim()) {
      setValidationError("Package is required for stdio transport");
      return;
    }

    if (transport === "http" && !url.trim()) {
      setValidationError("URL is required for http transport");
      return;
    }

    const env =
      envEntries.length > 0
        ? Object.fromEntries(envEntries.filter(([k]) => k.trim()))
        : undefined;
    const headers =
      headerEntries.length > 0
        ? Object.fromEntries(headerEntries.filter(([k]) => k.trim()))
        : undefined;

    let parsedArgs: string[] | undefined;
    if (args.trim()) {
      try {
        const parsed = JSON.parse(args.trim());
        if (Array.isArray(parsed)) {
          parsedArgs = parsed as string[];
        } else {
          parsedArgs = args
            .split(",")
            .map((a) => a.trim())
            .filter(Boolean);
        }
      } catch {
        parsedArgs = args
          .split(",")
          .map((a) => a.trim())
          .filter(Boolean);
      }
    }

    await createServer.mutateAsync({
      name,
      transport,
      ...(transport === "stdio"
        ? {
            package: pkg,
            command: command.trim() || undefined,
            args: parsedArgs,
            env,
          }
        : {
            url,
            headers,
          }),
    });
    resetForm();
    setFormMode("select");
  }

  async function handleDelete(serverName: string) {
    await deleteServer.mutateAsync(serverName);
  }

  // Tools that are ready (have matching provider key or no config needed) and not manually added as MCP server
  const readyTools = useMemo(
    () =>
      mcpTools.filter((tool) => {
        if (serverNames.has(tool.name)) return false; // already has manual MCP server
        const needsConfig = (tool.required_config?.length ?? 0) > 0;
        if (!needsConfig) return true; // no config needed = always ready
        const providerKeys = getToolProviderKeys(tool);
        return providerKeys.some((p) => configuredProviders.has(p));
      }),
    [mcpTools, serverNames, configuredProviders],
  );

  // Tools that need manual config (have required fields without provider key match)
  const availableTools = useMemo(
    () =>
      mcpTools.filter((tool) => {
        if (serverNames.has(tool.name)) return false;
        if (readyTools.some((r) => r.name === tool.name)) return false;
        return true;
      }),
    [mcpTools, serverNames, readyTools],
  );

  return (
    <div className="space-y-6">
      {/* ── Ready to use ── */}
      {!isLoading && !catalogLoading && readyTools.length > 0 && (
        <div className="overflow-hidden rounded-md border border-edge bg-surface">
          <div className="border-b border-edge px-5 py-3.5">
            <span className="eyebrow">Ready to use</span>
          </div>
          <div className="divide-y divide-edge">
            {readyTools.map((tool) => {
              const providerKeys = getToolProviderKeys(tool);
              const matchingKey = allKeys?.find((k) =>
                providerKeys.includes(k.provider),
              );
              const hasKey = !!matchingKey;
              const ToolIcon = TOOL_ICONS[tool.name] ?? Puzzle;
              return (
                <div key={tool.name} className="px-5 py-4">
                  <div className="flex items-center gap-3">
                    <div className="flex size-9 shrink-0 items-center justify-center rounded-md border border-ok/30 bg-ok/10">
                      <ToolIcon className="size-4 text-ok" />
                    </div>
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-medium text-fg capitalize">
                          {tool.name}
                        </span>
                        <CircleCheck className="size-3.5 text-ok" />
                        {matchingKey?.source === "env" && (
                          <span className="rounded-[4px] border border-warn/30 bg-warn/10 px-1.5 py-0.5 font-mono text-[10px] tracking-wider text-warn uppercase">
                            env
                          </span>
                        )}
                      </div>
                      <p className="mt-0.5 text-xs text-fg-4">
                        {tool.description}
                      </p>
                      {hasKey && (
                        <p className="mt-1 flex items-center gap-1 text-[10px] text-fg-3">
                          <KeyRound className="size-3" />
                          {matchingKey.source === "env"
                            ? `Using ${matchingKey.scope} environment variable`
                            : `Using key "${matchingKey.name}" from Provider keys`}
                        </p>
                      )}
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}

      {/* ── Manually configured MCP servers ── */}
      {isLoading ? (
        <LoadingSkeleton />
      ) : servers && servers.length > 0 ? (
        <div className="overflow-hidden rounded-md border border-edge bg-surface">
          <div className="border-b border-edge px-5 py-3.5">
            <span className="eyebrow">Custom servers</span>
          </div>
          <div className="divide-y divide-edge">
            {servers.map((s) => (
              <MCPServerCard
                key={s.name}
                server={s}
                onDelete={handleDelete}
                isDeleting={deleteServer.isPending}
              />
            ))}
          </div>
        </div>
      ) : null}

      {/* ── Add more ── */}
      {formMode === "select" && (availableTools.length > 0 || true) && (
        <div className="overflow-hidden rounded-md border border-edge bg-surface">
          <div className="border-b border-edge px-5 py-3.5">
            <span className="eyebrow">Add MCP server</span>
          </div>

          <div className="p-5">
            {catalogLoading ? (
              <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
                {[1, 2, 3].map((i) => (
                  <div
                    key={i}
                    className="h-24 animate-pulse rounded-md bg-surface-alt"
                  />
                ))}
              </div>
            ) : (
              <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
                {mcpTools.map((tool) => {
                  const alreadyReady = readyTools.some(
                    (r) => r.name === tool.name,
                  );
                  const alreadyAdded = serverNames.has(tool.name);
                  const isDisabled = alreadyAdded || alreadyReady;
                  const needsConfig = (tool.required_config?.length ?? 0) > 0;
                  const providerKeys = getToolProviderKeys(tool);
                  const hasKey = providerKeys.some((p) =>
                    configuredProviders.has(p),
                  );
                  const ToolIcon = TOOL_ICONS[tool.name] ?? Puzzle;
                  return (
                    <button
                      key={tool.name}
                      type="button"
                      disabled={isDisabled}
                      onClick={() => selectPreset(tool)}
                      className={`group relative flex flex-col items-start gap-2 rounded-md border p-4 text-left transition-colors ${
                        isDisabled
                          ? "cursor-default border-ok/25 bg-ok/10 opacity-70"
                          : "border-edge bg-surface-alt hover:border-accent-muted hover:bg-accent-soft"
                      }`}
                    >
                      <div className="flex w-full items-center justify-between">
                        <ToolIcon
                          className={`size-5 ${isDisabled ? "text-ok" : "text-fg-3 transition-colors group-hover:text-accent"}`}
                        />
                        {isDisabled && (
                          <CircleCheck className="size-3.5 text-ok" />
                        )}
                      </div>
                      <div>
                        <p className="text-sm font-medium text-fg capitalize">
                          {tool.name}
                        </p>
                        <p className="mt-0.5 line-clamp-2 text-xs text-fg-4">
                          {tool.description}
                        </p>
                      </div>
                      {!isDisabled && needsConfig && !hasKey && (
                        <span className="text-[10px] text-fg-4">
                          {tool.required_config!.length} config field
                          {tool.required_config!.length !== 1 && "s"}
                        </span>
                      )}
                    </button>
                  );
                })}

                {/* Custom card */}
                <button
                  type="button"
                  onClick={() => setFormMode("custom")}
                  className="group flex flex-col items-start gap-2 rounded-md border border-dashed border-edge p-4 text-left transition-colors hover:border-accent-muted hover:bg-accent-soft"
                >
                  <Server className="size-5 text-fg-3 transition-colors group-hover:text-accent" />
                  <div>
                    <p className="text-sm font-medium text-fg">Custom</p>
                    <p className="mt-0.5 text-xs text-fg-4">
                      Manual server configuration
                    </p>
                  </div>
                </button>
              </div>
            )}
          </div>
        </div>
      )}

      {/* ── Preset form ── */}
      {formMode === "preset" && selectedTool && (
        <form
          onSubmit={(e) => void handlePresetSubmit(e)}
          className="overflow-hidden rounded-md border border-edge bg-surface"
        >
          <div className="flex items-center justify-between border-b border-edge px-5 py-3.5">
            <span className="eyebrow">{selectedTool.name}</span>
            <button
              type="button"
              onClick={goBack}
              className="flex items-center gap-1 text-xs font-medium text-fg-3 transition-colors hover:text-fg"
            >
              <ArrowLeft className="size-3.5" />
              Back
            </button>
          </div>

          <div className="p-5">
            {/* Connection info */}
            {selectedTool.mcp_transport === "http" ? (
              <div className="mb-4">
                <label className="mb-1.5 flex items-center gap-1.5 text-xs font-medium text-fg-2">
                  <Cloud className="size-3.5 text-info" />
                  Server URL
                  <span className="ml-auto rounded-[4px] border border-info/30 bg-info/10 px-1.5 py-0.5 font-mono text-[10px] tracking-wider text-info uppercase">
                    http
                  </span>
                </label>
                <input
                  type="url"
                  value={presetUrl}
                  onChange={(e) => setPresetUrl(e.target.value)}
                  placeholder="https://mcp.sentry.dev/mcp"
                  required
                  className={inputCls}
                />
              </div>
            ) : selectedTool.mcp_package ? (
              <div className="mb-4 flex items-center gap-2 rounded-md border border-edge bg-surface-alt px-3 py-2">
                <Terminal className="size-3.5 shrink-0 text-fg-3" />
                <p className="font-mono text-xs text-fg-3">
                  {selectedTool.mcp_command ?? "npx"} {selectedTool.mcp_package}
                </p>
                <span className="ml-auto rounded-[4px] border border-edge bg-surface px-1.5 py-0.5 font-mono text-[10px] tracking-wider text-fg-3 uppercase">
                  stdio
                </span>
              </div>
            ) : null}

            {selectedTool.required_config &&
            selectedTool.required_config.length > 0 ? (
              <div className="space-y-3">
                {selectedTool.required_config.map((field) => (
                  <div key={field.name}>
                    <label className="mb-1.5 flex items-center gap-1.5 text-xs font-medium text-fg-2">
                      {field.sensitive && (
                        <KeyRound className="size-3 text-fg-4" />
                      )}
                      <span className="font-mono">{field.name}</span>
                      <span className="font-normal text-fg-4">
                        — {field.description}
                      </span>
                    </label>
                    <input
                      type={field.sensitive ? "password" : "text"}
                      value={presetEnv[field.name] ?? ""}
                      onChange={(e) =>
                        setPresetEnv((prev) => ({
                          ...prev,
                          [field.name]: e.target.value,
                        }))
                      }
                      placeholder={field.env_var ?? field.name}
                      className={inputCls}
                    />
                  </div>
                ))}
              </div>
            ) : (
              <p className="py-4 text-center text-sm text-fg-4">
                No configuration needed
              </p>
            )}

            {validationError && (
              <p className="mt-3 text-xs text-danger">{validationError}</p>
            )}

            <button
              type="submit"
              disabled={createServer.isPending}
              className="mt-4 flex items-center gap-2 rounded-md bg-accent px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-accent-hover disabled:opacity-50"
            >
              {createServer.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Plus className="size-4" />
              )}
              Add server
            </button>
          </div>
        </form>
      )}

      {/* ── Custom form ── */}
      {formMode === "custom" && (
        <form
          onSubmit={(e) => void handleCustomSubmit(e)}
          className="overflow-hidden rounded-md border border-edge bg-surface"
        >
          <div className="flex items-center justify-between border-b border-edge px-5 py-3.5">
            <span className="eyebrow">Add MCP server</span>
            <button
              type="button"
              onClick={goBack}
              className="flex items-center gap-1 text-xs font-medium text-fg-3 transition-colors hover:text-fg"
            >
              <ArrowLeft className="size-3.5" />
              Back
            </button>
          </div>

          <div className="p-5">
            {/* Transport toggle */}
            <div className="mb-4 flex w-fit gap-1 rounded-md border border-edge bg-surface-alt p-1">
              {(["stdio", "http"] as const).map((t) => (
                <button
                  key={t}
                  type="button"
                  onClick={() => {
                    setTransport(t);
                    setValidationError("");
                  }}
                  className={`rounded-[4px] px-4 py-1.5 font-mono text-xs tracking-wider uppercase transition-colors ${
                    transport === t
                      ? "bg-accent text-white"
                      : "text-fg-3 hover:text-fg"
                  }`}
                >
                  {t}
                </button>
              ))}
            </div>

            <div className="space-y-3">
              <input
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Name"
                required
                className={inputCls}
              />

              {transport === "stdio" ? (
                <>
                  <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
                    <input
                      type="text"
                      value={pkg}
                      onChange={(e) => setPkg(e.target.value)}
                      placeholder="Package (required)"
                      required
                      className={inputCls}
                    />
                    <input
                      type="text"
                      value={command}
                      onChange={(e) => setCommand(e.target.value)}
                      placeholder="Command (default: npx)"
                      className={inputCls}
                    />
                    <input
                      type="text"
                      value={args}
                      onChange={(e) => setArgs(e.target.value)}
                      placeholder="Args (comma-separated or JSON)"
                      className={inputCls}
                    />
                  </div>

                  <div>
                    <label className="mb-1.5 flex items-center gap-1.5 text-xs font-medium text-fg-2">
                      <KeyRound className="size-3 text-fg-4" />
                      Environment variables
                    </label>
                    <KeyValueEditor
                      entries={envEntries}
                      onChange={setEnvEntries}
                      keyPlaceholder="ENV_VAR"
                      valuePlaceholder="value"
                    />
                  </div>
                </>
              ) : (
                <>
                  <input
                    type="url"
                    value={url}
                    onChange={(e) => setUrl(e.target.value)}
                    placeholder="URL (required)"
                    required
                    className={inputCls}
                  />

                  <div>
                    <label className="mb-1.5 flex items-center gap-1.5 text-xs font-medium text-fg-2">
                      <Globe className="size-3 text-fg-4" />
                      Headers
                    </label>
                    <KeyValueEditor
                      entries={headerEntries}
                      onChange={setHeaderEntries}
                      keyPlaceholder="Header-Name"
                      valuePlaceholder="value"
                    />
                  </div>
                </>
              )}
            </div>

            {validationError && (
              <p className="mt-3 text-xs text-danger">{validationError}</p>
            )}

            <button
              type="submit"
              disabled={createServer.isPending}
              className="mt-4 flex items-center gap-2 rounded-md bg-accent px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-accent-hover disabled:opacity-50"
            >
              {createServer.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Plus className="size-4" />
              )}
              Add server
            </button>
          </div>
        </form>
      )}
    </div>
  );
}

function WorkspacesTab() {
  const { data: workspaces, isLoading } = useWorkspaces();
  const deleteWorkspace = useDeleteWorkspace();
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null);

  async function handleDelete(sessionId: string) {
    await deleteWorkspace.mutateAsync(sessionId);
    setConfirmDelete(null);
  }

  const totalMB =
    Math.round((workspaces?.reduce((sum, w) => sum + w.size_mb, 0) ?? 0) * 10) /
    10;

  return (
    <div className="space-y-6">
      {!isLoading && workspaces && workspaces.length > 0 && (
        <p className="text-sm text-fg-3">
          Total disk usage:{" "}
          <span className="font-mono font-semibold text-fg">{totalMB} MB</span>
        </p>
      )}

      {isLoading ? (
        <LoadingSkeleton />
      ) : workspaces && workspaces.length > 0 ? (
        <div className="overflow-hidden rounded-md border border-edge bg-surface">
          <div className="border-b border-edge px-5 py-3.5">
            <span className="eyebrow">Workspaces</span>
          </div>
          <div className="divide-y divide-edge">
            {workspaces.map((w) => (
              <div
                key={w.session_id}
                className="flex items-center justify-between px-5 py-4"
              >
                <div className="flex items-center gap-3">
                  <div className="flex size-9 items-center justify-center rounded-md border border-edge bg-surface-alt">
                    <Folder className="size-4 text-fg-3" />
                  </div>
                  <div>
                    <span className="font-mono text-sm text-fg">
                      {w.session_id.slice(0, 12)}...
                    </span>
                    <p className="font-mono text-xs text-fg-4">{w.path}</p>
                  </div>
                </div>
                <div className="flex items-center gap-4">
                  <span className="font-mono text-sm text-fg-3">
                    {Math.round(w.size_mb * 10) / 10} MB
                  </span>
                  {confirmDelete === w.session_id ? (
                    <div className="flex items-center gap-2">
                      <button
                        onClick={() => void handleDelete(w.session_id)}
                        disabled={deleteWorkspace.isPending}
                        className="rounded-md border border-danger/30 px-3 py-1.5 text-xs font-medium text-danger transition-colors hover:bg-danger/10"
                      >
                        Delete
                      </button>
                      <button
                        onClick={() => setConfirmDelete(null)}
                        className="text-xs text-fg-3 transition-colors hover:text-fg"
                      >
                        Cancel
                      </button>
                    </div>
                  ) : (
                    <button
                      onClick={() => setConfirmDelete(w.session_id)}
                      className="rounded-md p-2 text-fg-4 transition-colors hover:bg-danger/10 hover:text-danger"
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
          <FolderOpen className="mb-3 size-6 text-fg-4" strokeWidth={1.75} />
          <p className="text-sm text-fg-3">No workspaces found.</p>
        </div>
      )}
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

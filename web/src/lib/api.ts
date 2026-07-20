import type {
  Session,
  SessionType,
  CreateSessionRequest,
  PRStatus,
  HealthResponse,
  ProviderKey,
  CreateKeyRequest,
  KeyVerifyResult,
  MCPServer,
  CreateMCPServerRequest,
  Workspace,
  WorkflowDefinition,
  RunWorkflowRequest,
  WorkflowConfig,
  CreateWorkflowConfigRequest,
  Repository,
  PullRequest,
  ToolDefinition,
  CLIEntry,
  SentryOrganization,
  SentryProject,
  SentryIssue,
  Tenant,
  CreateTenantRequest,
  CreateTenantResult,
  UpdateTenantRequest,
  TenantUsageSummary,
  MeResponse,
  MyUsage,
  KeyPoolEntry,
  AddKeyPoolRequest,
} from "../types";

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

async function request<T>(
  serverUrl: string,
  path: string,
  token: string,
  options: RequestInit = {},
): Promise<T> {
  const url = `${serverUrl}/api/v1${path}`;
  const res = await fetch(url, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
      ...options.headers,
    },
  });

  if (!res.ok) {
    const body = await res.text().catch(() => "");
    throw new ApiError(res.status, body || res.statusText);
  }

  if (res.status === 204) {
    return undefined as T;
  }

  return res.json() as Promise<T>;
}

export function createApiClient(serverUrl: string, token: string) {
  const get = <T>(path: string) => request<T>(serverUrl, path, token);
  const post = <T>(path: string, body?: unknown) =>
    request<T>(serverUrl, path, token, {
      method: "POST",
      body: body ? JSON.stringify(body) : undefined,
    });
  const del = <T>(path: string) =>
    request<T>(serverUrl, path, token, { method: "DELETE" });
  const patch = <T>(path: string, body?: unknown) =>
    request<T>(serverUrl, path, token, {
      method: "PATCH",
      body: body ? JSON.stringify(body) : undefined,
    });

  return {
    // Sessions
    listSessions: () =>
      get<{ sessions: Session[] }>("/sessions").then((r) => r.sessions),
    createSession: (req: CreateSessionRequest) =>
      post<Session>("/sessions", req),
    getSession: (id: string, include?: string) =>
      get<Session>(`/sessions/${id}${include ? `?include=${include}` : ""}`),
    cancelSession: (id: string) => post<void>(`/sessions/${id}/cancel`),
    instructSession: (id: string, prompt: string) =>
      post<void>(`/sessions/${id}/instruct`, { prompt }),
    createPR: (
      id: string,
      req?: { title?: string; description?: string; target_branch?: string },
    ) => post<Session>(`/sessions/${id}/create-pr`, req),
    pushToPR: (id: string) =>
      post<{ pr_url: string; branch: string; message: string }>(
        `/sessions/${id}/push`,
      ),
    reviewSession: (id: string, req?: { cli?: string; model?: string }) =>
      post<{ id: string; status: string }>(`/sessions/${id}/review`, req),
    postReviewComments: (id: string) =>
      post<{ posted: boolean; message: string }>(`/sessions/${id}/post-review`),
    getPRStatus: (id: string) => get<PRStatus>(`/sessions/${id}/pr-status`),

    // Session Types
    listSessionTypes: () =>
      get<{ session_types: SessionType[] }>("/session-types").then(
        (r) => r.session_types,
      ),

    // Repositories
    listRepositories: (providerKey: string) =>
      get<{ repositories: Repository[] }>(
        `/repositories?provider_key=${encodeURIComponent(providerKey)}`,
      ).then((r) => r.repositories),

    listBranches: (providerKey: string, repo: string) =>
      get<{ branches: { name: string; default: boolean }[] }>(
        `/branches?provider_key=${encodeURIComponent(providerKey)}&repo=${encodeURIComponent(repo)}`,
      ).then((r) => r.branches),

    listPullRequests: (providerKey: string, repo: string) =>
      get<{ pull_requests: PullRequest[] }>(
        `/pull-requests?provider_key=${encodeURIComponent(providerKey)}&repo=${encodeURIComponent(repo)}`,
      ).then((r) => r.pull_requests),

    // Tools
    listToolsCatalog: () =>
      get<{ tools: ToolDefinition[] }>("/tools/catalog").then((r) => r.tools),

    // CLI
    listCLIs: () => get<{ cli: CLIEntry[] }>("/cli").then((r) => r.cli),

    // Keys
    listKeys: () => get<{ keys: ProviderKey[] }>("/keys").then((r) => r.keys),
    createKey: (req: CreateKeyRequest) => post<void>("/keys", req),
    deleteKey: (name: string) => del<void>(`/keys/${name}`),
    verifyKey: (name: string) => get<KeyVerifyResult>(`/keys/${name}/verify`),

    // MCP Servers
    listMCPServers: () =>
      get<{ servers: MCPServer[] }>("/mcp/servers").then((r) => r.servers),
    createMCPServer: (req: CreateMCPServerRequest) =>
      post<void>("/mcp/servers", req),
    deleteMCPServer: (name: string) => del<void>(`/mcp/servers/${name}`),

    // Workspaces
    listWorkspaces: () =>
      get<{ workspaces: Workspace[] }>("/workspaces").then((r) => r.workspaces),
    deleteWorkspace: (sessionId: string) =>
      del<void>(`/workspaces/${sessionId}`),

    // Workflows
    listWorkflows: () =>
      get<{ workflows: WorkflowDefinition[] }>("/workflows").then(
        (r) => r.workflows,
      ),
    getWorkflow: (name: string) =>
      get<WorkflowDefinition>(`/workflows/${encodeURIComponent(name)}`),
    deleteWorkflow: (name: string) =>
      del<void>(`/workflows/${encodeURIComponent(name)}`),

    // Workflow Runs (preset → session)
    runWorkflow: (name: string, req?: RunWorkflowRequest) =>
      post<{ session_id: string; workflow_name: string }>(
        `/workflows/${encodeURIComponent(name)}/run`,
        req,
      ),

    // Workflow Configs (saved configurations)
    listWorkflowConfigs: () =>
      get<{ configs: WorkflowConfig[] }>("/workflow-configs").then(
        (r) => r.configs,
      ),
    createWorkflowConfig: (req: CreateWorkflowConfigRequest) =>
      post<{ id: number; name: string }>("/workflow-configs", req),
    deleteWorkflowConfig: (id: number) => del<void>(`/workflow-configs/${id}`),
    runWorkflowConfig: (id: number) =>
      post<{ session_id: string; config_id: number; config_name: string }>(
        `/workflow-configs/${id}/run`,
      ),

    // Sentry (proxied through BE)
    listSentryOrganizations: (keyName: string) =>
      get<{ organizations: SentryOrganization[] }>(
        `/sentry/organizations?key_name=${encodeURIComponent(keyName)}`,
      ).then((r) => r.organizations),

    listSentryProjects: (keyName: string, org: string, region?: string) =>
      get<{ projects: SentryProject[] }>(
        `/sentry/projects?key_name=${encodeURIComponent(keyName)}&org=${encodeURIComponent(org)}${region ? `&region=${encodeURIComponent(region)}` : ""}`,
      ).then((r) => r.projects),

    listSentryIssues: (
      keyName: string,
      org: string,
      project: string,
      opts?: { query?: string; sort?: string; limit?: number; region?: string },
    ) => {
      const params = new URLSearchParams({
        key_name: keyName,
        org,
        project,
      });
      if (opts?.query) params.set("query", opts.query);
      if (opts?.sort) params.set("sort", opts.sort);
      if (opts?.limit) params.set("limit", String(opts.limit));
      if (opts?.region) params.set("region", opts.region);
      return get<{ issues: SentryIssue[] }>(
        `/sentry/issues?${params.toString()}`,
      ).then((r) => r.issues);
    },

    // Admin: Tenants
    listTenants: () =>
      get<{ tenants: Tenant[] }>("/admin/tenants/").then((r) => r.tenants),
    createTenant: (req: CreateTenantRequest) =>
      post<CreateTenantResult>("/admin/tenants/", req),
    getTenant: (id: string) => get<Tenant>(`/admin/tenants/${id}`),
    updateTenant: (id: string, req: UpdateTenantRequest) =>
      patch<Tenant>(`/admin/tenants/${id}`, req),
    deleteTenant: (id: string) => del<void>(`/admin/tenants/${id}`),
    getTenantUsage: (id: string, period?: string) =>
      get<TenantUsageSummary>(
        `/admin/tenants/${id}/usage${period ? `?period=${encodeURIComponent(period)}` : ""}`,
      ),

    // Me (tenant self-service)
    getMe: () => get<MeResponse>("/me"),
    getMyUsage: (period?: string) =>
      get<MyUsage>(
        `/me/usage${period ? `?period=${encodeURIComponent(period)}` : ""}`,
      ),

    // Admin: Key Pool
    listKeyPool: () =>
      get<{ keys: KeyPoolEntry[] }>("/admin/key-pool/").then((r) => r.keys),
    addKeyPoolEntry: (req: AddKeyPoolRequest) =>
      post<KeyPoolEntry>("/admin/key-pool/", req),
    deleteKeyPoolEntry: (id: string) => del<void>(`/admin/key-pool/${id}`),

    // Health
    getHealth: () => get<HealthResponse>("/health"),
  };
}

export type ApiClient = ReturnType<typeof createApiClient>;

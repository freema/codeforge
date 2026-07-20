export type TenantTier = "free" | "pro" | "enterprise";

export interface Tenant {
  id: string;
  name: string;
  slug: string;
  tier: string;
  max_sessions_per_day: number;
  max_concurrent_sessions: number;
  max_budget_usd_per_session: number;
  allowed_clis: string;
  allowed_models?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateTenantRequest {
  name: string;
  slug: string;
  tier: TenantTier;
}

export interface CreateTenantResult {
  tenant: Tenant;
  api_token: string;
}

export interface UpdateTenantRequest {
  name?: string;
  tier?: string;
  max_sessions_per_day?: number;
  max_concurrent_sessions?: number;
  max_budget_usd_per_session?: number;
  allowed_clis?: string;
  allowed_models?: string;
}

export interface TenantUsageSummary {
  total_sessions: number;
  total_input_tokens: number;
  total_output_tokens: number;
  total_cost_usd: number;
}

export interface KeyPoolEntry {
  id: string;
  provider: string;
  weight: number;
  active: boolean;
  created_at: string;
}

export interface AddKeyPoolRequest {
  provider: string;
  token: string;
  weight?: number;
}

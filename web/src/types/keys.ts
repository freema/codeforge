export interface ProviderKey {
  name: string;
  provider: string;
  scope?: string;
  base_url?: string;
  source?: "db" | "env";
}

export interface CreateKeyRequest {
  name: string;
  provider: string;
  token: string;
  scope?: string;
  base_url?: string;
}

export interface KeyVerifyResult {
  name: string;
  provider: string;
  valid: boolean;
  username?: string;
  email?: string;
  scopes?: string;
  error?: string;
}

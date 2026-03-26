export interface MCPServer {
  name: string;
  transport?: string; // "stdio" (default) | "http"
  // stdio fields
  command?: string;
  package?: string;
  args?: string[];
  env?: Record<string, string>;
  // http fields
  url?: string;
  headers?: Record<string, string>;
  created_at?: string;
}

export interface CreateMCPServerRequest {
  name: string;
  transport?: string;
  command?: string;
  package?: string;
  args?: string[];
  env?: Record<string, string>;
  url?: string;
  headers?: Record<string, string>;
}

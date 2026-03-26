export interface HealthResponse {
  status: string;
  version: string;
  redis: string;
  sqlite: string;
  workspace_disk_usage_mb: number;
}

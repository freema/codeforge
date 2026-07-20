import type { CreateSessionRequest } from "./session";

export interface Schedule {
  id: string;
  name: string;
  cron: string;
  enabled: boolean;
  session_request: CreateSessionRequest;
  last_run_at?: string;
  last_session_id?: string;
  next_run_at?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateScheduleRequest {
  name: string;
  cron: string;
  enabled?: boolean;
  session_request: CreateSessionRequest;
}

export interface UpdateScheduleRequest {
  name?: string;
  cron?: string;
  enabled?: boolean;
  session_request?: CreateSessionRequest;
}

export interface RunScheduleResult {
  schedule_id: string;
  session_id: string;
}

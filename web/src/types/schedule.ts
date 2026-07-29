import type { CreateSessionRequest } from "./session";

export interface Schedule {
  id: string;
  name: string;
  cron: string;
  enabled: boolean;
  session_request: CreateSessionRequest;
  timezone?: string;
  consecutive_failures: number;
  disabled_reason?: string;
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
  timezone?: string;
  session_request: CreateSessionRequest;
}

export interface UpdateScheduleRequest {
  name?: string;
  cron?: string;
  enabled?: boolean;
  timezone?: string;
  session_request?: CreateSessionRequest;
}

export type ScheduleRunTrigger = "cron" | "manual";

export type ScheduleRunStatus =
  | "fired"
  | "fire_failed"
  | "skipped_overlap"
  | "session_completed"
  | "session_failed";

export interface ScheduleRun {
  id: string;
  schedule_id: string;
  session_id?: string;
  trigger: ScheduleRunTrigger;
  status: ScheduleRunStatus;
  error?: string;
  created_at: string;
  updated_at?: string;
}

export interface RunScheduleResult {
  schedule_id: string;
  session_id: string;
}

import type { CreateSessionRequest } from "./session";

export interface Schedule {
  id: string;
  name: string;
  cron: string;
  enabled: boolean;
  /** Present on custom schedules; absent when blueprint-backed. */
  session_request?: CreateSessionRequest;
  /** Present when the schedule runs a blueprint instead of a raw request. */
  blueprint_id?: string;
  blueprint_params?: Record<string, string>;
  timezone?: string;
  consecutive_failures: number;
  disabled_reason?: string;
  last_run_at?: string;
  last_session_id?: string;
  next_run_at?: string;
  created_at: string;
  updated_at: string;
}

/** Exactly one of session_request or blueprint_id must be set. */
export interface CreateScheduleRequest {
  name: string;
  cron: string;
  enabled?: boolean;
  timezone?: string;
  session_request?: CreateSessionRequest;
  blueprint_id?: string;
  blueprint_params?: Record<string, string>;
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

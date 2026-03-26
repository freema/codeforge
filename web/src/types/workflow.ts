export type RunStatus =
  | "pending"
  | "running"
  | "completed"
  | "failed"
  | "cancelled";

export type StepStatus =
  | "pending"
  | "running"
  | "completed"
  | "failed"
  | "skipped";

export type StepType = "fetch" | "session" | "action";

export interface ParameterDefinition {
  name: string;
  required: boolean;
  default?: string;
}

export interface StepDefinition {
  name: string;
  type: StepType;
  config: Record<string, unknown>;
}

export interface WorkflowDefinition {
  name: string;
  description: string;
  builtin: boolean;
  steps: StepDefinition[];
  parameters: ParameterDefinition[];
  created_at: string;
}

export interface WorkflowRunStep {
  run_id: string;
  step_name: string;
  step_type: StepType;
  status: StepStatus;
  result?: string;
  task_id?: string;
  error?: string;
  started_at?: string;
  finished_at?: string;
}

export interface WorkflowRun {
  id: string;
  workflow_name: string;
  status: RunStatus;
  params?: Record<string, string>;
  error?: string;
  steps?: WorkflowRunStep[];
  created_at: string;
  started_at?: string;
  finished_at?: string;
}

export interface CreateWorkflowRequest {
  name: string;
  description?: string;
  steps: StepDefinition[];
  parameters?: ParameterDefinition[];
}

export interface RunWorkflowRequest {
  params?: Record<string, string>;
}

export interface WorkflowConfig {
  id: number;
  name: string;
  workflow: string;
  params: Record<string, string>;
  timeout_seconds?: number;
  created_at: string;
}

export interface CreateWorkflowConfigRequest {
  name: string;
  workflow: string;
  params: Record<string, string>;
  timeout_seconds?: number;
}

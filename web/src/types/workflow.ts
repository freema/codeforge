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

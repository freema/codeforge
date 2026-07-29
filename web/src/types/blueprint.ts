import type { CreateSessionRequest } from "./session";

export interface ParameterDefinition {
  name: string;
  required: boolean;
  default?: string;
}

/**
 * A blueprint's request template: CreateSessionRequest-shaped, string fields
 * may contain {{.Params.x}} expressions, plus one extra optional top-level
 * field tool_key_ref (a key registry entry injected into tool auth on run).
 */
export interface BlueprintRequestTemplate extends CreateSessionRequest {
  tool_key_ref?: string;
}

/**
 * A named, reusable session request template (operator-only). A preset is a
 * blueprint with parameters=[]; a parameterized blueprint asks for values at
 * run time.
 */
export interface Blueprint {
  id: string;
  name: string;
  description: string;
  builtin: boolean;
  request: BlueprintRequestTemplate;
  parameters: ParameterDefinition[];
  created_at: string;
  updated_at: string;
}

export interface CreateBlueprintRequest {
  name: string;
  description?: string;
  request: BlueprintRequestTemplate;
  parameters?: ParameterDefinition[];
}

export interface RunBlueprintRequest {
  params?: Record<string, string>;
  prompt?: string;
}

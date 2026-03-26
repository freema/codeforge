import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi } from "./useApi";
import type { CreateWorkflowRequest, RunWorkflowRequest } from "../types";

export function useCreateWorkflow() {
  const api = useApi();
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (req: CreateWorkflowRequest) => api.createWorkflow(req),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["workflows"] }),
  });
}

export function useDeleteWorkflow() {
  const api = useApi();
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (name: string) => api.deleteWorkflow(name),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["workflows"] }),
  });
}

export function useRunWorkflow() {
  const api = useApi();

  return useMutation({
    mutationFn: ({ name, ...req }: { name: string } & RunWorkflowRequest) =>
      api.runWorkflow(name, req),
  });
}

export function useCancelWorkflowRun() {
  const api = useApi();
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (runId: string) => api.cancelWorkflowRun(runId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["workflowRuns"] });
      qc.invalidateQueries({ queryKey: ["workflowRun"] });
    },
  });
}

export function useCancelAllWorkflowRuns() {
  const api = useApi();
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (workflowName?: string) =>
      api.cancelAllWorkflowRuns(workflowName),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["workflowRuns"] });
      qc.invalidateQueries({ queryKey: ["workflowRun"] });
    },
  });
}

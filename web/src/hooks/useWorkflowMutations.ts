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


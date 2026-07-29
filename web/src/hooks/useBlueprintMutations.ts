import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi } from "./useApi";
import type { CreateBlueprintRequest, RunBlueprintRequest } from "../types";

export function useCreateBlueprint() {
  const api = useApi();
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (req: CreateBlueprintRequest) => api.createBlueprint(req),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["blueprints"] }),
  });
}

export function useDeleteBlueprint() {
  const api = useApi();
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => api.deleteBlueprint(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["blueprints"] }),
  });
}

export function useRunBlueprint() {
  const api = useApi();

  return useMutation({
    mutationFn: ({ id, ...req }: { id: string } & RunBlueprintRequest) =>
      api.runBlueprint(id, req),
  });
}

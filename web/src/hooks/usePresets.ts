import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi } from "./useApi";
import type { CreatePresetRequest } from "../types";

export function usePresets(enabled = true) {
  const api = useApi();

  return useQuery({
    queryKey: ["presets"],
    queryFn: () => api.listPresets(),
    enabled,
  });
}

export function useCreatePreset() {
  const api = useApi();
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (req: CreatePresetRequest) => api.createPreset(req),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["presets"] }),
  });
}

export function useDeletePreset() {
  const api = useApi();
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => api.deletePreset(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["presets"] }),
  });
}

export function useRunPreset() {
  const api = useApi();

  return useMutation({
    mutationFn: ({ id, prompt }: { id: string; prompt?: string }) =>
      api.runPreset(id, prompt),
  });
}

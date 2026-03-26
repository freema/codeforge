import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi } from "./useApi";
import type { CreateKeyRequest } from "../types";

export function useKeys() {
  const api = useApi();

  return useQuery({
    queryKey: ["keys"],
    queryFn: () => api.listKeys(),
  });
}

export function useCreateKey() {
  const api = useApi();
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (req: CreateKeyRequest) => api.createKey(req),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["keys"] }),
  });
}

export function useDeleteKey() {
  const api = useApi();
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (name: string) => api.deleteKey(name),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["keys"] }),
  });
}

export function useVerifyKey() {
  const api = useApi();

  return useMutation({
    mutationFn: (name: string) => api.verifyKey(name),
  });
}

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi } from "./useApi";
import type {
  AddKeyPoolRequest,
  CreateTenantRequest,
  UpdateTenantRequest,
} from "../types";

export function useTenants() {
  const api = useApi();

  return useQuery({
    queryKey: ["tenants"],
    queryFn: () => api.listTenants(),
  });
}

export function useTenant(id: string) {
  const api = useApi();

  return useQuery({
    queryKey: ["tenants", id],
    queryFn: () => api.getTenant(id),
    enabled: !!id,
  });
}

export function useTenantUsage(id: string, period?: string) {
  const api = useApi();

  return useQuery({
    queryKey: ["tenants", id, "usage", period ?? "7d"],
    queryFn: () => api.getTenantUsage(id, period),
    enabled: !!id,
  });
}

export function useCreateTenant() {
  const api = useApi();
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (req: CreateTenantRequest) => api.createTenant(req),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["tenants"] }),
  });
}

export function useUpdateTenant() {
  const api = useApi();
  const qc = useQueryClient();

  return useMutation({
    mutationFn: ({ id, req }: { id: string; req: UpdateTenantRequest }) =>
      api.updateTenant(id, req),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["tenants"] }),
  });
}

export function useDeleteTenant() {
  const api = useApi();
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => api.deleteTenant(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["tenants"] }),
  });
}

export function useKeyPool() {
  const api = useApi();

  return useQuery({
    queryKey: ["key-pool"],
    queryFn: () => api.listKeyPool(),
  });
}

export function useAddKeyPoolEntry() {
  const api = useApi();
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (req: AddKeyPoolRequest) => api.addKeyPoolEntry(req),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["key-pool"] }),
  });
}

export function useDeleteKeyPoolEntry() {
  const api = useApi();
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => api.deleteKeyPoolEntry(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["key-pool"] }),
  });
}

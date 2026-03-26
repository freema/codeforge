import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi } from "./useApi";
import type { CreateMCPServerRequest } from "../types";

export function useMCPServers() {
  const api = useApi();

  return useQuery({
    queryKey: ["mcpServers"],
    queryFn: () => api.listMCPServers(),
  });
}

export function useCreateMCPServer() {
  const api = useApi();
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (req: CreateMCPServerRequest) => api.createMCPServer(req),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["mcpServers"] }),
  });
}

export function useDeleteMCPServer() {
  const api = useApi();
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (name: string) => api.deleteMCPServer(name),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["mcpServers"] }),
  });
}

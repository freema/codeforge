import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi } from "./useApi";

export function useWorkspaces() {
  const api = useApi();

  return useQuery({
    queryKey: ["workspaces"],
    queryFn: () => api.listWorkspaces(),
  });
}

export function useDeleteWorkspace() {
  const api = useApi();
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (sessionId: string) => api.deleteWorkspace(sessionId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["workspaces"] }),
  });
}

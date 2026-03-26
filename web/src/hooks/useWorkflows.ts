import { useQuery } from "@tanstack/react-query";
import { useApi } from "./useApi";

export function useWorkflows() {
  const api = useApi();

  return useQuery({
    queryKey: ["workflows"],
    queryFn: () => api.listWorkflows(),
    refetchInterval: 30_000,
  });
}

export function useWorkflow(name: string | undefined) {
  const api = useApi();

  return useQuery({
    queryKey: ["workflow", name],
    queryFn: () => api.getWorkflow(name!),
    enabled: !!name,
  });
}

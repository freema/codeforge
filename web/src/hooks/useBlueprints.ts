import { useQuery } from "@tanstack/react-query";
import { useApi } from "./useApi";

export function useBlueprints(enabled = true) {
  const api = useApi();

  return useQuery({
    queryKey: ["blueprints"],
    queryFn: () => api.listBlueprints(),
    enabled,
  });
}

export function useBlueprint(id: string | undefined) {
  const api = useApi();

  return useQuery({
    queryKey: ["blueprint", id],
    queryFn: () => api.getBlueprint(id!),
    enabled: !!id,
  });
}

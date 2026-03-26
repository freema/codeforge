import { useQuery } from "@tanstack/react-query";
import { useApi } from "./useApi";

export function useRepositories(providerKey: string | undefined) {
  const api = useApi();

  return useQuery({
    queryKey: ["repositories", providerKey],
    queryFn: () => api.listRepositories(providerKey!),
    enabled: !!providerKey,
    staleTime: 60_000,
  });
}

import { useQuery } from "@tanstack/react-query";
import { useApi } from "./useApi";

export function useBranches(
  providerKey: string | undefined,
  repo: string | undefined,
) {
  const api = useApi();

  return useQuery({
    queryKey: ["branches", providerKey, repo],
    queryFn: () => api.listBranches(providerKey!, repo!),
    enabled: !!providerKey && !!repo,
    staleTime: 60_000,
  });
}

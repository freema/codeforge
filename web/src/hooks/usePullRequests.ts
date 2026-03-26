import { useQuery } from "@tanstack/react-query";
import { useApi } from "./useApi";

export function usePullRequests(
  providerKey: string | undefined,
  repo: string | undefined,
) {
  const api = useApi();

  return useQuery({
    queryKey: ["pull-requests", providerKey, repo],
    queryFn: () => api.listPullRequests(providerKey!, repo!),
    enabled: !!providerKey && !!repo,
    staleTime: 30_000,
  });
}

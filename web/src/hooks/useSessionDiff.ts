import { useQuery } from "@tanstack/react-query";
import { useApi } from "./useApi";
import { ApiError } from "../lib/api";

export function useSessionDiff(id: string | undefined) {
  const api = useApi();

  return useQuery({
    queryKey: ["session-diff", id],
    queryFn: () => api.getSessionDiff(id!),
    enabled: !!id,
    staleTime: 30_000,
    retry: (failureCount, error) => {
      // 404 means the workspace expired — retrying will not help.
      if (error instanceof ApiError && error.status === 404) return false;
      return failureCount < 2;
    },
  });
}

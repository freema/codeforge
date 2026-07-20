import { useQuery } from "@tanstack/react-query";
import { useApi } from "./useApi";

export function useMe() {
  const api = useApi();

  return useQuery({
    queryKey: ["me"],
    queryFn: () => api.getMe(),
  });
}

export function useMyUsage(period: string, enabled = true) {
  const api = useApi();

  return useQuery({
    queryKey: ["me", "usage", period],
    queryFn: () => api.getMyUsage(period),
    enabled,
  });
}

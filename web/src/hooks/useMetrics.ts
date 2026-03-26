import { useQuery } from "@tanstack/react-query";
import { useApi } from "./useApi";

export function useMetrics() {
  const api = useApi();

  return useQuery({
    queryKey: ["metrics"],
    queryFn: () => api.getMetrics(),
    refetchInterval: 30_000,
  });
}

import { useQuery } from "@tanstack/react-query";
import { useApi } from "./useApi";

export function useHealth() {
  const api = useApi();

  return useQuery({
    queryKey: ["health"],
    queryFn: () => api.getHealth(),
    refetchInterval: 30_000,
  });
}

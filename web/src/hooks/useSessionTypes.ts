import { useQuery } from "@tanstack/react-query";
import { useApi } from "./useApi";

export function useSessionTypes() {
  const api = useApi();

  return useQuery({
    queryKey: ["session-types"],
    queryFn: () => api.listSessionTypes(),
    staleTime: 5 * 60_000,
  });
}

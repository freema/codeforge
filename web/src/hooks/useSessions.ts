import { useQuery } from "@tanstack/react-query";
import { useApi } from "./useApi";

export function useSessions() {
  const api = useApi();

  return useQuery({
    queryKey: ["sessions"],
    queryFn: () => api.listSessions(),
    refetchInterval: 15_000,
  });
}

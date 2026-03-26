import { useQuery } from "@tanstack/react-query";
import { useApi } from "./useApi";

export function useToolsCatalog() {
  const api = useApi();

  return useQuery({
    queryKey: ["tools-catalog"],
    queryFn: () => api.listToolsCatalog(),
    staleTime: 120_000,
  });
}

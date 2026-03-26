import { useQuery } from "@tanstack/react-query";
import { useApi } from "./useApi";

export function useCLIs() {
  const api = useApi();

  return useQuery({
    queryKey: ["clis"],
    queryFn: () => api.listCLIs(),
    staleTime: 60_000,
  });
}

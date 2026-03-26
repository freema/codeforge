import { useQuery } from "@tanstack/react-query";
import { useApi } from "./useApi";
import type { SessionStatus } from "../types";

const ACTIVE_STATUSES: SessionStatus[] = [
  "pending",
  "cloning",
  "running",
  "reviewing",
  "awaiting_instruction",
  "creating_pr",
  "cancelling",
];

export function useSession(id: string | undefined, include?: string) {
  const api = useApi();

  return useQuery({
    queryKey: ["session", id, include],
    queryFn: () => api.getSession(id!, include),
    enabled: !!id,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      if (status && ACTIVE_STATUSES.includes(status)) return 5000;
      return false;
    },
  });
}

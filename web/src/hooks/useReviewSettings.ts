import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi } from "./useApi";
import type { UpdateReviewSettingsRequest } from "../types";

export function useReviewSettings() {
  const api = useApi();

  return useQuery({
    queryKey: ["review-settings"],
    queryFn: () => api.getReviewSettings(),
  });
}

export function useUpdateReviewSettings() {
  const api = useApi();
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (req: UpdateReviewSettingsRequest) =>
      api.updateReviewSettings(req),
    // PUT returns the updated settings (incl. effective values) — seed the
    // cache directly instead of refetching.
    onSuccess: (data) => qc.setQueryData(["review-settings"], data),
  });
}

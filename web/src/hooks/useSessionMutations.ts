import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useApi } from "./useApi";
import type { CreateSessionRequest } from "../types";

export function useCreateSession() {
  const api = useApi();

  return useMutation({
    mutationFn: (req: CreateSessionRequest) => api.createSession(req),
  });
}

export function useCancelSession() {
  const api = useApi();
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => api.cancelSession(id),
    onSuccess: (_data, id) =>
      qc.invalidateQueries({ queryKey: ["session", id] }),
  });
}

export function useInstructSession() {
  const api = useApi();
  const qc = useQueryClient();

  return useMutation({
    mutationFn: ({ id, prompt }: { id: string; prompt: string }) =>
      api.instructSession(id, prompt),
    onSuccess: (_data, { id }) =>
      qc.invalidateQueries({ queryKey: ["session", id] }),
  });
}

export function useCreatePR() {
  const api = useApi();
  const qc = useQueryClient();

  return useMutation({
    mutationFn: ({
      id,
      ...req
    }: {
      id: string;
      title?: string;
      description?: string;
      target_branch?: string;
    }) => api.createPR(id, req),
    onSuccess: (_data, { id }) =>
      qc.invalidateQueries({ queryKey: ["session", id] }),
  });
}

export function usePushToPR() {
  const api = useApi();
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => api.pushToPR(id),
    onSuccess: (_data, id) =>
      qc.invalidateQueries({ queryKey: ["session", id] }),
  });
}

export function useReviewSession() {
  const api = useApi();
  const qc = useQueryClient();

  return useMutation({
    mutationFn: ({
      id,
      cli,
      model,
    }: {
      id: string;
      cli?: string;
      model?: string;
    }) => api.reviewSession(id, { cli, model }),
    onSuccess: (_data, { id }) =>
      qc.invalidateQueries({ queryKey: ["session", id] }),
  });
}

export function usePostReviewComments() {
  const api = useApi();
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => api.postReviewComments(id),
    onSuccess: (_data, id) =>
      qc.invalidateQueries({ queryKey: ["session", id] }),
  });
}

export function usePRStatus(sessionId: string | undefined, hasPR: boolean) {
  const api = useApi();
  return useQuery({
    queryKey: ["pr-status", sessionId],
    queryFn: () => api.getPRStatus(sessionId!),
    enabled: !!sessionId && hasPR,
    staleTime: 30_000,
    refetchInterval: 60_000,
  });
}

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi } from "./useApi";
import type { CreateScheduleRequest, UpdateScheduleRequest } from "../types";

export function useSchedules() {
  const api = useApi();

  return useQuery({
    queryKey: ["schedules"],
    queryFn: () => api.listSchedules(),
  });
}

export function useCreateSchedule() {
  const api = useApi();
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (req: CreateScheduleRequest) => api.createSchedule(req),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["schedules"] }),
  });
}

export function useUpdateSchedule() {
  const api = useApi();
  const qc = useQueryClient();

  return useMutation({
    mutationFn: ({ id, req }: { id: string; req: UpdateScheduleRequest }) =>
      api.updateSchedule(id, req),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["schedules"] }),
  });
}

export function useDeleteSchedule() {
  const api = useApi();
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => api.deleteSchedule(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["schedules"] }),
  });
}

export function useRunSchedule() {
  const api = useApi();
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => api.runSchedule(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["schedules"] }),
  });
}

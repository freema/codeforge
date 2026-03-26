import { useEffect, useRef, useState, useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import { useApi } from "./useApi";
import { connectToWorkflowRunStream } from "../lib/stream";
import { useAuth } from "../context/AuthContext";
import type { StreamEvent, RunStatus } from "../types";

const ACTIVE_STATUSES: RunStatus[] = ["pending", "running"];

export function useWorkflowRuns(workflowName?: string) {
  const api = useApi();

  return useQuery({
    queryKey: ["workflowRuns", workflowName],
    queryFn: () => api.listWorkflowRuns(workflowName),
    refetchInterval: 10_000,
  });
}

export function useWorkflowRun(runId: string | undefined) {
  const api = useApi();

  return useQuery({
    queryKey: ["workflowRun", runId],
    queryFn: () => api.getWorkflowRun(runId!),
    enabled: !!runId,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      if (status && ACTIVE_STATUSES.includes(status)) return 5000;
      return false;
    },
  });
}

interface WorkflowStreamState {
  events: StreamEvent[];
  connected: boolean;
  runStatus: RunStatus | null;
  error: string | null;
}

export function useWorkflowRunStream(runId: string | undefined) {
  const { serverUrl, token } = useAuth();
  const [state, setState] = useState<WorkflowStreamState>({
    events: [],
    connected: false,
    runStatus: null,
    error: null,
  });
  const controllerRef = useRef<AbortController | null>(null);

  const connect = useCallback(() => {
    if (!runId) return;

    controllerRef.current?.abort();

    const controller = connectToWorkflowRunStream(serverUrl, runId, token, {
      onConnected: (data) => {
        setState((prev) => ({
          ...prev,
          connected: true,
          runStatus: data.status,
          error: null,
        }));
      },
      onEvent: (event) => {
        setState((prev) => ({
          ...prev,
          events: [...prev.events, event],
        }));
      },
      onDone: (data) => {
        setState((prev) => ({
          ...prev,
          connected: false,
          runStatus: data.status,
        }));
      },
      onError: (err) => {
        setState((prev) => ({
          ...prev,
          connected: false,
          error: err.message,
        }));
      },
    });

    controllerRef.current = controller;
  }, [runId, serverUrl, token]);

  useEffect(() => {
    connect();
    return () => controllerRef.current?.abort();
  }, [connect]);

  const reconnect = useCallback(() => {
    setState((prev) => ({ ...prev, events: [], error: null }));
    connect();
  }, [connect]);

  return { ...state, reconnect };
}

import { useEffect, useRef, useState, useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { connectToSessionStream } from "../lib/stream";
import { useAuth } from "../context/AuthContext";
import type { StreamEvent, SessionStatus } from "../types";

interface StreamState {
  events: StreamEvent[];
  connected: boolean;
  sessionStatus: SessionStatus | null;
  error: string | null;
}

export function useSessionStream(sessionId: string | undefined) {
  const { serverUrl, token } = useAuth();
  const queryClient = useQueryClient();
  const [state, setState] = useState<StreamState>({
    events: [],
    connected: false,
    sessionStatus: null,
    error: null,
  });
  const controllerRef = useRef<AbortController | null>(null);

  const connect = useCallback(() => {
    if (!sessionId) return;

    controllerRef.current?.abort();

    const controller = connectToSessionStream(serverUrl, sessionId, token, {
      onConnected: (data) => {
        setState((prev) => ({
          ...prev,
          connected: true,
          sessionStatus: data.status,
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
          sessionStatus: data.status,
        }));
        // Invalidate session query so UI gets fresh data (review_result, status, etc.)
        void queryClient.invalidateQueries({
          queryKey: ["session", sessionId],
        });
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
  }, [sessionId, serverUrl, token, queryClient]);

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

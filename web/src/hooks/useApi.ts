import { useMemo } from "react";
import { createApiClient } from "../lib/api";
import { useAuth } from "../context/AuthContext";

export function useApi() {
  const { serverUrl, token } = useAuth();
  return useMemo(() => createApiClient(serverUrl, token), [serverUrl, token]);
}

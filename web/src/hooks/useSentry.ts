import { useQuery } from "@tanstack/react-query";
import { useApi } from "./useApi";
import { useRunWorkflow } from "./useWorkflowMutations";

export function useSentryOrganizations(keyName: string | undefined) {
  const api = useApi();

  return useQuery({
    queryKey: ["sentryOrganizations", keyName],
    queryFn: () => api.listSentryOrganizations(keyName!),
    enabled: !!keyName,
    staleTime: 60_000,
  });
}

export function useSentryProjects(
  keyName: string | undefined,
  org: string | undefined,
  region?: string,
) {
  const api = useApi();

  return useQuery({
    queryKey: ["sentryProjects", keyName, org, region],
    queryFn: () => api.listSentryProjects(keyName!, org!, region),
    enabled: !!keyName && !!org,
    staleTime: 60_000,
  });
}

export function useSentryIssues(
  keyName: string | undefined,
  org: string | undefined,
  project: string | undefined,
  opts?: { query?: string; sort?: string; region?: string },
) {
  const api = useApi();

  return useQuery({
    queryKey: [
      "sentryIssues",
      keyName,
      org,
      project,
      opts?.query,
      opts?.sort,
      opts?.region,
    ],
    queryFn: () =>
      api.listSentryIssues(keyName!, org!, project!, {
        query: opts?.query,
        sort: opts?.sort,
        limit: 50,
        region: opts?.region,
      }),
    enabled: !!keyName && !!org && !!project,
    refetchInterval: 30_000,
  });
}

export function useSentryIssue(
  keyName: string | undefined,
  issueId: string | undefined,
) {
  const api = useApi();

  return useQuery({
    queryKey: ["sentryIssue", keyName, issueId],
    queryFn: () => api.getSentryIssue(keyName!, issueId!),
    enabled: !!keyName && !!issueId,
  });
}

export function useSentryLatestEvent(
  keyName: string | undefined,
  issueId: string | undefined,
) {
  const api = useApi();

  return useQuery({
    queryKey: ["sentryLatestEvent", keyName, issueId],
    queryFn: () => api.getSentryLatestEvent(keyName!, issueId!),
    enabled: !!keyName && !!issueId,
  });
}

export function useFixSentryIssue() {
  return useRunWorkflow();
}

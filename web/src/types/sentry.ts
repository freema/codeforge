export interface SentryOrganization {
  id: string;
  slug: string;
  name: string;
  region: string;
}

export interface SentryIssue {
  id: string;
  shortId: string;
  title: string;
  culprit: string;
  count: string;
  userCount: number;
  firstSeen: string;
  lastSeen: string;
  level: "fatal" | "error" | "warning" | "info";
  status: "unresolved" | "resolved" | "ignored";
  platform: string;
  metadata: { type?: string; value?: string };
}

export interface SentryEvent {
  id: string;
  eventID: string;
  title: string;
  entries: SentryEventEntry[];
  tags: { key: string; value: string }[];
  user?: { id?: string; email?: string; ip_address?: string };
}

export interface SentryEventEntry {
  type: "exception" | "breadcrumbs" | "request" | "message";
  data: unknown;
}

export interface SentryProject {
  id: string;
  slug: string;
  name: string;
  organization: { id: string; slug: string; name: string };
  platform?: string;
}

export interface SentryConfig {
  label?: string;
  key_name: string;
  org_slug: string;
  project_slug: string;
  repo_url: string;
  provider_key?: string;
}

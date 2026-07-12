import { queryOptions, useQuery } from "@tanstack/react-query";

import { api } from "../api";
import type { AgentSource, AgentSourceSnapshot } from "../types";
import { workspaceKeys } from "../workspace/queries";

/**
 * AgentWaker source query keys. Namespaced under the workspace-scoped prefix so
 * invalidation composes with other workspace caches.
 */
export const agentSourceKeys = {
  all: (wsId: string) => ["workspaces", wsId, "agent-sources"] as const,
  list: (wsId: string) => [...agentSourceKeys.all(wsId), "list"] as const,
  detail: (wsId: string, id: string) =>
    [...agentSourceKeys.all(wsId), "detail", id] as const,
  snapshots: (wsId: string, id: string) =>
    [...agentSourceKeys.all(wsId), "snapshots", id] as const,
};

export function agentSourceListOptions(wsId: string) {
  return queryOptions({
    queryKey: agentSourceKeys.list(wsId),
    // The workspace is implied by the auth context; the key is workspace-scoped
    // so cache invalidation composes correctly.
    queryFn: () => api.listAgentSources(),
  });
}

export function agentSourceSnapshotsOptions(wsId: string, sourceId: string) {
  return queryOptions({
    queryKey: agentSourceKeys.snapshots(wsId, sourceId),
    queryFn: () => api.listAgentSourceSnapshots(sourceId),
    enabled: Boolean(sourceId),
  });
}

export function useAgentSources(wsId: string) {
  return useQuery(agentSourceListOptions(wsId));
}

export function useAgentSourceSnapshots(
  wsId: string,
  sourceId: string | undefined,
) {
  return useQuery({
    ...agentSourceSnapshotsOptions(wsId, sourceId ?? ""),
    enabled: Boolean(sourceId),
  });
}

export type { AgentSource, AgentSourceSnapshot };

/** Re-export so call sites can invalidate alongside the list. */
export { workspaceKeys };

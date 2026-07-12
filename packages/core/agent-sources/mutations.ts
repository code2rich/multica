import { useMutation, useQueryClient } from "@tanstack/react-query";

import { api } from "../api";
import type {
  AgentWakerScanRequest,
  CreateAgentSourceRequest,
  UpdateAgentSourceRequest,
} from "../types";
import { agentSourceKeys } from "./queries";

/**
 * Create a new AgentWaker source configuration. The source starts in `pending`
 * and moves to `scanning` when a scan is initiated.
 */
export function useCreateAgentSource(wsId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateAgentSourceRequest) =>
      api.createAgentSource(data),
    onSettled: () => {
      qc.invalidateQueries({ queryKey: agentSourceKeys.list(wsId) });
    },
  });
}

export function useUpdateAgentSource(wsId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      sourceId,
      data,
    }: {
      sourceId: string;
      data: UpdateAgentSourceRequest;
    }) => api.updateAgentSource(sourceId, data),
    onSettled: () => {
      qc.invalidateQueries({ queryKey: agentSourceKeys.list(wsId) });
    },
  });
}

export function useDeleteAgentSource(wsId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (sourceId: string) => api.deleteAgentSource(sourceId),
    onSettled: () => {
      qc.invalidateQueries({ queryKey: agentSourceKeys.list(wsId) });
    },
  });
}

/**
 * Initiate a read-only scan. Returns the in-flight request; the caller polls
 * `api.getAgentSourceScanRequest` until the status is terminal. The scan does
 * NOT mutate any agent/skill/capability/env state.
 */
export function useInitiateAgentSourceScan(wsId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (sourceId: string) => api.initiateAgentSourceScan(sourceId),
    onSettled: (_data, _err, sourceId) => {
      // The source flips to scanning/ready on the server; refresh both the
      // source list and this source's snapshot history.
      qc.invalidateQueries({ queryKey: agentSourceKeys.list(wsId) });
      qc.invalidateQueries({
        queryKey: agentSourceKeys.snapshots(wsId, sourceId),
      });
    },
  });
}

/**
 * Poll an in-flight scan until it reaches a terminal status. Terminal statuses
 * are completed / failed / timeout. Returns the latest request record.
 */
export async function pollAgentSourceScan(
  sourceId: string,
  requestId: string,
  opts: {
    intervalMs?: number;
    timeoutMs?: number;
    signal?: AbortSignal;
  } = {},
): Promise<AgentWakerScanRequest> {
  const intervalMs = opts.intervalMs ?? 1500;
  const timeoutMs = opts.timeoutMs ?? 5 * 60 * 1000;
  const start = Date.now();
  const terminal = new Set(["completed", "failed", "timeout"]);
  for (;;) {
    const req = await api.getAgentSourceScanRequest(sourceId, requestId);
    if (terminal.has(req.status)) return req;
    if (Date.now() - start > timeoutMs) {
      throw new Error("agent source scan timed out waiting for daemon");
    }
    await new Promise((resolve) => setTimeout(resolve, intervalMs));
    if (opts.signal?.aborted) throw new Error("aborted");
  }
}

"use client";

import { useState } from "react";
import {
  AlertTriangle,
  CheckCircle2,
  FolderTree,
  Loader2,
  Plus,
  RefreshCw,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@multica/ui/components/ui/button";
import { Card, CardContent } from "@multica/ui/components/ui/card";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { Badge } from "@multica/ui/components/ui/badge";
import {
  NativeSelect,
  NativeSelectOption,
} from "@multica/ui/components/ui/native-select";
import {
  useAgentSources,
  useAgentSourceSnapshots,
} from "@multica/core/agent-sources";
import {
  useCreateAgentSource,
  useDeleteAgentSource,
  useInitiateAgentSourceScan,
  useApplyAgentSource,
  agentSourceKeys,
  pollAgentSourceScan,
} from "@multica/core/agent-sources";
import { runtimeListOptions } from "@multica/core/runtimes";
import { useWorkspaceId } from "@multica/core/hooks";
import type { AgentSource } from "@multica/core/types";

import { useT } from "../../i18n";

type ApplyFeedback = {
  status: "applying" | "success" | "error";
  message: string;
};

/**
 * AgentWakerTab is the workspace-level configuration surface for AgentWaker
 * directory integration (M1): configure a daemon-owned absolute root, trigger a
 * read-only scan, and preview the sanitized result.
 *
 * No env values are ever shown here — previews carry key names, configured
 * booleans, and value digests only. The scan does not mutate any agent, skill,
 * capability, or env state (apply lands in M2).
 */
export function AgentWakerTab() {
  const wsId = useWorkspaceId();
  const { t } = useT("settings");
  const { data: sources = [], isLoading } = useAgentSources(wsId);
  const { data: runtimes = [] } = useQuery(runtimeListOptions(wsId));
  const queryClient = useQueryClient();

  const createSource = useCreateAgentSource(wsId);
  const deleteSource = useDeleteAgentSource(wsId);
  const initiateScan = useInitiateAgentSourceScan(wsId);
  const applySource = useApplyAgentSource(wsId);

  const [daemonRuntimeId, setDaemonRuntimeId] = useState("");
  const [localPath, setLocalPath] = useState("");
  const [syncMode, setSyncMode] = useState<"manual" | "scheduled" | "watch-assisted">("manual");
  const [scanningSourceIds, setScanningSourceIds] = useState<Set<string>>(
    () => new Set(),
  );
  const [applyFeedbackBySource, setApplyFeedbackBySource] = useState<
    Record<string, ApplyFeedback>
  >({});

  const onlineRuntimes = runtimes.filter((r) => r.status === "online");

  const handleCreate = async () => {
    if (!daemonRuntimeId || !localPath) {
      toast.error(t(($) => $.agentwaker.form_required));
      return;
    }
    try {
      await createSource.mutateAsync({
        daemon_runtime_id: daemonRuntimeId,
        local_path: localPath,
        sync_mode: syncMode,
      });
      toast.success(t(($) => $.agentwaker.create_success));
      setLocalPath("");
    } catch (err) {
      toast.error(humanizeError(err) ?? t(($) => $.agentwaker.create_failed));
    }
  };

  const handleScan = async (source: AgentSource) => {
    setApplyFeedbackBySource((current) => {
      const next = { ...current };
      delete next[source.id];
      return next;
    });
    setScanningSourceIds((current) => new Set(current).add(source.id));
    try {
      const initiated = await initiateScan.mutateAsync(source.id);
      toast.info(t(($) => $.agentwaker.scan_started));
      // Poll until terminal. The UI shows the sanitized snapshot once stored.
      const result = await pollAgentSourceScan(source.id, initiated.id);
      // The mutation's onSettled refresh happens when the scan is only queued.
      // Refresh again after the daemon reaches a terminal state so the newly
      // stored preview and final source status appear without a page reload.
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: agentSourceKeys.list(wsId) }),
        queryClient.invalidateQueries({
          queryKey: agentSourceKeys.snapshots(wsId, source.id),
        }),
      ]);
      if (result.status === "completed") {
        toast.success(t(($) => $.agentwaker.scan_complete));
      } else {
        toast.error(
          result.error
            ? `${t(($) => $.agentwaker.scan_failed)}: ${result.error}`
            : t(($) => $.agentwaker.scan_failed),
        );
      }
    } catch (err) {
      toast.error(humanizeError(err) ?? t(($) => $.agentwaker.scan_failed));
    } finally {
      setScanningSourceIds((current) => {
        const next = new Set(current);
        next.delete(source.id);
        return next;
      });
    }
  };

  const handleDelete = async (source: AgentSource) => {
    try {
      await deleteSource.mutateAsync(source.id);
      toast.success(t(($) => $.agentwaker.delete_success));
    } catch (err) {
      toast.error(humanizeError(err) ?? t(($) => $.agentwaker.delete_failed));
    }
  };

  const handleApply = async (source: AgentSource, snapshotId: string) => {
    setApplyFeedbackBySource((current) => ({
      ...current,
      [source.id]: {
        status: "applying",
        message: t(($) => $.agentwaker.status_applying),
      },
    }));
    try {
      await applySource.mutateAsync({
        sourceId: source.id,
        snapshotId,
      });
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: agentSourceKeys.list(wsId) }),
        queryClient.invalidateQueries({
          queryKey: agentSourceKeys.snapshots(wsId, source.id),
        }),
      ]);
      const message = t(($) => $.agentwaker.apply_success);
      setApplyFeedbackBySource((current) => ({
        ...current,
        [source.id]: { status: "success", message },
      }));
      toast.success(message);
    } catch (err) {
      const message =
        humanizeError(err) ?? t(($) => $.agentwaker.apply_failed);
      setApplyFeedbackBySource((current) => ({
        ...current,
        [source.id]: { status: "error", message },
      }));
      toast.error(message);
    }
  };

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold">{t(($) => $.agentwaker.section_title)}</h2>
        <p className="text-sm text-muted-foreground mt-1">
          {t(($) => $.agentwaker.section_description)}
        </p>
      </div>

      {/* Configure a new source */}
      <Card>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label>{t(($) => $.agentwaker.field_daemon)}</Label>
            <NativeSelect
              value={daemonRuntimeId}
              onChange={(e) => setDaemonRuntimeId(e.target.value)}
              disabled={onlineRuntimes.length === 0}
            >
              <NativeSelectOption value="">
                {onlineRuntimes.length === 0
                  ? t(($) => $.agentwaker.no_online_daemons)
                  : t(($) => $.agentwaker.select_daemon)}
              </NativeSelectOption>
              {onlineRuntimes.map((rt) => (
                <NativeSelectOption key={rt.id} value={rt.id}>
                  {rt.name ?? rt.id}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </div>

          <div className="space-y-2">
            <Label>{t(($) => $.agentwaker.field_path)}</Label>
            <Input
              value={localPath}
              onChange={(e) => setLocalPath(e.target.value)}
              placeholder="/absolute/path/to/agentwaker"
              spellCheck={false}
            />
            <p className="text-xs text-muted-foreground">
              {t(($) => $.agentwaker.field_path_hint)}
            </p>
          </div>

          <div className="space-y-2">
            <Label>{t(($) => $.agentwaker.field_sync_mode)}</Label>
            <NativeSelect
              value={syncMode}
              onChange={(e) =>
                setSyncMode(e.target.value as typeof syncMode)
              }
            >
              <NativeSelectOption value="manual">{t(($) => $.agentwaker.sync_manual)}</NativeSelectOption>
              <NativeSelectOption value="scheduled">{t(($) => $.agentwaker.sync_scheduled)}</NativeSelectOption>
              <NativeSelectOption value="watch-assisted">{t(($) => $.agentwaker.sync_watch)}</NativeSelectOption>
            </NativeSelect>
          </div>

          <Button onClick={handleCreate} disabled={createSource.isPending}>
            <Plus className="h-4 w-4" />
            {t(($) => $.agentwaker.add_source)}
          </Button>
        </CardContent>
      </Card>

      {/* Configured sources */}
      {isLoading ? (
        <p className="text-sm text-muted-foreground">{t(($) => $.agentwaker.loading)}</p>
      ) : sources.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t(($) => $.agentwaker.no_sources)}</p>
      ) : (
        <div className="space-y-3">
          {sources.map((source) => (
            <SourceRow
              key={source.id}
              source={source}
              wsId={wsId}
              onScan={() => handleScan(source)}
              onApply={(snapshotId) => handleApply(source, snapshotId)}
              onDelete={() => handleDelete(source)}
              scanning={scanningSourceIds.has(source.id)}
              applying={
                applySource.isPending &&
                applySource.variables?.sourceId === source.id
              }
              applyFeedback={applyFeedbackBySource[source.id]}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function SourceRow({
  source,
  wsId,
  onScan,
  onApply,
  onDelete,
  scanning,
  applying,
  applyFeedback,
}: {
  source: AgentSource;
  wsId: string;
  onScan: () => void;
  onApply: (snapshotId: string) => void;
  onDelete: () => void;
  scanning: boolean;
  applying: boolean;
  applyFeedback?: ApplyFeedback;
}) {
  const { t } = useT("settings");
  const { data: snapshots = [] } = useAgentSourceSnapshots(wsId, source.id);
  const latestPreview = snapshots.find((s) => s.status === "preview");

  return (
    <Card>
      <CardContent className="space-y-3">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0 space-y-1">
            <div className="flex items-center gap-2">
              <FolderTree className="h-4 w-4 shrink-0 text-muted-foreground" />
              <code className="text-sm truncate">{source.local_path}</code>
            </div>
            <div className="flex items-center gap-2 flex-wrap">
              <SourceStatusBadge status={source.status} />
              <Badge variant="outline">{source.sync_mode}</Badge>
              {source.last_scanned_at && (
                <span className="text-xs text-muted-foreground">
                  {t(($) => $.agentwaker.scanned_at)}{" "}
                  {new Date(source.last_scanned_at).toLocaleString()}
                </span>
              )}
              {source.last_applied_at && (
                <span className="text-xs text-muted-foreground">
                  {t(($) => $.agentwaker.applied_at)}{" "}
                  {new Date(source.last_applied_at).toLocaleString()}
                </span>
              )}
            </div>
          </div>
          <div className="flex items-center gap-2 shrink-0">
            {latestPreview && (
              <Button
                variant="default"
                size="sm"
                onClick={() => onApply(latestPreview.id)}
                disabled={applying || scanning || applyFeedback?.status === "applying"}
                aria-busy={applying || applyFeedback?.status === "applying"}
              >
                {applying || applyFeedback?.status === "applying" ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <CheckCircle2 className="h-4 w-4" />
                )}
                {applying || applyFeedback?.status === "applying"
                  ? t(($) => $.agentwaker.status_applying)
                  : t(($) => $.agentwaker.apply)}
              </Button>
            )}
            <Button variant="outline" size="sm" onClick={onScan} disabled={scanning || applying}>
              <RefreshCw className={scanning ? "animate-spin h-4 w-4" : "h-4 w-4"} />
              {t(($) => $.agentwaker.scan)}
            </Button>
            <Button variant="ghost" size="sm" onClick={onDelete} disabled={scanning || applying}>
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
        </div>

        {source.status === "failed" && (
          <div className="flex items-start gap-2 text-sm text-destructive">
            <AlertTriangle className="h-4 w-4 shrink-0 mt-0.5" />
            <span>{t(($) => $.agentwaker.scan_failed_hint)}</span>
          </div>
        )}

        {applyFeedback?.status !== "applying" && applyFeedback && (
          <div
            role={applyFeedback.status === "error" ? "alert" : "status"}
            aria-live="polite"
            className={
              applyFeedback.status === "success"
                ? "flex items-center gap-2 rounded-md border border-success/25 bg-success/10 px-3 py-2 text-sm text-success"
                : "flex items-start gap-2 rounded-md border border-destructive/25 bg-destructive/10 px-3 py-2 text-sm text-destructive"
            }
          >
            {applyFeedback.status === "success" ? (
              <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0" />
            ) : (
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
            )}
            <span>{applyFeedback.message}</span>
          </div>
        )}

        {latestPreview && <PreviewSummary snapshot={latestPreview} />}
      </CardContent>
    </Card>
  );
}

/**
 * PreviewSummary renders the sanitized scan counts: capabilities, roles, role
 * skills, env declarations, MCP servers, and any validation diagnostics. No env
 * values are ever shown — only key names and configured/digest metadata.
 */
function PreviewSummary({
  snapshot,
}: {
  snapshot: import("@multica/core/types").AgentSourceSnapshot;
}) {
  const { t } = useT("settings");
  const manifest = parseManifest(snapshot.manifest);
  const errors = snapshot.diagnostics.filter((d) => d.severity === "error");
  const warnings = snapshot.diagnostics.filter((d) => d.severity === "warning");

  const skillCount = manifest.roles.reduce((n, r) => n + r.skills.length, 0);
  const envDeclCount = manifest.roles.reduce((n, r) => n + r.env.length, 0);
  const mcpCount = manifest.roles.reduce(
    (n, r) => n + (r.mcp.has_servers ? r.mcp.server_count : 0),
    0,
  );
  const bindingCount = manifest.roles.reduce(
    (n, r) => n + r.capability_bindings.length,
    0,
  );

  return (
    <div className="border-t pt-3 space-y-2">
      <div className="flex items-center gap-2 flex-wrap">
        <CheckCircle2 className="h-4 w-4 text-muted-foreground" />
        <span className="text-xs font-medium text-muted-foreground">
          {t(($) => $.agentwaker.preview_title)}
        </span>
      </div>
      <div className="flex items-center gap-2 flex-wrap">
        <Badge variant="secondary">
          {t(($) => $.agentwaker.count_capabilities, { count: manifest.capabilities.length })}
        </Badge>
        <Badge variant="secondary">
          {t(($) => $.agentwaker.count_roles, { count: manifest.roles.length })}
        </Badge>
        <Badge variant="secondary">
          {t(($) => $.agentwaker.count_skills, { count: skillCount })}
        </Badge>
        <Badge variant="secondary">
          {t(($) => $.agentwaker.count_bindings, { count: bindingCount })}
        </Badge>
        <Badge variant="secondary">
          {t(($) => $.agentwaker.count_env, { count: envDeclCount })}
        </Badge>
        <Badge variant="secondary">
          {t(($) => $.agentwaker.count_mcp, { count: mcpCount })}
        </Badge>
      </div>
      {(errors.length > 0 || warnings.length > 0) && (
        <ul className="text-xs space-y-1">
          {errors.map((d, i) => (
            <li key={`e${i}`} className="text-destructive flex gap-1">
              <AlertTriangle className="h-3 w-3 shrink-0 mt-0.5" />
              <span>{d.path ? `${d.path}: ` : ""}{d.message}</span>
            </li>
          ))}
          {warnings.map((d, i) => (
            <li key={`w${i}`} className="text-muted-foreground flex gap-1">
              <AlertTriangle className="h-3 w-3 shrink-0 mt-0.5" />
              <span>{d.path ? `${d.path}: ` : ""}{d.message}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function SourceStatusBadge({ status }: { status: AgentSource["status"] }) {
  const { t } = useT("settings");
  const variant =
    status === "ready"
      ? "default"
      : status === "failed" || status === "partial"
        ? "destructive"
        : "outline";
  const labels: Record<AgentSource["status"], string> = {
    pending: t(($) => $.agentwaker.status_pending),
    scanning: t(($) => $.agentwaker.status_scanning),
    ready: t(($) => $.agentwaker.status_ready),
    applying: t(($) => $.agentwaker.status_applying),
    partial: t(($) => $.agentwaker.status_partial),
    failed: t(($) => $.agentwaker.status_failed),
    offline: t(($) => $.agentwaker.status_offline),
  };
  return (
    <Badge variant={variant as "default" | "destructive" | "outline"}>
      {labels[status]}
    </Badge>
  );
}

// --- helpers ---

/**
 * parseManifest defensively extracts the sanitized manifest into the typed
 * shape. The manifest is value-free and shape-stable, but individual fields may
 * evolve across scanner versions, so every access is optional-chained. Any
 * parse failure yields an empty manifest rather than throwing.
 */
function parseManifest(raw: unknown): {
  capabilities: { id: string }[];
  roles: {
    skills: { id: string }[];
    env: { name: string }[];
    capability_bindings: { id: string }[];
    mcp: { has_servers: boolean; server_count: number };
  }[];
} {
  if (!raw || typeof raw !== "object") return emptyManifest();
  const m = raw as Record<string, unknown>;
  const capabilities = Array.isArray(m.capabilities)
    ? m.capabilities.filter((c): c is { id: string } => isObjWith(c, "id"))
    : [];
  const roles = Array.isArray(m.roles)
    ? m.roles.map((r) => ({
        skills: Array.isArray((r as { skills?: unknown[] }).skills)
          ? ((r as { skills: unknown[] }).skills as unknown[]).filter(
              (s): s is { id: string } => isObjWith(s, "id"),
            )
          : [],
        env: Array.isArray((r as { env?: unknown[] }).env)
          ? ((r as { env: unknown[] }).env as unknown[]).filter(
              (e): e is { name: string } => isObjWith(e, "name"),
            )
          : [],
        capability_bindings: Array.isArray(
          (r as { capability_bindings?: unknown[] }).capability_bindings,
        )
          ? ((r as { capability_bindings: unknown[] }).capability_bindings as unknown[]).filter(
              (b): b is { id: string } => isObjWith(b, "id"),
            )
          : [],
        mcp: isObjWith(r, "mcp") && isObjWith((r as { mcp: unknown }).mcp, "has_servers")
          ? ((r as { mcp: { has_servers: boolean; server_count: number } }).mcp)
          : { has_servers: false, server_count: 0 },
      }))
    : [];
  return { capabilities, roles };
}

function emptyManifest() {
  return { capabilities: [], roles: [] };
}

function isObjWith<T extends string>(v: unknown, key: T): v is Record<T, unknown> {
  return typeof v === "object" && v !== null && key in v;
}

function humanizeError(err: unknown): string | null {
  if (err instanceof Error) return err.message;
  if (typeof err === "string") return err;
  return null;
}

"use client";

import { useMemo, useState } from "react";
import {
  ArrowUpRight,
  CircleHelp,
  Hash,
  History,
  MessageSquare,
  Workflow,
} from "lucide-react";
import { useQueries, useQuery } from "@tanstack/react-query";
import type {
  AgentTask,
  Issue,
  TaskFailureReason,
} from "@multica/core/types";
import { workspaceTaskRunsOptions } from "@multica/core/agents";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import { agentListOptions } from "@multica/core/workspace/queries";
import { issueDetailOptions } from "@multica/core/issues/queries";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@multica/ui/components/ui/tooltip";
import { AppLink } from "../../navigation";
import { ActorAvatar } from "../../common/actor-avatar";
import { BreadcrumbHeader } from "../../layout/breadcrumb-header";
import { TranscriptButton } from "../../common/task-transcript";
import { taskStatusConfig } from "../../agents/config";
import { failureReasonLabel } from "../../agents/components/tabs/task-failure";
import {
  activeTaskTimeText,
  formatDurationMs,
} from "../../agents/components/tabs/activity-tab";
import { useT, useTimeAgo } from "../../i18n";

const TERMINAL_STATUSES = new Set(["completed", "failed", "cancelled"]);
const STATUS_OPTIONS = [
  "completed",
  "failed",
  "cancelled",
  "running",
  "queued",
] as const;
const PAGE_SIZE = 50;

/**
 * Workspace-wide call-chain page (Part D) at `/[workspaceSlug]/runs`.
 *
 * Lists every agent task run across the workspace with one-click access to
 * each run's complete `task_message` call chain via TranscriptButton. This is
 * the "analyze the full invocation history" surface — distinct from the
 * per-agent Runs tab/page, which scopes to one agent.
 *
 * Status and agent filters narrow the list. The data comes from
 * `GET /api/workspace-task-runs` (server enforces private-agent gating), and
 * the agent names/avatars resolve from the shared workspace agent-list cache.
 */
export function WorkspaceRunsPage() {
  const { t } = useT("agents");
  const wsId = useWorkspaceId();
  const [statusFilter, setStatusFilter] = useState<string>("");
  const [page, setPage] = useState(0);

  const { data: tasks = [], isLoading } = useQuery(
    workspaceTaskRunsOptions(wsId, {
      status: statusFilter || undefined,
      limit: PAGE_SIZE,
      offset: page * PAGE_SIZE,
    }),
  );

  // Shared agent-list cache for names/avatars — zero extra fetch when the
  // Agents page has already warmed it.
  const { data: agents = [] } = useQuery(agentListOptions(wsId));
  const agentMap = useMemo(() => {
    const m = new Map<string, string>();
    for (const a of agents) m.set(a.id, a.name);
    return m;
  }, [agents]);

  // Resolve issue titles for rendered rows.
  const issueIds = useMemo(
    () =>
      Array.from(
        new Set(tasks.map((t) => t.issue_id).filter((id) => id !== "")),
      ),
    [tasks],
  );
  const issueQueries = useQueries({
    queries: issueIds.map((id) => issueDetailOptions(wsId, id)),
  });
  const issueMap = useMemo(() => {
    const m = new Map<string, Issue>();
    issueQueries.forEach((q, i) => {
      const id = issueIds[i]!;
      if (q.data) m.set(id, q.data);
    });
    return m;
  }, [issueQueries, issueIds]);

  return (
    <div className="flex flex-1 min-h-0 flex-col">
      <BreadcrumbHeader
        segments={[]}
        leaf={
          <div className="flex items-center gap-1.5">
            <History className="h-3.5 w-3.5 text-muted-foreground" />
            <h1 className="min-w-0 truncate text-sm font-medium text-foreground">
              {t(($) => $.tab_body.runs.page_title)}
            </h1>
          </div>
        }
      />

      <div className="flex-1 min-h-0 overflow-y-auto p-3 md:p-6">
        {/* Status filter row. */}
        <div className="mb-4 flex flex-wrap items-center gap-1.5">
          <FilterChip
            label={t(($) => $.tab_body.runs.filter_all)}
            active={statusFilter === ""}
            onClick={() => {
              setStatusFilter("");
              setPage(0);
            }}
          />
          {STATUS_OPTIONS.map((s) => {
            const cfg = taskStatusConfig[s];
            if (!cfg) return null;
            return (
              <FilterChip
                key={s}
                label={s}
                active={statusFilter === s}
                onClick={() => {
                  setStatusFilter(s);
                  setPage(0);
                }}
              />
            );
          })}
        </div>

        {isLoading ? (
          <div className="space-y-2">
            {Array.from({ length: 6 }).map((_, i) => (
              <Skeleton key={i} className="h-16 w-full" />
            ))}
          </div>
        ) : tasks.length === 0 ? (
          <p className="py-16 text-center text-sm text-muted-foreground">
            {t(($) => $.tab_body.runs.empty)}
          </p>
        ) : (
          <div className="space-y-1.5">
            {tasks.map((task) => (
              <GlobalRunRow
                key={task.id}
                task={task}
                agentName={agentMap.get(task.agent_id) ?? ""}
                issueMap={issueMap}
              />
            ))}
          </div>
        )}

        {/* Paging controls. */}
        {!isLoading && tasks.length > 0 && (
          <div className="mt-4 flex items-center justify-center gap-3 text-xs text-muted-foreground">
            <button
              type="button"
              disabled={page === 0}
              onClick={() => setPage((p) => Math.max(0, p - 1))}
              className="rounded px-2 py-1 transition-colors hover:text-foreground disabled:opacity-40"
            >
              ←
            </button>
            <span>{page * PAGE_SIZE + 1}–{page * PAGE_SIZE + tasks.length}</span>
            <button
              type="button"
              disabled={tasks.length < PAGE_SIZE}
              onClick={() => setPage((p) => p + 1)}
              className="rounded px-2 py-1 transition-colors hover:text-foreground disabled:opacity-40"
            >
              →
            </button>
          </div>
        )}
      </div>
    </div>
  );
}

function FilterChip({
  label,
  active,
  onClick,
}: {
  label: string;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`rounded-full border px-2.5 py-1 text-xs transition-colors ${
        active
          ? "border-foreground/30 bg-accent text-accent-foreground"
          : "border-transparent text-muted-foreground hover:bg-accent/50 hover:text-foreground"
      }`}
    >
      {label}
    </button>
  );
}

function Sep() {
  return <span className="mx-1 text-muted-foreground/40">·</span>;
}

function GlobalRunRow({
  task,
  agentName,
  issueMap,
}: {
  task: AgentTask;
  agentName: string;
  issueMap: Map<string, Issue>;
}) {
  const { t } = useT("agents");
  const timeAgo = useTimeAgo();
  const paths = useWorkspacePaths();
  const cfg = taskStatusConfig[task.status] ?? taskStatusConfig.queued!;
  const Icon = cfg.icon;
  const hasIssue = task.issue_id !== "";
  const issue = hasIssue ? issueMap.get(task.issue_id) : undefined;
  const isRunning = task.status === "running";
  const isTerminal = TERMINAL_STATUSES.has(task.status);
  const showTranscript = task.status !== "queued";

  const sourceFallback = !hasIssue
    ? task.kind === "quick_create"
      ? isTerminal
        ? t(($) => $.tab_body.activity.source_quick_create)
        : t(($) => $.tab_body.activity.source_creating_issue)
      : task.chat_session_id
        ? t(($) => $.tab_body.activity.source_chat_session)
        : task.autopilot_run_id
          ? t(($) => $.tab_body.activity.source_autopilot_run)
          : t(($) => $.tab_body.activity.source_untracked)
    : null;

  const SourceIcon = hasIssue
    ? Hash
    : task.chat_session_id
      ? MessageSquare
      : task.autopilot_run_id
        ? Workflow
        : CircleHelp;
  const sourceLabel = hasIssue
    ? t(($) => $.tab_body.activity.source_issue)
    : task.chat_session_id
      ? t(($) => $.tab_body.activity.source_chat)
      : task.autopilot_run_id
        ? t(($) => $.tab_body.activity.source_autopilot)
        : t(($) => $.tab_body.activity.source_untracked);

  const timeText = isTerminal
    ? task.completed_at
      ? timeAgo(task.completed_at)
      : "—"
    : activeTaskTimeText(task, t, timeAgo);

  const failureLabel =
    task.status === "failed" && task.failure_reason
      ? failureReasonLabel[task.failure_reason as TaskFailureReason]
      : null;

  let durationText: string | null = null;
  if (isTerminal && task.started_at && task.completed_at) {
    const dur =
      new Date(task.completed_at).getTime() -
      new Date(task.started_at).getTime();
    if (dur > 0) durationText = formatDurationMs(dur);
  }

  const rowClass = `group flex items-center gap-3 rounded-md border px-3 py-2.5 ${
    isRunning ? "border-brand/40 bg-brand/5" : ""
  }`;

  return (
    <div className={rowClass}>
      <ActorAvatar actorType="agent" actorId={task.agent_id} size="sm" />
      <Icon
        className={`h-4 w-4 shrink-0 ${cfg.color} ${
          isRunning ? "animate-spin" : ""
        }`}
      />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-1.5">
          <span className="shrink-0 truncate text-sm font-medium">
            {agentName || "—"}
          </span>
          <SourceIcon
            className="h-3 w-3 shrink-0 text-muted-foreground/70"
            aria-label={sourceLabel}
          />
          {issue && (
            <span className="shrink-0 font-mono text-xs text-muted-foreground">
              {issue.identifier}
            </span>
          )}
          {task.trigger_summary ? (
            <Tooltip>
              <TooltipTrigger
                render={
                  <span className="truncate text-sm text-muted-foreground">
                    {issue?.title ??
                      (hasIssue
                        ? t(($) => $.tab_body.activity.issue_short_fallback, { prefix: task.issue_id.slice(0, 8) })
                        : (sourceFallback ?? t(($) => $.tab_body.activity.source_untracked)))}
                  </span>
                }
              />
              <TooltipContent className="max-w-md">
                <div className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground/80">
                  {t(($) => $.tab_body.activity.triggered_by)}
                </div>
                <div className="mt-0.5 whitespace-pre-wrap text-xs">
                  {task.trigger_summary}
                </div>
              </TooltipContent>
            </Tooltip>
          ) : (
            <span className="truncate text-sm text-muted-foreground">
              {issue?.title ??
                (hasIssue
                  ? t(($) => $.tab_body.activity.issue_short_fallback, { prefix: task.issue_id.slice(0, 8) })
                  : (sourceFallback ?? t(($) => $.tab_body.activity.source_untracked)))}
            </span>
          )}
        </div>
        <div className="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-muted-foreground">
          <span>{timeText}</span>
          {durationText && (
            <>
              <Sep />
              <span>{durationText}</span>
            </>
          )}
          {failureLabel && (
            <>
              <Sep />
              <span className="text-destructive">{failureLabel}</span>
            </>
          )}
        </div>
      </div>

      <div className="ml-2 flex shrink-0 items-center gap-0.5 opacity-0 transition-opacity duration-100 group-hover:opacity-100 group-focus-within:opacity-100">
        {hasIssue && (
          <Tooltip>
            <TooltipTrigger
              render={<AppLink href={paths.issueDetail(task.issue_id)} />}
              aria-label={t(($) => $.tab_body.activity.open_issue_aria)}
              className="flex items-center justify-center rounded p-1 text-muted-foreground hover:text-foreground hover:bg-accent/50 transition-colors"
            >
              <ArrowUpRight className="h-3.5 w-3.5" />
            </TooltipTrigger>
            <TooltipContent>{t(($) => $.tab_body.activity.open_issue_tooltip)}</TooltipContent>
          </Tooltip>
        )}
        {showTranscript && (
          <TranscriptButton
            task={task}
            agentName={agentName}
            isLive={isRunning}
            title={t(($) => $.tab_body.runs.transcript_tooltip)}
          />
        )}
      </div>
    </div>
  );
}

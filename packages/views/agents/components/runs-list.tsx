"use client";

import { useMemo, useState } from "react";
import {
  ArrowUpRight,
  CircleHelp,
  Hash,
  MessageSquare,
  Workflow,
} from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@multica/ui/components/ui/tooltip";
import { useQueries } from "@tanstack/react-query";
import type {
  Agent,
  AgentTask,
  Issue,
  TaskFailureReason,
} from "@multica/core/types";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import { issueDetailOptions } from "@multica/core/issues/queries";
import { AppLink } from "../../navigation";
import { TranscriptButton } from "../../common/task-transcript";
import { taskStatusConfig } from "../config";
import { failureReasonLabel } from "./tabs/task-failure";
import {
  activeTaskTimeText,
  formatDurationMs,
} from "./tabs/activity-tab";
import { useT, useTimeAgo } from "../../i18n";

// Run history pagination: larger cohorts than Activity's "recent work" since
// this surface exists specifically to browse / analyze past runs. Tasks are
// already fully cached client-side (one listAgentTasks for the whole agent),
// so "Show more" is a pure state flip — zero extra fetches.
export const RUNS_INITIAL = 20;
export const RUNS_PAGE = 50;

const TERMINAL_STATUSES = new Set(["completed", "failed", "cancelled"]);
const ACTIVE_STATUSES = new Set(["running", "queued", "dispatched"]);

interface RunsListProps {
  agent: Agent;
  /**
   * All tasks for this agent. The list filters to terminal + active, sorts
   * terminal by completed_at descending (active first, then recent), and
   * paginates client-side.
   */
  tasks: AgentTask[];
  /** Optional pre-filter by status — e.g. from a query param on the runs page. */
  statusFilter?: string | null;
}

/**
 * Shared run-history list used by both the agent detail "Runs" tab (C1) and
 * the deep-linkable `/agents/[id]/runs` page (C2).
 *
 * Unlike Activity's "Recent work", this surface intentionally INCLUDES chat
 * tasks (`chat_session_id` set): the purpose here is "see every call this
 * agent made and its full invocation chain", not the Activity tab's "what is
 * this agent doing for the team". TranscriptButton (lazy fetch) backs every
 * row so the full `task_message` call chain is one click away regardless of
 * task type or terminal state.
 */
export function RunsList({ agent, tasks, statusFilter }: RunsListProps) {
  const { t } = useT("agents");
  const wsId = useWorkspaceId();
  const [displayLimit, setDisplayLimit] = useState(RUNS_INITIAL);

  // All tasks (terminal + active), optionally narrowed by a status filter.
  // Terminal sorted by completed_at desc; active sorted to the top.
  const allRuns = useMemo(() => {
    const filtered = statusFilter
      ? tasks.filter((t) => t.status === statusFilter)
      : tasks.filter(
          (t) => TERMINAL_STATUSES.has(t.status) || ACTIVE_STATUSES.has(t.status),
        );
    return [...filtered].sort((a, b) => {
      const aActive = ACTIVE_STATUSES.has(a.status) ? 1 : 0;
      const bActive = ACTIVE_STATUSES.has(b.status) ? 1 : 0;
      if (aActive !== bActive) return bActive - aActive;
      const aKey = new Date(a.completed_at ?? a.created_at).getTime();
      const bKey = new Date(b.completed_at ?? b.created_at).getTime();
      return bKey - aKey;
    });
  }, [tasks, statusFilter]);

  const runs = useMemo(
    () => allRuns.slice(0, displayLimit),
    [allRuns, displayLimit],
  );
  const hasMore = allRuns.length > runs.length;

  // Resolve issue identifiers + titles for any task we'll render, sharing the
  // same issueDetailOptions cache the rest of the app uses.
  const issueIds = useMemo(
    () =>
      Array.from(
        new Set(runs.map((t) => t.issue_id).filter((id) => id !== "")),
      ),
    [runs],
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

  const subtitle =
    allRuns.length === 0
      ? t(($) => $.tab_body.runs.subtitle_empty)
      : hasMore
        ? t(($) => $.tab_body.runs.subtitle_progress, {
            shown: runs.length,
            total: allRuns.length,
          })
        : t(($) => $.tab_body.runs.subtitle_latest, { count: allRuns.length });

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-baseline gap-2">
        <span className="text-[11px] text-muted-foreground/70">{subtitle}</span>
      </div>
      {allRuns.length === 0 ? (
        <p className="text-xs italic text-muted-foreground/60">
          {t(($) => $.tab_body.runs.empty)}
        </p>
      ) : (
        <>
          <div className="space-y-1.5">
            {runs.map((task) => (
              <RunRow
                key={task.id}
                task={task}
                issueMap={issueMap}
                agent={agent}
              />
            ))}
          </div>
          {hasMore && (
            <button
              type="button"
              onClick={() => setDisplayLimit((n) => n + RUNS_PAGE)}
              className="mt-2 self-start rounded text-xs text-muted-foreground transition-colors hover:text-foreground"
            >
              {t(($) => $.tab_body.runs.show_more)}
            </button>
          )}
        </>
      )}
    </div>
  );
}

function Sep() {
  return <span className="mx-1 text-muted-foreground/40">·</span>;
}

function RunRow({
  task,
  issueMap,
  agent,
}: {
  task: AgentTask;
  issueMap: Map<string, Issue>;
  agent: Agent;
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
  // Queued tasks have no messages yet — hiding the transcript button avoids a
  // guaranteed "No execution data recorded." dialog open.
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

  // Duration only for terminal rows.
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
      <Icon
        className={`h-4 w-4 shrink-0 ${cfg.color} ${
          isRunning ? "animate-spin" : ""
        }`}
      />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-1.5">
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
                  <span className="truncate text-sm">
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
            <span className="truncate text-sm">
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
            agentName={agent.name}
            isLive={isRunning}
            title={t(($) => $.tab_body.runs.transcript_tooltip)}
          />
        )}
      </div>
    </div>
  );
}

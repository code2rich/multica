"use client";

import { ArrowUpRight } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import type { Agent } from "@multica/core/types";
import { agentTasksOptions } from "@multica/core/agents";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import { AppLink } from "../../../navigation";
import { RunsList } from "../runs-list";
import { useT } from "../../../i18n";

interface RunsTabProps {
  agent: Agent;
}

/**
 * Agent detail "Runs" tab (C1) — a dedicated surface for browsing the agent's
 * full run history with one-click access to each run's complete call chain
 * (task_message transcript).
 *
 * Distinct from the Activity tab's "Recent work" in two ways:
 *  - Includes chat tasks (Activity deliberately hides them). This tab exists
 *    to answer "show me every call this agent made", chat included.
 *  - Larger default cohort + show-more paging, suited to browsing/analysis
 *    rather than the Activity tab's quick "what just happened" scan.
 *
 * Shares the same `agentTasksOptions` cache as Activity, so opening this tab
 * adds zero extra fetches once the detail page is hydrated. WS `task:` events
 * invalidate that cache in useRealtimeSync, keeping the list live.
 */
export function RunsTab({ agent }: RunsTabProps) {
  const { t } = useT("agents");
  const wsId = useWorkspaceId();
  const paths = useWorkspacePaths();
  const { data: agentTasks = [] } = useQuery(agentTasksOptions(wsId, agent.id));

  return (
    <div className="flex flex-col gap-3 p-6">
      <div className="flex items-center justify-between gap-2">
        <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          {t(($) => $.tab_body.runs.section_title)}
        </h3>
        <AppLink
          href={paths.agentRuns(agent.id)}
          className="flex items-center gap-1 text-xs text-muted-foreground transition-colors hover:text-foreground"
        >
          {t(($) => $.tab_body.runs.open_full_page)}
          <ArrowUpRight className="h-3 w-3" />
        </AppLink>
      </div>
      <RunsList agent={agent} tasks={agentTasks} />
    </div>
  );
}

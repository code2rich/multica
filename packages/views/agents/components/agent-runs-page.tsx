"use client";

import { ArrowLeft, AlertCircle, Lock } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { api, ApiError } from "@multica/core/api";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import { agentListOptions } from "@multica/core/workspace/queries";
import { agentTasksOptions } from "@multica/core/agents";
import { Button } from "@multica/ui/components/ui/button";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import { AppLink } from "../../navigation";
import { BreadcrumbHeader } from "../../layout/breadcrumb-header";
import { RunsList } from "./runs-list";
import { useT } from "../../i18n";

interface AgentRunsPageProps {
  agentId: string;
}

/**
 * Deep-linkable per-agent run history page (C2) at
 * `/[workspaceSlug]/agents/[id]/runs`.
 *
 * The first three-level route in the dashboard, but it follows the same shape
 * as every other detail route: a thin web `page.tsx` forwards the id here, the
 * workspace slug is supplied by the layout's provider. Backed by the same
 * `GET /api/agents/{id}/tasks` endpoint (with private-agent gating already
 * enforced server-side) and the same `agentTasksOptions` cache the Runs tab
 * and Activity tab read from.
 *
 * Shares `RunsList` with the Runs tab — the page just adds a breadcrumb
 * header (with a back-to-detail link) and reads an optional `?status=`
 * query so a filtered view can be shared via URL.
 */
export function AgentRunsPage({ agentId }: AgentRunsPageProps) {
  const { t } = useT("agents");
  const wsId = useWorkspaceId();
  const paths = useWorkspacePaths();

  const {
    data: agents = [],
    isLoading: agentsLoading,
  } = useQuery(agentListOptions(wsId));
  const agent = agents.find((a) => a.id === agentId) ?? null;

  // Disambiguate "doesn't exist" (404) from "private agent you can't see"
  // (403) when the agent is missing from the workspace list. Fires only after
  // the list settles, so the common path makes zero extra requests.
  const { error: detailError } = useQuery({
    queryKey: ["agent-detail-probe", wsId, agentId],
    queryFn: () => api.getAgent(agentId),
    enabled: !agentsLoading && !agent && !!agentId,
    retry: false,
  });
  const isForbidden =
    detailError instanceof ApiError && detailError.status === 403;
  const isNotFound = !agentsLoading && !agent && !isForbidden;

  const { data: agentTasks = [] } = useQuery(
    agentTasksOptions(wsId, agentId),
  );

  // Header + body vary by resolved state.
  return (
    <div className="flex flex-1 min-h-0 flex-col">
      <BreadcrumbHeader
        segments={[
          { href: paths.agents(), label: t(($) => $.page.title) },
          ...(agent
            ? [{ href: paths.agentDetail(agent.id), label: agent.name }]
            : []),
        ]}
        leaf={
          <h1 className="min-w-0 truncate text-sm font-medium text-foreground">
            {t(($) => $.tab_body.runs.page_title)}
          </h1>
        }
        actions={
          <Button
            variant="ghost"
            size="sm"
            render={
              <AppLink
                href={agent ? paths.agentDetail(agent.id) : paths.agents()}
              />
            }
          >
            <ArrowLeft className="h-3.5 w-3.5" />
            {t(($) => $.detail.back_to_agents)}
          </Button>
        }
      />

      <div className="flex-1 min-h-0 overflow-y-auto p-3 md:p-6">
        {agentsLoading && !agent ? (
          <div className="space-y-2">
            <Skeleton className="h-8 w-48" />
            <Skeleton className="h-16 w-full" />
            <Skeleton className="h-16 w-full" />
            <Skeleton className="h-16 w-full" />
          </div>
        ) : isForbidden ? (
          <div className="flex flex-col items-center justify-center gap-2 py-16 text-center text-sm text-muted-foreground">
            <Lock className="h-5 w-5" />
            <p>{t(($) => $.detail.no_access_title)}</p>
          </div>
        ) : isNotFound ? (
          <div className="flex flex-col items-center justify-center gap-2 py-16 text-center text-sm text-muted-foreground">
            <AlertCircle className="h-5 w-5" />
            <p>{t(($) => $.detail.not_found_default)}</p>
          </div>
        ) : agent ? (
          <RunsList agent={agent} tasks={agentTasks} />
        ) : null}
      </div>
    </div>
  );
}

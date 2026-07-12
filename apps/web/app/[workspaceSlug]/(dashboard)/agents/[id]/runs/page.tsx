"use client";

import { use } from "react";
import { AgentRunsPage } from "@multica/views/agents";

export default function AgentRunsRoute({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = use(params);
  return <AgentRunsPage agentId={id} />;
}

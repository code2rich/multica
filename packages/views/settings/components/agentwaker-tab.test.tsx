// @vitest-environment jsdom

import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "@multica/core/i18n/react";
import enCommon from "../../locales/en/common.json";
import enSettings from "../../locales/en/settings.json";
import { AgentWakerTab } from "./agentwaker-tab";

const mockApply = vi.hoisted(() => vi.fn());
const mockInvalidate = vi.hoisted(() => vi.fn().mockResolvedValue(undefined));
const mockToastSuccess = vi.hoisted(() => vi.fn());
const mockToastError = vi.hoisted(() => vi.fn());

vi.mock("sonner", () => ({
  toast: {
    error: mockToastError,
    info: vi.fn(),
    success: mockToastSuccess,
  },
}));

vi.mock("@tanstack/react-query", () => ({
  useQuery: () => ({ data: [], isLoading: false }),
  useQueryClient: () => ({ invalidateQueries: mockInvalidate }),
}));

vi.mock("@multica/core/hooks", () => ({
  useWorkspaceId: () => "workspace-1",
}));

vi.mock("@multica/core/runtimes", () => ({
  runtimeListOptions: () => ({ queryKey: ["runtimes"] }),
}));

vi.mock("@multica/core/agent-sources", () => ({
  agentSourceKeys: {
    list: (wsId: string) => ["workspaces", wsId, "agent-sources", "list"],
    snapshots: (wsId: string, sourceId: string) => [
      "workspaces",
      wsId,
      "agent-sources",
      "snapshots",
      sourceId,
    ],
  },
  pollAgentSourceScan: vi.fn(),
  useAgentSources: () => ({
    data: [
      {
        id: "source-1",
        workspace_id: "workspace-1",
        kind: "agentwaker_directory",
        daemon_runtime_id: "runtime-1",
        local_path: "/workspace/agentwaker",
        sync_mode: "manual",
        status: "ready",
        last_scanned_at: "2026-07-15T14:57:27Z",
        created_at: "2026-07-15T14:00:00Z",
        updated_at: "2026-07-15T14:57:27Z",
      },
    ],
    isLoading: false,
  }),
  useAgentSourceSnapshots: () => ({
    data: [
      {
        id: "snapshot-1",
        source_id: "source-1",
        directory_hash: "hash",
        schema_versions: {},
        manifest: { capabilities: [], roles: [] },
        status: "preview",
        diagnostics: [],
        created_at: "2026-07-15T14:57:27Z",
      },
    ],
  }),
  useApplyAgentSource: () => ({
    mutateAsync: mockApply,
    isPending: false,
    variables: undefined,
  }),
  useCreateAgentSource: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useDeleteAgentSource: () => ({ mutateAsync: vi.fn() }),
  useInitiateAgentSourceScan: () => ({ mutateAsync: vi.fn() }),
}));

function renderTab() {
  return render(
    <I18nProvider
      locale="en"
      resources={{ en: { common: enCommon, settings: enSettings } }}
    >
      <AgentWakerTab />
    </I18nProvider>,
  );
}

describe("AgentWakerTab apply feedback", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockInvalidate.mockResolvedValue(undefined);
  });

  it("shows an immediate applying state and a persistent success confirmation", async () => {
    let resolveApply: (() => void) | undefined;
    mockApply.mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          resolveApply = resolve;
        }),
    );
    const user = userEvent.setup();
    renderTab();

    await user.click(screen.getByRole("button", { name: "Apply" }));

    const applyingButton = screen.getByRole("button", { name: "Applying" });
    expect(applyingButton).toBeDisabled();
    expect(applyingButton).toHaveAttribute("aria-busy", "true");

    await act(async () => resolveApply?.());

    expect(
      await screen.findByRole("status", {
        name: "",
      }),
    ).toHaveTextContent("Snapshot applied successfully.");
    expect(mockToastSuccess).toHaveBeenCalledWith(
      "Snapshot applied successfully.",
    );
  });

  it("shows the returned error inline when apply fails", async () => {
    mockApply.mockRejectedValue(new Error("permission denied"));
    const user = userEvent.setup();
    renderTab();

    await user.click(screen.getByRole("button", { name: "Apply" }));

    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent("permission denied"),
    );
    expect(mockToastError).toHaveBeenCalledWith("permission denied");
  });
});

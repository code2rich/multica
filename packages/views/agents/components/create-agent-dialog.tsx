"use client";

import { useEffect, useRef, useState } from "react";
import {
  AlertCircle,
  ChevronRight,
  Folder,
  FolderOpen,
  Globe,
  Lock,
  Pencil,
  Plus,
} from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { ModelDropdown } from "./model-dropdown";
import { RuntimePicker, isRuntimeUsableForUser } from "./runtime-picker";
import { InstructionsEditor } from "./instructions-editor";
import { SkillMultiSelect } from "./skill-multi-select";
import { AvatarUploadControl } from "../../common/avatar-upload-control";
import { api } from "@multica/core/api";
import { useWorkspaceId } from "@multica/core/hooks";
import { useFeatureEnabled } from "@multica/core/config";
import { COMPOSIO_MCP_APPS_FLAG } from "@multica/core/feature-flags";
import {
  skillDetailOptions,
  workspaceKeys,
} from "@multica/core/workspace/queries";
import type {
  Agent,
  AgentInvocationTargetInput,
  AgentPermissionMode,
  AgentVisibility,
  RuntimeDevice,
  MemberWithUser,
  CreateAgentRequest,
  UpdateAgentRequest,
} from "@multica/core/types";
import { isImeComposing } from "@multica/core/utils";
import {
  buildAgentIconUrl,
  defaultAgentIconKey,
} from "@multica/ui/lib/agent-icon-url";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@multica/ui/components/ui/dialog";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@multica/ui/components/ui/tooltip";
import { Button } from "@multica/ui/components/ui/button";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { toast } from "sonner";
import {
  AGENT_DESCRIPTION_MAX_LENGTH,
  VISIBILITY_DESCRIPTION,
  VISIBILITY_LABEL,
} from "@multica/core/agents";
import { ActorAvatar } from "../../common/actor-avatar";
import { CharCounter } from "./char-counter";
import { useT } from "../../i18n";
import {
  readAgentDirectory,
  type AgentDirectoryImportResult,
} from "../lib/agent-import";

type Method = "chooser" | "manual" | "import";

// Shared props for the two creation forms — both need runtimes / members and
// route back through the same onCreate callback (which does cache + navigate).
interface FormProps {
  runtimes: RuntimeDevice[];
  runtimesLoading?: boolean;
  members: MemberWithUser[];
  currentUserId: string | null;
  onClose: () => void;
  // Returns the created Agent so the form can run follow-up steps (skills,
  // squad join). The caller handles cache + navigation.
  onCreate: (data: CreateAgentRequest) => Promise<Agent | void>;
}

export function CreateAgentDialog({
  runtimes,
  runtimesLoading,
  members,
  currentUserId,
  template,
  squadId,
  onClose,
  onCreate,
  onImported,
}: {
  runtimes: RuntimeDevice[];
  runtimesLoading?: boolean;
  members: MemberWithUser[];
  currentUserId: string | null;
  // When provided, the dialog opens in "Duplicate" mode: the visible
  // fields (name / description / runtime / visibility / model) are
  // pre-populated from this agent, and the hidden fields
  // (instructions / custom_args / custom_env / max_concurrent_tasks)
  // are forwarded to the create call so the new agent is a true clone.
  // Skills are copied separately by the caller after createAgent
  // succeeds — they're not part of CreateAgentRequest.
  template?: Agent | null;
  // When set, every successful create is followed by
  // addSquadMember(squadId, agent) so the new agent joins this squad.
  // If the squad-join call fails the agent still exists and the dialog
  // surfaces a warning toast — the user can add it manually from the
  // Members tab.
  squadId?: string;
  onClose: () => void;
  // Returns the created Agent so the dialog can run a follow-up
  // setAgentSkills with the IDs the user picked in the form. Pre-skill-
  // section callers can keep returning `void`; the dialog tolerates a
  // falsy return (no follow-up runs).
  onCreate: (data: CreateAgentRequest) => Promise<Agent | void>;
  // Called after a directory import finishes (agent created OR overwritten).
  // The caller handles cache invalidation + navigation to the agent detail
  // page. Separate from onCreate because the overwrite path uses updateAgent
  // + updateAgentEnv instead of createAgent, so it can't flow through the
  // same callback.
  onImported?: (agent: Agent) => void;
}) {
  const { t } = useT("agents");
  const isDuplicate = !!template;
  // Duplicate and squad-context opens skip the chooser — they're always the
  // manual form with pre-filled fields, so the clone / squad-join flow stays
  // unchanged.
  const [method, setMethod] = useState<Method>(
    isDuplicate || squadId ? "manual" : "chooser",
  );

  return (
    <Dialog open onOpenChange={(v) => { if (!v) onClose(); }}>
      <DialogContent className="p-0 gap-0 flex flex-col overflow-hidden !top-1/2 !left-1/2 !-translate-x-1/2 !-translate-y-1/2 !w-full !max-w-2xl !h-[85vh]">
        <DialogHeader className="border-b px-5 py-3 space-y-0">
          <div className="flex items-center gap-2">
            {method !== "chooser" && !isDuplicate && !squadId && (
              <Tooltip>
                <TooltipTrigger
                  render={
                    <button
                      type="button"
                      onClick={() => setMethod("chooser")}
                      className="-ml-1 rounded-sm p-1 text-muted-foreground opacity-70 transition-opacity hover:bg-accent/60 hover:opacity-100"
                      aria-label={t(($) => $.create_dialog.back_aria)}
                    >
                      <ChevronRight className="h-3.5 w-3.5 rotate-180" />
                    </button>
                  }
                />
                <TooltipContent side="bottom">{t(($) => $.create_dialog.back)}</TooltipContent>
              </Tooltip>
            )}
            <DialogTitle className="text-base font-semibold">
              {isDuplicate
                ? t(($) => $.create_dialog.title_duplicate)
                : method === "import"
                  ? t(($) => $.create_dialog.title_import)
                  : t(($) => $.create_dialog.title_create)}
            </DialogTitle>
          </div>
          {isDuplicate && template && (
            <DialogDescription className="mt-1 text-xs">
              {t(($) => $.create_dialog.description_duplicate, { name: template.name })}
            </DialogDescription>
          )}
          {!isDuplicate && method === "chooser" && (
            <DialogDescription className="mt-1 text-xs">
              {t(($) => $.create_dialog.description_create)}
            </DialogDescription>
          )}
          {!isDuplicate && method === "import" && (
            <DialogDescription className="mt-1 text-xs">
              {t(($) => $.create_dialog.import.description)}
            </DialogDescription>
          )}
        </DialogHeader>

        {method === "chooser" && (
          <MethodChooser onChoose={setMethod} />
        )}
        {method === "manual" && (
          <ManualAgentForm
            runtimes={runtimes}
            runtimesLoading={runtimesLoading}
            members={members}
            currentUserId={currentUserId}
            template={template}
            squadId={squadId}
            onClose={onClose}
            onCreate={onCreate}
          />
        )}
        {method === "import" && (
          <ImportAgentForm
            runtimes={runtimes}
            runtimesLoading={runtimesLoading}
            members={members}
            currentUserId={currentUserId}
            onClose={onClose}
            onCreate={onCreate}
            onImported={onImported}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}

// ---------------------------------------------------------------------------
// Method chooser
// ---------------------------------------------------------------------------

function MethodChooser({ onChoose }: { onChoose: (m: Method) => void }) {
  const { t } = useT("agents");
  const methods: { key: Method; icon: typeof Plus; titleKey: "manual" | "import" }[] = [
    { key: "manual", icon: Pencil, titleKey: "manual" },
    { key: "import", icon: FolderOpen, titleKey: "import" },
  ];
  return (
    <div className="flex-1 overflow-y-auto p-5">
      <div className="grid gap-2">
        {methods.map(({ key, icon: Icon, titleKey }) => (
          <button
            key={key}
            type="button"
            onClick={() => onChoose(key)}
            className="group flex items-start gap-3 rounded-lg border bg-card p-4 text-left transition-colors hover:border-primary/40 hover:bg-accent/40"
          >
            <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground group-hover:text-foreground">
              <Icon className="h-4 w-4" />
            </div>
            <div className="min-w-0 flex-1">
              <div className="text-sm font-medium">
                {t(($) => $.create_dialog.method_card[`${titleKey}_title`])}
              </div>
              <div className="mt-0.5 text-xs text-muted-foreground">
                {t(($) => $.create_dialog.method_card[`${titleKey}_desc`])}
              </div>
            </div>
            <ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground/40 transition-colors group-hover:text-muted-foreground" />
          </button>
        ))}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Manual create / duplicate form (the original create flow)
// ---------------------------------------------------------------------------

function ManualAgentForm({
  runtimes,
  runtimesLoading,
  members,
  currentUserId,
  template,
  squadId,
  onClose,
  onCreate,
}: FormProps & {
  template?: Agent | null;
  squadId?: string;
}) {
  const { t } = useT("agents");
  const isDuplicate = !!template;
  const queryClient = useQueryClient();
  const wsId = useWorkspaceId();
  const accessPickerEnabled = useFeatureEnabled(COMPOSIO_MCP_APPS_FLAG, false);

  // Name defaults: duplicate uses "<original> copy". Manual-create starts blank.
  const [name, setName] = useState(
    template ? `${template.name}${t(($) => $.create_dialog.duplicate_copy_suffix)}` : "",
  );
  const [description, setDescription] = useState(template?.description ?? "");
  const [visibility, setVisibility] = useState<AgentVisibility>(
    template?.visibility ?? "workspace",
  );

  const [permissionMode, setPermissionMode] = useState<AgentPermissionMode>(
    template?.permission_mode ?? "public_to",
  );
  const [workspaceTargetOn, setWorkspaceTargetOn] = useState<boolean>(() => {
    if (template) {
      return (template.invocation_targets ?? []).some(
        (tgt) => tgt.target_type === "workspace",
      );
    }
    return true;
  });
  const [selectedMemberIds, setSelectedMemberIds] = useState<Set<string>>(
    () =>
      new Set(
        (template?.invocation_targets ?? [])
          .filter((tgt) => tgt.target_type === "member" && tgt.target_id)
          .map((tgt) => tgt.target_id as string),
      ),
  );

  const templateTeamTargets: AgentInvocationTargetInput[] = (
    template?.invocation_targets ?? []
  )
    .filter((tgt) => tgt.target_type === "team" && tgt.target_id)
    .map((tgt) => ({
      target_type: "team" as const,
      target_id: tgt.target_id as string,
    }));

  const [model, setModel] = useState(template?.model ?? "");
  const [instructions, setInstructions] = useState(template?.instructions ?? "");
  const [avatarUrl, setAvatarUrl] = useState<string | null>(template?.avatar_url ?? null);
  // Tracks whether the user made an explicit avatar choice (uploaded a photo
  // or picked an icon). Until they do, the avatar tracks a name-derived
  // default icon so a freshly-created agent always has a distinct identity
  // and the user can see/override it before submitting. Duplicating an agent
  // that already had an avatar counts as explicit so the clone keeps it.
  const [userPickedAvatar, setUserPickedAvatar] = useState(
    !!template?.avatar_url,
  );
  useEffect(() => {
    if (userPickedAvatar) return;
    const trimmed = name.trim();
    setAvatarUrl(trimmed ? buildAgentIconUrl(defaultAgentIconKey(trimmed)) : null);
  }, [name, userPickedAvatar]);
  const [selectedSkillIds, setSelectedSkillIds] = useState<Set<string>>(
    () => new Set(template?.skills.map((s) => s.id) ?? []),
  );
  const [creating, setCreating] = useState(false);

  const [selectedRuntimeId, setSelectedRuntimeId] = useState(() => {
    const templateRuntime = template?.runtime_id
      ? runtimes.find((r) => r.id === template.runtime_id)
      : undefined;
    if (templateRuntime && isRuntimeUsableForUser(templateRuntime, currentUserId)) {
      return templateRuntime.id;
    }
    return "";
  });

  const selectedRuntime = runtimes.find((d) => d.id === selectedRuntimeId) ?? null;
  const selectedRuntimeLocked =
    selectedRuntime != null &&
    !isRuntimeUsableForUser(selectedRuntime, currentUserId);

  const attachToSquad = async (agentId: string, displayName: string) => {
    if (!squadId) return;
    try {
      await api.addSquadMember(squadId, {
        member_type: "agent",
        member_id: agentId,
      });
      if (wsId) {
        queryClient.invalidateQueries({
          queryKey: [...workspaceKeys.squads(wsId), squadId, "members"],
        });
        queryClient.invalidateQueries({
          queryKey: [...workspaceKeys.squads(wsId), squadId],
        });
      }
    } catch (err) {
      toast.warning(
        t(($) => $.create_dialog.squad_join_failed_toast, {
          name: displayName,
          error: err instanceof Error ? err.message : "unknown error",
        }),
      );
    }
  };

  const handleSubmit = async () => {
    if (!name.trim() || !selectedRuntime || selectedRuntimeLocked) return;
    setCreating(true);

    try {
      const trimmedInstructions = instructions.trim();
      const data: CreateAgentRequest = {
        name: name.trim(),
        description: description.trim(),
        runtime_id: selectedRuntime.id,
        model: model.trim() || undefined,
        instructions: trimmedInstructions || undefined,
        avatar_url: avatarUrl ?? undefined,
      };
      if (accessPickerEnabled) {
        const invocationTargets: AgentInvocationTargetInput[] = [];
        if (permissionMode === "public_to") {
          if (workspaceTargetOn) {
            invocationTargets.push({ target_type: "workspace" });
          }
          for (const id of selectedMemberIds) {
            invocationTargets.push({ target_type: "member", target_id: id });
          }
          for (const tgt of templateTeamTargets) {
            invocationTargets.push(tgt);
          }
        }
        const collapseToPrivate =
          permissionMode === "public_to" && invocationTargets.length === 0;
        data.permission_mode = collapseToPrivate ? "private" : permissionMode;
        data.invocation_targets = collapseToPrivate ? [] : invocationTargets;
      } else {
        data.visibility = visibility;
      }
      if (template) {
        if (template.custom_args.length) data.custom_args = template.custom_args;
        if (template.max_concurrent_tasks) {
          data.max_concurrent_tasks = template.max_concurrent_tasks;
        }
      }
      const createdAgent = await onCreate(data);
      if (createdAgent && selectedSkillIds.size > 0) {
        try {
          await api.setAgentSkills(createdAgent.id, {
            skill_ids: [...selectedSkillIds],
          });
          if (wsId) {
            queryClient.invalidateQueries({ queryKey: workspaceKeys.agents(wsId) });
          }
        } catch (skillErr) {
          toast.warning(
            t(($) => $.create_dialog.skill_attach_failed_toast, {
              error:
                skillErr instanceof Error ? skillErr.message : "unknown error",
            }),
          );
        }
      }
      if (createdAgent && squadId) {
        await attachToSquad(createdAgent.id, createdAgent.name);
      }
      onClose();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t(($) => $.create_dialog.create_failed_toast));
      setCreating(false);
    }
  };

  return (
    <>
      <div className="flex-1 overflow-y-auto p-5">
        <div className="space-y-4 min-w-0">
          <div className="flex items-start gap-4">
            <AvatarUploadControl
              variant="agent"
              value={avatarUrl}
              name={name}
              size={64}
              onUploaded={(url) => {
                setAvatarUrl(url);
                setUserPickedAvatar(true);
              }}
              onIconPick={(key) => {
                setAvatarUrl(buildAgentIconUrl(key));
                setUserPickedAvatar(true);
              }}
              onClear={() => {
                setAvatarUrl(null);
                setUserPickedAvatar(false);
              }}
            />
            <div className="flex-1 min-w-0 space-y-3">
              <div>
                <Label className="text-xs text-muted-foreground">{t(($) => $.create_dialog.name_label)}</Label>
                <Input
                  autoFocus
                  type="text"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder={t(($) => $.create_dialog.name_placeholder)}
                  className="mt-1"
                  onKeyDown={(e) => {
                    if (isImeComposing(e)) return;
                    if (e.key === "Enter") handleSubmit();
                  }}
                />
              </div>

              <div>
                <Label className="text-xs text-muted-foreground">{t(($) => $.create_dialog.description_label)}</Label>
                <Input
                  type="text"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder={t(($) => $.create_dialog.description_placeholder)}
                  maxLength={AGENT_DESCRIPTION_MAX_LENGTH}
                  className="mt-1"
                />
                <div className="mt-1">
                  <CharCounter
                    length={[...description].length}
                    max={AGENT_DESCRIPTION_MAX_LENGTH}
                  />
                </div>
              </div>
            </div>
          </div>

          {accessPickerEnabled ? (
            <AccessSection
              permissionMode={permissionMode}
              onPermissionModeChange={setPermissionMode}
              workspaceTargetOn={workspaceTargetOn}
              onWorkspaceTargetChange={setWorkspaceTargetOn}
              selectedMemberIds={selectedMemberIds}
              onSelectedMemberIdsChange={setSelectedMemberIds}
              members={members}
              currentUserId={currentUserId}
            />
          ) : (
            <div>
              <Label className="text-xs text-muted-foreground">{t(($) => $.create_dialog.visibility_label)}</Label>
              <div className="mt-1.5 flex gap-2">
                <button
                  type="button"
                  onClick={() => setVisibility("workspace")}
                  className={`flex flex-1 items-center gap-2 rounded-lg border px-3 py-2.5 text-sm transition-colors ${
                    visibility === "workspace"
                      ? "border-primary bg-primary/5"
                      : "border-border hover:bg-muted"
                  }`}
                >
                  <Globe className="h-4 w-4 shrink-0 text-muted-foreground" />
                  <div className="text-left">
                    <div className="font-medium">{VISIBILITY_LABEL.workspace}</div>
                    <div className="text-xs text-muted-foreground">
                      {VISIBILITY_DESCRIPTION.workspace}
                    </div>
                  </div>
                </button>
                <button
                  type="button"
                  onClick={() => setVisibility("private")}
                  className={`flex flex-1 items-center gap-2 rounded-lg border px-3 py-2.5 text-sm transition-colors ${
                    visibility === "private"
                      ? "border-primary bg-primary/5"
                      : "border-border hover:bg-muted"
                  }`}
                >
                  <Lock className="h-4 w-4 shrink-0 text-muted-foreground" />
                  <div className="text-left">
                    <div className="font-medium">{VISIBILITY_LABEL.private}</div>
                    <div className="text-xs text-muted-foreground">
                      {VISIBILITY_DESCRIPTION.private}
                    </div>
                  </div>
                </button>
              </div>
            </div>
          )}

          <RuntimePicker
            runtimes={runtimes}
            runtimesLoading={runtimesLoading}
            members={members}
            currentUserId={currentUserId}
            selectedRuntimeId={selectedRuntimeId}
            onSelect={setSelectedRuntimeId}
          />

          <ModelDropdown
            runtimeId={selectedRuntime?.id ?? null}
            runtimeOnline={selectedRuntime?.status === "online"}
            value={model}
            onChange={setModel}
            disabled={!selectedRuntime}
          />

          <InstructionsEditor
            value={instructions}
            onChange={setInstructions}
            placeholder={
              isDuplicate
                ? t(($) => $.create_dialog.instructions.placeholder_duplicate)
                : t(($) => $.create_dialog.instructions.placeholder_blank)
            }
          />

          <SkillMultiSelect
            selectedIds={selectedSkillIds}
            onChange={setSelectedSkillIds}
          />
        </div>
      </div>

      <div className="flex items-center justify-end gap-2 border-t bg-background px-5 py-3">
        <Button variant="ghost" onClick={onClose}>
          {t(($) => $.create_dialog.cancel)}
        </Button>
        <Button
          onClick={handleSubmit}
          disabled={
            creating || !name.trim() || !selectedRuntime || selectedRuntimeLocked
          }
          title={
            selectedRuntimeLocked
              ? t(($) => $.create_dialog.runtime_private_locked_tooltip)
              : undefined
          }
        >
          {creating ? t(($) => $.create_dialog.creating) : t(($) => $.create_dialog.create)}
        </Button>
      </div>
    </>
  );
}

// ---------------------------------------------------------------------------
// Import form — parse an agentwaker role directory and create agent + skills
// ---------------------------------------------------------------------------

function ImportAgentForm({
  runtimes,
  runtimesLoading,
  members,
  currentUserId,
  onClose,
  onCreate,
  onImported,
}: FormProps & {
  onImported?: (agent: Agent) => void;
}) {
  const { t } = useT("agents");
  const queryClient = useQueryClient();
  const wsId = useWorkspaceId();
  const fileInputRef = useRef<HTMLInputElement>(null);

  const [importData, setImportData] = useState<AgentDirectoryImportResult | null>(null);
  const [directoryName, setDirectoryName] = useState("");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [selectedRuntimeId, setSelectedRuntimeId] = useState("");
  const [model, setModel] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const selectedRuntime = runtimes.find((d) => d.id === selectedRuntimeId) ?? null;
  const selectedRuntimeLocked =
    selectedRuntime != null &&
    !isRuntimeUsableForUser(selectedRuntime, currentUserId);

  const handleFileSelect = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(e.target.files ?? []);
    if (files.length === 0) return;
    setError("");
    const firstFile = files[0]!;
    const dirName =
      (firstFile.webkitRelativePath ?? "").split("/")[0] || firstFile.name;
    setDirectoryName(dirName);
    try {
      const data = await readAgentDirectory(files);
      if (data.errors.length > 0) {
        setError(data.errors.join(" "));
        // Still surface partial results — instructions / skills may be usable.
      }
      setImportData(data);
      setName(data.name);
      setDescription(data.description);
    } catch (err) {
      setError(err instanceof Error ? err.message : t(($) => $.create_dialog.import.fallback_error));
      setImportData(null);
    }
  };

  const submit = async () => {
    if (!importData) return;
    const trimmedName = name.trim();
    if (!trimmedName || !selectedRuntime || selectedRuntimeLocked) return;
    setLoading(true);
    setError("");
    try {
      // --- Phase 1: Create or overwrite the agent ---
      //
      // If an agent with the same name already exists in this workspace,
      // overwrite it (instructions, description, model, mcp_config, env)
      // instead of creating a new one. The backend rejects `custom_env` on
      // PUT /api/agents/{id} (MUL-2600), so env flows through the separate
      // /env endpoint. On create, both env and mcp ride along in one call.
      let existingAgent: Agent | undefined;
      try {
        // include_archived ensures we find same-named agents even if they
        // were archived. The DB unique constraint (workspace_id, name)
        // applies to ALL rows regardless of archived_at, so an archived
        // agent still blocks creation — we must overwrite it (not skip it)
        // to avoid a 409.
        const agents = await api.listAgents({
          workspace_id: wsId,
          include_archived: true,
        });
        existingAgent = agents.find((a) => a.name === trimmedName);
      } catch {
        // If the list call fails, proceed to create — a 409 will surface.
      }

      let agent: Agent;
      const hasEnv = Object.keys(importData.customEnv).length > 0;
      if (existingAgent) {
        // Overwrite path: updateAgent handles instructions / mcp_config /
        // description / model / runtime; env goes through updateAgentEnv.
        const updateData: UpdateAgentRequest = {
          description: description.trim(),
          instructions: importData.instructions,
          runtime_id: selectedRuntime.id,
          model: model.trim() || undefined,
          mcp_config: importData.mcpConfig ?? null,
          profile_html: importData.profileHtml || null,
        };
        agent = await api.updateAgent(existingAgent.id, updateData);
        // If the existing agent was archived, restore it so the imported
        // agent is immediately usable — the DB unique constraint blocks
        // creating a new same-named agent while the archived one exists,
        // so overwriting + restoring is the right move.
        if (existingAgent.archived_at) {
          try {
            agent = await api.restoreAgent(existingAgent.id);
          } catch {
            // Non-fatal: agent is updated but still archived. The user
            // can restore it manually from the agent list.
          }
        }
        if (hasEnv) {
          try {
            await api.updateAgentEnv(existingAgent.id, {
              custom_env: importData.customEnv,
            });
          } catch (envErr) {
            // Non-fatal: agent is updated, env can be set from the detail page.
            toast.warning(
              t(($) => $.create_dialog.import.env_update_failed_toast, {
                error:
                  envErr instanceof Error ? envErr.message : "unknown error",
              }),
            );
          }
        }
      } else {
        // Create path: try to create via onCreate (which does api.createAgent
        // + cache + navigate). If a same-named agent appeared between the
        // listAgents check and now (race), or listAgents failed silently, the
        // backend returns 409. We catch that, look up the existing agent, and
        // fall back to the overwrite path so the import still succeeds.
        try {
          const created = await onCreate({
            name: trimmedName,
            description: description.trim() || undefined,
            instructions: importData.instructions || undefined,
            runtime_id: selectedRuntime.id,
            model: model.trim() || undefined,
            custom_env: hasEnv ? importData.customEnv : undefined,
            mcp_config: importData.mcpConfig ?? undefined,
            profile_html: importData.profileHtml || undefined,
            visibility: "workspace",
          });
          agent = created as Agent;
        } catch (createErr) {
          const msg =
            createErr instanceof Error ? createErr.message : "unknown error";
          if (/\b(409|already exists)\b/i.test(msg)) {
            // 409 conflict: the agent was created between our list check and
            // this create. Find it and overwrite instead.
            const agents = await api.listAgents({
              workspace_id: wsId,
              include_archived: true,
            });
            const found = agents.find((a) => a.name === trimmedName);
            if (!found) {
              throw createErr;
            }
            agent = await api.updateAgent(found.id, {
              description: description.trim(),
              instructions: importData.instructions,
              runtime_id: selectedRuntime.id,
              model: model.trim() || undefined,
              mcp_config: importData.mcpConfig ?? null,
              profile_html: importData.profileHtml || null,
            });
            if (found.archived_at) {
              try {
                agent = await api.restoreAgent(found.id);
              } catch {
                // Non-fatal: agent is updated but still archived.
              }
            }
            if (hasEnv) {
              try {
                await api.updateAgentEnv(found.id, {
                  custom_env: importData.customEnv,
                });
              } catch (envErr) {
                toast.warning(
                  t(($) => $.create_dialog.import.env_update_failed_toast, {
                    error:
                      envErr instanceof Error ? envErr.message : "unknown error",
                  }),
                );
              }
            }
            // Mark that we went through the overwrite fallback so Phase 3
            // navigates via onImported instead of relying on onCreate's
            // navigation (which never ran).
            existingAgent = found;
          } else {
            throw createErr;
          }
        }
      }

      // --- Phase 2: Create or overwrite skills, then attach ---
      //
      // Each sub-skill from the directory is created fresh or, when a
      // same-named skill already exists, overwritten via updateSkill (which
      // does a full file replacement — the backend deletes all old files
      // then inserts the new set). Overwrite is limited to the skill creator
      // or workspace owner/admin (canManageSkill); a 403 here is surfaced as
      // a per-skill failure rather than aborting the whole import.
      if (agent && importData.skills.length > 0) {
        const skillIds: string[] = [];
        const failures: string[] = [];
        // Pre-fetch existing skills so we can overwrite without a per-skill
        // round-trip to detect the 409.
        let existingSkills: { id: string; name: string }[] = [];
        try {
          existingSkills = (await api.listSkills()).map((s) => ({
            id: s.id,
            name: s.name,
          }));
        } catch {
          // If the list fails, we fall back to create-only.
        }
        const existingByName = new Map(
          existingSkills.map((s) => [s.name, s.id]),
        );

        await Promise.all(
          importData.skills.map(async (skill) => {
            const existingId = existingByName.get(skill.name);
            try {
              let result;
              if (existingId) {
                // Overwrite: updateSkill replaces content + files wholesale.
                result = await api.updateSkill(existingId, {
                  description: skill.description,
                  content: skill.content,
                  files: skill.files,
                });
              } else {
                result = await api.createSkill({
                  name: skill.name,
                  description: skill.description,
                  content: skill.content,
                  files: skill.files,
                });
              }
              skillIds.push(result.id);
              if (wsId) {
                queryClient.setQueryData(
                  skillDetailOptions(wsId, result.id).queryKey,
                  result,
                );
              }
            } catch (err) {
              failures.push(
                `${skill.name}: ${err instanceof Error ? err.message : "unknown error"}`,
              );
            }
          }),
        );

        if (skillIds.length > 0) {
          try {
            await api.setAgentSkills(agent.id, { skill_ids: skillIds });
          } catch (err) {
            toast.warning(
              t(($) => $.create_dialog.skill_attach_failed_toast, {
                error:
                  err instanceof Error ? err.message : "unknown error",
              }),
            );
          }
        }
        if (wsId) {
          queryClient.invalidateQueries({ queryKey: workspaceKeys.skills(wsId) });
          queryClient.invalidateQueries({ queryKey: workspaceKeys.agents(wsId) });
        }
        if (failures.length > 0) {
          toast.warning(
            t(($) => $.create_dialog.import.skill_create_failed_toast, {
              error: failures.join("; "),
            }),
          );
        }
      }

      // --- Phase 3: Navigate to the agent detail page ---
      //
      // The create path already navigated inside onCreate (handleCreate in
      // agents-page.tsx). The overwrite path needs explicit navigation, so
      // the caller provides onImported for that. We call it only for the
      // overwrite case to avoid a double navigation on create.
      if (existingAgent && onImported) {
        onImported(agent);
      }
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : t(($) => $.create_dialog.import.fallback_error));
      setLoading(false);
    }
  };

  const canSubmit =
    !loading &&
    !selectedRuntimeLocked &&
    !!importData &&
    name.trim().length > 0 &&
    !!selectedRuntime;

  return (
    <>
      <input
        ref={fileInputRef}
        type="file"
        // @ts-expect-error non-standard attributes for directory picker
        webkitdirectory=""
        directory=""
        multiple
        className="hidden"
        onChange={handleFileSelect}
      />
      <div className="flex-1 overflow-y-auto p-5">
        <div className="space-y-4 min-w-0">
          {!importData ? (
            <button
              type="button"
              onClick={() => fileInputRef.current?.click()}
              className="flex w-full flex-col items-center justify-center gap-2 rounded-lg border border-dashed bg-card px-4 py-8 text-sm text-muted-foreground transition-colors hover:border-primary/40 hover:bg-accent/40"
            >
              <FolderOpen className="h-8 w-8 text-muted-foreground/60" />
              <span className="font-medium text-foreground">
                {t(($) => $.create_dialog.import.choose_button)}
              </span>
              <span className="text-xs">{t(($) => $.create_dialog.import.choose_hint)}</span>
            </button>
          ) : (
            <div className="space-y-3">
              {/* Selected directory banner */}
              <div className="flex items-center gap-2 rounded-md border bg-card px-3 py-2">
                <Folder className="h-4 w-4 shrink-0 text-muted-foreground" />
                <span className="min-w-0 flex-1 truncate text-sm font-medium">
                  {directoryName}
                </span>
                <button
                  type="button"
                  onClick={() => fileInputRef.current?.click()}
                  className="text-xs text-brand underline decoration-brand/40 underline-offset-2 hover:decoration-brand"
                >
                  {t(($) => $.create_dialog.import.change_button)}
                </button>
              </div>

              {/* Name + description (editable — user can rename on import) */}
              <div className="space-y-1.5">
                <Label className="text-xs text-muted-foreground">
                  {t(($) => $.create_dialog.name_label)}
                </Label>
                <Input
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder={t(($) => $.create_dialog.name_placeholder)}
                />
              </div>
              <div className="space-y-1.5">
                <Label className="text-xs text-muted-foreground">
                  {t(($) => $.create_dialog.description_label)}
                </Label>
                <Input
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder={t(($) => $.create_dialog.description_placeholder)}
                  maxLength={AGENT_DESCRIPTION_MAX_LENGTH}
                />
              </div>

              {/* Parsed summary: skills / env / MCP */}
              <div className="rounded-lg border bg-muted/30 px-3 py-2.5 space-y-1.5">
                <div className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
                  {t(($) => $.create_dialog.import.preview_label)}
                </div>
                <SummaryRow
                  label={t(($) => $.create_dialog.import.skills_count, {
                    count: importData.skills.length,
                  })}
                >
                  {importData.skills.map((s) => (
                    <span
                      key={s.name}
                      className="inline-block rounded bg-background px-1.5 py-0.5 text-[11px]"
                    >
                      {s.name}
                    </span>
                  ))}
                </SummaryRow>
                <SummaryRow
                  label={t(($) => $.create_dialog.import.env_count, {
                    count: Object.keys(importData.customEnv).length,
                  })}
                >
                  {Object.keys(importData.customEnv)
                    .slice(0, 8)
                    .map((k) => (
                      <span
                        key={k}
                        className="inline-block rounded bg-background px-1.5 py-0.5 text-[11px] font-mono"
                      >
                        {k}
                      </span>
                    ))}
                  {Object.keys(importData.customEnv).length > 8 && (
                    <span className="text-[11px] text-muted-foreground">
                      +{Object.keys(importData.customEnv).length - 8}
                    </span>
                  )}
                </SummaryRow>
                <div className="flex items-center gap-1.5 text-xs">
                  {importData.mcpConfig ? (
                    <>
                      <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" />
                      {t(($) => $.create_dialog.import.mcp_detected)}
                    </>
                  ) : (
                    <>
                      <span className="h-1.5 w-1.5 rounded-full bg-muted-foreground/40" />
                      {t(($) => $.create_dialog.import.mcp_none)}
                    </>
                  )}
                </div>
                {importData.instructions && (
                  <div>
                    <div className="text-[11px] text-muted-foreground">
                      {t(($) => $.create_dialog.import.instructions_preview)}
                    </div>
                    <pre className="mt-1 max-h-24 overflow-y-auto whitespace-pre-wrap rounded bg-background px-2 py-1.5 text-[11px] text-muted-foreground">
                      {importData.instructions.slice(0, 500)}
                      {importData.instructions.length > 500 ? "…" : ""}
                    </pre>
                  </div>
                )}
                {importData.skipped.length > 0 && (
                  <div className="text-[11px] text-amber-700 dark:text-amber-400">
                    {t(($) => $.create_dialog.import.skipped_hint, {
                      count: importData.skipped.length,
                    })}
                  </div>
                )}
              </div>

              {/* Runtime + model — agent cannot be created without a runtime */}
              <RuntimePicker
                runtimes={runtimes}
                runtimesLoading={runtimesLoading}
                members={members}
                currentUserId={currentUserId}
                selectedRuntimeId={selectedRuntimeId}
                onSelect={setSelectedRuntimeId}
              />
              <ModelDropdown
                runtimeId={selectedRuntime?.id ?? null}
                runtimeOnline={selectedRuntime?.status === "online"}
                value={model}
                onChange={setModel}
                disabled={!selectedRuntime}
              />

              {error && (
                <div className="flex items-start gap-2 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">
                  <AlertCircle className="h-3.5 w-3.5 shrink-0 mt-0.5" />
                  <span className="min-w-0 flex-1">{error}</span>
                </div>
              )}
            </div>
          )}
        </div>
      </div>

      <div className="flex items-center justify-end gap-2 border-t bg-background px-5 py-3">
        <Button variant="ghost" onClick={onClose}>
          {t(($) => $.create_dialog.cancel)}
        </Button>
        <Button
          onClick={submit}
          disabled={!canSubmit}
          title={
            selectedRuntimeLocked
              ? t(($) => $.create_dialog.runtime_private_locked_tooltip)
              : undefined
          }
        >
          {loading
            ? t(($) => $.create_dialog.import.importing)
            : t(($) => $.create_dialog.import.submit)}
        </Button>
      </div>
    </>
  );
}

function SummaryRow({
  label,
  children,
}: {
  label: string;
  children?: React.ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      <span className="text-xs text-muted-foreground">{label}</span>
      {children}
    </div>
  );
}

// ---------------------------------------------------------------------------
// AccessSection — inline access editor for the create/duplicate flow
// ---------------------------------------------------------------------------

/**
 * AccessSection — inline access editor for the create/duplicate flow, gated
 * on `COMPOSIO_MCP_APPS_FLAG`. Mirrors the semantics of
 * `AccessPicker` on the agent detail page: the underlying model is
 * `permission_mode` + `invocation_targets` (MUL-3963), not the legacy
 * `visibility`.
 */
function AccessSection({
  permissionMode,
  onPermissionModeChange,
  workspaceTargetOn,
  onWorkspaceTargetChange,
  selectedMemberIds,
  onSelectedMemberIdsChange,
  members,
  currentUserId,
}: {
  permissionMode: AgentPermissionMode;
  onPermissionModeChange: (next: AgentPermissionMode) => void;
  workspaceTargetOn: boolean;
  onWorkspaceTargetChange: (next: boolean) => void;
  selectedMemberIds: Set<string>;
  onSelectedMemberIdsChange: (next: Set<string>) => void;
  members: MemberWithUser[];
  currentUserId: string | null;
}) {
  const { t } = useT("agents");
  const isPrivate = permissionMode === "private";

  const otherMembers = members.filter((m) => m.user_id !== currentUserId);
  const hasAnyGrant = workspaceTargetOn || selectedMemberIds.size > 0;

  const toggleMember = (userId: string, checked: boolean) => {
    const next = new Set(selectedMemberIds);
    if (checked) next.add(userId);
    else next.delete(userId);
    onSelectedMemberIdsChange(next);
  };

  return (
    <div>
      <Label className="text-xs text-muted-foreground">
        {t(($) => $.create_dialog.access.label)}
      </Label>
      <div className="mt-1.5 flex gap-2">
        <button
          type="button"
          onClick={() => onPermissionModeChange("private")}
          className={`flex flex-1 items-center gap-2 rounded-lg border px-3 py-2.5 text-sm transition-colors ${
            isPrivate
              ? "border-primary bg-primary/5"
              : "border-border hover:bg-muted"
          }`}
        >
          <Lock className="h-4 w-4 shrink-0 text-muted-foreground" />
          <div className="text-left">
            <div className="font-medium">
              {t(($) => $.create_dialog.access.private_title)}
            </div>
            <div className="text-xs text-muted-foreground">
              {t(($) => $.create_dialog.access.private_desc)}
            </div>
          </div>
        </button>
        <button
          type="button"
          onClick={() => onPermissionModeChange("public_to")}
          className={`flex flex-1 items-center gap-2 rounded-lg border px-3 py-2.5 text-sm transition-colors ${
            !isPrivate
              ? "border-primary bg-primary/5"
              : "border-border hover:bg-muted"
          }`}
        >
          <Globe className="h-4 w-4 shrink-0 text-muted-foreground" />
          <div className="text-left">
            <div className="font-medium">
              {t(($) => $.create_dialog.access.public_title)}
            </div>
            <div className="text-xs text-muted-foreground">
              {t(($) => $.create_dialog.access.public_desc)}
            </div>
          </div>
        </button>
      </div>

      {!isPrivate && (
        <div className="mt-2 rounded-lg border bg-muted/30 px-3 py-2">
          <label className="flex cursor-pointer items-center gap-2 rounded-md py-1 text-sm">
            <Checkbox
              checked={workspaceTargetOn}
              onCheckedChange={(v) => onWorkspaceTargetChange(v === true)}
              aria-label={t(($) => $.create_dialog.access.public_workspace_option)}
            />
            <Globe className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
            <span className="min-w-0 flex-1">
              {t(($) => $.create_dialog.access.public_workspace_option)}
            </span>
          </label>

          <div className="mt-2 border-t pt-2">
            <div className="pb-1 text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
              {t(($) => $.create_dialog.access.public_members_group)}
            </div>
            {otherMembers.length === 0 ? (
              <div className="py-1 text-xs text-muted-foreground">
                {t(($) => $.create_dialog.access.public_members_empty)}
              </div>
            ) : (
              <div className="max-h-40 overflow-y-auto">
                {otherMembers.map((m) => {
                  const checked = selectedMemberIds.has(m.user_id);
                  return (
                    <label
                      key={m.user_id}
                      className="flex cursor-pointer items-center gap-2 rounded-md px-1 py-1 text-sm hover:bg-background/60"
                    >
                      <Checkbox
                        checked={checked}
                        onCheckedChange={(v) =>
                          toggleMember(m.user_id, v === true)
                        }
                        aria-label={m.name}
                      />
                      <ActorAvatar
                        actorType="member"
                        actorId={m.user_id}
                        size="sm"
                      />
                      <span className="min-w-0 flex-1 truncate">{m.name}</span>
                    </label>
                  );
                })}
              </div>
            )}
          </div>

          {!hasAnyGrant && (
            <div className="mt-2 text-xs text-amber-700 dark:text-amber-400">
              {t(($) => $.create_dialog.access.public_targets_empty_hint)}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

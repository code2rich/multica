"use client";

import { useRef, useState } from "react";
import { AlertCircle, Folder, FolderOpen, Loader2 } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import { useWorkspaceId } from "@multica/core/hooks";
import {
  skillDetailOptions,
  workspaceKeys,
} from "@multica/core/workspace/queries";
import type { Agent, UpdateAgentRequest } from "@multica/core/types";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@multica/ui/components/ui/dialog";
import { Button } from "@multica/ui/components/ui/button";
import { toast } from "sonner";
import { useT } from "../../i18n";
import {
  readAgentDirectory,
  type AgentDirectoryImportResult,
} from "../lib/agent-import";

/**
 * OverwriteAgentDialog — overwrite an existing agent from an agentwaker role
 * directory. Opens from the agent list's row actions menu.
 *
 * Unlike the import flow in CreateAgentDialog, this targets a specific agent
 * (no runtime picker — the agent already has one) and always takes the
 * overwrite path (no create-vs-overwrite branching).
 *
 * Overwrites: instructions, description, mcp_config, profile_html, custom_env,
 * and skills (create-or-overwrite per skill, then setAgentSkills). If the
 * agent was archived, it is restored so the overwritten agent is immediately
 * usable.
 */
export function OverwriteAgentDialog({
  agent,
  onClose,
}: {
  agent: Agent;
  onClose: () => void;
}) {
  const { t } = useT("agents");
  const queryClient = useQueryClient();
  const wsId = useWorkspaceId();
  const fileInputRef = useRef<HTMLInputElement>(null);

  const [importData, setImportData] = useState<AgentDirectoryImportResult | null>(null);
  const [directoryName, setDirectoryName] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

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
      }
      setImportData(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : t(($) => $.create_dialog.import.fallback_error));
      setImportData(null);
    }
  };

  const submit = async () => {
    if (!importData) return;
    setLoading(true);
    setError("");
    try {
      // 1. Update the agent core fields.
      const updateData: UpdateAgentRequest = {
        description: importData.description || undefined,
        instructions: importData.instructions || undefined,
        mcp_config: importData.mcpConfig ?? null,
        profile_html: importData.profileHtml || null,
      };
      await api.updateAgent(agent.id, updateData);

      // 2. Update env (separate endpoint — PUT /api/agents/{id} rejects
      //    custom_env per MUL-2600).
      const hasEnv = Object.keys(importData.customEnv).length > 0;
      if (hasEnv) {
        try {
          await api.updateAgentEnv(agent.id, {
            custom_env: importData.customEnv,
          });
        } catch (envErr) {
          toast.warning(
            t(($) => $.create_dialog.import.env_update_failed_toast, {
              error: envErr instanceof Error ? envErr.message : "unknown error",
            }),
          );
        }
      }

      // 3. Create or overwrite skills, then attach.
      if (importData.skills.length > 0) {
        const skillIds: string[] = [];
        const failures: string[] = [];
        let existingSkills: { id: string; name: string }[] = [];
        try {
          existingSkills = (await api.listSkills()).map((s) => ({
            id: s.id,
            name: s.name,
          }));
        } catch {
          // Fall back to create-only.
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
                error: err instanceof Error ? err.message : "unknown error",
              }),
            );
          }
        }
        if (wsId) {
          queryClient.invalidateQueries({ queryKey: workspaceKeys.skills(wsId) });
        }
        if (failures.length > 0) {
          toast.warning(
            t(($) => $.create_dialog.import.skill_create_failed_toast, {
              error: failures.join("; "),
            }),
          );
        }
      }

      // 4. Restore if archived.
      if (agent.archived_at) {
        try {
          await api.restoreAgent(agent.id);
        } catch {
          // Non-fatal: agent is updated but still archived.
        }
      }

      if (wsId) {
        queryClient.invalidateQueries({ queryKey: workspaceKeys.agents(wsId) });
      }
      toast.success(t(($) => $.create_dialog.import.overwrite_success_toast, { name: agent.name }));
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : t(($) => $.create_dialog.import.fallback_error));
      setLoading(false);
    }
  };

  const canSubmit = !loading && !!importData;

  return (
    <Dialog open onOpenChange={(v) => { if (!v) onClose(); }}>
      <DialogContent className="p-0 gap-0 flex flex-col overflow-hidden !top-1/2 !left-1/2 !-translate-x-1/2 !-translate-y-1/2 !w-full !max-w-2xl !h-[85vh]">
        <DialogHeader className="border-b px-5 py-3 space-y-0">
          <DialogTitle className="text-base font-semibold">
            {t(($) => $.create_dialog.import.overwrite_title)}
          </DialogTitle>
          <DialogDescription className="mt-1 text-xs">
            {t(($) => $.create_dialog.import.overwrite_description, { name: agent.name })}
          </DialogDescription>
        </DialogHeader>

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

                {/* Parsed summary */}
                <div className="rounded-lg border bg-muted/30 px-3 py-2.5 space-y-1.5">
                  <div className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
                    {t(($) => $.create_dialog.import.preview_label)}
                  </div>
                  <div className="flex flex-wrap items-center gap-1.5">
                    <span className="text-xs text-muted-foreground">
                      {t(($) => $.create_dialog.import.skills_count, {
                        count: importData.skills.length,
                      })}
                    </span>
                    {importData.skills.map((s) => (
                      <span
                        key={s.name}
                        className="inline-block rounded bg-background px-1.5 py-0.5 text-[11px]"
                      >
                        {s.name}
                      </span>
                    ))}
                  </div>
                  <div className="flex flex-wrap items-center gap-1.5">
                    <span className="text-xs text-muted-foreground">
                      {t(($) => $.create_dialog.import.env_count, {
                        count: Object.keys(importData.customEnv).length,
                      })}
                    </span>
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
                  </div>
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
                  <div className="flex items-center gap-1.5 text-xs">
                    {importData.profileHtml ? (
                      <>
                        <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" />
                        {t(($) => $.create_dialog.import.profile_detected)}
                      </>
                    ) : (
                      <>
                        <span className="h-1.5 w-1.5 rounded-full bg-muted-foreground/40" />
                        {t(($) => $.create_dialog.import.profile_none)}
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
                </div>

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
          <Button onClick={submit} disabled={!canSubmit}>
            {loading ? (
              <>
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
                {t(($) => $.create_dialog.import.importing)}
              </>
            ) : (
              t(($) => $.create_dialog.import.overwrite_submit)
            )}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

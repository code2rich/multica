"use client";

import { useCallback, useEffect, useState } from "react";
import { ArrowLeft, Loader2, Save } from "lucide-react";
import type { Agent } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { ContentEditor } from "../../../editor/content-editor";
import { ReadonlyContent } from "../../../editor/readonly-content";
import { useT } from "../../../i18n";
import {
  LocalizedContentToggle,
  type ContentLanguage,
} from "../../../i18n/localized-content-toggle";
import { findAgentSourceFile } from "../../agent-source-files";

export function InstructionsTab({
  agent,
  onSave,
  onDirtyChange,
}: {
  agent: Agent;
  onSave: (instructions: string) => Promise<void>;
  onDirtyChange?: (dirty: boolean) => void;
}) {
  const { t, i18n } = useT("agents");
  const hasChinese = Boolean(agent.instructions_zh?.trim());
  const [language, setLanguage] = useState<ContentLanguage>(
    hasChinese && i18n.language.startsWith("zh") ? "zh" : "en",
  );
  const [value, setValue] = useState(agent.instructions ?? "");
  const [activeSourcePath, setActiveSourcePath] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const isDirty = value !== (agent.instructions ?? "");

  // Sync when switching between agents.
  useEffect(() => {
    setValue(agent.instructions ?? "");
    setLanguage(
      agent.instructions_zh?.trim() && i18n.language.startsWith("zh")
        ? "zh"
        : "en",
    );
    setActiveSourcePath(null);
  }, [agent.id, agent.instructions, agent.instructions_zh, i18n.language]);

  const activeSourceFile = agent.source_files?.find(
    (file) => file.path === activeSourcePath,
  );
  const handleSourceLink = useCallback(
    (href: string) => {
      const result = findAgentSourceFile(
        agent.source_files,
        activeSourcePath ?? "",
        href,
      );
      if (!result) return false;
      setActiveSourcePath(result.path);
      return true;
    },
    [activeSourcePath, agent.source_files],
  );

  // Report dirty state up so the parent can guard tab switches.
  useEffect(() => {
    onDirtyChange?.(isDirty);
  }, [isDirty, onDirtyChange]);

  const handleSave = async () => {
    setSaving(true);
    try {
      await onSave(value);
    } catch {
      // toast handled by parent
    } finally {
      setSaving(false);
    }
  };

  return (
    // Fill the parent TabContent (h-full flex-col): helper + footer take
    // their natural height, the editor wrapper fills the rest. Without this
    // the Save row scrolls off-screen as the user writes longer prompts.
    <div className="flex h-full flex-col gap-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="text-xs text-muted-foreground">
            {language === "en"
              ? t(($) => $.tab_body.instructions.intro)
              : t(($) => $.tab_body.instructions.localized_intro)}
          </p>
          <p className="mt-1 text-[11px] font-medium text-muted-foreground">
            {language === "en"
              ? t(($) => $.tab_body.instructions.execution_source)
              : t(($) => $.tab_body.instructions.display_only)}
          </p>
        </div>
        <LocalizedContentToggle
          value={language}
          onChange={(nextLanguage) => {
            setLanguage(nextLanguage);
            setActiveSourcePath(null);
          }}
          hasChinese={hasChinese}
          ariaLabel={t(($) => $.tab_body.instructions.language_toggle)}
        />
      </div>

      <div
        // flex-1 min-h-0 so the wrapper claims the leftover height in the
        // column. overflow-y-auto so very long prompts scroll inside the
        // editor instead of pushing the Save row down.
        className="flex-1 min-h-0 overflow-y-auto rounded-md border bg-background px-4 py-3 transition-colors focus-within:border-input"
      >
        {language === "en" ? <ContentEditor
          // Keyed by agent id so navigating between agents fully remounts the
          // editor — Tiptap's `defaultValue` is read once, so without the key
          // the second agent's instructions wouldn't load.
          key={agent.id}
          defaultValue={value}
          onUpdate={setValue}
          placeholder={t(($) => $.tab_body.instructions.placeholder)}
          debounceMs={150}
          // Mention has no business meaning in agent system prompts — typing
          // `@` would just confuse users with a member/agent picker.
          disableMentions
          // min-h-full lets the editor fill the wrapper even when the user
          // has typed nothing yet, so the click target matches the visual
          // box. Combined with the wrapper's overflow-y-auto, long content
          // grows past the wrapper height and scrolls within it.
          className="min-h-full"
        /> : activeSourcePath ? (
          <div className="min-h-full">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="mb-3 -ml-2 max-w-full"
              onClick={() => setActiveSourcePath(null)}
            >
              <ArrowLeft className="h-3.5 w-3.5" />
              <span className="truncate">
                {t(($) => $.tab_body.instructions.back_to_overview)}
              </span>
            </Button>
            <div className="mb-3 break-all font-mono text-xs text-muted-foreground">
              {activeSourcePath}
            </div>
            {activeSourceFile ? (
              /\.mdx?$/i.test(activeSourceFile.path) ? (
                <ReadonlyContent
                  content={activeSourceFile.content}
                  className="min-h-full"
                  onLinkClick={handleSourceLink}
                />
              ) : (
                <pre className="overflow-x-auto whitespace-pre-wrap break-words rounded-md bg-muted/40 p-3 font-mono text-xs">
                  {activeSourceFile.content}
                </pre>
              )
            ) : (
              <p className="text-sm text-muted-foreground">
                {t(($) => $.tab_body.instructions.source_file_unavailable)}
              </p>
            )}
          </div>
        ) : (
          <ReadonlyContent
            content={agent.instructions_zh ?? ""}
            className="min-h-full"
            onLinkClick={handleSourceLink}
          />
        )}
      </div>

      {language === "en" && <div className="flex items-center justify-end gap-3">
        {isDirty && (
          <span className="text-xs text-muted-foreground">{t(($) => $.tab_body.common.unsaved_changes)}</span>
        )}
        <Button
          size="sm"
          onClick={handleSave}
          disabled={!isDirty || saving}
        >
          {saving ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          ) : (
            <Save className="h-3.5 w-3.5" />
          )}
          {t(($) => $.tab_body.common.save)}
        </Button>
      </div>}
    </div>
  );
}

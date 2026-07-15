"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { Eraser, Eye, Loader2, Save, Upload } from "lucide-react";
import type { Agent } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { toast } from "sonner";
import { useT } from "../../../i18n";
import { HtmlPreviewBody } from "../../../editor/html-preview-body";

/**
 * ProfileTab — agent persona / profile HTML editor + preview.
 *
 * Renders the `agent.profile_html` content (from an agentwaker role import
 * or a manual upload) inside a sandboxed iframe. The user can toggle between
 * a live preview and the HTML source editor, upload an HTML file from disk,
 * or clear the profile entirely.
 *
 * Follows the same dirty / save / toast pattern as McpConfigTab.
 */
export function ProfileTab({
  agent,
  onSave,
  onDirtyChange,
}: {
  agent: Agent;
  onSave: (updates: { profile_html: string | null }) => Promise<void>;
  onDirtyChange?: (dirty: boolean) => void;
}) {
  const { t } = useT("agents");

  const original = useMemo(() => agent.profile_html ?? "", [agent.profile_html]);
  const [text, setText] = useState(original);
  const [saving, setSaving] = useState(false);
  const [mode, setMode] = useState<"preview" | "edit">(
    original ? "preview" : "edit",
  );
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Sync local draft when the agent prop changes (e.g. after a save
  // invalidates the cache and a fresh agent arrives). Same guard as
  // McpConfigTab: only sync when the user has no in-flight edits.
  const previousOriginalRef = useRef(original);
  useEffect(() => {
    setText((current) =>
      current === previousOriginalRef.current ? original : current,
    );
    previousOriginalRef.current = original;
  }, [original]);

  const dirty = text !== original;

  useEffect(() => {
    onDirtyChange?.(dirty);
  }, [dirty, onDirtyChange]);

  const handleSave = async () => {
    setSaving(true);
    try {
      // Empty string → null so the backend clears the column (ClearAgentProfileHTML)
      // rather than storing an empty TEXT. Matches the mcp_config clear pattern.
      await onSave({ profile_html: text.trim() || null });
      toast.success(t(($) => $.tab_body.profile.saved_toast));
    } catch (err) {
      toast.error(
        err instanceof Error && err.message
          ? err.message
          : t(($) => $.tab_body.profile.save_failed_toast),
      );
    } finally {
      setSaving(false);
    }
  };

  const handleClear = () => {
    setText("");
    setMode("edit");
  };

  const handleFileUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    try {
      const reader = new FileReader();
      reader.onload = () => {
        const result = reader.result;
        if (typeof result === "string") {
          setText(result);
          setMode("preview");
        }
      };
      reader.onerror = () => {
        toast.error(t(($) => $.tab_body.profile.upload_failed_toast));
      };
      reader.readAsText(file);
    } catch {
      toast.error(t(($) => $.tab_body.profile.upload_failed_toast));
    }
    // Reset so the same file can be re-selected.
    e.target.value = "";
  };

  const hasContent = text.trim().length > 0;

  return (
    <div className="flex h-full flex-col space-y-3">
      <input
        ref={fileInputRef}
        type="file"
        accept=".html,.htm,text/html"
        className="hidden"
        onChange={handleFileUpload}
      />

      <div className="flex items-start justify-between gap-3">
        <p className="text-xs text-muted-foreground">
          {t(($) => $.tab_body.profile.intro)}
        </p>
        <div className="flex shrink-0 items-center gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => fileInputRef.current?.click()}
          >
            <Upload className="h-3 w-3" />
            {t(($) => $.tab_body.profile.upload_button)}
          </Button>
          {hasContent && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={handleClear}
            >
              <Eraser className="h-3 w-3" />
              {t(($) => $.tab_body.profile.clear_action)}
            </Button>
          )}
        </div>
      </div>

      {/* Mode toggle: preview / edit */}
      <div className="flex gap-1">
        <button
          type="button"
          onClick={() => setMode("preview")}
          className={`flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${
            mode === "preview"
              ? "bg-muted text-foreground"
              : "text-muted-foreground hover:text-foreground"
          }`}
        >
          <Eye className="h-3 w-3" />
          {t(($) => $.tab_body.profile.preview_mode)}
        </button>
        <button
          type="button"
          onClick={() => setMode("edit")}
          className={`flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${
            mode === "edit"
              ? "bg-muted text-foreground"
              : "text-muted-foreground hover:text-foreground"
          }`}
        >
          {t(($) => $.tab_body.profile.edit_mode)}
        </button>
      </div>

      {mode === "preview" ? (
        hasContent ? (
          <HtmlPreviewBody
            source={{ kind: "inline", html: text }}
            title={t(($) => $.tab_body.profile.preview_aria)}
            className="rounded-md border"
            iframeClassName="rounded-md"
            autoResize
          />
        ) : (
          <div className="flex flex-1 items-center justify-center rounded-md border border-dashed text-sm text-muted-foreground">
            {t(($) => $.tab_body.profile.empty_hint)}
          </div>
        )
      ) : (
        <Textarea
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder={t(($) => $.tab_body.profile.placeholder)}
          spellCheck={false}
          className="min-h-[70dvh] flex-1 font-mono text-xs"
        />
      )}

      <div className="flex items-center justify-end gap-3">
        {dirty && (
          <span className="text-xs text-muted-foreground">
            {t(($) => $.tab_body.common.unsaved_changes)}
          </span>
        )}
        <Button
          onClick={handleSave}
          disabled={!dirty || saving}
          size="sm"
        >
          {saving ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          ) : (
            <Save className="h-3.5 w-3.5" />
          )}
          {t(($) => $.tab_body.common.save)}
        </Button>
      </div>
    </div>
  );
}

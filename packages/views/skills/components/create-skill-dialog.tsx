"use client";

import { useRef, useState } from "react";
import {
  AlertCircle,
  ArrowLeft,
  ChevronRight,
  Download,
  File as FileIcon,
  Folder,
  FolderOpen,
  HardDrive,
  Loader2,
  Pencil,
  Plus,
  X as XIcon,
} from "lucide-react";
import { toast } from "sonner";
import { useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import type { Skill } from "@multica/core/types";
import { useWorkspaceId } from "@multica/core/hooks";
import { isImeComposing } from "@multica/core/utils";
import {
  skillDetailOptions,
  workspaceKeys,
} from "@multica/core/workspace/queries";
import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@multica/ui/components/ui/tooltip";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { useScrollFade } from "@multica/ui/hooks/use-scroll-fade";
import { cn } from "@multica/ui/lib/utils";
import { openExternal } from "../../platform";
import { RuntimeLocalSkillImportPanel } from "./runtime-local-skill-import-panel";
import { useT } from "../../i18n";
import { isNameConflictError } from "../lib/utils";
import { readSkillDirectory, type DirectorySkillImportResult } from "../lib/directory-import";

type Method = "chooser" | "manual" | "url" | "runtime" | "directory";

function seedAfterCreate(
  qc: ReturnType<typeof useQueryClient>,
  wsId: string,
  skill: Skill,
) {
  qc.setQueryData(skillDetailOptions(wsId, skill.id).queryKey, skill);
  qc.invalidateQueries({ queryKey: workspaceKeys.skills(wsId) });
  qc.invalidateQueries({ queryKey: workspaceKeys.agents(wsId) });
}

// ---------------------------------------------------------------------------
// Chooser — initial method picker (3 cards)
// ---------------------------------------------------------------------------

function MethodChooser({ onChoose }: { onChoose: (m: Method) => void }) {
  const { t } = useT("skills");
  const methods: {
    key: Method;
    icon: typeof Plus;
    titleKey: "manual" | "url" | "runtime" | "directory";
  }[] = [
    { key: "manual", icon: Plus, titleKey: "manual" },
    { key: "directory", icon: FolderOpen, titleKey: "directory" },
    { key: "url", icon: Download, titleKey: "url" },
    { key: "runtime", icon: HardDrive, titleKey: "runtime" },
  ];
  return (
    <div className="grid gap-2 p-5">
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
              {t(($) => $.create.method_card[`${titleKey}_title`])}
            </div>
            <div className="mt-0.5 text-xs text-muted-foreground">
              {t(($) => $.create.method_card[`${titleKey}_desc`])}
            </div>
          </div>
          <ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground/40 transition-colors group-hover:text-muted-foreground" />
        </button>
      ))}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Manual form
// ---------------------------------------------------------------------------

function ManualForm({
  onCreated,
  onCancel,
}: {
  onCreated: (skill: Skill) => void;
  onCancel: () => void;
}) {
  const { t } = useT("skills");
  const qc = useQueryClient();
  const wsId = useWorkspaceId();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const scrollRef = useRef<HTMLDivElement>(null);
  const fadeStyle = useScrollFade(scrollRef);

  const submit = async () => {
    const trimmed = name.trim();
    if (!trimmed) return;
    setLoading(true);
    setError("");
    try {
      const skill = await api.createSkill({
        name: trimmed,
        description: description.trim(),
      });
      seedAfterCreate(qc, wsId, skill);
      toast.success(t(($) => $.create.manual.toast_created));
      onCreated(skill);
    } catch (err) {
      setError(err instanceof Error ? err.message : t(($) => $.create.manual.fallback_error));
      setLoading(false);
    }
  };

  return (
    <>
      <div
        ref={scrollRef}
        style={fadeStyle}
        className="flex-1 min-h-0 space-y-4 overflow-y-auto px-5 py-4"
      >
        <div className="space-y-1.5">
          <Label
            htmlFor="create-skill-name"
            className="text-xs text-muted-foreground"
          >
            {t(($) => $.create.manual.name_label)}
          </Label>
          <Input
            id="create-skill-name"
            autoFocus
            value={name}
            onChange={(e) => {
              setName(e.target.value);
              setError("");
            }}
            placeholder={t(($) => $.create.manual.name_placeholder)}
            onKeyDown={(e) => {
              if (isImeComposing(e)) return;
              if (e.key === "Enter") submit();
            }}
          />
          <p className="text-xs text-muted-foreground">
            {t(($) => $.create.manual.name_hint)}
          </p>
        </div>

        <div className="space-y-1.5">
          <Label
            htmlFor="create-skill-desc"
            className="text-xs text-muted-foreground"
          >
            <Pencil className="h-3 w-3" />
            {t(($) => $.create.manual.description_label)}
          </Label>
          <Textarea
            id="create-skill-desc"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder={t(($) => $.create.manual.description_placeholder)}
            rows={3}
            className="resize-none"
          />
        </div>

        {error && (
          <div
            role="alert"
            className="flex items-start gap-2 rounded-md bg-destructive/10 px-3 py-2 text-xs text-destructive"
          >
            <AlertCircle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
            <span>
              {error}
              {isNameConflictError(error) && (
                <>{t(($) => $.create.manual.name_conflict_hint)}</>
              )}
            </span>
          </div>
        )}
      </div>

      <div className="flex shrink-0 items-center justify-end gap-2 border-t bg-muted/30 px-5 py-3">
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={onCancel}
          disabled={loading}
        >
          {t(($) => $.create.manual.cancel)}
        </Button>
        <Button
          type="button"
          size="sm"
          onClick={submit}
          disabled={!name.trim() || loading}
        >
          {loading ? (
            <>
              <Loader2 className="h-3 w-3 animate-spin" />
              {t(($) => $.create.manual.submitting)}
            </>
          ) : (
            t(($) => $.create.manual.submit)
          )}
        </Button>
      </div>
    </>
  );
}

// ---------------------------------------------------------------------------
// URL import form
// ---------------------------------------------------------------------------

type DetectedSource = "clawhub" | "skills.sh" | "github" | null;

function detectUrlSource(url: string): DetectedSource {
  const u = url.trim().toLowerCase();
  if (u.includes("clawhub.ai")) return "clawhub";
  if (u.includes("skills.sh")) return "skills.sh";
  if (u.includes("github.com")) return "github";
  return null;
}

function SourceCard({
  label,
  exampleHost,
  browseUrl,
  active,
}: {
  label: string;
  exampleHost: string;
  browseUrl: string;
  active: boolean;
}) {
  return (
    <div
      className={`rounded-md border px-3 py-2.5 transition-colors ${
        active ? "border-primary bg-primary/5" : ""
      }`}
    >
      <div className="text-xs font-medium">{label}</div>
      <button
        type="button"
        onClick={() => openExternal(browseUrl)}
        className="mt-0.5 block max-w-full truncate text-left font-mono text-xs text-brand underline decoration-brand/40 underline-offset-2 hover:decoration-brand"
      >
        {exampleHost}
      </button>
    </div>
  );
}

function UrlForm({
  onCreated,
  onCancel,
}: {
  onCreated: (skill: Skill) => void;
  onCancel: () => void;
}) {
  const { t } = useT("skills");
  const qc = useQueryClient();
  const wsId = useWorkspaceId();
  const [url, setUrl] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const source = detectUrlSource(url);
  const scrollRef = useRef<HTMLDivElement>(null);
  const fadeStyle = useScrollFade(scrollRef);

  const submit = async () => {
    const trimmed = url.trim();
    if (!trimmed) return;
    setLoading(true);
    setError("");
    try {
      const skill = await api.importSkill({ url: trimmed });
      seedAfterCreate(qc, wsId, skill);
      toast.success(t(($) => $.create.url.toast_imported));
      onCreated(skill);
    } catch (err) {
      setError(err instanceof Error ? err.message : t(($) => $.create.url.fallback_error));
      setLoading(false);
    }
  };

  const submittingLabel = (() => {
    if (!loading) return t(($) => $.create.url.import);
    if (source === "clawhub") return t(($) => $.create.url.importing_clawhub);
    if (source === "skills.sh") return t(($) => $.create.url.importing_skills_sh);
    if (source === "github") return t(($) => $.create.url.importing_github);
    return t(($) => $.create.url.importing);
  })();

  return (
    <>
      <div
        ref={scrollRef}
        style={fadeStyle}
        className="flex-1 min-h-0 space-y-4 overflow-y-auto px-5 py-4"
      >
        <div className="space-y-1.5">
          <Label htmlFor="import-url" className="text-xs text-muted-foreground">
            {t(($) => $.create.url.url_label)}
          </Label>
          <Input
            id="import-url"
            autoFocus
            value={url}
            onChange={(e) => {
              setUrl(e.target.value);
              setError("");
            }}
            placeholder="https://clawhub.ai/owner/skill"
            className="font-mono text-sm"
            onKeyDown={(e) => {
              if (e.key === "Enter") submit();
            }}
          />
        </div>

        <div>
          <p className="mb-2 text-xs text-muted-foreground">
            {t(($) => $.create.url.supported_sources)}
          </p>
          <div className="grid grid-cols-3 gap-2">
            <SourceCard
              label="ClawHub"
              exampleHost="clawhub.ai/owner/skill"
              browseUrl="https://clawhub.ai"
              active={source === "clawhub"}
            />
            <SourceCard
              label="Skills.sh"
              exampleHost="skills.sh/owner/repo/skill"
              browseUrl="https://skills.sh"
              active={source === "skills.sh"}
            />
            <SourceCard
              label="GitHub"
              exampleHost="github.com/owner/repo"
              browseUrl="https://github.com"
              active={source === "github"}
            />
          </div>
        </div>

        {error && (
          <div
            role="alert"
            className="flex items-start gap-2 rounded-md bg-destructive/10 px-3 py-2 text-xs text-destructive"
          >
            <AlertCircle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
            <span>
              {error}
              {isNameConflictError(error) && (
                <>{t(($) => $.create.url.name_conflict_hint)}</>
              )}
            </span>
          </div>
        )}
      </div>

      <div className="flex shrink-0 items-center justify-end gap-2 border-t bg-muted/30 px-5 py-3">
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={onCancel}
          disabled={loading}
        >
          {t(($) => $.create.url.cancel)}
        </Button>
        <Button
          type="button"
          size="sm"
          onClick={submit}
          disabled={!url.trim() || loading}
        >
          {loading ? (
            <>
              <Loader2 className="h-3 w-3 animate-spin" />
              {submittingLabel}
            </>
          ) : (
            <>
              <Download className="h-3 w-3" />
              {submittingLabel}
            </>
          )}
        </Button>
      </div>
    </>
  );
}

// ---------------------------------------------------------------------------
// Directory import form
// ---------------------------------------------------------------------------

function DirectoryForm({
  onCreated,
  onCancel,
}: {
  onCreated: (skill: Skill) => void;
  onCancel: () => void;
}) {
  const { t } = useT("skills");
  const qc = useQueryClient();
  const wsId = useWorkspaceId();
  const fileInputRef = useRef<HTMLInputElement>(null);

  const [importData, setImportData] = useState<DirectorySkillImportResult | null>(null);
  const [directoryName, setDirectoryName] = useState("");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const scrollRef = useRef<HTMLDivElement>(null);
  const fadeStyle = useScrollFade(scrollRef);

  const handleFileSelect = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(e.target.files ?? []);
    if (files.length === 0) return;
    setError("");
    const firstFile = files[0]!;
    const dirName = (firstFile.webkitRelativePath ?? "").split("/")[0] || firstFile.name;
    setDirectoryName(dirName);
    try {
      const data = await readSkillDirectory(files);
      if (data.errors.length > 0) {
        setError(data.errors.join(" "));
        setImportData(null);
        return;
      }
      setImportData(data);
      setName(data.name);
      setDescription(data.description ?? "");
    } catch (err) {
      setError(err instanceof Error ? err.message : t(($) => $.create.directory.fallback_error));
      setImportData(null);
    }
  };

  const submit = async () => {
    if (!importData) return;
    const trimmed = name.trim();
    if (!trimmed) return;
    setLoading(true);
    setError("");
    try {
      const skill = await api.createSkill({
        name: trimmed,
        description: description.trim(),
        content: importData.content,
        files: importData.files,
      });
      seedAfterCreate(qc, wsId, skill);
      toast.success(t(($) => $.create.directory.toast_created));
      onCreated(skill);
    } catch (err) {
      setError(err instanceof Error ? err.message : t(($) => $.create.directory.fallback_error));
      setLoading(false);
    }
  };

  return (
    <>
      <div
        ref={scrollRef}
        style={fadeStyle}
        className="flex-1 min-h-0 space-y-4 overflow-y-auto px-5 py-4"
      >
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

        {!importData ? (
          <button
            type="button"
            onClick={() => fileInputRef.current?.click()}
            className="flex w-full flex-col items-center justify-center gap-2 rounded-lg border border-dashed bg-card px-4 py-8 text-sm text-muted-foreground transition-colors hover:border-primary/40 hover:bg-accent/40"
          >
            <FolderOpen className="h-8 w-8 text-muted-foreground/60" />
            <span className="font-medium text-foreground">
              {t(($) => $.create.directory.choose_button)}
            </span>
            <span className="text-xs">{t(($) => $.create.directory.choose_hint)}</span>
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
                {t(($) => $.create.directory.change_button)}
              </button>
            </div>

            <div className="space-y-1.5">
              <Label
                htmlFor="create-skill-dir-name"
                className="text-xs text-muted-foreground"
              >
                {t(($) => $.create.directory.name_label)}
              </Label>
              <Input
                id="create-skill-dir-name"
                value={name}
                onChange={(e) => {
                  setName(e.target.value);
                  setError("");
                }}
                placeholder={t(($) => $.create.directory.name_placeholder)}
                onKeyDown={(e) => {
                  if (isImeComposing(e)) return;
                  if (e.key === "Enter") submit();
                }}
              />
              <p className="text-xs text-muted-foreground">
                {t(($) => $.create.directory.name_hint)}
              </p>
            </div>

            <div className="space-y-1.5">
              <Label
                htmlFor="create-skill-dir-desc"
                className="text-xs text-muted-foreground"
              >
                <Pencil className="h-3 w-3" />
                {t(($) => $.create.directory.description_label)}
              </Label>
              <Textarea
                id="create-skill-dir-desc"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder={t(($) => $.create.directory.description_placeholder)}
                rows={2}
                className="resize-none"
              />
            </div>

            <div className="space-y-1.5">
              <div className="flex items-center justify-between">
                <Label className="text-xs text-muted-foreground">
                  {t(($) => $.create.directory.files_label, {
                    count: importData.files.length,
                  })}
                </Label>
                {importData.skipped.length > 0 && (
                  <span className="text-xs text-muted-foreground">
                    {t(($) => $.create.directory.skipped_label, {
                      count: importData.skipped.length,
                    })}
                  </span>
                )}
              </div>
              <div className="max-h-40 space-y-0.5 overflow-y-auto rounded-md border bg-card p-2">
                {importData.files.length === 0 ? (
                  <p className="px-1 py-2 text-xs text-muted-foreground">
                    {t(($) => $.create.directory.no_files)}
                  </p>
                ) : (
                  importData.files.map((f) => (
                    <div
                      key={f.path}
                      className="flex items-center gap-2 px-1 py-0.5 text-xs"
                    >
                      <FileIcon className="h-3 w-3 shrink-0 text-muted-foreground" />
                      <span className="min-w-0 flex-1 truncate font-mono">
                        {f.path}
                      </span>
                    </div>
                  ))
                )}
              </div>
            </div>
          </div>
        )}

        {error && (
          <div
            role="alert"
            className="flex items-start gap-2 rounded-md bg-destructive/10 px-3 py-2 text-xs text-destructive"
          >
            <AlertCircle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
            <span>
              {error}
              {isNameConflictError(error) && (
                <>{t(($) => $.create.directory.name_conflict_hint)}</>
              )}
            </span>
          </div>
        )}
      </div>

      <div className="flex shrink-0 items-center justify-end gap-2 border-t bg-muted/30 px-5 py-3">
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={onCancel}
          disabled={loading}
        >
          {t(($) => $.create.directory.cancel)}
        </Button>
        <Button
          type="button"
          size="sm"
          onClick={submit}
          disabled={!importData || !name.trim() || loading}
        >
          {loading ? (
            <>
              <Loader2 className="h-3 w-3 animate-spin" />
              {t(($) => $.create.directory.submitting)}
            </>
          ) : (
            <>
              <FolderOpen className="h-3 w-3" />
              {t(($) => $.create.directory.submit)}
            </>
          )}
        </Button>
      </div>
    </>
  );
}

// ---------------------------------------------------------------------------
// Root dialog
// ---------------------------------------------------------------------------

export function CreateSkillDialog({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated?: (skill: Skill) => void;
}) {
  const { t } = useT("skills");
  const [method, setMethod] = useState<Method>("chooser");

  const handleCreated = (skill: Skill) => {
    onCreated?.(skill);
    onClose();
  };

  const wide = method === "runtime";

  return (
    <Dialog open onOpenChange={(v) => !v && onClose()}>
      <DialogContent
        showCloseButton={false}
        className={cn(
          "flex flex-col gap-0 overflow-hidden p-0",
          "!transition-all !duration-300 !ease-out",
          wide
            ? "!h-[min(600px,85vh)] !max-w-2xl !w-full"
            : "!h-auto !max-h-[85vh] !max-w-md !w-full",
        )}
      >
        {/* Header */}
        <div className="flex shrink-0 items-start justify-between gap-3 border-b px-5 pt-4 pb-3">
          <div className="flex items-center gap-2 min-w-0">
            {method !== "chooser" && (
              <Tooltip>
                <TooltipTrigger
                  render={
                    <button
                      type="button"
                      onClick={() => setMethod("chooser")}
                      className="-ml-1 rounded-sm p-1 text-muted-foreground opacity-70 transition-opacity hover:bg-accent/60 hover:opacity-100"
                      aria-label={t(($) => $.create.back_aria)}
                    >
                      <ArrowLeft className="h-3.5 w-3.5" />
                    </button>
                  }
                />
                <TooltipContent side="bottom">{t(($) => $.create.back)}</TooltipContent>
              </Tooltip>
            )}
            <div className="min-w-0">
              <DialogTitle className="truncate text-base font-medium">
                {t(($) => $.create.method[method].title)}
              </DialogTitle>
              <p className="mt-0.5 text-xs text-muted-foreground">
                {t(($) => $.create.method[method].desc)}
              </p>
            </div>
          </div>
          <Tooltip>
            <TooltipTrigger
              render={
                <button
                  type="button"
                  onClick={onClose}
                  className="rounded-sm p-1 text-muted-foreground opacity-70 transition-opacity hover:bg-accent/60 hover:opacity-100"
                  aria-label={t(($) => $.create.close_aria)}
                >
                  <XIcon className="h-3.5 w-3.5" />
                </button>
              }
            />
            <TooltipContent side="bottom">{t(($) => $.create.close)}</TooltipContent>
          </Tooltip>
        </div>

        {/* Method body — each form owns its scroll middle + footer */}
        {method === "chooser" && <MethodChooser onChoose={setMethod} />}
        {method === "manual" && (
          <ManualForm
            onCreated={handleCreated}
            onCancel={() => setMethod("chooser")}
          />
        )}
        {method === "url" && (
          <UrlForm
            onCreated={handleCreated}
            onCancel={() => setMethod("chooser")}
          />
        )}
        {method === "runtime" && (
          <RuntimeLocalSkillImportPanel
            onImported={handleCreated}
            onBulkDone={onClose}
          />
        )}
        {method === "directory" && (
          <DirectoryForm
            onCreated={handleCreated}
            onCancel={() => setMethod("chooser")}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}

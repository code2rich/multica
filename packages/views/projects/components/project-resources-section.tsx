"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  ChevronRight,
  FolderGit,
  FolderOpen,
  Pencil,
  Plus,
  Search,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";
import {
  projectResourcesOptions,
  useCreateProjectResource,
  useDeleteProjectResource,
  useUpdateProjectResource,
} from "@multica/core/projects";
import { useWorkspaceId } from "@multica/core/hooks";
import { useCurrentWorkspace } from "@multica/core/paths";
import type {
  GithubRepoResourceRef,
  LocalDirectoryResourceRef,
  ProjectResource,
} from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@multica/ui/components/ui/popover";
import {
  Tooltip,
  TooltipTrigger,
  TooltipContent,
} from "@multica/ui/components/ui/tooltip";
import { useT } from "../../i18n";

// Project Resources sidebar section.
//
// Type-dispatched at the row + add-flow level. Add a new resource_type by:
//   (1) extending the server validator
//   (2) extending ProjectResourceType in @multica/core/types
//   (3) adding a render case in ResourceRow and an add-control here
function isGithubRef(r: ProjectResource): r is ProjectResource & {
  resource_ref: GithubRepoResourceRef;
} {
  return r.resource_type === "github_repo";
}

function isLocalDirectoryRef(r: ProjectResource): r is ProjectResource & {
  resource_ref: LocalDirectoryResourceRef;
} {
  return r.resource_type === "local_directory";
}

export function ProjectResourcesSection({ projectId }: { projectId: string }) {
  const { t } = useT("projects");
  const wsId = useWorkspaceId();
  const workspace = useCurrentWorkspace();
  const [open, setOpen] = useState(true);
  const [addOpen, setAddOpen] = useState(false);
  const [repoSearch, setRepoSearch] = useState("");

  const { data: resources = [] } = useQuery(
    projectResourcesOptions(wsId, projectId),
  );
  const createResource = useCreateProjectResource(wsId, projectId);
  const updateResource = useUpdateProjectResource(wsId, projectId);
  const deleteResource = useDeleteProjectResource(wsId, projectId);

  // Local directory resources are managed via CLI; the web UI shows them
  // read-only.
  const canEdit = false;
  const localDaemonId = null;

  const attachedUrls = new Set(
    resources.filter(isGithubRef).map((r) => r.resource_ref.url),
  );

  const repoQuery = repoSearch.trim().toLowerCase();
  const filteredRepos =
    workspace?.repos?.filter((repo) => repo.url.toLowerCase().includes(repoQuery)) ?? [];

  const handleAttach = async (url: string) => {
    try {
      await createResource.mutateAsync({
        resource_type: "github_repo",
        resource_ref: { url },
      });
      toast.success(t(($) => $.resources.toast_attached));
    } catch (err) {
      const msg = err instanceof Error ? err.message : t(($) => $.resources.toast_attach_failed);
      toast.error(msg);
    }
  };

  const handleRemove = async (resource: ProjectResource) => {
    try {
      await deleteResource.mutateAsync(resource.id);
      toast.success(t(($) => $.resources.toast_removed));
    } catch (err) {
      toast.error(
        err instanceof Error && err.message
          ? err.message
          : t(($) => $.resources.toast_remove_failed),
      );
    }
  };

  const handleRenameLocalDirectory = async (
    resource: ProjectResource & { resource_ref: LocalDirectoryResourceRef },
    nextLabel: string,
  ) => {
    const trimmed = nextLabel.trim();
    const previous = resource.resource_ref.label ?? resource.label ?? "";
    if (trimmed === previous.trim()) return;
    try {
      await updateResource.mutateAsync({
        resourceId: resource.id,
        data: {
          resource_ref: {
            ...resource.resource_ref,
            label: trimmed,
          },
        },
      });
      toast.success(t(($) => $.resources.toast_local_renamed));
    } catch (err) {
      const msg =
        err instanceof Error
          ? err.message
          : t(($) => $.resources.toast_local_rename_failed);
      toast.error(msg);
    }
  };

  return (
    <div>
      <button
        type="button"
        className={`flex w-full items-center gap-1 rounded-md px-2 py-1 text-xs font-medium transition-colors mb-2 hover:bg-accent/70 ${open ? "" : "text-muted-foreground hover:text-foreground"}`}
        onClick={() => setOpen(!open)}
      >
        {t(($) => $.resources.section_header)}
        <ChevronRight
          className={`!size-3 shrink-0 stroke-[2.5] text-muted-foreground transition-transform ${open ? "rotate-90" : ""}`}
        />
      </button>
      {open && (
        <div className="pl-2 space-y-1.5">
          {resources.length === 0 && (
            <p className="text-xs text-muted-foreground">
              {t(($) => $.resources.empty)}
            </p>
          )}
          {resources.length > 0 && (
            <div className="max-h-64 space-y-1.5 overflow-y-auto pr-1">
              {resources.map((resource) => (
                <ResourceRow
                  key={resource.id}
                  resource={resource}
                  localDaemonId={localDaemonId}
                  canEdit={canEdit}
                  onRemove={() => handleRemove(resource)}
                  onRenameLocalDirectory={handleRenameLocalDirectory}
                />
              ))}
            </div>
          )}
          <Popover
            open={addOpen}
            onOpenChange={(v) => {
              setAddOpen(v);
              if (!v) setRepoSearch("");
            }}
          >
            <PopoverTrigger
              render={
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-7 px-2 text-xs text-muted-foreground hover:text-foreground"
                >
                  <Plus className="size-3" />
                  {t(($) => $.resources.add_button)}
                </Button>
              }
            />
            <PopoverContent align="start" className="w-72 p-2 space-y-2">
              <div className="text-xs font-medium text-muted-foreground">
                {t(($) => $.resources.popover_title)}
              </div>
              {workspace?.repos && workspace.repos.length > 0 && (
                <>
                  <div className="relative">
                    <Search className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
                    <input
                      type="text"
                      value={repoSearch}
                      onChange={(e) => setRepoSearch(e.target.value)}
                      aria-label={t(($) => $.resources.repos_search_placeholder)}
                      placeholder={t(($) => $.resources.repos_search_placeholder)}
                      className="h-8 w-full rounded-md border bg-transparent pl-7 pr-2 text-xs outline-none placeholder:text-muted-foreground focus-visible:ring-1 focus-visible:ring-ring"
                    />
                  </div>
                  <div className="max-h-48 space-y-1 overflow-y-auto">
                    {filteredRepos.length === 0 && repoQuery && (
                      <p className="py-2 text-center text-xs text-muted-foreground">
                        {t(($) => $.resources.repos_search_empty)}
                      </p>
                    )}
                    {filteredRepos.map((repo) => {
                      const isAttached = attachedUrls.has(repo.url);
                      const isDisabled = isAttached || createResource.isPending;
                      return (
                        // Use aria-disabled instead of the native `disabled` attribute so
                        // hover events still reach the tooltip trigger on attached rows
                        // (browsers suppress pointer events on disabled form controls).
                        <button
                          key={repo.url}
                          type="button"
                          aria-disabled={isDisabled}
                          onClick={async () => {
                            if (isDisabled) return;
                            await handleAttach(repo.url);
                            setAddOpen(false);
                          }}
                          className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-xs text-left hover:bg-accent transition-colors aria-disabled:opacity-50 aria-disabled:cursor-not-allowed aria-disabled:hover:bg-transparent"
                        >
                          <FolderGit className="size-3.5" />
                          <Tooltip>
                            <TooltipTrigger
                              render={
                                <span className="truncate flex-1">{repo.url}</span>
                              }
                            />
                            <TooltipContent side="top">{repo.url}</TooltipContent>
                          </Tooltip>
                          {isAttached && (
                            <span className="text-[10px] text-muted-foreground">
                              {t(($) => $.resources.attached_badge)}
                            </span>
                          )}
                        </button>
                      );
                    })}
                  </div>
                </>
              )}
              <CustomRepoForm
                onSubmit={async (url) => {
                  await handleAttach(url);
                  setAddOpen(false);
                }}
              />
            </PopoverContent>
          </Popover>
        </div>
      )}
    </div>
  );
}

interface ResourceRowProps {
  resource: ProjectResource;
  localDaemonId: string | null;
  canEdit: boolean;
  onRemove: () => void;
  onRenameLocalDirectory: (
    resource: ProjectResource & { resource_ref: LocalDirectoryResourceRef },
    nextLabel: string,
  ) => Promise<void>;
}

function ResourceRow({
  resource,
  localDaemonId,
  canEdit,
  onRemove,
  onRenameLocalDirectory,
}: ResourceRowProps) {
  const { t } = useT("projects");
  if (isGithubRef(resource)) {
    const ref = resource.resource_ref;
    const display = resource.label || (ref.ref ? `${ref.url} @ ${ref.ref}` : ref.url);
    const tooltip = ref.ref ? `${ref.url}\nref: ${ref.ref}` : ref.url;
    return (
      <div className="flex items-center gap-2 text-xs group">
        <FolderGit className="size-3.5 text-muted-foreground shrink-0" />
        <Tooltip>
          <TooltipTrigger
            render={
              <a
                href={ref.url}
                target="_blank"
                rel="noopener noreferrer"
                className="truncate flex-1 hover:underline"
              >
                {display}
              </a>
            }
          />
          <TooltipContent side="top" className="whitespace-pre-line">{tooltip}</TooltipContent>
        </Tooltip>
        <button
          type="button"
          onClick={onRemove}
          className="opacity-0 group-hover:opacity-100 transition-opacity rounded-sm p-0.5 hover:bg-accent"
          title={t(($) => $.resources.remove_tooltip)}
        >
          <Trash2 className="size-3 text-muted-foreground" />
        </button>
      </div>
    );
  }

  if (isLocalDirectoryRef(resource)) {
    return (
      <LocalDirectoryRow
        resource={resource}
        localDaemonId={localDaemonId}
        canEdit={canEdit}
        onRemove={onRemove}
        onRename={onRenameLocalDirectory}
      />
    );
  }

  return (
    <div className="flex items-center gap-2 text-xs text-muted-foreground">
      <span className="truncate flex-1">
        {resource.label || resource.resource_type}
      </span>
      <button
        type="button"
        onClick={onRemove}
        className="rounded-sm p-0.5 hover:bg-accent"
        title={t(($) => $.resources.remove_tooltip)}
      >
        <Trash2 className="size-3" />
      </button>
    </div>
  );
}

interface LocalDirectoryRowProps {
  resource: ProjectResource & { resource_ref: LocalDirectoryResourceRef };
  localDaemonId: string | null;
  canEdit: boolean;
  onRemove: () => void;
  onRename: (
    resource: ProjectResource & { resource_ref: LocalDirectoryResourceRef },
    nextLabel: string,
  ) => Promise<void>;
}

function LocalDirectoryRow({
  resource,
  localDaemonId,
  canEdit,
  onRemove,
  onRename,
}: LocalDirectoryRowProps) {
  const { t } = useT("projects");
  const ref = resource.resource_ref;
  const display = (ref.label || resource.label || ref.local_path).trim() ||
    ref.local_path;
  const isForeignDaemon =
    localDaemonId !== null && ref.daemon_id !== localDaemonId;
  const isLocalUnknown = localDaemonId === null;
  // "disabled" in the spec sense — visual de-emphasis + no chat hint, and
  // rename is hidden on foreign / unknown-daemon rows because the label
  // belongs to the owning device. Delete stays available so the user can
  // drop a stale registration from any device.
  const mismatch = isForeignDaemon || isLocalUnknown;

  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(display);

  const startEdit = () => {
    setDraft(display);
    setEditing(true);
  };
  const commit = async () => {
    setEditing(false);
    await onRename(resource, draft);
  };
  const cancel = () => {
    setEditing(false);
    setDraft(display);
  };

  return (
    <div
      className={`flex items-center gap-2 text-xs group ${
        mismatch ? "opacity-60" : ""
      }`}
    >
      <FolderOpen className="size-3.5 text-muted-foreground shrink-0" />
      {editing ? (
        <input
          autoFocus
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onBlur={() => void commit()}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              void commit();
            } else if (e.key === "Escape") {
              e.preventDefault();
              cancel();
            }
          }}
          className="flex-1 min-w-0 rounded-sm border bg-transparent px-1 py-0.5 text-xs outline-none focus-visible:ring-1 focus-visible:ring-ring"
          aria-label={t(($) => $.resources.local_rename_label)}
        />
      ) : (
        <Tooltip>
          <TooltipTrigger
            render={
              <span className="truncate flex-1">{display}</span>
            }
          />
          <TooltipContent side="top">
            <div className="space-y-0.5 text-[11px]">
              <div className="font-mono">{ref.local_path}</div>
              {mismatch && (
                <div className="text-muted-foreground">
                  {isLocalUnknown
                    ? t(($) => $.resources.local_no_daemon_tooltip)
                    : t(($) => $.resources.local_other_machine_tooltip)}
                </div>
              )}
            </div>
          </TooltipContent>
        </Tooltip>
      )}
      {canEdit && !mismatch && !editing && (
        <button
          type="button"
          onClick={startEdit}
          className="opacity-0 group-hover:opacity-100 transition-opacity rounded-sm p-0.5 hover:bg-accent"
          title={t(($) => $.resources.local_rename_tooltip)}
        >
          <Pencil className="size-3 text-muted-foreground" />
        </button>
      )}
      <button
        type="button"
        onClick={onRemove}
        className="opacity-0 group-hover:opacity-100 transition-opacity rounded-sm p-0.5 hover:bg-accent"
        title={t(($) => $.resources.remove_tooltip)}
      >
        <Trash2 className="size-3 text-muted-foreground" />
      </button>
    </div>
  );
}

function CustomRepoForm({
  onSubmit,
}: {
  onSubmit: (url: string) => Promise<void> | void;
}) {
  const { t } = useT("projects");
  const [url, setUrl] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const handle = async (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = url.trim();
    if (!trimmed) return;
    setSubmitting(true);
    try {
      await onSubmit(trimmed);
      setUrl("");
    } finally {
      setSubmitting(false);
    }
  };
  return (
    <form onSubmit={handle} className="flex items-center gap-1.5 pt-1 border-t">
      <input
        type="text"
        value={url}
        onChange={(e) => setUrl(e.target.value)}
        placeholder={t(($) => $.resources.url_placeholder)}
        className="flex-1 bg-transparent text-xs px-2 py-1 outline-none placeholder:text-muted-foreground"
      />
      <Button
        type="submit"
        size="sm"
        variant="ghost"
        className="h-6 px-2 text-xs"
        disabled={!url.trim() || submitting}
      >
        {t(($) => $.resources.url_submit)}
      </Button>
    </form>
  );
}



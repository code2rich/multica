"use client";

import { useCallback } from "react";
import { toast } from "sonner";
import { api } from "@multica/core/api";
import { useWorkspaceSlug } from "@multica/core/paths";
import { resolvePublicFileUrl } from "@multica/core/workspace/avatar-url";
import { useT } from "../i18n";

function attachmentDownloadEndpoint(
  attachmentId: string,
  workspaceSlug: string,
): string {
  const params = new URLSearchParams({ workspace_slug: workspaceSlug });
  const path = `/api/attachments/${encodeURIComponent(attachmentId)}/download`;
  const endpoint = `${path}?${params.toString()}`;
  return resolvePublicFileUrl(endpoint) ?? endpoint;
}

function triggerBrowserDownload(url: string): void {
  const anchor = document.createElement("a");
  anchor.href = url;
  // Keep the click in the current browsing context. For same-origin API
  // downloads this hint lets Chromium/Safari use Content-Disposition's
  // filename without opening a blank tab. If the endpoint later 302s to
  // CloudFront/S3, the server signs that redirect with an attachment
  // disposition; the browser follows it natively without buffering the file
  // into JS memory.
  anchor.download = "";
  anchor.rel = "noopener";
  anchor.style.display = "none";
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
}

/**
 * Returns a callback that downloads an attachment by ID.
 *
 * The hook refreshes attachment metadata first so permission/API failures
 * still produce the existing toast instead of a silent failed navigation. It
 * then clicks a temporary same-origin `/api/attachments/{id}/download?workspace_slug=...`
 * anchor. The backend owns CloudFront / S3 presign / proxy selection and the
 * download Content-Disposition, so large files stay in the browser's native
 * download pipeline.
 */
export function useDownloadAttachment(): (attachmentId: string) => Promise<void> {
  const { t } = useT("editor");
  const workspaceSlug = useWorkspaceSlug();
  return useCallback(
    async (attachmentId: string) => {
      const failed = () => toast.error(t(($) => $.attachment.download_failed));

      try {
        await api.getAttachment(attachmentId);
        if (typeof document === "undefined") {
          failed();
          return;
        }
        if (!workspaceSlug) {
          failed();
          return;
        }
        triggerBrowserDownload(
          attachmentDownloadEndpoint(attachmentId, workspaceSlug),
        );
      } catch {
        failed();
      }
    },
    [t, workspaceSlug],
  );
}

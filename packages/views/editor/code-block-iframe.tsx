"use client";

/**
 * Shared HTML preview iframe.
 *
 * Used by:
 *   - InlineHtmlIframe inside AttachmentCard (HTML attachments inline preview)
 *   - CodeBlockView for fenced ```html blocks (editable Tiptap NodeView)
 *   - HtmlBlockPreview for fenced ```html blocks (ReadonlyContent)
 *   - AttachmentPreviewModal's full-screen HTML kind
 *
 * Sandbox semantics:
 *   sandbox="allow-scripts" (NOT "allow-same-origin")
 *   → iframe runs in an opaque origin: scripts execute (chart JS works),
 *     but cookie / localStorage / parent access / top-nav / popups / forms
 *     remain blocked. This is the standard "preview untrusted HTML" model
 *     (HTML spec §iframe sandbox, MDN, Claude artifacts, v0.dev preview).
 *
 * The server-side `text/plain` + `nosniff` defense at
 * /api/attachments/{id}/content remains untouched — we only feed iframe.srcDoc
 * the text body we fetched, never point iframe.src at the proxy URL.
 */

import { useEffect, useRef, useState } from "react";
import { cn } from "@multica/ui/lib/utils";

const AUTO_HEIGHT_MESSAGE = "multica:html-preview-height";
const AUTO_HEIGHT_MIN_PX = 560;
const AUTO_HEIGHT_MAX_PX = 12_000;
const AUTO_HEIGHT_VIEWPORT_OFFSET_PX = 360;

const AUTO_HEIGHT_REPORTER = `<script>
(function(){
  var frame = 0;
  function reportHeight() {
    if (frame) cancelAnimationFrame(frame);
    frame = requestAnimationFrame(function() {
      var body = document.body;
      var root = document.documentElement;
      var height = Math.max(
        body ? body.scrollHeight : 0,
        body ? body.offsetHeight : 0,
        root ? root.scrollHeight : 0,
        root ? root.offsetHeight : 0
      );
      parent.postMessage({ type: '${AUTO_HEIGHT_MESSAGE}', height: height }, '*');
    });
  }
  window.addEventListener('load', reportHeight);
  window.addEventListener('resize', reportHeight);
  document.addEventListener('DOMContentLoaded', reportHeight);
  if (typeof ResizeObserver !== 'undefined') {
    new ResizeObserver(reportHeight).observe(document.documentElement);
  } else if (typeof MutationObserver !== 'undefined') {
    new MutationObserver(reportHeight).observe(document.documentElement, {
      childList: true,
      subtree: true,
      attributes: true
    });
  }
  reportHeight();
})();
</script>`;

function viewportHeightFloor() {
  if (typeof window === "undefined") return AUTO_HEIGHT_MIN_PX;
  return Math.max(
    AUTO_HEIGHT_MIN_PX,
    window.innerHeight - AUTO_HEIGHT_VIEWPORT_OFFSET_PX,
  );
}

interface CodeBlockIframeProps {
  /** Document source for srcDoc. Empty string renders a blank frame. */
  html: string;
  /** Iframe title for accessibility. */
  title: string;
  className?: string;
  /** Tailwind height token; defaults to h-[480px]. */
  heightClassName?: string;
  /** Grow the iframe to its document height, with viewport and safety bounds. */
  autoResize?: boolean;
}

export function CodeBlockIframe({
  html,
  title,
  className,
  heightClassName = "h-[480px]",
  autoResize = false,
}: CodeBlockIframeProps) {
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const [autoHeight, setAutoHeight] = useState(viewportHeightFloor);

  useEffect(() => {
    if (!autoResize) return;

    const updateViewportFloor = () => {
      setAutoHeight((current) => Math.max(current, viewportHeightFloor()));
    };
    const handleMessage = (event: MessageEvent<unknown>) => {
      if (event.source !== iframeRef.current?.contentWindow) return;
      if (!event.data || typeof event.data !== "object") return;

      const payload = event.data as { type?: unknown; height?: unknown };
      if (
        payload.type !== AUTO_HEIGHT_MESSAGE ||
        typeof payload.height !== "number" ||
        !Number.isFinite(payload.height)
      ) {
        return;
      }

      setAutoHeight(
        Math.min(
          AUTO_HEIGHT_MAX_PX,
          Math.max(viewportHeightFloor(), Math.ceil(payload.height)),
        ),
      );
    };

    window.addEventListener("message", handleMessage);
    window.addEventListener("resize", updateViewportFloor);
    return () => {
      window.removeEventListener("message", handleMessage);
      window.removeEventListener("resize", updateViewportFloor);
    };
  }, [autoResize]);

  useEffect(() => {
    if (autoResize) setAutoHeight(viewportHeightFloor());
  }, [autoResize, html]);

  return (
    <iframe
      ref={iframeRef}
      // srcDoc keeps the body in the parent's process but isolated to an
      // opaque origin via sandbox. Critical that we never combine
      // `allow-scripts` with `allow-same-origin` — that pairing defeats the
      // sandbox per the HTML spec (notes on the sandbox attribute).
      srcDoc={autoResize ? html + AUTO_HEIGHT_REPORTER : html}
      sandbox="allow-scripts"
      title={title}
      style={autoResize ? { height: `${autoHeight}px` } : undefined}
      className={cn(
        "w-full rounded-md border border-border bg-background transition-[height] duration-200",
        !autoResize && heightClassName,
        className,
      )}
    />
  );
}

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";

// Hoisted mock for the API singleton: vi.mock factories cannot reference
// outside-of-scope vars, but vi.hoisted runs before the import graph.
const getAttachmentMock = vi.hoisted(() => vi.fn());
const getBaseUrlMock = vi.hoisted(() => vi.fn(() => ""));
const useWorkspaceSlugMock = vi.hoisted(() =>
  vi.fn<() => string | null>(() => "acme"),
);

vi.mock("@multica/core/api", () => ({
  api: { getAttachment: getAttachmentMock, getBaseUrl: getBaseUrlMock },
}));

vi.mock("@multica/core/paths", () => ({
  useWorkspaceSlug: useWorkspaceSlugMock,
}));

vi.mock("sonner", () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}));

vi.mock("../i18n", () => ({
  useT: () => ({
    t: (
      sel: (s: { attachment: { download_failed: string } }) => string,
    ) =>
      sel({
        attachment: {
          download_failed:
            "Couldn't fetch a download link. Try again in a moment.",
        },
      }),
  }),
}));

import { useDownloadAttachment } from "./use-download-attachment";
import { toast } from "sonner";

const SIGNED_URL =
  "https://static.example.test/file.md?Policy=p&Signature=s&Key-Pair-Id=k";

beforeEach(() => {
  vi.clearAllMocks();
  // Default: same-origin API (empty base).
  getBaseUrlMock.mockReturnValue("");
  useWorkspaceSlugMock.mockReturnValue("acme");
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("useDownloadAttachment", () => {
  it("clicks the unified download endpoint without opening a blank tab or buffering a Blob", async () => {
    getAttachmentMock.mockResolvedValueOnce({
      id: "att-1",
      url: "https://static.example.test/file.md",
      // CloudFront mode may still return a signed CDN URL from metadata;
      // Web download must ignore it and enter through the same-origin
      // endpoint so the server owns cloudfront/presign/proxy selection.
      download_url: SIGNED_URL,
      filename: "file.md",
    });

    const openSpy = vi.spyOn(window, "open");
    const clickSpy = vi
      .spyOn(HTMLAnchorElement.prototype, "click")
      .mockImplementation(() => {});
    const appendSpy = vi.spyOn(document.body, "appendChild");

    const { result } = renderHook(() => useDownloadAttachment());

    await act(async () => {
      await result.current("att-1");
    });

    expect(getAttachmentMock).toHaveBeenCalledWith("att-1");
    expect(openSpy).not.toHaveBeenCalled();
    expect(clickSpy).toHaveBeenCalledOnce();

    const anchor = appendSpy.mock.calls
      .map(([node]) => node)
      .find((node): node is HTMLAnchorElement =>
        node instanceof HTMLAnchorElement,
      );
    expect(anchor).toBeDefined();
    expect(anchor!.getAttribute("href")).toBe(
      "/api/attachments/att-1/download?workspace_slug=acme",
    );
    expect(anchor!.href).toBe(
      "http://localhost:3000/api/attachments/att-1/download?workspace_slug=acme",
    );
    // Empty download attribute intentionally defers the final filename to the
    // endpoint / redirected object Content-Disposition header.
    expect(anchor!.getAttribute("download")).toBe("");
    expect(anchor!.isConnected).toBe(false);
  });

  it("resolves the unified download endpoint against a configured API base", async () => {
    getBaseUrlMock.mockReturnValue("https://api.example.test/");
    getAttachmentMock.mockResolvedValueOnce({
      id: "att 1/slash",
      url: "https://static.example.test/file.md",
      download_url: SIGNED_URL,
      filename: "file.md",
    });
    const clickSpy = vi
      .spyOn(HTMLAnchorElement.prototype, "click")
      .mockImplementation(() => {});
    const appendSpy = vi.spyOn(document.body, "appendChild");

    const { result } = renderHook(() => useDownloadAttachment());

    await act(async () => {
      await result.current("att 1/slash");
    });

    expect(clickSpy).toHaveBeenCalledOnce();
    const anchor = appendSpy.mock.calls
      .map(([node]) => node)
      .find((node): node is HTMLAnchorElement =>
        node instanceof HTMLAnchorElement,
      );
    expect(anchor).toBeDefined();
    expect(anchor!.href).toBe(
      "https://api.example.test/api/attachments/att%201%2Fslash/download?workspace_slug=acme",
    );
  });

  it("encodes the workspace slug into the bare navigation URL instead of relying on custom headers", async () => {
    useWorkspaceSlugMock.mockReturnValueOnce("team/space");
    getAttachmentMock.mockResolvedValueOnce({
      id: "att-1",
      url: "https://static.example.test/file.md",
      download_url: SIGNED_URL,
      filename: "file.md",
    });
    const clickSpy = vi
      .spyOn(HTMLAnchorElement.prototype, "click")
      .mockImplementation(() => {});
    const appendSpy = vi.spyOn(document.body, "appendChild");

    const { result } = renderHook(() => useDownloadAttachment());

    await act(async () => {
      await result.current("att-1");
    });

    expect(clickSpy).toHaveBeenCalledOnce();
    const anchor = appendSpy.mock.calls
      .map(([node]) => node)
      .find((node): node is HTMLAnchorElement =>
        node instanceof HTMLAnchorElement,
      );
    expect(anchor).toBeDefined();
    expect(anchor!.getAttribute("href")).toBe(
      "/api/attachments/att-1/download?workspace_slug=team%2Fspace",
    );
  });

  it("shows a toast and does not click a download link when the workspace slug is missing", async () => {
    useWorkspaceSlugMock.mockReturnValueOnce(null);
    getAttachmentMock.mockResolvedValueOnce({
      id: "att-1",
      url: "https://static.example.test/file.md",
      download_url: SIGNED_URL,
      filename: "file.md",
    });
    const clickSpy = vi
      .spyOn(HTMLAnchorElement.prototype, "click")
      .mockImplementation(() => {});

    const { result } = renderHook(() => useDownloadAttachment());

    await act(async () => {
      await result.current("att-1");
    });

    expect(clickSpy).not.toHaveBeenCalled();
    await waitFor(() => expect(toast.error).toHaveBeenCalled());
  });

  it("shows a toast and does not click a download link when the metadata preflight fails", async () => {
    getAttachmentMock.mockRejectedValueOnce(new Error("boom"));
    const openSpy = vi.spyOn(window, "open");
    const clickSpy = vi
      .spyOn(HTMLAnchorElement.prototype, "click")
      .mockImplementation(() => {});

    const { result } = renderHook(() => useDownloadAttachment());

    await act(async () => {
      await result.current("att-1");
    });

    expect(openSpy).not.toHaveBeenCalled();
    expect(clickSpy).not.toHaveBeenCalled();
    await waitFor(() => expect(toast.error).toHaveBeenCalled());
  });
});

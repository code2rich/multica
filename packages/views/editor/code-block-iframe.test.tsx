// @vitest-environment jsdom

import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { CodeBlockIframe } from "./code-block-iframe";

describe("CodeBlockIframe auto resize", () => {
  it("grows to the height reported by its sandboxed document", () => {
    render(
      <CodeBlockIframe
        html="<main>Profile</main>"
        title="Profile preview"
        autoResize
      />,
    );

    const iframe = screen.getByTitle<HTMLIFrameElement>("Profile preview");
    fireEvent(
      window,
      new MessageEvent("message", {
        source: iframe.contentWindow,
        data: { type: "multica:html-preview-height", height: 1_240 },
      }),
    );

    expect(iframe).toHaveStyle({ height: "1240px" });
    expect(iframe.srcdoc).toContain("multica:html-preview-height");
  });

  it("ignores resize messages from a different window", () => {
    render(
      <CodeBlockIframe
        html="<main>Profile</main>"
        title="Profile preview"
        autoResize
      />,
    );

    const iframe = screen.getByTitle<HTMLIFrameElement>("Profile preview");
    const initialHeight = iframe.style.height;
    const otherIframe = document.createElement("iframe");
    document.body.appendChild(otherIframe);

    fireEvent(
      window,
      new MessageEvent("message", {
        source: otherIframe.contentWindow,
        data: { type: "multica:html-preview-height", height: 1_240 },
      }),
    );

    expect(iframe.style.height).toBe(initialHeight);
  });

  it("keeps ordinary previews on their explicit height class", () => {
    render(
      <CodeBlockIframe
        html="<main>Attachment</main>"
        title="Attachment preview"
        heightClassName="h-[320px]"
      />,
    );

    const iframe = screen.getByTitle<HTMLIFrameElement>("Attachment preview");
    expect(iframe).toHaveClass("h-[320px]");
    expect(iframe.style.height).toBe("");
    expect(iframe.srcdoc).not.toContain("multica:html-preview-height");
  });
});

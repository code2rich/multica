import { describe, expect, it } from "vitest";
import { findAgentSourceFile, resolveAgentSourcePath } from "./agent-source-files";

describe("resolveAgentSourcePath", () => {
  it("resolves links from the role root and nested source documents", () => {
    expect(resolveAgentSourcePath("", "agent-soul/PROFILE.yaml")).toBe(
      "agent-soul/PROFILE.yaml",
    );
    expect(resolveAgentSourcePath("agent-soul/BIBLE.md", "../env/.env.example")).toBe(
      "env/.env.example",
    );
  });

  it("rejects external, absolute, fragment, and escaping links", () => {
    expect(resolveAgentSourcePath("", "https://example.com")).toBeNull();
    expect(resolveAgentSourcePath("", "/etc/passwd")).toBeNull();
    expect(resolveAgentSourcePath("", "#section")).toBeNull();
    expect(resolveAgentSourcePath("", "../../secret")).toBeNull();
  });

  it("finds packaged content while preserving an unavailable path", () => {
    const files = [
      { path: "agent-soul/PROFILE.yaml", content: "id: test" },
      { path: "env/.env", content: "TOKEN=secret" },
    ];
    expect(findAgentSourceFile(files, "", "agent-soul/PROFILE.yaml")?.file).toEqual(files[0]);
    expect(findAgentSourceFile(files, "", "env/.env")?.file).toEqual(files[1]);

    expect(findAgentSourceFile(files, "", "missing.md")?.file).toBeUndefined();
  });
});

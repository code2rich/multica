import { describe, it, expect } from "vitest";
import {
  isAgentIconUrl,
  agentIconKeyFromUrl,
  buildAgentIconUrl,
  defaultAgentIconKey,
} from "@multica/ui/lib/agent-icon-url";
import { AGENT_ICON_KEYS } from "@multica/ui/components/common/agent-icons";

describe("isAgentIconUrl", () => {
  it("matches the icon: prefix regardless of key validity", () => {
    expect(isAgentIconUrl("icon:robot")).toBe(true);
    // Prefix-only match: key validity is agentIconKeyFromUrl's concern, so a
    // bogus key still counts as an icon url (it just resolves to the derived
    // default at render time).
    expect(isAgentIconUrl("icon:bogus")).toBe(true);
  });

  it("rejects photo urls, bare strings, and empty input", () => {
    expect(isAgentIconUrl("/uploads/a.png")).toBe(false);
    expect(isAgentIconUrl("https://host/a.png")).toBe(false);
    expect(isAgentIconUrl("robot")).toBe(false);
    expect(isAgentIconUrl("")).toBe(false);
    expect(isAgentIconUrl(null)).toBe(false);
    expect(isAgentIconUrl(undefined)).toBe(false);
  });
});

describe("agentIconKeyFromUrl", () => {
  it("returns the registered key for a known icon url", () => {
    expect(agentIconKeyFromUrl("icon:robot")).toBe("robot");
  });

  it("returns null for an unregistered key", () => {
    expect(agentIconKeyFromUrl("icon:bogus")).toBeNull();
  });

  it("returns null for non-icon urls and empty input", () => {
    expect(agentIconKeyFromUrl("/uploads/a.png")).toBeNull();
    expect(agentIconKeyFromUrl("https://host/a.png")).toBeNull();
    expect(agentIconKeyFromUrl(null)).toBeNull();
  });
});

describe("buildAgentIconUrl", () => {
  it("round-trips every registered key through agentIconKeyFromUrl", () => {
    for (const key of AGENT_ICON_KEYS) {
      const url = buildAgentIconUrl(key);
      expect(url).toBe(`icon:${key}`);
      expect(agentIconKeyFromUrl(url)).toBe(key);
    }
  });
});

describe("defaultAgentIconKey", () => {
  it("is deterministic — same name yields the same key", () => {
    expect(defaultAgentIconKey("Alice")).toBe(defaultAgentIconKey("Alice"));
  });

  it("always returns a registered key, including for edge-case names", () => {
    const names = ["", "  ", "Alice", "Bob", "A very long agent name 42", "机器人"];
    for (const n of names) {
      expect(AGENT_ICON_KEYS).toContain(defaultAgentIconKey(n));
    }
  });

  it("distributes distinct names across the palette", () => {
    const keys = new Set(
      [
        "Alice",
        "Bob",
        "Carol",
        "Dave",
        "Eve",
        "Frank",
        "Grace",
        "Heidi",
      ].map(defaultAgentIconKey),
    );
    // Sanity: a spread of names shouldn't all collide onto one icon.
    expect(keys.size).toBeGreaterThan(1);
  });

  it("trims surrounding whitespace before hashing", () => {
    expect(defaultAgentIconKey("Alice")).toBe(defaultAgentIconKey("  Alice  "));
  });
});

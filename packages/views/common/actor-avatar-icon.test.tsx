// @vitest-environment jsdom

import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import { ActorAvatar } from "@multica/ui/components/common/actor-avatar";

/** Fill of the icon's background disc — the first <circle> in the SVG. */
function discFill(container: HTMLElement): string | null {
  const circle = container.querySelector("svg circle");
  return circle ? circle.getAttribute("fill") : null;
}

describe("ActorAvatar agent icon rendering", () => {
  it("renders the matching built-in icon for an icon: url (no <img>)", () => {
    const { container } = render(
      <ActorAvatar name="A" initials="" isAgent avatarUrl="icon:robot" />,
    );
    expect(container.querySelector("img")).toBeNull();
    expect(container.querySelector("svg")).not.toBeNull();
    // robot's disc is slate (#64748b) — proves the right icon rendered, not a
    // generic placeholder.
    expect(discFill(container)).toBe("#64748b");
  });

  it("falls back to the name-derived icon for an unknown icon key", () => {
    const { container } = render(
      <ActorAvatar name="Alice" initials="" isAgent avatarUrl="icon:bogus" />,
    );
    expect(container.querySelector("img")).toBeNull();
    expect(container.querySelector("svg")).not.toBeNull();
    // Derived icon always has a real disc color.
    expect(discFill(container)).not.toBeNull();
  });

  it("renders a name-derived icon when an agent has no avatar url", () => {
    const { container } = render(
      <ActorAvatar name="Alice" initials="" isAgent />,
    );
    expect(container.querySelector("img")).toBeNull();
    expect(container.querySelector("svg")).not.toBeNull();
  });

  it("still renders an <img> for a real photo url", () => {
    const { container } = render(
      <ActorAvatar name="A" initials="" isAgent avatarUrl="/uploads/a.png" />,
    );
    expect(container.querySelector("img")).not.toBeNull();
    expect(container.querySelector("svg")).toBeNull();
  });

  it("renders initials (no icon) for a member actor", () => {
    const { container } = render(
      <ActorAvatar name="Alice Lee" initials="AL" />,
    );
    expect(container.querySelector("svg")).toBeNull();
    expect(container.textContent).toContain("AL");
  });
});

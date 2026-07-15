import { describe, expect, it } from "vitest";
import { localizedSkillDescription } from "./localized-skill-description";

describe("localizedSkillDescription", () => {
  const skill = {
    description: "English execution description",
    description_zh: "中文展示描述",
  };

  it("uses Chinese presentation text for a Chinese UI", () => {
    expect(localizedSkillDescription(skill, "zh-Hans")).toBe("中文展示描述");
  });

  it("keeps English as the fallback and execution-facing description", () => {
    expect(localizedSkillDescription(skill, "en")).toBe(
      "English execution description",
    );
    expect(
      localizedSkillDescription(
        { description: "English fallback", description_zh: "" },
        "zh-Hans",
      ),
    ).toBe("English fallback");
  });
});

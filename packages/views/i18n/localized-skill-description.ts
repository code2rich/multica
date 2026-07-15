export function localizedSkillDescription(
  skill: { description?: string; description_zh?: string },
  language: string,
) {
  if (language.startsWith("zh") && skill.description_zh?.trim()) {
    return skill.description_zh;
  }
  return skill.description ?? "";
}

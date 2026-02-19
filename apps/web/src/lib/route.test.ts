import { describe, expect, it } from "vitest";
import { normalizeSkillsPath } from "./route";

describe("normalizeSkillsPath", () => {
  it("redirects /skills to /skills/", () => {
    expect(normalizeSkillsPath("/skills")).toBe("/skills/");
  });

  it("keeps other paths unchanged", () => {
    expect(normalizeSkillsPath("/skills/")).toBeNull();
    expect(normalizeSkillsPath("/")).toBeNull();
    expect(normalizeSkillsPath("/dashboard")).toBeNull();
  });
});

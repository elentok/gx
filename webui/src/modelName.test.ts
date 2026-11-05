// @vitest-environment node
import { describe, expect, it } from "vitest";
import { type ModelLabel, modelName } from "./modelName";

describe("modelName", () => {
  const rows: [string, string | undefined, string][] = [
    ["claude-sonnet-5-5", "claude", "Claude Sonnet 5.5"],
    ["claude-sonnet-5-5", undefined, "Claude Sonnet 5.5"],
    ["claude-opus-4-1-20250805", "claude", "Claude Opus 4.1"],
    ["claude-opus-5[1m]", undefined, "Claude Opus 5 (1M)"],
    ["sonnet[1m]", undefined, "Claude Sonnet (1M)"],
    ["claude-opus-5[other]", undefined, "Claude Opus 5"],
    ["sonnet", undefined, "Claude Sonnet"],
    ["opus", "claude", "Claude Opus"],
    ["haiku", undefined, "Claude Haiku"],
    ["claude-3-5-sonnet-20241022", undefined, "claude-3-5-sonnet-20241022"],
    ["claude-3-5-sonnet-20241022", "claude", "Claude claude-3-5-sonnet-20241022"],
    ["gpt-6.1-sol", undefined, "Codex GPT-6.1-Sol"],
    ["gpt-6.1-sol", "codex", "Codex GPT-6.1-Sol"],
    ["", "claude", "Claude CLI default"],
    ["", "codex", "Codex CLI default"],
    ["my-model", "codex", "Codex my-model"],
    ["no model", undefined, "no model"],
  ];

  it.each(rows)("%j with harness %j reads %j", (id, harness, want) => {
    expect(modelName(id, harness)).toBe(want);
  });

  it("rejects a raw string where a ModelLabel is expected", () => {
    const show = (_label: ModelLabel) => undefined;
    // @ts-expect-error a raw string is not a ModelLabel
    show("Claude Sonnet 5.5");
    show(modelName("sonnet"));
  });
});

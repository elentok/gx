import { describe, expect, it } from "vitest";
import { modelName } from "../../modelName";
import { renderThemed } from "../render";
import { HarnessLabel } from "./HarnessLabel";

const full = { harness: "codex", tierLabel: "Strongest", model: modelName("gpt-5.5", "codex"), effort: "xhigh" };
const none = modelName("");

describe("HarnessLabel", () => {
  it.each([
    ["claude", "claude-sonnet-5-5", "Claude Sonnet 5.5"],
    ["codex", "gpt-5.5", "Codex GPT-5.5"],
  ])("names the %s harness through the model name", (harness, id, name) => {
    const { container } = renderThemed(<HarnessLabel {...full} harness={harness} model={modelName(id, harness)} effort="" />);
    expect(container.textContent).toContain(name);
  });

  it("reads the joined name", () => {
    const { container } = renderThemed(<HarnessLabel {...full} />);
    expect(container.textContent).toBe("Codex GPT-5.5 · Strongest · xhigh");
  });

  it("reads <Harness> CLI default before a reply", () => {
    const { container } = renderThemed(<HarnessLabel {...full} model={modelName("", "codex")} effort="" />);
    expect(container.textContent).toContain("Codex CLI default");
  });

  it("shows only the harness when nothing else is known", () => {
    const { container } = renderThemed(<HarnessLabel harness="claude" tierLabel="" model={none} effort="" />);
    expect(container.textContent).toBe("Claude");
  });

  it("shows a note alone when there is no harness", () => {
    const { container } = renderThemed(<HarnessLabel harness="" tierLabel="" model={none} effort="" note="follows default" />);
    expect(container.textContent).toBe("follows default");
  });
});

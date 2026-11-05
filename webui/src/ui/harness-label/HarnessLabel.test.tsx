import { describe, expect, it } from "vitest";
import { modelName } from "../../modelName";
import { renderThemed } from "../render";
import { HarnessLabel } from "./HarnessLabel";

const full = { harness: "codex", tierLabel: "Strongest", model: modelName("gpt-5.5", "codex"), effort: "xhigh", switched: false };
const none = modelName("");

describe("HarnessLabel", () => {
  it.each([
    ["claude", "claude-sonnet-5-5", "Claude Sonnet 5.5"],
    ["codex", "gpt-5.5", "Codex GPT-5.5"],
  ])("names the %s harness through the model name", (harness, id, name) => {
    const { getByText } = renderThemed(<HarnessLabel {...full} harness={harness} model={modelName(id, harness)} effort="" />);
    expect(getByText(name)).toBeTruthy();
  });

  it("shows model with effort, the tier beside it and the tooltip on both", () => {
    const { getByText } = renderThemed(<HarnessLabel {...full} />);
    expect(getByText("Strongest").getAttribute("title")).toBe("Strongest tier");
    expect(getByText("Codex GPT-5.5 · xhigh").getAttribute("title")).toBe("Strongest tier");
  });

  it("reads the joined name in the plain form", () => {
    const { container } = renderThemed(<HarnessLabel {...full} plain />);
    expect(container.textContent).toBe("Codex GPT-5.5 · Strongest · xhigh");
  });

  it("reads <Harness> CLI default before a reply", () => {
    const { getByText } = renderThemed(<HarnessLabel {...full} model={modelName("", "codex")} effort="" />);
    expect(getByText("Codex CLI default")).toBeTruthy();
  });

  it("shows only the harness when nothing else is known", () => {
    const { container } = renderThemed(<HarnessLabel harness="claude" tierLabel="" model={none} effort="" switched={false} />);
    expect(container.textContent).toBe("Claude");
  });

  it("shows a note as its own chip, alone when there is no harness", () => {
    const { container } = renderThemed(<HarnessLabel harness="" tierLabel="" model={none} effort="" switched={false} note="follows default" />);
    expect(container.textContent).toBe("follows default");
  });

  it("notes a hand switch only when switched", () => {
    const on = renderThemed(<HarnessLabel {...full} switched />);
    expect(on.getByText("switched by hand")).toBeTruthy();
    on.unmount();
    const off = renderThemed(<HarnessLabel {...full} />);
    expect(off.queryByText("switched by hand")).toBeNull();
  });
});

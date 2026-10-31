import { describe, expect, it } from "vitest";
import { renderThemed } from "../render";
import { HarnessLabel } from "./HarnessLabel";

const full = { harness: "codex", tierLabel: "Strongest", model: "gpt-5.5", effort: "xhigh", switched: false };

describe("HarnessLabel", () => {
  it.each([
    ["claude", "Claude"],
    ["codex", "Codex"],
  ])("names the %s harness", (harness, name) => {
    const { getByText } = renderThemed(<HarnessLabel {...full} harness={harness} />);
    expect(getByText(name)).toBeTruthy();
  });

  it("shows tier and model with effort, the tooltip on the model", () => {
    const { getByText } = renderThemed(<HarnessLabel {...full} />);
    expect(getByText("Strongest")).toBeTruthy();
    expect(getByText("gpt-5.5 · xhigh").getAttribute("title")).toBe("Strongest tier");
  });

  it("reads Tier · CLI default before a reply", () => {
    const { getByText } = renderThemed(<HarnessLabel {...full} model="" effort="" />);
    expect(getByText("Strongest · CLI default")).toBeTruthy();
  });

  it("shows only the harness when nothing else is known", () => {
    const { container, queryByText } = renderThemed(<HarnessLabel harness="claude" tierLabel="" model="" effort="" switched={false} />);
    expect(queryByText(/·|CLI default/)).toBeNull();
    expect(container.textContent).toBe("Claude");
  });

  it("notes a hand switch only when switched", () => {
    const on = renderThemed(<HarnessLabel {...full} switched />);
    expect(on.getByText("switched by hand")).toBeTruthy();
    on.unmount();
    const off = renderThemed(<HarnessLabel {...full} />);
    expect(off.queryByText("switched by hand")).toBeNull();
  });
});

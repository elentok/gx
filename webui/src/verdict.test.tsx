import { fireEvent, screen, waitFor } from "@testing-library/react";
import { createElement } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { PendingRow } from "./generated/server";
import { VerdictConcurrencyCap, VerdictUsagePrefixClaude, VerdictUsagePrefixCodex } from "./generated/server";
import { renderThemed } from "./ui/render";
import { VerdictLine, verdictText } from "./verdict";

describe("verdictText", () => {
  const cap: PendingRow = {
    address: "gx:e/1",
    agent: "",
    verdict: VerdictConcurrencyCap,
    reason: "epic cap 3/2, global cap 4/4",
  };

  it.each([
    ["creating worktree", 1],
    ["installing dependencies", 2],
    ["starting agent", 3],
    ["sending prompt", 4],
  ])("labels the %s step", (label, position) => {
    expect(verdictText(undefined, "gx:e/1", { label, position })).toBe(`Launching: ${label} (${position}/4)`);
  });

  it("relabels the cap verdict and keeps the server's reason", () => {
    expect(verdictText([cap], "gx:e/1")).toBe("Waiting for a slot: epic cap 3/2, global cap 4/4");
  });

  it("never shows the launch step and the cap wait together", () => {
    expect(verdictText([cap], "gx:e/1", { label: "starting agent", position: 3 })).toBe(
      "Launching: starting agent (3/4)",
    );
  });

  it("renders other verdicts as the server sent them", () => {
    const row: PendingRow = { address: "gx:e/1", agent: "", verdict: "disk low", reason: "5G free" };
    expect(verdictText([row], "gx:e/1")).toBe("disk low: 5G free");
    expect(verdictText([{ ...row, verdict: "claimed" }], "gx:e/1")).toBeUndefined();
  });
});

const gated = `${VerdictUsagePrefixClaude}99% of the weekly limit, resets 2026-11-02T00:00:00Z`;
const gatedCodex = `${VerdictUsagePrefixCodex}99% of the weekly limit, resets 2026-11-02T00:00:00Z`;

describe("VerdictLine usage override", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("has no button on other verdicts", () => {
    renderThemed(createElement(VerdictLine, { text: "rate-limited: resets 2026-11-02T00:00:00Z" }));
    expect(screen.queryByRole("button", { name: "Override" })).toBeNull();
  });

  it("shows the button on a Codex reason", () => {
    renderThemed(createElement(VerdictLine, { text: gatedCodex }));
    expect(screen.getByRole("button", { name: "Override" })).toBeTruthy();
  });

  it.each([
    ["claude", gated],
    ["codex", gatedCodex],
  ])("posts the %s harness to the override route when clicked", async (harness, text) => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ windows: ["weekly"] })));
    vi.stubGlobal("fetch", fetchMock);
    renderThemed(createElement(VerdictLine, { text }));
    fireEvent.click(screen.getByRole("button", { name: "Override" }));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock.mock.calls[0][0]).toBe("/v1/usage/override");
    expect(fetchMock.mock.calls[0][1].method).toBe("POST");
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ harness });
  });

  it("shows a refusal inline", async () => {
    const override = vi.fn().mockResolvedValue({ refused: true, message: "no usage window is at its limit" });
    renderThemed(createElement(VerdictLine, { text: gated, overrideUsage: override }));
    fireEvent.click(screen.getByRole("button", { name: "Override" }));
    expect(await screen.findByText("no usage window is at its limit")).toBeTruthy();
  });
});

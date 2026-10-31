import { fireEvent, screen, waitFor } from "@testing-library/react";
import { createElement } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { VerdictUsagePrefixClaude } from "./generated/server";
import { renderThemed } from "./ui/render";
import { VerdictLine } from "./verdict";

const gated = `${VerdictUsagePrefixClaude}99% of the weekly limit, resets 2026-11-02T00:00:00Z`;

describe("VerdictLine usage override", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("has no button on other verdicts", () => {
    renderThemed(createElement(VerdictLine, { text: "rate-limited: resets 2026-11-02T00:00:00Z" }));
    expect(screen.queryByRole("button", { name: "Override" })).toBeNull();
  });

  it("posts to the override route when clicked", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ windows: ["weekly"] })));
    vi.stubGlobal("fetch", fetchMock);
    renderThemed(createElement(VerdictLine, { text: gated }));
    fireEvent.click(screen.getByRole("button", { name: "Override" }));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock.mock.calls[0][0]).toBe("/v1/usage/override");
    expect(fetchMock.mock.calls[0][1].method).toBe("POST");
  });

  it("shows a refusal inline", async () => {
    const override = vi.fn().mockResolvedValue({ refused: true, message: "no usage window is at its limit" });
    renderThemed(createElement(VerdictLine, { text: gated, overrideUsage: override }));
    fireEvent.click(screen.getByRole("button", { name: "Override" }));
    expect(await screen.findByText("no usage window is at its limit")).toBeTruthy();
  });
});

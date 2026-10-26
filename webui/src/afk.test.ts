import { screen } from "@testing-library/react";
import { createElement } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { postAfk, toggleAfk } from "./afk";
import { renderThemed } from "./ui/render";

describe("postAfk", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("posts the wanted mode as JSON", async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true });
    vi.stubGlobal("fetch", fetchMock);
    await postAfk(true);
    expect(fetchMock).toHaveBeenCalledWith("/v1/afk", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: '{"on":true}',
    });
  });

  it("rejects when the server refuses", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("not allowed", { status: 403 })));
    await expect(postAfk(false)).rejects.toThrow("not allowed");
  });

  it("shows an error toast when the write fails", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("AFK is locked", { status: 500 })));
    renderThemed(createElement("div"));
    toggleAfk(false);
    expect(await screen.findByText("AFK is locked")).toBeTruthy();
  });
});

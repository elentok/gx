import { afterEach, describe, expect, it, vi } from "vitest";
import { postAfk } from "./afk";

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
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: false, status: 403 }));
    await expect(postAfk(false)).rejects.toThrow("HTTP 403");
  });
});

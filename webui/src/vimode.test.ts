import { renderHook, screen } from "@testing-library/react";
import { createElement } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ConnectionContext, type Connection } from "./connection";
import { postViMode, toggleViMode, useViMode } from "./vimode";
import { renderThemed } from "./ui/render";

describe("postViMode", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("posts the wanted mode as JSON", async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true });
    vi.stubGlobal("fetch", fetchMock);
    await postViMode(true);
    expect(fetchMock).toHaveBeenCalledWith("/v1/vimode", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: '{"on":true}',
    });
  });

  it("rejects when the server refuses", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("not allowed", { status: 403 })));
    await expect(postViMode(false)).rejects.toThrow("not allowed");
  });

  it("shows an error toast when the write fails", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("Vi mode is locked", { status: 500 })));
    renderThemed(createElement("div"));
    toggleViMode(false);
    expect(await screen.findByText("Vi mode is locked")).toBeTruthy();
  });
});

describe("useViMode", () => {
  const conn = (vi_mode?: boolean) => {
    const state = { snapshot: { vi_mode }, connected: true };
    return { subscribe: () => () => {}, getState: () => state } as unknown as Connection;
  };

  it("returns the snapshot value", () => {
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      createElement(ConnectionContext.Provider, { value: conn(true) }, children);
    expect(renderHook(() => useViMode(), { wrapper }).result.current).toBe(true);
  });

  it("returns false without a provider and does not throw", () => {
    expect(renderHook(() => useViMode()).result.current).toBe(false);
  });
});

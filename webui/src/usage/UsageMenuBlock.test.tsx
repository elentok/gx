import { act, cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Connection, EventListener } from "../connection";
import {
  EventBudgetChanged,
  EventLimitsChanged,
  UsageModeLimits,
  UsageModeSpending,
  type UsageSummary,
} from "../generated/server";
import { renderThemed } from "../ui/render";
import { TopBar } from "../ui/top-bar/TopBar";
import { UsageMenuBlock } from "./UsageMenuBlock";
import type { FetchSummary } from "./useUsageSummary";

const summary = (mode: string): UsageSummary =>
  ({ mode, limits: { windows: [] }, spend: { today: 0, since_sunday: 0, since_first: 0 } }) as unknown as UsageSummary;

function fakeConn() {
  const listeners = new Set<EventListener>();
  const conn = {
    onEvent(l: EventListener) {
      listeners.add(l);
      return () => listeners.delete(l);
    },
  } as unknown as Connection;
  return { conn, listeners, emit: (type: string) => act(() => listeners.forEach((l) => l(undefined, type))) };
}

function setup(fetcher: FetchSummary) {
  const c = fakeConn();
  renderThemed(
    <TopBar connected build="v1" onToggleAfk={() => {}} menuBlock={<UsageMenuBlock conn={c.conn} fetcher={fetcher} />} />,
  );
  return {
    ...c,
    open: () => fireEvent.click(screen.getByRole("button", { name: "Menu" })),
    flush: () => act(async () => {}),
  };
}

beforeEach(() => vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] }));
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("UsageMenuBlock", () => {
  it("fetches once on open, not while closed, and stops listening on close", async () => {
    const fetcher = vi.fn<FetchSummary>(async () => summary(UsageModeLimits));
    const t = setup(fetcher);
    expect(fetcher).not.toHaveBeenCalled();
    expect(t.listeners.size).toBe(0);
    t.open();
    await screen.findByText("Claude limits");
    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(t.listeners.size).toBe(1);
    fireEvent.click(screen.getByRole("button", { name: "Menu" }));
    await vi.waitFor(() => expect(t.listeners.size).toBe(0));
    act(() => void vi.advanceTimersByTime(120_000));
    expect(fetcher).toHaveBeenCalledTimes(1);
  });

  it("refetches every 60 seconds while open", async () => {
    const fetcher = vi.fn<FetchSummary>(async () => summary(UsageModeLimits));
    const t = setup(fetcher);
    t.open();
    await screen.findByText("Claude limits");
    act(() => void vi.advanceTimersByTime(60_000));
    expect(fetcher).toHaveBeenCalledTimes(2);
  });

  it("refetches on limits-changed and budget-changed only", async () => {
    const fetcher = vi.fn<FetchSummary>(async () => summary(UsageModeLimits));
    const t = setup(fetcher);
    t.open();
    await screen.findByText("Claude limits");
    await t.emit(EventLimitsChanged);
    expect(fetcher).toHaveBeenCalledTimes(2);
    await t.emit(EventBudgetChanged);
    expect(fetcher).toHaveBeenCalledTimes(3);
    await t.emit("ticket-changed");
    await t.emit("health-changed");
    expect(fetcher).toHaveBeenCalledTimes(3);
  });

  it("renders nothing, no separators, before the first fetch and after a failed refresh", async () => {
    let resolve!: (s: UsageSummary) => void;
    const fetcher = vi.fn<FetchSummary>(() => new Promise((r) => (resolve = r)));
    const t = setup(fetcher);
    t.open();
    const menu = (await screen.findByRole("menuitem", { name: "Settings" })).closest("[role=menu]")!;
    expect(menu.querySelectorAll("[role=separator]").length).toBe(0);
    await act(async () => resolve(summary(UsageModeLimits)));
    expect(menu.querySelectorAll("[role=separator]").length).toBe(2);

    fetcher.mockRejectedValueOnce(new Error("down"));
    await t.emit(EventLimitsChanged);
    await t.flush();
    expect(screen.queryByText("Claude limits")).toBeNull();
    expect(menu.querySelectorAll("[role=separator]").length).toBe(0);

    fetcher.mockResolvedValueOnce(summary(UsageModeLimits));
    await t.emit(EventLimitsChanged);
    await t.flush();
    expect(screen.getByText("Claude limits")).toBeTruthy();
  });

  it("shows the title of the response mode only", async () => {
    const fetcher = vi.fn<FetchSummary>(async () => summary(UsageModeSpending));
    const t = setup(fetcher);
    t.open();
    await screen.findByText("gx spend (est.)");
    expect(screen.queryByText("Claude limits")).toBeNull();
  });

  it("orders the menu: Settings, AFK, separator, block, separator, build", async () => {
    const t = setup(async () => summary(UsageModeLimits));
    t.open();
    await screen.findByText("Claude limits");
    const menu = screen.getByRole("menuitem", { name: "Settings" }).closest("[role=menu]")!;
    const kinds = [...menu.children].map((el) => el.getAttribute("role") ?? el.getAttribute("data-testid") ?? el.textContent);
    expect(kinds).toEqual(["menuitem", "menuitemcheckbox", "separator", "Claude limits", "separator", "server-build"]);
  });

  it("does not let an older slow response replace a newer one", async () => {
    const resolvers: Array<(s: UsageSummary) => void> = [];
    const fetcher = vi.fn<FetchSummary>(() => new Promise((r) => resolvers.push(r)));
    const t = setup(fetcher);
    t.open();
    await t.flush();
    await t.emit(EventLimitsChanged);
    expect(resolvers.length).toBe(2);
    await act(async () => resolvers[1](summary(UsageModeSpending)));
    await act(async () => resolvers[0](summary(UsageModeLimits)));
    expect(screen.getByText("gx spend (est.)")).toBeTruthy();
    expect(screen.queryByText("Claude limits")).toBeNull();
  });
});

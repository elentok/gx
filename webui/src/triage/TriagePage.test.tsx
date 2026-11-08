import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { MapStep, SessionInfo, TriageView } from "../generated/server";
import { appRoutes, type RouteFetchers } from "../routes";
import { renderThemed } from "../ui/render";

afterEach(cleanup);

const STEPS: [string, string][] = [
  ["verify", "Verify"],
  ["plan", "Plan"],
  ["approve", "Approve"],
  ["write", "Write"],
  ["done", "Done"],
];
const stepsAt = (current: number): MapStep[] =>
  STEPS.map(([step, label], i) => ({ step, label, state: i < current ? "done" : i === current ? "current" : "todo" }));

const row = (n: number, verdict?: string) => ({ address: `gx:follow-ups/${n}`, title: `Row ${n}`, verdict });

const viewOf = (over: Partial<TriageView> = {}): TriageView => ({
  project: "gx",
  state: "live",
  flow: "triage",
  steps: stepsAt(0),
  step_note: "2 of 3 verdicts saved",
  ring_percent: 20,
  rows: [row(1, "relevant"), row(2, "obsolete"), row(3)],
  open_questions: 0,
  action_counts: {},
  changed_rows: 0,
  destinations: [],
  can_abandon: false,
  history: [],
  ...over,
});

const session = (state: "idle" | "working" | "exited", over: Partial<SessionInfo> = {}): SessionInfo => ({
  id: "s-triage",
  project: "gx",
  dir: "/x",
  harness: "claude",
  flow: "triage",
  flow_key: "gx",
  status: { state },
  status_changed_at: new Date().toISOString(),
  run_started_at: new Date().toISOString(),
  unread: {},
  actionable: {},
  auto_advance: false,
  auto_queue: false,
  ...over,
});

function render(sessions: SessionInfo[], fetchers: RouteFetchers = {}) {
  const state = { snapshot: { sessions }, connected: true, loadedOnce: true, tick: 0 } as unknown as ConnectionState;
  const events = new Set<(address: string | undefined, type?: string) => void>();
  const conn = {
    getState: () => state,
    subscribe: () => () => {},
    onEvent: (l: (address: string | undefined, type?: string) => void) => (events.add(l), () => events.delete(l)),
    refresh: () => {},
  } as unknown as Connection;
  const router = createMemoryRouter(
    appRoutes({ epicList: async () => ({ epics: [], follow_ups: [] }), triage: async () => ({ kind: "ok", detail: viewOf() }), ...fetchers }),
    { initialEntries: ["/triage/gx"] },
  );
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
  return { emit: (address: string, type = "triage-changed") => act(() => events.forEach((l) => l(address, type))) };
}

const onPhone = async (body: () => Promise<void>) => {
  vi.stubGlobal("matchMedia", () => ({ matches: false, addEventListener() {}, removeEventListener() {} }));
  try {
    await body();
  } finally {
    vi.unstubAllGlobals();
  }
};

test("the Verify header shows the title, the step summary, five steps with the first current, and the follow-ups link", async () => {
  render([session("working")]);
  expect((await screen.findByRole("heading", { level: 1 })).textContent).toBe("Triage: gx");
  expect((await screen.findByTestId("page-header-subtitle")).textContent).toBe("Verify · 2 of 3 verdicts saved");
  const steps = document.querySelectorAll("[data-step]");
  expect([...steps].map((s) => s.getAttribute("data-step"))).toEqual(["verify", "plan", "approve", "write", "done"]);
  expect(steps[0].getAttribute("data-state")).toBe("current");
  expect(screen.getByRole("link", { name: "Triage of gx:follow-ups ↗" }).getAttribute("href")).toBe("#/e/gx:follow-ups");
  expect(document.title).toBe("gx · Triage · gx");
});

test("the strip names the verdicts saved while a session runs", async () => {
  render([session("working")]);
  await screen.findByText(/Triage session running · verdicts saved 2 of 3/);
});

test("Continue is disabled while a session runs and enabled once it is stopped, and posts the open route", async () => {
  const openTriage = vi.fn(async () => ({}));
  render([session("working")], { openTriage });
  const button = (await screen.findByRole("button", { name: "Continue" })) as HTMLButtonElement;
  expect(button.disabled).toBe(true);
  cleanup();
  render([session("exited")], { openTriage });
  const stopped = (await screen.findByRole("button", { name: "Continue" })) as HTMLButtonElement;
  expect(stopped.disabled).toBe(false);
  fireEvent.click(stopped);
  await waitFor(() => expect(openTriage).toHaveBeenCalledWith({ epic: "gx" }));
});

test("with no session the header button reads Start", async () => {
  render([]);
  expect(((await screen.findByRole("button", { name: "Start" })) as HTMLButtonElement).disabled).toBe(false);
});

test("a triage-changed event for the project refetches the view", async () => {
  const triage = vi.fn(async () => ({ kind: "ok" as const, detail: viewOf() }));
  const { emit } = render([session("working")], { triage });
  await screen.findByRole("heading", { level: 1 });
  const calls = triage.mock.calls.length;
  emit("gx");
  await waitFor(() => expect(triage.mock.calls.length).toBeGreaterThan(calls));
});

test("on a phone there is no Progress panel", () =>
  onPhone(async () => {
    render([session("working")]);
    await screen.findByRole("heading", { level: 1 });
    expect(screen.queryByRole("region", { name: "Progress" })).toBeNull();
  }));

test("a wide page shows the Progress panel", async () => {
  render([session("working")]);
  await screen.findByRole("heading", { level: 1 });
  expect(screen.queryByRole("region", { name: "Progress" })).toBeTruthy();
});

test("a project with no record shows the empty state", async () => {
  render([], { triage: async () => ({ kind: "not-found" }) });
  await screen.findByText("No Triage for gx");
  cleanup();
  render([], { triage: async () => ({ kind: "ok", detail: viewOf({ state: "none" }) }) });
  await screen.findByText("No Triage for gx");
});

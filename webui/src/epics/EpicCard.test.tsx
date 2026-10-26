import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import type { EpicListRow } from "../generated/server";
import { renderThemed } from "../ui/render";
import { EpicCard } from "./EpicCard";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

const row = (over: Partial<EpicListRow> = {}): EpicListRow => ({
  address: "gx:blog",
  project: "gx",
  name: "blog",
  kind: "loop",
  state: "running",
  done: 2,
  total: 5,
  queued: false,
  queueable: false,
  blocked_by_epics: [],
  addresses: ["gx:blog"],
  card_kind: "implementation",
  section: "in-progress",
  status_words: "Implementing...",
  ring_progress: 40,
  actions: [],
  ...over,
});

const needsYou = (over: Partial<EpicListRow> = {}) =>
  row({
    state: "paused",
    section: "needs-you",
    state_pill: "Paused",
    reason: "06 Retry the launch: zero commits",
    actions: [{ kind: "open-ticket", label: "Open ticket", target: "gx:blog/06" }],
    ...over,
  });

const text = (el: HTMLElement) => el.textContent ?? "";

test("the title is the only link and points at the epic page", () => {
  renderThemed(<EpicCard row={needsYou()} />);
  const links = screen.getAllByRole("link");
  expect(links).toHaveLength(1);
  expect(links[0].getAttribute("href")).toBe("#/e/gx:blog");
});

test("title targets: one-off ticket, follow-ups epic, joined card's active part, pending Design", () => {
  const href = (r: EpicListRow) => {
    renderThemed(<EpicCard row={r} />);
    const h = screen.getByRole("link").getAttribute("href");
    cleanup();
    return h;
  };
  expect(href(row({ address: "gx:one-offs/03", card_kind: "one-off", addresses: ["gx:one-offs/03"] }))).toBe("#/t/gx:one-offs/03");
  expect(href(row({ address: "gx:follow-ups", card_kind: "follow-ups", addresses: ["gx:follow-ups"] }))).toBe("#/e/gx:follow-ups");
  expect(href(row({ address: "gx:blog", addresses: ["gx:blog-impl", "gx:blog"] }))).toBe("#/e/gx:blog-impl");
  expect(href(row({ address: "gx:new", card_kind: "design", map_pending: true }))).toBe("#/d/gx:new");
});

test("the pill and reason show on a Needs-you card; a quiet card shows neither", () => {
  renderThemed(<EpicCard row={needsYou()} />);
  expect(text(screen.getByTestId("pill"))).toContain("Paused");
  expect(text(screen.getByTestId("reason"))).toContain("zero commits");
  cleanup();
  renderThemed(<EpicCard row={row()} />);
  expect(screen.queryByTestId("pill")).toBeNull();
  expect(screen.queryByTestId("reason")).toBeNull();
});

test("a non-Needs-you card shows a reason but no pill", () => {
  renderThemed(<EpicCard row={row({ state: "queued", reason: "Starts after agent-runner-phase2" })} />);
  expect(screen.queryByTestId("pill")).toBeNull();
  expect(text(screen.getByTestId("reason"))).toContain("Starts after");
});

test.each([
  ["queued", { state: "queued", status_words: "Queued #1" }],
  ["running", {}],
  ["finished", { state: "finished", section: "finished", status_words: "Merged" }],
  ["draft", { state: "draft", section: "ready-to-design", status_words: "Draft" }],
])("%s card has no footer", (_, over) => {
  renderThemed(<EpicCard row={row(over)} />);
  expect(screen.queryByRole("button")).toBeNull();
  expect(screen.queryByTestId("footer")).toBeNull();
});

test("ring labels: percent, n/m, ellipsis, empty", () => {
  renderThemed(<EpicCard row={row({ card_kind: "design", ring_progress: 57 })} />);
  expect(screen.getByText("57%")).toBeTruthy();
  cleanup();
  renderThemed(<EpicCard row={row()} />);
  expect(screen.getByText("2/5")).toBeTruthy();
  cleanup();
  renderThemed(<EpicCard row={row({ card_kind: "design", status_words: "Creating tickets (3 so far)..." })} />);
  expect(screen.getByText("…")).toBeTruthy();
  expect(screen.getByTestId("comet")).toBeTruthy();
  cleanup();
  renderThemed(<EpicCard row={row({ card_kind: "one-off", state: "queued" })} />);
  expect(screen.queryByText(/%|\d\/\d/)).toBeNull();
});

test("Queue posts in place, then reports the change", async () => {
  const queue = vi.fn().mockResolvedValue({});
  const onChanged = vi.fn();
  const r = row({ state: "ready", section: "ready-to-queue", actions: [{ kind: "queue", label: "Queue" }] });
  renderThemed(<EpicCard row={r} writes={{ queue }} onChanged={onChanged} />);
  fireEvent.click(screen.getByRole("button", { name: "Queue" }));
  await waitFor(() => expect(onChanged).toHaveBeenCalled());
  expect(queue).toHaveBeenCalledWith({ address: "gx:blog", state: "ready" });
});

test("a refused Queue shows its reason next to the button", async () => {
  const queue = vi.fn().mockResolvedValue({ refused: true, reason: "epic-draft", message: "gx:blog is a draft" });
  const onChanged = vi.fn();
  const r = row({ state: "ready", section: "ready-to-queue", actions: [{ kind: "queue", label: "Queue" }] });
  renderThemed(<EpicCard row={r} writes={{ queue }} onChanged={onChanged} />);
  fireEvent.click(screen.getByRole("button", { name: "Queue" }));
  const alert = await within(screen.getByTestId("footer")).findByRole("alert");
  expect(text(alert)).toContain("gx:blog is a draft");
  expect(onChanged).not.toHaveBeenCalled();
});

test("a refused Merge shows its reason next to the button", async () => {
  const merge = vi.fn().mockResolvedValue({ refused: true, reason: "x", message: "Conflicts with main" });
  const r = row({ state: "merge-blocked", section: "needs-you", state_pill: "Merge blocked", actions: [{ kind: "merge", label: "Merge" }] });
  renderThemed(<EpicCard row={r} writes={{ merge }} />);
  fireEvent.click(screen.getByRole("button", { name: "Merge" }));
  const alert = await within(screen.getByTestId("footer")).findByRole("alert");
  expect(text(alert)).toContain("Conflicts with main");
});

test("Design posts open-design, then navigates to the Design page", async () => {
  const openDesign = vi.fn().mockResolvedValue({});
  const navigate = vi.fn();
  const r = row({ card_kind: "design", state: "waiting-to-design", actions: [{ kind: "design", label: "Design", target: "gx:blog" }] });
  renderThemed(<EpicCard row={r} writes={{ openDesign, navigate }} />);
  fireEvent.click(screen.getByRole("button", { name: "Design" }));
  await waitFor(() => expect(navigate).toHaveBeenCalledWith("#/d/gx:blog"));
  expect(openDesign).toHaveBeenCalledWith({ epic: "gx:blog" });
});

test("a refused Design stays on the page and says why", async () => {
  const openDesign = vi.fn().mockResolvedValue({ refused: true, message: "Already running" });
  const navigate = vi.fn();
  const r = row({ card_kind: "design", actions: [{ kind: "continue", label: "Continue", target: "gx:blog" }] });
  renderThemed(<EpicCard row={r} writes={{ openDesign, navigate }} />);
  fireEvent.click(screen.getByRole("button", { name: "Continue" }));
  expect(text(await screen.findByRole("alert"))).toContain("Already running");
  expect(navigate).not.toHaveBeenCalled();
});

test.each([
  ["answer", "gx:blog/06", "#/t/gx:blog/06"],
  ["open-ticket", "gx:blog/06", "#/t/gx:blog/06"],
  ["open-epic", "gx:blog", "#/e/gx:blog"],
])("%s navigates", (kind, target, href) => {
  const navigate = vi.fn();
  renderThemed(<EpicCard row={needsYou({ actions: [{ kind, label: "Go", target }] })} writes={{ navigate }} />);
  fireEvent.click(screen.getByRole("button", { name: "Go" }));
  expect(navigate).toHaveBeenCalledWith(href);
});

test("Answer opens the link the server sent, even on a joined implementation card", () => {
  const navigate = vi.fn();
  const answer = { kind: "answer", label: "Answer", target: "gx:blog", link: "#/d/gx:blog" };
  renderThemed(<EpicCard row={needsYou({ card_kind: "implementation", actions: [answer] })} writes={{ navigate }} />);
  fireEvent.click(screen.getByRole("button", { name: "Answer" }));
  expect(navigate).toHaveBeenCalledWith("#/d/gx:blog");
});

test("Triage opens a dialog with the command and a copy button", async () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.assign(navigator, { clipboard: { writeText } });
  const r = row({ card_kind: "follow-ups", actions: [{ kind: "triage", label: "Triage", target: "gx" }] });
  renderThemed(<EpicCard row={r} />);
  fireEvent.click(screen.getByRole("button", { name: "Triage" }));
  const dialog = screen.getByRole("dialog");
  expect(text(dialog)).toContain("Triage UI not yet implemented, for now run:");
  expect(text(dialog)).toContain("/gx-triage-follow-ups --project gx");
  fireEvent.click(within(dialog).getByRole("button", { name: "Copy" }));
  await waitFor(() => expect(writeText).toHaveBeenCalledWith("/gx-triage-follow-ups --project gx"));
});

function openTriage() {
  const r = row({ card_kind: "follow-ups", actions: [{ kind: "triage", label: "Triage", target: "gx" }] });
  renderThemed(<EpicCard row={r} />);
  fireEvent.click(screen.getByRole("button", { name: "Triage" }));
  return screen.getByRole("dialog");
}

describe("Triage copy without navigator.clipboard", () => {
  beforeEach(() => {
    Object.assign(navigator, { clipboard: undefined });
  });

  test("falls back to execCommand and shows Copied", async () => {
    const execCommand = vi.fn().mockReturnValue(true);
    Object.assign(document, { execCommand });
    const dialog = openTriage();
    fireEvent.click(within(dialog).getByRole("button", { name: "Copy" }));
    await waitFor(() => expect(within(dialog).getByRole("button", { name: "Copied" })).toBeTruthy());
    expect(execCommand).toHaveBeenCalledWith("copy");
  });

  test("shows Copy failed and selects the command when execCommand fails", async () => {
    Object.assign(document, { execCommand: vi.fn().mockReturnValue(false) });
    const dialog = openTriage();
    fireEvent.click(within(dialog).getByRole("button", { name: "Copy" }));
    await waitFor(() => expect(within(dialog).getByRole("button", { name: "Copy failed" })).toBeTruthy());
    expect(window.getSelection()?.toString()).toBe("/gx-triage-follow-ups --project gx");
  });
});

test("a countdown reason ticks from the deadline", () => {
  vi.useFakeTimers();
  const ends = new Date(Date.now() + 12_000).toISOString();
  renderThemed(<EpicCard row={row({ card_kind: "design", countdown_ends_at: ends })} />);
  expect(text(screen.getByTestId("reason"))).toContain("Next run starts in 12s");
  act(() => {
    vi.advanceTimersByTime(3000);
  });
  expect(text(screen.getByTestId("reason"))).toContain("Next run starts in 9s");
});

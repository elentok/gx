import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { HoldQueuedText, ReasonHoldLiveSessions, type Workspace } from "../generated/server";
import { renderThemed } from "../ui/render";
import { WideContext } from "../useWide";
import { ItemMenu, type ItemRef, type ItemWriters } from "./itemActions";

afterEach(cleanup);

const workspaces: Workspace[] = [
  { name: "Default", default: true, members: 1 },
  { name: "Work", members: 2 },
];

const writers = (over: Partial<ItemWriters> = {}): ItemWriters => ({
  move: vi.fn(async () => ({})),
  hold: vi.fn(async () => ({ held: [] })),
  unhold: vi.fn(async () => ({ held: [] })),
  ...over,
});

const design: ItemRef = { key: "p:shop", name: "Shop", kind: "design", workspace: "Work" };

async function open(item: ItemRef, w: ItemWriters) {
  renderThemed(
    <WideContext.Provider value={true}>
      <ItemMenu item={item} workspaces={workspaces} writers={w} />
    </WideContext.Provider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "More actions" }));
  await screen.findAllByRole("menuitem");
}

const labels = () => screen.getAllByRole("menuitem").map((i) => i.textContent);

test("a Design offers Move and Hold, never Remove; Move lists every workspace with the current one marked and posts the pick", async () => {
  const w = writers();
  await open(design, w);
  expect(labels()).toEqual(["Move to workspace›", "Hold"]);
  fireEvent.click(screen.getByRole("menuitem", { name: "Move to workspace" }));
  const work = await screen.findByRole("menuitem", { name: /Work/ });
  expect(work.getAttribute("aria-current")).toBe("true");
  expect(work.textContent).toContain("✓");
  const dflt = screen.getByRole("menuitem", { name: "Default" });
  expect(dflt.getAttribute("aria-current")).toBe("false");
  fireEvent.click(dflt);
  await waitFor(() => expect(w.move).toHaveBeenCalledWith("p:shop", "Default"));
});

test("a Default member lists Default as the current workspace", async () => {
  await open({ ...design, workspace: "Default" }, writers());
  fireEvent.click(screen.getByRole("menuitem", { name: "Move to workspace" }));
  expect((await screen.findByRole("menuitem", { name: /Default/ })).getAttribute("aria-current")).toBe("true");
});

test("a queued epic's Hold is disabled with the reason", async () => {
  const w = writers();
  await open({ ...design, kind: "epic", queued: true }, w);
  const hold = screen.getByRole("menuitem", { name: /Hold/ });
  expect(hold.textContent).toContain(HoldQueuedText);
  expect(hold.getAttribute("aria-disabled") ?? hold.getAttribute("data-disabled")).not.toBeNull();
});

test("holding a Design with live sessions names them, then re-posts with yes", async () => {
  const hold = vi
    .fn<ItemWriters["hold"]>()
    .mockResolvedValueOnce({ held: [], refused: true, reason: ReasonHoldLiveSessions, sessions: ["scratch", "draft"] })
    .mockResolvedValueOnce({ held: ["p:shop"] });
  await open(design, writers({ hold }));
  fireEvent.click(screen.getByRole("menuitem", { name: "Hold" }));
  expect((await screen.findByText(/scratch, draft/)).textContent).toContain("ends its live sessions");
  expect(hold).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: "Hold and end sessions" }));
  await waitFor(() => expect(hold).toHaveBeenLastCalledWith("p:shop", true));
});

test("Take off hold posts with no confirmation", async () => {
  const w = writers();
  await open({ ...design, held: true }, w);
  fireEvent.click(screen.getByRole("menuitem", { name: "Take off hold" }));
  await waitFor(() => expect(w.unhold).toHaveBeenCalledWith("p:shop"));
});

test("a chat can move but not be held", async () => {
  await open({ ...design, kind: "chat" }, writers());
  expect(labels()).toEqual(["Move to workspace›"]);
});

test("an item with nothing to offer renders no button", () => {
  renderThemed(<ItemMenu item={{ ...design, kind: "chat" }} workspaces={[]} writers={writers()} />);
  expect(screen.queryByRole("button", { name: "More actions" })).toBeNull();
});

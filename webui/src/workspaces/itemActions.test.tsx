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
  remove: vi.fn(async () => ({})),
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

test("a Design in a named workspace offers Move, Remove and Hold; Move posts the picked workspace", async () => {
  const w = writers();
  await open(design, w);
  expect(labels()).toEqual(["Move to workspace", "Remove from workspace", "Hold"]);
  fireEvent.click(screen.getByRole("menuitem", { name: "Move to workspace" }));
  fireEvent.click(await screen.findByRole("button", { name: "Default" }));
  await waitFor(() => expect(w.move).toHaveBeenCalledWith("p:shop", "Default"));
});

test("a Default member has no Remove", async () => {
  await open({ ...design, workspace: "Default" }, writers());
  expect(labels()).toEqual(["Move to workspace", "Hold"]);
});

test("Remove asks first, then posts", async () => {
  const w = writers();
  await open(design, w);
  fireEvent.click(screen.getByRole("menuitem", { name: "Remove from workspace" }));
  expect(w.remove).not.toHaveBeenCalled();
  fireEvent.click(await screen.findByRole("button", { name: "Remove" }));
  await waitFor(() => expect(w.remove).toHaveBeenCalledWith("p:shop"));
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

test("a chat can move and be removed but not held", async () => {
  await open({ ...design, kind: "chat" }, writers());
  expect(labels()).toEqual(["Move to workspace", "Remove from workspace"]);
});

test("an item with nothing to offer renders no button", () => {
  renderThemed(<ItemMenu item={{ ...design, kind: "chat", workspace: "Default" }} workspaces={[{ name: "Default", default: true, members: 1 }]} writers={writers()} />);
  expect(screen.queryByRole("button", { name: "More actions" })).toBeNull();
});

import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { stateToken } from "../design/open";
import type { SessionInfo } from "../generated/server";
import { renderThemed } from "../ui/render";
import { SessionActions } from "./SessionActions";
import { exitedDesigns, removeSession } from "./sessionWrites";

afterEach(cleanup);

const session = (over: Partial<SessionInfo> = {}): SessionInfo => ({
  id: "s-a",
  name: "worker",
  project: "gx",
  dir: "/x",
  harness: "claude",
  status: { state: "idle" },
  status_changed_at: "2026-01-01T00:00:00Z",
  run_started_at: "2026-01-01T00:00:00Z",
  unread: {},
  actionable: {},
  auto_advance: true,
  ...over,
});
const openMenu = async () => fireEvent.click(screen.getByRole("button", { name: "More actions" }));
const exited = (s: SessionInfo): SessionInfo => ({ ...s, status: { state: "exited" }, status_changed_at: "2026-01-01T00:00:05Z" });

test("Remove on a live session stops it, then removes it with the fresh token", async () => {
  const live = session();
  const stop = vi.fn(async () => ({ session: exited(live) }));
  const remove = vi.fn(async () => ({ session: exited(live) }));
  const onRemoved = vi.fn();
  renderThemed(<SessionActions session={live} onRemoved={onRemoved} stop={stop} remove={remove} />);
  await openMenu();
  fireEvent.click(await screen.findByRole("menuitem", { name: "Remove" }));
  fireEvent.click(await screen.findByRole("button", { name: "Remove" }));
  await waitFor(() => expect(onRemoved).toHaveBeenCalled());
  expect(stop).toHaveBeenCalledWith({ id: "s-a", state: stateToken(live) });
  expect(remove).toHaveBeenCalledWith({ id: "s-a", state: stateToken(exited(live)) });
});

test("an exited session offers Remove but no Stop", async () => {
  renderThemed(<SessionActions session={exited(session())} onRemoved={() => {}} />);
  await openMenu();
  expect(await screen.findByRole("menuitem", { name: "Remove" })).toBeTruthy();
  expect(screen.queryByRole("menuitem", { name: "Stop" })).toBeNull();
});

test("a Queue agent has no Stop or Remove", () => {
  renderThemed(<SessionActions session={session({ queue: true })} onRemoved={() => {}} />);
  expect(screen.queryByRole("button")).toBeNull();
});

test("a refused stop skips the removal", async () => {
  const stop = vi.fn(async () => ({ refused: true, reason: "stale" }));
  const remove = vi.fn();
  const res = await removeSession(session(), { stop, remove });
  expect(res.refused).toBe(true);
  expect(remove).not.toHaveBeenCalled();
});

test("exitedDesigns picks only exited design sessions", () => {
  const list = [session({ id: "1", design: true }), exited(session({ id: "2", design: true })), exited(session({ id: "3" }))];
  expect(exitedDesigns(list).map((s) => s.id)).toEqual(["2"]);
});

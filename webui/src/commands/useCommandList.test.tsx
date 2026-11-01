import { act, render } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { SessionCommandsResponse } from "../generated/server";
import { clearCommandListCache, useCommandList, type CommandList, type FetchCommands } from "./useCommandList";

const resp = (names: string[], degraded = false): SessionCommandsResponse => ({
  degraded,
  entries: names.map((name) => ({ name, aliases: [], arg_hint: "", description: "", source: "built-in" })),
});

let last: CommandList;
function Probe({ id, fetcher }: { id: string; fetcher: FetchCommands }) {
  last = useCommandList({ sessionId: id }, "/", fetcher);
  return null;
}
const names = () => last.entries.map((e) => e.name);

function Gated({ fetcher, enabled }: { fetcher: FetchCommands; enabled: boolean }) {
  last = useCommandList({ sessionId: "a" }, "/", fetcher, enabled);
  return null;
}

it("makes no read while disabled, then reads when it turns on", async () => {
  const fetcher = vi.fn<FetchCommands>().mockResolvedValue(resp(["help"]));
  const view = render(<Gated fetcher={fetcher} enabled={false} />);
  await act(async () => {});
  expect(fetcher).not.toHaveBeenCalled();
  view.rerender(<Gated fetcher={fetcher} enabled />);
  await act(async () => {});
  expect(fetcher).toHaveBeenCalledTimes(1);
  expect(names()).toEqual(["help"]);
});

it("re-checks staleness on each open: inside 60 seconds no read, after it a read", async () => {
  const fetcher = vi.fn<FetchCommands>().mockResolvedValue(resp(["help"]));
  const view = render(<Gated fetcher={fetcher} enabled />);
  await act(async () => {});
  view.rerender(<Gated fetcher={fetcher} enabled={false} />);
  view.rerender(<Gated fetcher={fetcher} enabled />);
  await act(async () => {});
  expect(fetcher).toHaveBeenCalledTimes(1);
  view.rerender(<Gated fetcher={fetcher} enabled={false} />);
  vi.setSystemTime(Date.now() + 61_000);
  view.rerender(<Gated fetcher={fetcher} enabled />);
  await act(async () => {});
  expect(fetcher).toHaveBeenCalledTimes(2);
});

beforeEach(() => {
  vi.useFakeTimers();
  clearCommandListCache();
});
afterEach(() => vi.useRealTimers());

it("shows loading with no cache, then the list", async () => {
  const fetcher = vi.fn<FetchCommands>().mockResolvedValue(resp(["help"]));
  render(<Probe id="a" fetcher={fetcher} />);
  expect(last.status).toBe("loading");
  await act(async () => {});
  expect(last).toMatchObject({ status: "ready", degraded: false });
  expect(names()).toEqual(["help"]);
});

it("shows the cached list at once and skips the read while it is fresh", async () => {
  const fetcher = vi.fn<FetchCommands>().mockResolvedValue(resp(["help"]));
  const first = render(<Probe id="a" fetcher={fetcher} />);
  await act(async () => {});
  first.unmount();
  render(<Probe id="a" fetcher={fetcher} />);
  expect(last.status).toBe("ready");
  expect(names()).toEqual(["help"]);
  expect(fetcher).toHaveBeenCalledTimes(1);
});

it("refreshes after 60 seconds, showing the old list until the read ends", async () => {
  const fetcher = vi.fn<FetchCommands>().mockResolvedValueOnce(resp(["old"])).mockResolvedValueOnce(resp(["new"]));
  const first = render(<Probe id="a" fetcher={fetcher} />);
  await act(async () => {});
  first.unmount();
  vi.setSystemTime(Date.now() + 61_000);
  render(<Probe id="a" fetcher={fetcher} />);
  expect(last.status).toBe("ready");
  expect(names()).toEqual(["old"]);
  await act(async () => {});
  expect(fetcher).toHaveBeenCalledTimes(2);
  expect(names()).toEqual(["new"]);
});

it("reports the degraded flag with the built-ins the server sent", async () => {
  const fetcher = vi.fn<FetchCommands>().mockResolvedValue(resp(["help"], true));
  render(<Probe id="a" fetcher={fetcher} />);
  await act(async () => {});
  expect(last).toMatchObject({ status: "ready", degraded: true });
  expect(names()).toEqual(["help"]);
});

it("fails with an empty list on a network error", async () => {
  const fetcher = vi.fn<FetchCommands>().mockRejectedValue(new Error("offline"));
  render(<Probe id="a" fetcher={fetcher} />);
  await act(async () => {});
  expect(last).toMatchObject({ status: "failed", entries: [] });
});

it("keeps the cached list when a refresh fails", async () => {
  const fetcher = vi.fn<FetchCommands>().mockResolvedValueOnce(resp(["old"])).mockRejectedValueOnce(new Error("offline"));
  const first = render(<Probe id="a" fetcher={fetcher} />);
  await act(async () => {});
  first.unmount();
  vi.setSystemTime(Date.now() + 61_000);
  render(<Probe id="a" fetcher={fetcher} />);
  await act(async () => {});
  expect(last.status).toBe("ready");
  expect(names()).toEqual(["old"]);
});

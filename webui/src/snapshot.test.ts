import { afterEach, expect, test, vi } from "vitest";
import { fetchSnapshot, type Snapshot } from "./snapshot";

afterEach(() => vi.unstubAllGlobals());

test("fetchSnapshot returns the restart fields", async () => {
  const wire: Partial<Snapshot> = {
    seq: 3,
    build: "v1.2.3",
    instance: "abc123",
    migration_failures: [{ project: "proj", error: "boom", retry_command: "gx tickets migrate '/store/proj'" }],
  };
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify(wire), { status: 200 })));

  const snap = await fetchSnapshot();

  expect(snap.instance).toBe("abc123");
  expect(snap.build).toBe("v1.2.3");
  expect(snap.migration_failures).toEqual(wire.migration_failures);
});

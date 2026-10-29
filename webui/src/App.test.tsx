import { act, waitFor } from "@testing-library/react";
import { expect, test } from "vitest";
import { App } from "./App";
import { readRoundDraft, writeRoundDraft } from "./drafts";
import type { Snapshot } from "./snapshot";
import { renderThemed } from "./ui/render";

const snapshotListing = (...ids: string[]) =>
  ({
    seq: 1,
    budget: { day: "", total: 0, soft_limit: 0, hard_limit: 0, budget_paused: false, hard_latched: false },
    health: {
      cpu: { level: 0, state: "unknown", reason: "waiting", read_at: "" },
      memory: { level: 0, state: "unknown", reason: "waiting", read_at: "" },
      disk: { level: 0, state: "unknown", reason: "waiting", read_at: "" },
    },
    tickets: [],
    pending: [],
    sessions: ids.map((id) => ({ id })),
    epics: [],
    recently_finished: [],
  }) as unknown as Snapshot;

test("only a live snapshot deletes the drafts of sessions it does not list", async () => {
  writeRoundDraft("a", "r", { n: 1 });
  writeRoundDraft("b", "r", { n: 2 });

  let release: (s: Snapshot) => void = () => {};
  const live = new Promise<Snapshot>((resolve) => (release = resolve));
  renderThemed(
    <App
      deps={{
        cache: { load: () => snapshotListing("a"), save: () => {} },
        fetchSnapshot: () => live,
        openStream: () => new Promise(() => {}),
      }}
    />,
  );

  expect(readRoundDraft("b", "r")).toEqual({ n: 2 });

  await act(async () => release(snapshotListing("a")));
  await waitFor(() => expect(readRoundDraft("b", "r")).toBeNull());
  expect(readRoundDraft("a", "r")).toEqual({ n: 1 });
});

test("a live snapshot with a null session list renders and deletes no drafts", async () => {
  writeRoundDraft("a", "r", { n: 1 });

  const unreadable = { ...snapshotListing(), sessions: null } as unknown as Snapshot;
  let release: (s: Snapshot) => void = () => {};
  const live = new Promise<Snapshot>((resolve) => (release = resolve));
  renderThemed(
    <App
      deps={{
        cache: { load: () => snapshotListing("a"), save: () => {} },
        fetchSnapshot: () => live,
        openStream: () => new Promise(() => {}),
      }}
    />,
  );

  await act(async () => release(unreadable));
  expect(readRoundDraft("a", "r")).toEqual({ n: 1 });
});

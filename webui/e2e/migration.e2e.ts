import { expect, test, type Fixture } from "./fixture";

// Flows for the migration the server runs when it starts (no flag, no command code). They run on the
// migrate-* scenarios: the real `gx server` runs on the store, and `fixture.serverCtl` kills and starts
// a bare `gx server` process the way launchd does (see testutil/gxfixture/migrate.go).

const LIST = "applied-migrations.txt";
const ticket = "demo/demo-epic/issues/01-broken-shape.md";

/** Waits until the handshake no longer says migrating. */
const migrated = (fixture: Fixture) =>
  expect
    .poll(async () => JSON.parse(await fixture.cli("server", "status", "--json")).migrating, { timeout: 30_000 })
    .toBeFalsy();

/** The names in a project's list file, one per line. */
const names = async (fixture: Fixture, project: string) =>
  (await fixture.readStoreFile(`${project}/${LIST}`)).split("\n").filter(Boolean);

/** The server log (server.log, which the bare process appends to across restarts). */
const serverLog = (fixture: Fixture) => fixture.cli("server", "logs");

/** How many migration notes (containing `text`) the server log holds; a step that ran logs its notes. */
const noteCount = async (fixture: Fixture, text = "") =>
  (await serverLog(fixture)).split("\n").filter((l) => l.includes("migration note") && l.includes(text)).length;

/** Restarts the bare server and waits for its migration to end. */
const restartBare = async (fixture: Fixture) => {
  await fixture.serverCtl("kill");
  await fixture.serverCtl("start");
  await expect.poll(() => fixture.cli("server", "status", "--json").then((s) => JSON.parse(s).running).catch(() => false), { timeout: 30_000 }).toBe(true);
  await migrated(fixture);
};

test("161. Server migrates on start", async ({ start }) => {
  const fixture = await start("migrate-pending");
  await migrated(fixture);

  await test.step("the old-shape ticket is in the new shape", async () => {
    expect(await fixture.readStoreFile(ticket)).toContain("status: paused");
    expect(await fixture.cli("tickets", "show", "demo:demo-epic/01")).toContain("paused");
  });

  const listed = await names(fixture, "demo");
  await test.step("the list holds the step names, one per line, with no repeat", async () => {
    expect(listed.length).toBeGreaterThan(0);
    expect(new Set(listed).size).toBe(listed.length);
  });

  await test.step("a second start runs no step", async () => {
    const notes = await noteCount(fixture);
    await restartBare(fixture);
    expect(await noteCount(fixture)).toBe(notes);
    expect(await names(fixture, "demo")).toEqual(listed);
  });
});

test("163. Per-project list", async ({ start }) => {
  const fixture = await start("migrate-pending");
  await migrated(fixture);
  const full = await names(fixture, "demo");

  await test.step("a second project has a ticket in the old shape and no list", async () => {
    await fixture.writeStoreFile("other/project.json", await fixture.readStoreFile("demo/project.json"));
    await fixture.writeStoreFile("other/other-epic/ticket.md", (await fixture.readStoreFile("demo/demo-epic/ticket.md")).replace("demo-epic", "other-epic"));
    await fixture.writeStoreFile("other/other-epic/issues/01-broken-shape.md", (await fixture.readStoreFile(ticket)).replace("status: paused", "status: needs-answer"));
  });

  await restartBare(fixture);

  await test.step("only the project without a list ran its steps", async () => {
    expect(await fixture.readStoreFile("other/other-epic/issues/01-broken-shape.md")).toContain("status: paused");
    expect(await names(fixture, "demo")).toEqual(full);
  });

  await test.step("both lists are full", async () => {
    expect(await names(fixture, "other")).toEqual(full);
  });
});

test("168. Crash restart and login start", async ({ start }) => {
  const fixture = await start("migrate-pending");
  await migrated(fixture);
  const first = await names(fixture, "demo");

  await test.step("the first start migrated", async () => {
    expect(await fixture.readStoreFile(ticket)).toContain("status: paused");
    expect(first.length).toBeGreaterThan(0);
  });

  await test.step("the second start, from a bare process, ran only the added step", async () => {
    const notes = await noteCount(fixture);
    await fixture.setInject([{ name: "e2e-added", notes: ["e2e added step ran"] }]);
    await restartBare(fixture);
    expect(await names(fixture, "demo")).toEqual([...first, "e2e-added"]);
    // The fixture store has a second project, scratch, which also had the full list.
    expect(await noteCount(fixture)).toBe(notes + 2);
    expect(await noteCount(fixture, "e2e added step ran")).toBe(2);
    expect(await names(fixture, "scratch")).toEqual([...first, "e2e-added"]);
    expect(await serverLog(fixture)).toContain("e2e added step ran");
  });
});

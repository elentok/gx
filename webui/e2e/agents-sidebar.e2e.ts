import { expect, test } from "./fixture";
import { go, row, sidebar, sessions } from "./agentsKit";

// Flows 72, 74, 89, 92 and 113 moved to AgentsPage.sidebar.test.tsx and TopBar.test.tsx.
// Marks, order and the tooltip of this flow are unit-tested there too; only the live SSE updates stay.
test("73. Agents sidebar", async ({ page, start }) => {
  const fixture = await start("agents");
  const list = await sessions(fixture);
  await go(page, "#/agents");
  const side = sidebar(page);
  const visible = list.filter((s) => !s.design);
  const live = list.filter((s) => s.status.state !== "exited").length;
  await expect(side.getByText(`${live} live`)).toBeVisible();

  // Live updates: a stopped idle session changes its mark and the count with no reload.
  const idle = visible.find((s) => s.status.state === "idle" && s.harness === "claude")!;
  await fixture.cli("agents", "stop", idle.id);
  await expect(row(page, idle).getByRole("img", { name: "exited", exact: true })).toBeVisible();
  await expect(side.getByText(`${live - 1} live`)).toBeVisible();

  // A removed Exited session leaves, a new one arrives.
  const exited = list.find((s) => s.status.state === "exited")!;
  await fixture.cli("agents", "remove", exited.id);
  await expect(row(page, exited)).toHaveCount(0);
  const before = new Set(list.map((s) => s.id));
  await fixture.cli("agents", "create", "--project", "demo", "--prompt", "hello");
  await expect.poll(async () => (await sessions(fixture)).filter((s) => !before.has(s.id)).length).toBe(1);
  const added = (await sessions(fixture)).find((s) => !before.has(s.id))!;
  await expect(row(page, added)).toBeVisible();
  await expect(side.getByText(`${live} live`)).toBeVisible();
});

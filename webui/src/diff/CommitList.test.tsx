import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import { COMMITS, filesForCommits, renderDiff } from "./renderDiff.testutil"

afterEach(cleanup)

vi.mock("@pierre/diffs/react", () => ({
  PatchDiff: (p: Record<string, unknown>) => (
    <div data-testid="pierre" data-patch={p.patch as string} />
  ),
}))

const noEpic = async () => ({ kind: "not-found" as const })
const EPIC = {
  kind: "ok" as const,
  detail: { rows: [{ address: "gx:e-impl/03", title: "Add the thing" }] },
}

function open(truncated = false) {
  const fetchFiles = vi.fn(filesForCommits)
  const router = renderDiff("/diff?project=gx&base=main&tip=dev&epic=gx:e-impl", {
    diffFiles: fetchFiles,
    diffPatch: async () => "p",
    diffCommits: async () => ({ commits: COMMITS, truncated }),
    diffCommitPatches: async () => [],
    epic: (async () => EPIC) as never,
    archivedEpic: noEpic,
  })
  return { fetchFiles, router }
}
const disclosure = () => screen.findByTestId("commits-disclosure")
const paths = () => screen.queryAllByTestId("tree-file").map((n) => n.textContent)
const toggle = () =>
  within(screen.getByTestId("commits-disclosure")).getByRole("button", { expanded: false })
const pick = (name: string) =>
  fireEvent.click(within(screen.getByTestId("commits-disclosure")).getByRole("button", { name }))

test("collapsed it reads COMMITS BY TICKET (n); opened it starts with All commits", async () => {
  open()
  const d = await disclosure()
  const header = toggle()
  expect(header.textContent).toContain("COMMITS BY TICKET (5)")
  expect(within(d).queryByText("All commits")).toBeNull()
  fireEvent.click(header)
  const rows = within(d).getAllByRole("button").slice(1)
  expect(rows[0].textContent).toBe("All commits")
  expect(rows[1].textContent).toContain("Add the thing")
})

test("narrowing to a ticket, Design or Other commits filters the tree and sets the chip", async () => {
  const { fetchFiles } = open()
  await disclosure()
  fireEvent.click(toggle())
  pick("Add the thing 2")
  await waitFor(() =>
    expect(fetchFiles).toHaveBeenLastCalledWith("gx", "main", "dev", ["c2", "c4"]),
  )
  await waitFor(() => expect(paths()).toHaveLength(1))
  expect(screen.getByTestId("narrow-chip").textContent).toContain("Showing: Add the thing")
  expect(screen.getByTestId("commits-narrowing").textContent).toBe("Add the thing")
  pick("Design 1")
  await waitFor(() => expect(fetchFiles).toHaveBeenLastCalledWith("gx", "main", "dev", ["c1"]))
  pick("Other commits 1")
  await waitFor(() => expect(fetchFiles).toHaveBeenLastCalledWith("gx", "main", "dev", ["c5"]))
  await waitFor(() => expect(paths()).toHaveLength(0))
})

test("a single commit narrows to it; the merge commit is labelled", async () => {
  const { fetchFiles } = open()
  await disclosure()
  fireEvent.click(toggle())
  expect(screen.getByText("merge")).toBeTruthy()
  pick("Add b")
  await waitFor(() => expect(fetchFiles).toHaveBeenLastCalledWith("gx", "main", "dev", ["c3"]))
  expect(screen.getByTestId("narrow-chip").textContent).toContain("Showing: Ticket 07")
})

test("the chip's x and All commits restore the full tree", async () => {
  const { router } = open()
  await disclosure()
  fireEvent.click(toggle())
  await waitFor(() => expect(paths().length).toBeGreaterThan(1))
  const full = paths().length
  pick("Add b")
  await waitFor(() => expect(paths()).toHaveLength(1))
  fireEvent.click(screen.getByRole("button", { name: "Clear narrowing" }))
  await waitFor(() => expect(paths()).toHaveLength(full))
  expect(screen.queryByTestId("narrow-chip")).toBeNull()
  expect(router.state.location.search).not.toContain("commits=")
  pick("Add b")
  await waitFor(() => expect(paths()).toHaveLength(1))
  pick("All commits")
  await waitFor(() => expect(paths()).toHaveLength(full))
})

test("a truncated commit list says so in the disclosure", async () => {
  open(true)
  await disclosure()
  fireEvent.click(toggle())
  expect(screen.getByTestId("commits-truncated").textContent).toBe("Showing the first 500 commits")
})

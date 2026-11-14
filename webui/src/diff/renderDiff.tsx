import { createMemoryRouter, RouterProvider } from "react-router"
import type { DiffCommit, DiffFile } from "../generated/server"
import type { DiffFilesResult, FetchDiffFiles } from "./api"
import { ConnectionContext, type Connection, type ConnectionState } from "../connection"
import { appRoutes } from "../routes"
import { renderThemed } from "../ui/render"
import { type Device, DeviceContext } from "../useWide"

const commit = (sha: string, subject: string, files: string[], over: Partial<DiffCommit> = {}) => ({
  sha,
  subject,
  time: "2026-01-01T00:00:00Z",
  files,
  ...over,
})

/** A range with two tickets, a design commit, an earlier trailer-less commit and a merge. */
export const COMMITS: DiffCommit[] = [
  commit("c1", "e: plan it", ["docs/p.md"]),
  commit("c2", "Add a", ["src/a.go"], { ticket: "gx:e-impl/03" }),
  commit("c3", "Add b", ["src/b.go"], { ticket: "gx:e-impl/07" }),
  commit("c4", "Fix a", ["src/a.go"], { ticket: "gx:e-impl/03" }),
  commit("c5", "Merge branch", [], { merge: true }),
]

const ALL_FILES = ["docs/p.md", "src/a.go", "src/b.go"]

/** The file-list fetcher of the COMMITS range: the files of the given commits, or all of them. */
export const filesForCommits: FetchDiffFiles = async (_p, _a, _b, shas) => {
  const paths = shas
    ? [...new Set(COMMITS.filter((c) => shas.includes(c.sha)).flatMap((c) => c.files))]
    : ALL_FILES
  const files: DiffFile[] = paths.map((path) => ({ path, status: "M", added: 1, deleted: 0 }))
  return {
    kind: "files",
    files: { first_sha: "a", second_sha: "b", merge_base: "a", files },
  } satisfies DiffFilesResult
}

/** Renders the app at a Diff page path with the given fetchers injected; for the Diff page tests. */
export function renderDiff(
  path: string,
  fetchers: Parameters<typeof appRoutes>[0],
  device?: Device,
) {
  const state = {
    snapshot: { sessions: [], pending: [] },
    connected: true,
    loadedOnce: true,
    tick: 0,
  }
  const conn = {
    getState: () => state as unknown as ConnectionState,
    subscribe: () => () => {},
    onEvent: () => () => {},
    refresh: () => {},
  } as unknown as Connection
  const router = createMemoryRouter(appRoutes(fetchers), { initialEntries: [path] })
  renderThemed(
    <DeviceContext.Provider value={device}>
      <ConnectionContext.Provider value={conn}>
        <RouterProvider router={router} />
      </ConnectionContext.Provider>
    </DeviceContext.Provider>,
  )
  return router
}

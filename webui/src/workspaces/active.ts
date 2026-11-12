import {
  SectionOnHold,
  type EpicListRow,
  type SessionPlacement,
  type Workspace,
} from "../generated/server"
import { isCurrent } from "../epics/list"

export const ACTIVE_KEY = "gx.workspace.active"

export function readStoredWorkspace(): string | null {
  try {
    return localStorage.getItem(ACTIVE_KEY) || null
  } catch {
    return null
  }
}

export function writeStoredWorkspace(name: string): void {
  try {
    localStorage.setItem(ACTIVE_KEY, name)
  } catch {
    // Private mode or blocked storage: the choice lasts until reload.
  }
}

export interface ResolveInput {
  /** The router pathname, e.g. `/e/demo:blog`. */
  route: string
  rows: EpicListRow[]
  placements: SessionPlacement[]
  workspaces: Workspace[]
  stored: string | null
}

export const defaultWorkspace = (workspaces: Workspace[]): string =>
  (workspaces.find((w) => w.default) ?? workspaces[0])?.name ?? ""

/** The workspace an epic address stands for, or null when it has none (not found, held, One-off, Follow-ups). */
function epicWorkspace(rows: EpicListRow[], address: string): string | null {
  const row = rows.find((r) => isCurrent(r, address))
  if (!row || row.section === SectionOnHold) return null
  return row.workspace || null
}

function itemWorkspace({ route, rows, placements }: ResolveInput): string | null {
  const [, kind, ...rest] = route.split("/")
  const id = decodeURIComponent(rest.join("/"))
  switch (kind) {
    case "d":
    case "e":
      return epicWorkspace(rows, id)
    case "t":
      return epicWorkspace(rows, id) ?? epicWorkspace(rows, id.replace(/\/[^/]*$/, ""))
    case "agents": {
      const p = placements.find((x) => x.session === id)
      if (p?.kind === "chat") return p.workspace || null
      if (p?.kind === "epic" && p.epic) return epicWorkspace(rows, p.epic)
      return null
    }
    default:
      return null
  }
}

/** An agent route whose placement the snapshot does not know yet (a just-started agent): not found, unlike found without a workspace. */
export function routePending({
  route,
  placements,
}: Pick<ResolveInput, "route" | "placements">): boolean {
  const [, kind, ...rest] = route.split("/")
  if (kind !== "agents") return false
  const id = decodeURIComponent(rest.join("/"))
  return id !== "" && !placements.some((x) => x.session === id)
}

/** The active workspace: the open item's, else the stored one, and Default when that is gone. */
export function resolveActive(input: ResolveInput): string {
  const exists = (name: string | null): name is string =>
    name !== null && input.workspaces.some((w) => w.name === name)
  const fromItem = itemWorkspace(input)
  if (exists(fromItem)) return fromItem
  return exists(input.stored) ? input.stored : defaultWorkspace(input.workspaces)
}

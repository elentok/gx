import type { DiffCommit } from "../generated/server"

export type CommitGroup = {
  key: string
  kind: "ticket" | "design" | "other"
  label: string
  commits: DiffCommit[]
}

const ticketId = (address: string) => address.slice(address.lastIndexOf("/") + 1)

/** The map's name: an epic address `<project>:<epic>` with the epic's `-impl` suffix dropped. */
function mapName(epic: string): string {
  return epic.slice(epic.indexOf(":") + 1).replace(/-impl$/, "")
}

/**
 * Groups the commit call's commits (already in range order, trailers already canonical): ticket
 * groups by first commit, then "Design" (only with an epic), then "Other commits". `titles` maps a
 * ticket address to its title; an unknown ticket reads "Ticket <id>".
 */
export function groupCommits(
  commits: DiffCommit[],
  epic: string | undefined,
  titles: ReadonlyMap<string, string>,
): CommitGroup[] {
  const tickets = new Map<string, CommitGroup>()
  const design: DiffCommit[] = []
  const other: DiffCommit[] = []
  const designPrefix = epic ? `${mapName(epic)}:` : undefined
  for (const c of commits) {
    if (c.ticket) {
      let g = tickets.get(c.ticket)
      if (!g) {
        const label = titles.get(c.ticket) ?? `Ticket ${ticketId(c.ticket)}`
        g = { key: c.ticket, kind: "ticket", label, commits: [] }
        tickets.set(c.ticket, g)
      }
      g.commits.push(c)
    } else if (designPrefix && c.subject.startsWith(designPrefix)) {
      design.push(c)
    } else {
      other.push(c)
    }
  }
  const out = [...tickets.values()]
  if (design.length) out.push({ key: "design", kind: "design", label: "Design", commits: design })
  if (other.length)
    out.push({ key: "other", kind: "other", label: "Other commits", commits: other })
  return out
}

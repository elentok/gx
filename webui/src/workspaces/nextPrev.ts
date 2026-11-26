// Which sidebar row the next and previous steps open. Pure: the caller reads the DOM and keeps the
// last row.

import { parseDiffUrl } from "../diff/diffUrl"
import { currentAddress } from "../hrefs"
import type { Sidebar, SidebarAgent, SidebarRow } from "./sidebarModel"

/** The `data-nav` values SidebarRows puts on its anchors. */
export const rowNavId = (key: string) => `row:${key}`
export const agentNavId = (id: string) => `agent:${id}`

export interface NextPrevInput {
  pathname: string // the router location's pathname
  search: string // the router location's search, "?..." or ""
  model: Sidebar // from buildSidebar
  rendered: readonly string[] // data-nav values of the rendered rows, top to bottom
  lastRow: string | undefined // the remembered nav id of this tab
}

export interface NavTarget {
  id: string
  href: string // as in the model, "#/..."
}

export interface NextPrev {
  pageRow: string | undefined // the first row that owns the open page; the caller remembers it
  next: NavTarget | undefined
  prev: NavTarget | undefined
}

/** What the open page shows, as far as a row can own it by more than its link. */
interface Page {
  href: string
  epic?: string
  /** The session whose own worktree the Diff page shows. */
  session?: string
}

function readPage(pathname: string, search: string): Page {
  const href = `#${pathname}`
  if (pathname !== "/diff") return { href, epic: currentAddress(pathname) }
  const { project, epic, worktree } = parseDiffUrl(new URLSearchParams(search))
  if (!worktree) return { href, epic }
  if (worktree.kind === "chat") return { href, session: worktree.id }
  if (worktree.kind === "design") return { href, epic: `${project}:${worktree.id}` }
  return { href, epic: worktree.id.replace(/\/[^/]*$/, "") }
}

interface Item extends NavTarget {
  owns: boolean
}

/** Every row of the model, in the order SidebarRows renders them. */
function modelItems({ rows, held, other }: Sidebar, page: Page): Item[] {
  const agentItem = (a: SidebarAgent): Item => ({
    id: agentNavId(a.id),
    href: a.href,
    owns: a.href === page.href || a.id === page.session,
  })
  const rowItem = (r: SidebarRow): Item => ({
    id: rowNavId(r.key),
    href: r.href,
    owns:
      r.href === page.href ||
      (!!page.epic && !!r.addresses?.includes(page.epic)) ||
      (r.kind === "chat" && r.key === page.session),
  })
  return [
    ...[...rows, ...held].flatMap((r) => [rowItem(r), ...r.agents.map(agentItem)]),
    ...other.map(agentItem),
  ]
}

export function nextPrev({ pathname, search, model, rendered, lastRow }: NextPrevInput): NextPrev {
  const items = modelItems(model, readPage(pathname, search))
  const place = (id: string) => items.findIndex((i) => i.id === id)
  const visible = rendered.filter((id) => place(id) >= 0)
  const target = (id: string | undefined): NavTarget | undefined => {
    const item = items.find((i) => i.id === id)
    return item && { id: item.id, href: item.href }
  }

  const pageRow = items.find((i) => i.owns)?.id
  const current = pageRow ?? (lastRow && place(lastRow) >= 0 ? lastRow : undefined)
  if (!current) return { pageRow, next: target(visible[0]), prev: target(visible.at(-1)) }

  const at = visible.indexOf(current)
  // A hidden current row sits just before the first rendered row that follows it in the model.
  const following = visible.findIndex((id) => place(id) > place(current))
  const gap = following < 0 ? visible.length : following
  return {
    pageRow,
    next: target(visible[at < 0 ? gap : at + 1]),
    prev: target(visible[(at < 0 ? gap : at) - 1]),
  }
}

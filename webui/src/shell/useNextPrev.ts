import { useEffect, useState } from "react"
import { useLocation, useNavigate } from "react-router"
import type { FetchEpicList } from "../epics/list"
import { nextPrev, type NavTarget } from "../workspaces/nextPrev"
import type { ItemNav } from "./chords"
import { LIST_ATTR, NAV_ATTR } from "./SidebarKeys"
import { useSidebarModel } from "./SidebarRows"

// Outside the `gx.page.` prefix, which pageCache clears.
const LAST_ROW_KEY = "gx.sidebar.last-row"

function readLastRow(): string | undefined {
  try {
    return sessionStorage.getItem(LAST_ROW_KEY) ?? undefined
  } catch {
    return undefined
  }
}

function writeLastRow(id: string) {
  try {
    sessionStorage.setItem(LAST_ROW_KEY, id)
  } catch {
    // The row is then remembered until a reload only.
  }
}

const list = () => document.querySelector(`[${LIST_ATTR}]`)
const anchors = (root: Element | null) => Array.from(root?.querySelectorAll(`a[${NAV_ATTR}]`) ?? [])
const navId = (el: Element) => el.getAttribute(NAV_ATTR) ?? ""

/**
 * The sidebar's rendered row ids, top to bottom. Held in state because a row expands with its own
 * local state, so a read during render would see the old DOM.
 */
function useRenderedRows(): string[] {
  const [rows, setRows] = useState<string[]>([])
  useEffect(() => {
    const root = list()
    if (!root) return
    const read = () => {
      const next = anchors(root).map(navId)
      setRows((prev) =>
        prev.length === next.length && prev.every((id, i) => id === next[i]) ? prev : next,
      )
    }
    read()
    const observer = new MutationObserver(read)
    observer.observe(root, { childList: true, subtree: true })
    return () => observer.disconnect()
  }, [])
  return rows
}

/**
 * The steps to the next and previous sidebar row, each absent with nowhere to go. It also remembers
 * the tab's last row, so the caller must stay mounted across page changes.
 */
export function useNextPrev(fetcher: FetchEpicList | undefined, active: string): ItemNav {
  const model = useSidebarModel(fetcher, active)
  const { pathname, search } = useLocation()
  const navigate = useNavigate()
  const rendered = useRenderedRows()
  const [lastRow, setLastRow] = useState(readLastRow)
  const found = model && nextPrev({ pathname, search, model, rendered, lastRow })
  const pageRow = found?.pageRow
  if (pageRow && pageRow !== lastRow) setLastRow(pageRow)
  useEffect(() => {
    if (lastRow) writeLastRow(lastRow)
  }, [lastRow])

  const step = (target: NavTarget | undefined) =>
    target &&
    (() => {
      void navigate(target.href.replace(/^#/, ""))
      anchors(list())
        .find((el) => navId(el) === target.id)
        ?.scrollIntoView?.({ block: "nearest" })
    })
  return { next: step(found?.next), prev: step(found?.prev) }
}

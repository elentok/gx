import { useEffect, useRef, type RefObject } from "react"
import type { SelectApi } from "../shell/SidebarSelect"
import { isTextTarget } from "../shell/keys"
import { hasLayer } from "../ui/modal/layers"
import { isMoveKey, moveInGrid } from "./gridNav"
import { selectIntent } from "./selectKeys"

/** Marks a page's card or row; its value is the item's select key. The sidebar's roving focus uses data-nav, so the two never match each other. */
export const PAGE_ATTR = "data-page-item"
/** Marks the element Enter clicks when the item itself is not the click target (a card's title link). */
export const PAGE_OPEN_ATTR = "data-page-open"

const ITEM = `[${PAGE_ATTR}]`

const itemsOf = (area: HTMLElement) => Array.from(area.querySelectorAll<HTMLElement>(ITEM))

/**
 * Keyboard for a page of cards or rows. The area is one tab stop (the focused item, or the first, has
 * tabindex 0, the rest -1). Arrows and hjkl move by position; `s` enters the page's select mode; Space and
 * Enter toggle in select mode, and Enter opens outside it. Escape is handled by useSelectApi.
 * Runs in the capture phase so that Enter toggles before an item's own click handler sees it.
 */
export function usePageKeys(area: RefObject<HTMLElement | null>, select: SelectApi) {
  const selectRef = useRef(select)
  selectRef.current = select
  const stop = useRef("")

  // After every render: the list of items changes with the data.
  useEffect(() => {
    const el = area.current
    if (!el) return
    const all = itemsOf(el)
    const current = all.find((it) => it.getAttribute(PAGE_ATTR) === stop.current) ?? all[0]
    for (const it of all) it.tabIndex = it === current ? 0 : -1
  })

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const el = area.current
      const target = e.target
      if (e.defaultPrevented || !el || !(target instanceof HTMLElement) || !el.contains(target))
        return
      const guarded = hasLayer() || isTextTarget(target)
      const item = target.hasAttribute(PAGE_ATTR) ? target : null
      const sel = selectRef.current
      const all = itemsOf(el)
      const intent = selectIntent(e, sel.state.on, guarded)
      if (intent === "enter-select") {
        if (all.length === 0) return
        e.preventDefault()
        sel.enter()
        return
      }
      if (!item) return
      if (intent === "toggle" || intent === "open") {
        e.preventDefault()
        e.stopPropagation()
        const key = item.getAttribute(PAGE_ATTR) ?? ""
        if (intent === "open")
          (item.querySelector<HTMLElement>(`[${PAGE_OPEN_ATTR}]`) ?? item).click()
        else if (sel.rows.find((r) => r.key === key)?.selectable !== false) sel.toggle(key)
        return
      }
      if (guarded || e.altKey || e.metaKey || e.ctrlKey || !isMoveKey(e.key)) return
      e.preventDefault()
      const points = all.map((it) => {
        const r = it.getBoundingClientRect()
        return { x: r.left, y: r.top }
      })
      all[moveInGrid(points, all.indexOf(item), e.key)]?.focus()
    }
    const onFocusIn = (e: FocusEvent) => {
      const item = e.target instanceof Element ? e.target.closest<HTMLElement>(ITEM) : null
      if (!item || !area.current?.contains(item)) return
      stop.current = item.getAttribute(PAGE_ATTR) ?? ""
      for (const it of itemsOf(area.current)) it.tabIndex = it === item ? 0 : -1
    }
    document.addEventListener("keydown", onKey, true)
    document.addEventListener("focusin", onFocusIn)
    return () => {
      document.removeEventListener("keydown", onKey, true)
      document.removeEventListener("focusin", onFocusIn)
    }
  }, [area])
}

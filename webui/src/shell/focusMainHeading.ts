/** Marks the page title element every landing page renders; it is focusable with tabIndex -1. */
export const MAIN_HEADING_ATTR = "data-main-heading"

/** The page the app shows. The app routes by hash, so the pathname alone never changes. */
export const locationKey = () => window.location.pathname + window.location.hash

export function focusMainHeading(): boolean {
  const heading = document.querySelector<HTMLElement>(`[${MAIN_HEADING_ATTR}]`)
  heading?.focus()
  return !!heading && document.activeElement === heading
}

/**
 * After an action removed the open page's entity: waits for navigation to leave `fromPath`, then focuses the heading
 * of the page the app landed on. Gives up after `timeoutMs` and focuses whatever heading is there.
 */
export function focusMainHeadingAfterLanding(fromPath: string, timeoutMs = 2000) {
  const start = Date.now()
  const tick = () => {
    const landed = locationKey() !== fromPath
    if (landed && focusMainHeading()) return
    if (Date.now() - start >= timeoutMs) return void focusMainHeading()
    setTimeout(tick, 50)
  }
  setTimeout(tick, 0)
}

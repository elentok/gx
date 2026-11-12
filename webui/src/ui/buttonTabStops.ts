/**
 * Safari skips buttons when it tabs, unless the user changed a Safari setting. A tabindex of 0 makes a button a tab
 * stop there. This sets it on every button that has no tabindex of its own, now and as the page changes, so a widget
 * that sets its own tabindex (a roving one uses -1) keeps it.
 */
const SELECTOR = "button:not([tabindex])"

function mark(root: ParentNode) {
  if (root instanceof Element && root.matches(SELECTOR)) root.setAttribute("tabindex", "0")
  root.querySelectorAll(SELECTOR).forEach((b) => b.setAttribute("tabindex", "0"))
}

/** Starts the marking on `root`; returns a function that stops it. */
export function installButtonTabStops(root: HTMLElement = document.body): () => void {
  mark(root)
  const observer = new MutationObserver((records) => {
    for (const r of records) r.addedNodes.forEach((n) => n instanceof Element && mark(n))
  })
  observer.observe(root, { childList: true, subtree: true })
  return () => observer.disconnect()
}

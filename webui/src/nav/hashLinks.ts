// Plain `<a href="#/...">` pushes a history entry with no router state, so the next page sees the
// "default" location key and BackLink treats it as a direct load. Routing these clicks through the
// router gives the entry a key.
export function routeHashLinkClicks(router: { navigate: (to: string) => unknown }): () => void {
  const onClick = (e: MouseEvent) => {
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    const a = e.target instanceof Element ? e.target.closest("a") : null;
    const href = a?.getAttribute("href");
    if (!a || !href?.startsWith("#/") || (a.target && a.target !== "_self")) return;
    e.preventDefault();
    void router.navigate(href.slice(1));
  };
  document.addEventListener("click", onClick);
  return () => document.removeEventListener("click", onClick);
}

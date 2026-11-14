export type ViewerTarget = {
  line?: number
  heading?: string
}

/** An absolute path with each segment percent-encoded and the slashes kept. */
export function encodePath(path: string): string {
  return path.split("/").map(encodeURIComponent).join("/")
}

/**
 * The hash href of a file viewer link. The path is the canonical one the server
 * returned: `~/` and relative paths mean nothing in a URL.
 */
export function viewerHref(path: string, target: ViewerTarget = {}): string {
  const segments = encodePath(path)
  const query: string[] = []
  if (target.line) query.push(`line=${target.line}`)
  if (target.heading) query.push(`heading=${encodeURIComponent(target.heading)}`)
  return `#/files${segments}${query.length ? `?${query.join("&")}` : ""}`
}

/** The full link for Copy link: page origin, `/`, then the hash form. */
export function viewerLink(
  path: string,
  target: ViewerTarget = {},
  origin: string = window.location.origin,
): string {
  return `${origin}/${viewerHref(path, target)}`
}

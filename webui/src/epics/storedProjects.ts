const PROJECT_KEY = "gx.epics.project"

/** The stored choice: a JSON array, or the plain project name an older version wrote. */
export function readStoredProjects(): string[] {
  try {
    const raw = localStorage.getItem(PROJECT_KEY)
    if (!raw) return []
    if (raw.startsWith("[")) {
      const parsed: unknown = JSON.parse(raw)
      return Array.isArray(parsed) ? parsed.filter((p): p is string => typeof p === "string") : []
    }
    return [raw]
  } catch {
    return []
  }
}

export function writeStoredProjects(chosen: string[]): void {
  try {
    localStorage.setItem(PROJECT_KEY, JSON.stringify(chosen))
  } catch {
    // Storage is a convenience; the choice still holds for the page.
  }
}

/**
 * The filter that applies: stored projects that are gone are ignored without rewriting them, and
 * the filter applies only when there is more than one project. None left acts as "All projects".
 */
export function activeProjectFilter(stored: string[], projects: string[]): string[] | undefined {
  const known = stored.filter((p) => projects.includes(p))
  return projects.length > 1 && known.length > 0 ? known : undefined
}

import type { SettingsCategory, SettingsEntry } from "../generated/server"

export const slugOf = (name: string) => name.toLowerCase().replace(/[^a-z0-9]+/g, "-")

/** A page tree for tests: the entries' categories and sections in first-seen order, slugs derived from the names. */
export function treeFromEntries(entries: Pick<SettingsEntry, "category" | "section">[]) {
  const categories: SettingsCategory[] = []
  for (const e of entries) {
    let c = categories.find((x) => x.name === e.category)
    if (!c) categories.push((c = { name: e.category, sections: [] }))
    if (!c.sections.some((s) => s.name === e.section))
      c.sections.push({ name: e.section, slug: slugOf(e.section), groups: [] })
  }
  return categories
}

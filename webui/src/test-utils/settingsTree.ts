import type { SettingsCategory, SettingsEntry } from "../generated/server"

export const slugOf = (name: string) => name.toLowerCase().replace(/[^a-z0-9]+/g, "-")

/** A page tree for tests: the entries' categories and sections in first-seen order, slugs derived from the names. */
export function treeFromEntries(entries: Pick<SettingsEntry, "category" | "section" | "group">[]) {
  const categories: SettingsCategory[] = []
  for (const e of entries) {
    let c = categories.find((x) => x.name === e.category)
    if (!c) categories.push((c = { name: e.category, sections: [] }))
    let s = c.sections.find((x) => x.name === e.section)
    if (!s) c.sections.push((s = { name: e.section, slug: slugOf(e.section), groups: [] }))
    if (!s.groups.some((g) => g.name === e.group)) s.groups.push({ name: e.group })
  }
  return categories
}

import type {
  ModelVersion,
  SessionInfo,
  SessionModelRow,
  SessionModelsResponse,
} from "../generated/server"
import { fuzzyScore } from "../nav/fuzzy"
import { modelName, type ModelLabel } from "../modelName"

export type PickerRow = {
  id: string
  label: ModelLabel
  current: boolean
  outdated: boolean
  efforts: string[]
  defaultEffort?: string
}

/** `family` is absent for the last group, "Other". */
export type PickerGroup = { family?: string; rows: PickerRow[] }

const cmpVersion = (a?: ModelVersion, b?: ModelVersion) =>
  (b?.major ?? 0) - (a?.major ?? 0) || (b?.minor ?? 0) - (a?.minor ?? 0)

/**
 * The picker's groups: families by their newest version (highest first, ties alphabetical), models
 * inside a family newest first, then "Other" in the server's order. The session's own model is
 * always listed, so a missing list still shows it.
 */
export function buildGroups(
  models: SessionModelsResponse | undefined,
  session: SessionInfo,
): PickerGroup[] {
  const harness = session.harness
  const rows: SessionModelRow[] = [...(models?.models ?? [])]
  const own = session.model ?? ""
  if (own && !rows.some((r) => r.current || r.id === own)) rows.push({ id: own, efforts: [] })
  const toRow = (r: SessionModelRow): PickerRow => ({
    id: r.id,
    label: modelName(r.id, harness),
    current: !!r.current || (own !== "" && r.id === own),
    outdated: !!r.latest,
    efforts: r.efforts ?? [],
    defaultEffort: r.switch_effort,
  })
  const byFamily = new Map<string, SessionModelRow[]>()
  const other: PickerRow[] = []
  for (const r of rows) {
    if (!r.family) other.push(toRow(r))
    else byFamily.set(r.family, [...(byFamily.get(r.family) ?? []), r])
  }
  const groups = [...byFamily].map(([family, list]) => {
    const sorted = [...list].sort((a, b) => cmpVersion(a.version, b.version))
    return { family, newest: sorted[0].version, rows: sorted.map(toRow) }
  })
  groups.sort((a, b) => cmpVersion(a.newest, b.newest) || a.family.localeCompare(b.family))
  const out: PickerGroup[] = groups.map(({ family, rows }) => ({ family, rows }))
  if (other.length) out.push({ rows: other })
  return out
}

/** The groups left after a fuzzy filter on the model label; empty groups drop out. */
export function filterGroups(groups: PickerGroup[], query: string): PickerGroup[] {
  const q = query.trim()
  return groups
    .map((g) => ({ ...g, rows: g.rows.filter((r) => fuzzyScore(r.label, q) !== null) }))
    .filter((g) => g.rows.length > 0)
}

// Mirrors claudeEfforts in modelrules/modelrules.go.
const CLAUDE_EFFORTS = ["low", "medium", "high", "xhigh", "max"]

/**
 * The effort a model runs with, following modelrules.Effort: an empty wanted effort stays empty,
 * a listed one is kept, else the route's `switch_effort`, else (Claude) the nearest lower level or
 * the lowest listed one. Empty for a model with no effort control.
 */
export function effortFor(row: PickerRow, wanted: string | undefined, harness?: string): string {
  if (row.efforts.length === 0 || !wanted) return ""
  if (row.efforts.includes(wanted)) return wanted
  if (row.defaultEffort && row.efforts.includes(row.defaultEffort)) return row.defaultEffort
  if (harness === "codex") return row.efforts[0]
  const rank = (e: string) => CLAUDE_EFFORTS.indexOf(e)
  const known = row.efforts.filter((e) => rank(e) >= 0).sort((a, b) => rank(a) - rank(b))
  const lower = known.filter((e) => rank(e) < rank(wanted))
  return lower.at(-1) ?? known[0] ?? row.efforts[0]
}

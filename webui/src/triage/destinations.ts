import type { TriageDestination, TriageRow } from "../generated/server";

export type ReviewAction = "ready" | "map" | "merge" | "close" | "keep";

/** The five action groups in the order the Review view lists them. */
export const GROUPS: { action: ReviewAction; name: string; short: string }[] = [
  { action: "ready", name: "Ready epic", short: "Ready" },
  { action: "map", name: "Map epics", short: "Map epic" },
  { action: "merge", name: "Merge into another", short: "Merge" },
  { action: "close", name: "Close", short: "Close" },
  { action: "keep", name: "Keep for later", short: "Keep" },
];

export interface DestinationChoice {
  /** The saved destination; absent for the typed new map epic. */
  value?: string;
  label: string;
  kind: "epic" | "row" | "new";
}

/** Ready and map rows go to an epic, so a row switching between them keeps its destination. */
export const takesEpic = (action: string | undefined) => action === "ready" || action === "map";

/** The destination choices for a row set to `action`; close and keep have none. */
export function destinationChoices(action: string | undefined, self: string, destinations: TriageDestination[], rows: TriageRow[]): DestinationChoice[] {
  if (takesEpic(action))
    return destinations.map((d) => (d.kind === "new-map-epic" ? { label: "New map epic…", kind: "new" } : { value: d.value, label: d.label, kind: "epic" }));
  if (action === "merge")
    return rows
      .filter((r) => r.address !== self && !r.gone && takesEpic(r.action))
      .map((r) => ({ value: r.address, label: r.title, kind: "row" }));
  return [];
}

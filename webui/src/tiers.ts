export const TIERS = [
  { id: "light", label: "Light" },
  { id: "worker", label: "Worker" },
  { id: "strongest", label: "Strongest" },
] as const

export type TierId = (typeof TIERS)[number]["id"]

/** The display label for a tier id, or "" when the id is empty or unknown. */
export const tierLabel = (id: string | undefined): string =>
  TIERS.find((t) => t.id === id)?.label ?? ""

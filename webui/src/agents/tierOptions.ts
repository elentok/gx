import { modelName } from "../modelName"
import { TIERS, type TierId } from "../tiers"
import type { ProjectWorktrees } from "../generated/server"

export type Harness = "claude" | "codex"
export const HARNESSES: { id: Harness; label: string }[] = [
  { id: "claude", label: "Claude" },
  { id: "codex", label: "Codex" },
]

/** Effort levels a person may pick; the server settles what a model really takes. */
export const EFFORTS: Record<Harness, string[]> = {
  claude: ["low", "medium", "high", "xhigh", "max"],
  codex: ["low", "medium", "high", "xhigh"],
}

/** The tiers of a project's harness, each with its configured model when the server knows it. */
export function tierOptions(current: ProjectWorktrees | undefined, harness: Harness) {
  // Older servers only expose Claude's tier models.
  const models =
    current?.tier_models?.[harness] ?? (harness === "claude" ? current?.tiers : undefined)
  return TIERS.map((t) => {
    const model = models?.[t.id as TierId]
    return {
      id: t.id,
      model,
      label: model ? `${t.label} (${modelName(model, harness)})` : t.label,
    }
  })
}

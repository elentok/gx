import type { SessionInfo } from "../generated/server";
import { HarnessLabel } from "../ui/harness-label/HarnessLabel";
import { TIERS } from "./NewSessionForm";

/** HarnessLabel fed from a session; a session with no tier shows the Worker label only while it has no model either. */
export function SessionHarnessLabel({ session: s }: { session: SessionInfo }) {
  const tierLabel = s.tier ? (TIERS.find((t) => t.id === s.tier)?.label ?? "") : "";
  const known = s.tier || s.model || s.effort;
  return (
    <HarnessLabel
      harness={s.harness}
      tierLabel={known && !s.model && !tierLabel ? (TIERS.find((t) => t.id === "worker")?.label ?? "") : tierLabel}
      model={s.model ?? ""}
      effort={s.effort ?? ""}
      switched={s.model_switched ?? false}
    />
  );
}

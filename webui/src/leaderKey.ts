import { isMac } from "./platform"
import type { Snapshot } from "./snapshot"

// Picks this platform's Leader key (like "cmd+e") from a snapshot; empty means chords are off.
export function leaderKeyOf(
  snap: Pick<Snapshot, "leader_key" | "leader_key_mac"> | null | undefined,
): string {
  return (isMac() ? snap?.leader_key_mac : snap?.leader_key) ?? ""
}

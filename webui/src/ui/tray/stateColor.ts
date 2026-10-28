import type { HealthState } from "../../generated/server";

// Normal is the faint grey; high and critical borrow the waiting and paused tones.
export const STATE_COLOR: Record<HealthState, string> = {
  normal: "text.faint",
  high: "tone.waiting.fg",
  critical: "tone.paused.fg",
  unknown: "text.faint",
};

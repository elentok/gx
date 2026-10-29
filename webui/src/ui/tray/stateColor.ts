import type { HealthState } from "../../generated/server";

// Normal is the faint grey; high and critical borrow the waiting and paused tones.
// `bg` is the tag's pill background; normal and unknown never show a tag.
export const STATE_COLOR: Record<HealthState, { fg: string; bg: string }> = {
  normal: { fg: "text.faint", bg: "text.faint" },
  high: { fg: "tone.waiting.fg", bg: "tone.waiting.bg" },
  critical: { fg: "tone.paused.fg", bg: "tone.paused.bg" },
  unknown: { fg: "text.faint", bg: "text.faint" },
};

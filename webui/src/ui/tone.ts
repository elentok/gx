export const TONES = ["running", "waiting", "paused", "deadlocked", "done", "muted"] as const;

export type Tone = (typeof TONES)[number];

export const TONES = ["running", "waiting", "parked", "deadlocked", "done", "muted"] as const;

export type Tone = (typeof TONES)[number];

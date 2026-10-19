export const TONES = ["running", "waiting", "parked", "deadlocked", "muted"] as const;

export type Tone = (typeof TONES)[number];

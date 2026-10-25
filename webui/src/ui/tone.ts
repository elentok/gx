export const TONES = ["running", "waiting", "paused", "deadlocked", "done", "muted"] as const;

export type Tone = (typeof TONES)[number];

export const KINDS = ["design", "implementation", "oneoff", "followups"] as const;

export type Kind = (typeof KINDS)[number];

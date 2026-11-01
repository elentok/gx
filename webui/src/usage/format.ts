import { formatAge } from "../epics/cardTime";
import {
  LimitChipDown,
  LimitChipUp,
  LimitForecastNeedsData,
  LimitForecastProjected,
  LimitsStaleAfterSeconds,
  type LimitBurnChip,
  type LimitForecast,
} from "../generated/server";
import type { Tone } from "../ui/tone";

export { formatCost } from "../epic/rows";

/** The whole number a row shows. Bar tone, text and the server's comparison all use this rule. */
export const wholePercent = (p: number): number => Math.round(p);

/** Takes the whole number the row shows, so 79.5 (shown as 80) is amber. */
export function limitTone(
  whole: number,
): Extract<Tone, "running" | "waiting" | "deadlocked"> {
  if (whole >= 95) return "deadlocked";
  if (whole >= 80) return "waiting";
  return "running";
}

export const isStale = (receivedAtSec: number, nowMs: number): boolean =>
  nowMs / 1000 - receivedAtSec > LimitsStaleAfterSeconds;

export const isPastReset = (resetsAtSec: number, nowMs: number): boolean =>
  resetsAtSec * 1000 <= nowMs;

export const resetsIn = (resetsAtSec: number, nowMs: number): string =>
  `resets in ${formatAge(resetsAtSec * 1000 - nowMs)}`;

export const seenAgo = (receivedAtSec: number, nowMs: number): string =>
  `seen ${formatAge(nowMs - receivedAtSec * 1000)} ago`;

const exactTimeFormat = new Intl.DateTimeFormat("en-US", {
  weekday: "short",
  month: "short",
  day: "numeric",
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
});

export const exactTime = (resetsAtSec: number): string =>
  exactTimeFormat.format(new Date(resetsAtSec * 1000));

const runsOutFormat = new Intl.DateTimeFormat("en-US", {
  weekday: "short",
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
});

/** The forecast line of a weekly row; undefined when the server sends none or the row shows none. Shared with the Stats burn cards. */
export function forecastLine(
  f: LimitForecast | undefined,
): { text: string; warn: boolean } | undefined {
  if (!f) return undefined;
  if (f.status === LimitForecastNeedsData)
    return { text: "Forecast needs 3 days of data", warn: false };
  if (f.status !== LimitForecastProjected) return undefined;
  if (f.runs_out_at)
    return {
      text: `Runs out ~${runsOutFormat.format(new Date(f.runs_out_at * 1000))}, before the reset`,
      warn: true,
    };
  return { text: `~${f.at_reset}% at reset`, warn: false };
}

export const chipText = (c: LimitBurnChip): string => {
  const mark =
    c.direction === LimitChipUp
      ? "▲"
      : c.direction === LimitChipDown
        ? "▼"
        : "=";
  return `${mark} ${c.today} vs ${c.yesterday} pts`;
};

export const chipTooltip = (c: LimitBurnChip): string =>
  `Burn today ${c.today} pts so far, yesterday ${c.yesterday}`;

const dayFormat = new Intl.DateTimeFormat("en-US", {
  weekday: "short",
  month: "short",
  day: "numeric",
});

/** "since Fri, Oct 30" from a "2026-10-30" ledger key. Built from parts in local time: Date.parse of the key would be UTC and shift the day west of UTC. */
export function sinceLabel(dayKey: string): string {
  const [y, m, d] = dayKey.split("-").map(Number);
  return `since ${dayFormat.format(new Date(y, m - 1, d))}`;
}

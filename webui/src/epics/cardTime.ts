import { CardTimeCreated, CardTimeElapsed, CardTimeFinished, CardTimeWaiting, type CardTimeKind } from "../generated/server";

const MINUTE = 60_000;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;
const WEEK = 7 * DAY;

/** "3h", "3h 5m": the second unit is dropped when it is zero. */
function units(big: number, bigUnit: string, small: number, smallUnit: string): string {
  return small === 0 ? `${big}${bigUnit}` : `${big}${bigUnit} ${small}${smallUnit}`;
}

/** At most two units, floored; a negative age counts as zero. */
export function formatAge(ms: number): string {
  const age = Math.max(0, ms);
  if (age < MINUTE) return "<1m";
  if (age < HOUR) return `${Math.floor(age / MINUTE)}m`;
  if (age < DAY) return units(Math.floor(age / HOUR), "h", Math.floor((age % HOUR) / MINUTE), "m");
  if (age < WEEK) return units(Math.floor(age / DAY), "d", Math.floor((age % DAY) / HOUR), "h");
  if (age < 30 * DAY) return units(Math.floor(age / WEEK), "w", Math.floor((age % WEEK) / DAY), "d");
  return `${Math.floor(age / WEEK)}w`;
}

/** The card's time label, or undefined when the card has no known start. */
export function timeLabel(kind: CardTimeKind | undefined, start: string | undefined, now: number): string | undefined {
  if (!kind || !start) return undefined;
  const t = Date.parse(start);
  if (Number.isNaN(t)) return undefined;
  const age = formatAge(now - t);
  switch (kind) {
    case CardTimeElapsed:
      return age;
    case CardTimeWaiting:
      return `waiting ${age}`;
    case CardTimeCreated:
      return `created ${age} ago`;
    case CardTimeFinished:
      return `${age} ago`;
    default:
      return undefined;
  }
}

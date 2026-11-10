export interface Point {
  x: number;
  y: number;
}

type Direction = "left" | "right" | "up" | "down";

const DIRECTIONS: Record<string, Direction> = {
  ArrowLeft: "left",
  h: "left",
  ArrowRight: "right",
  l: "right",
  ArrowUp: "up",
  k: "up",
  ArrowDown: "down",
  j: "down",
};

/** Two items whose tops differ by less than this are in the same row. */
const ROW_TOLERANCE = 1;

export const isMoveKey = (key: string) => key in DIRECTIONS;

/**
 * The index to focus after `key`, from the item at `from`. `points` are the items' top-left corners in
 * reading order. Left and Right step through that order, so they wrap from one row to the next. Up and
 * Down go to the nearest row above or below and, within it, the item closest in x; that also crosses
 * from one section's grid to the next. At an edge the index stays put.
 */
export function moveInGrid(points: Point[], from: number, key: string): number {
  const dir = DIRECTIONS[key];
  const cur = points[from];
  if (!dir || !cur) return from;
  if (dir === "left") return Math.max(from - 1, 0);
  if (dir === "right") return Math.min(from + 1, points.length - 1);
  const sign = dir === "down" ? 1 : -1;
  let rowY: number | undefined;
  for (const p of points) {
    const ahead = (p.y - cur.y) * sign > ROW_TOLERANCE;
    if (ahead && (rowY === undefined || (p.y - rowY) * sign < 0)) rowY = p.y;
  }
  if (rowY === undefined) return from;
  let best = from;
  let bestDist = Infinity;
  points.forEach((p, i) => {
    if (Math.abs(p.y - (rowY as number)) > ROW_TOLERANCE) return;
    const dist = Math.abs(p.x - cur.x);
    if (dist < bestDist) {
      best = i;
      bestDist = dist;
    }
  });
  return best;
}

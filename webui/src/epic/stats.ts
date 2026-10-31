import type { EpicDetail } from "./detail";
import { NO_COST, formatCost, formatDuration } from "./rows";

export interface StatRow {
  label: string;
  value: string;
  note?: string;
}

const DAY = 86400;
const TOP_REASONS = 2;

const formatTokens = (n: number) => (n >= 1e6 ? `${(n / 1e6).toFixed(1)}M` : n >= 1e3 ? `${(n / 1e3).toFixed(1)}k` : String(n));

// Lead times run to days, which formatDuration does not.
const formatLead = (secs: number) => (secs >= DAY ? `${Math.floor(secs / DAY)}d` : formatDuration(secs));

const sum = (counts: Record<string, number> = {}) => Object.values(counts).reduce((a, b) => a + b, 0);

// "name ×n" for the largest counts, biggest first.
const topCounts = (counts: Record<string, number> = {}, limit = Infinity) =>
  Object.entries(counts)
    .sort(([, a], [, b]) => b - a)
    .slice(0, limit)
    .map(([name, n]) => `${name} ×${n}`)
    .join(", ");

// The collapsed card's dim preview: lines (loop only), tokens and lead time, whichever are stamped.
export function statPreview(d: EpicDetail): string {
  const { lines, tokens, lead } = headlineValues(d);
  return [
    d.kind !== "map" && lines && `${lines} lines`,
    tokens && `${tokens} tokens`,
    lead && `lead time ${lead}`,
  ]
    .filter(Boolean)
    .join(" · ");
}

// The formatted values of the three headline rows, undefined when not stamped.
function headlineValues({ stats: s }: EpicDetail) {
  return {
    lines: s.lines_added === undefined ? undefined : `+${s.lines_added.toLocaleString("en-US")} −${(s.lines_removed ?? 0).toLocaleString("en-US")}`,
    tokens: s.total_tokens === undefined ? undefined : formatTokens(s.total_tokens),
    lead: s.lead_time === undefined ? undefined : s.lead_time_so_far ? `${formatLead(s.lead_time)} so far` : formatLead(s.lead_time),
  };
}

// A row with no stamped value is left out.
export function statRows(d: EpicDetail): StatRow[] {
  const { stats: s } = d;
  const f = s.friction;
  const { lines, tokens, lead } = headlineValues(d);
  const rows: StatRow[] = [];
  if (lines) rows.push({ label: "Lines", value: lines });
  if (tokens) rows.push({ label: "Tokens", value: tokens });
  // The server drops the cost of a Codex epic, so tokens without a cost means a dash.
  if (s.cost !== undefined) rows.push({ label: "Cost", value: formatCost(s.cost) });
  else if (tokens) rows.push({ label: "Cost", value: NO_COST });
  if (lead) {
    rows.push({
      label: "Lead time",
      value: lead,
      note: d.kind === "map" || s.lead_time_so_far ? undefined : "created → merged to trunk",
    });
  }
  if (f) {
    rows.push({ label: "Pauses", value: String(sum(f.pauses_by_reason)), note: topCounts(f.pauses_by_reason, TOP_REASONS) || undefined });
    rows.push({ label: "Run events", value: String(sum(f.run_events)), note: topCounts(f.run_events) || undefined });
    rows.push({ label: "Paused time", value: formatDuration(f.paused_secs) });
    if (f.web_share !== undefined) rows.push({ label: "Pause endings", value: `${Math.round(f.web_share * 100)}% web` });
  }
  return rows;
}

import type { RelatedTicket, TicketDetail } from "../generated/server";
import { RelationBlocker, RelationFork, RelationParent } from "../generated/server";
import { epicOfTicket } from "./detail";

const FENCE = /^\s*(```|~~~)/;
const H2 = /^## (.*?)\s*$/;

interface Part {
  heading: string | null;
  lines: string[];
}

// Splits a body at its `##` headings; a heading inside a code fence stays text.
function partsOf(body: string): Part[] {
  const parts: Part[] = [{ heading: null, lines: [] }];
  let fenced = false;
  for (const line of body.split("\n")) {
    if (FENCE.test(line)) fenced = !fenced;
    const h = !fenced && line.match(H2);
    if (h) parts.push({ heading: h[1], lines: [line] });
    else parts[parts.length - 1].lines.push(line);
  }
  return parts;
}

const text = (parts: Part[]) => parts.flatMap((p) => p.lines).join("\n").trim();

export interface PageText {
  resolution: string;
  description: string;
}

// A Paused ticket shows its Paused section in the action card; on any other ticket the section is
// a leftover and stays in the Description. A map ticket's Resolution and Question headings are
// the section headings, so they are not repeated.
export function pageText(d: TicketDetail): PageText {
  const parts = partsOf(d.body ?? "").filter((p) => !(d.pause_text && p.heading === "Paused"));
  if (!d.map_epic) return { resolution: "", description: text(parts) };
  const resolution = parts.find((p) => p.heading === "Resolution");
  const rest = parts
    .filter((p) => p !== resolution)
    .map((p) => (p.heading === "Question" ? { ...p, lines: p.lines.slice(1) } : p));
  return { resolution: resolution ? text([{ ...resolution, lines: resolution.lines.slice(1) }]) : "", description: text(rest) };
}

const REASONS: Record<string, string> = {
  [RelationBlocker]: "blocks this",
  [RelationParent]: "forked from",
};

export const reasonOf = (r: RelatedTicket) => `${REASONS[r.relation] ?? RelationFork} · ${r.status}`;

// A ticket of the page's own epic reads NN; another epic's reads epic/NN, with no project prefix.
export function relatedId(r: RelatedTicket, epic: string): string {
  if (!r.linkable) return r.address;
  const own = epicOfTicket(r.address) === epic;
  const tail = r.address.slice(r.address.indexOf(":") + 1);
  return own ? tail.slice(tail.lastIndexOf("/") + 1) : tail;
}

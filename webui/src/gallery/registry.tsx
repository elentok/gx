import type { ReactNode } from "react";
import canned from "../canned-snapshot.json";
import { Badge } from "../ui/badge/Badge";
import { Card } from "../ui/card/Card";
import { ProgressBar } from "../ui/progress-bar/ProgressBar";
import { TicketLine } from "../ui/ticket-line/TicketLine";
import { TONES, type Tone } from "../ui/tone";

export interface GalleryCase {
  label: string;
  node: ReactNode;
}

export interface GalleryEntry {
  name: string;
  cases: GalleryCase[];
}

const ticket = canned.tickets[0];
const longReason = "Waiting on a long chain of upstream tickets that all have to land first. ".repeat(4);

const perTone = (render: (tone: Tone) => ReactNode): GalleryCase[] =>
  TONES.map((tone) => ({ label: tone, node: render(tone) }));

// Add one entry per primitive; the gallery page renders every case.
export const galleryEntries: GalleryEntry[] = [
  { name: "Badge", cases: perTone((tone) => <Badge tone={tone}>{tone}</Badge>) },
  {
    name: "ProgressBar",
    cases: [
      ...perTone((tone) => <ProgressBar value={6} max={10} tone={tone} />),
      { label: "empty", node: <ProgressBar value={0} max={10} /> },
      { label: "full", node: <ProgressBar value={10} max={10} /> },
    ],
  },
  {
    name: "TicketLine",
    cases: [
      ...perTone((tone) => <TicketLine id={ticket.address} title={ticket.title} tone={tone} />),
      { label: "with reason", node: <TicketLine id={ticket.address} title={ticket.title} reason="Blocked by 01" /> },
      { label: "clamped reason", node: <TicketLine id={ticket.address} title={ticket.title} reason={longReason} /> },
    ],
  },
  {
    name: "Card",
    cases: [
      ...perTone((tone) => (
        <Card title={tone} tone={tone}>
          {ticket.title}
        </Card>
      )),
      { label: "no title", node: <Card>{ticket.title}</Card> },
    ],
  },
];

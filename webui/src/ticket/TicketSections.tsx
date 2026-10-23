import { Box, Text } from "@chakra-ui/react";
import type { TicketDetail } from "../generated/server";
import { Markdown } from "../markdown/Markdown";
import { SectionHeading } from "../page/SectionHeading";
import { TicketLine } from "../ui/ticket-line/TicketLine";
import type { Tone } from "../ui/tone";
import { runLine } from "./runs";
import { toneOfStatus } from "../epic/rows";
import { pageText, reasonOf, relatedId } from "./sections";

function Section({ id, tone, title, children }: { id: string; tone: Tone; title: string; children: React.ReactNode }) {
  return (
    <Box as="section" aria-labelledby={id} mt="6">
      <SectionHeading id={id} tone={tone}>
        {title}
      </SectionHeading>
      {children}
    </Box>
  );
}

const showsRuns = (d: TicketDetail) => !d.map_epic && d.runs.length > 0;

/** Whether any section shows, so the page can say it has nothing. */
export const hasSections = (d: TicketDetail) => {
  const { resolution, description } = pageText(d);
  return !!resolution || !!description || d.related.length > 0 || showsRuns(d);
};

// Resolution (map tickets), Related, the description, then Runs, each hidden when empty.
export function TicketSections({ detail, now }: { detail: TicketDetail; now: number }) {
  const { resolution, description } = pageText(detail);
  const epic = detail.epic;
  return (
    <>
      {resolution && (
        <Section id="ticket-resolution" tone="done" title="Resolution">
          <Markdown epic={epic} smallHeadings>
            {resolution}
          </Markdown>
        </Section>
      )}
      {detail.related.length > 0 && (
        <Section id="ticket-related" tone="muted" title="Related">
          <Box display="flex" flexDirection="column" gap="2">
            {detail.related.map((r) => (
              <TicketLine
                key={`${r.relation}:${r.address}`}
                id={relatedId(r, epic)}
                title={r.title}
                reason={reasonOf(r)}
                tone={toneOfStatus(r.status)}
                address={r.linkable ? r.address : undefined}
                inlineReason
              />
            ))}
          </Box>
        </Section>
      )}
      {description && (
        <Section id="ticket-description" tone="muted" title={detail.map_epic ? "Question" : "Description"}>
          <Markdown epic={epic} smallHeadings>
            {description}
          </Markdown>
        </Section>
      )}
      {showsRuns(detail) && (
        <Section id="ticket-runs" tone="muted" title="Runs">
          <Box as="ul" listStyleType="none" display="flex" flexDirection="column" gap="1">
            {detail.runs.map((r) => (
              <Text as="li" key={r.start} fontSize="sm">
                {runLine(r, now)}
              </Text>
            ))}
          </Box>
        </Section>
      )}
    </>
  );
}

export const NothingHere = () => (
  <Text mt="4" color="text.muted">
    Nothing here yet
  </Text>
);

import { Box, Link, Text } from "@chakra-ui/react";
import type { EpicView } from "../view";
import { Badge } from "../ui/badge/Badge";
import { Card } from "../ui/card/Card";
import { ProgressBar } from "../ui/progress-bar/ProgressBar";
import { TicketLine } from "../ui/ticket-line/TicketLine";

export function EpicCard({ epic }: { epic: EpicView }) {
  return (
    <Card tone={epic.tone}>
      <Box display="flex" justifyContent="space-between" alignItems="center" mb="1">
        <Badge tone={epic.tone}>{epic.badge}</Badge>
        <Text as="span" fontSize="sm" color="text.muted">
          {epic.project}
        </Text>
      </Box>
      <Text fontFamily="condensed" fontSize="xl" fontWeight="600" mb="2">
        <Link href={`#/e/${epic.address}`} color="inherit">
          {epic.name}
        </Link>
      </Text>
      <Box display="flex" alignItems="center" gap="2" mb="2">
        <Box flex="1">
          <ProgressBar value={epic.done} max={epic.total} tone={epic.tone} />
        </Box>
        <Text as="span" fontFamily="condensed" fontSize="sm">
          {epic.done}
          <Text as="span" color="text.muted">
            /{epic.total}
          </Text>
        </Text>
      </Box>
      {epic.tickets.map((t) => (
        <TicketLine
          key={t.address}
          id={t.id}
          title={t.title}
          reason={t.detail}
          tone={t.tone}
          unclipped={t.href !== undefined}
          href={t.href}
          hint={t.hint}
          state={t.state}
          linkLabel={t.linkLabel}
        />
      ))}
      {epic.reason && (
        <Text data-reason fontSize="sm" color="tone.deadlocked.fg" lineClamp="2">
          {epic.reason}
        </Text>
      )}
    </Card>
  );
}

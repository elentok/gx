import { Box, Heading, Input, Link, Text } from "@chakra-ui/react";
import { Fragment, useState } from "react";
import { useConnectionContext, useConnectionState } from "../connection";
import type { EpicListRow, FollowUpCount } from "../generated/server";
import { ConnectedTopBar } from "../nav/ConnectedTopBar";
import { Arc } from "../ui/arc/Arc";
import { Badge } from "../ui/badge/Badge";
import { Button } from "../ui/button/Button";
import { EmptyState } from "../ui/empty-state/EmptyState";
import { PageContainer } from "../ui/PageContainer";
import { Spinner } from "../ui/spinner/Spinner";
import { Strip } from "../ui/strip/Strip";
import { useEpicList, type FetchEpicList } from "./list";
import { QueueButton } from "./QueueButton";
import { searchEpics, sectionsOf, stateBadge, timePart } from "./view";

const Dim = ({ children }: { children: string }) => (
  <Text as="span" color="text.muted">
    {children}
  </Text>
);

const epicLink = (address: string) => `#/e/${address}`;

function EpicRow({
  row,
  now,
  withBadge,
  onQueued,
}: {
  row: EpicListRow;
  now: Date;
  withBadge?: boolean;
  onQueued: () => void;
}) {
  const badge = stateBadge(row.state);
  // Sections mark only a finished map; search rows mark every map.
  const marked = row.kind === "map" && (withBadge || row.state === "finished");
  const parts = [row.project, `${row.done}/${row.total}`, timePart(row, now), marked ? "map" : undefined].filter(
    (p): p is string => p !== undefined,
  );
  // Only unmerged blockers reach the row; the server filters them.
  const blockers = row.state === "ready" ? row.blocked_by_epics : [];
  return (
    <Box display="flex" alignItems="baseline" gap="2" py="1" fontFamily="condensed" fontSize="lg">
      {withBadge && <Badge tone={badge.tone}>{badge.label}</Badge>}
      {row.state === "running" && <Arc label="Running" />}
      <Link href={epicLink(row.address)} color="inherit" fontWeight="600">
        {row.name}
      </Link>
      <Dim>{parts.join(" · ")}</Dim>
      {blockers.length > 0 && (
        <Text as="span" color="text.muted">
          after{" "}
          {blockers.map((a, i) => (
            <Fragment key={a}>
              {i > 0 && ", "}
              <Link href={epicLink(a)} color="inherit">
                {a.slice(a.indexOf(":") + 1)}
              </Link>
            </Fragment>
          ))}
        </Text>
      )}
      {!withBadge && row.queueable && <QueueButton address={row.address} state={row.state} onQueued={onQueued} />}
    </Box>
  );
}

function FollowUps({ counts }: { counts: FollowUpCount[] }) {
  if (counts.length === 0) return null;
  const total = counts.reduce((sum, c) => sum + c.open, 0);
  return (
    <Text fontSize="sm" color="text.muted" ml="auto">
      Follow-ups {total}
      {counts.length > 1 &&
        counts.map((c) => (
          <Fragment key={c.project}>
            {" · "}
            <Link href={epicLink(`${c.project}:follow-ups`)} color="inherit">
              {c.project}
            </Link>{" "}
            {c.open}
          </Fragment>
        ))}
    </Text>
  );
}

export function EpicsPage({ fetchList }: { fetchList?: FetchEpicList }) {
  const conn = useConnectionContext();
  const { connected } = useConnectionState(conn);
  const { list, failed, retry } = useEpicList(fetchList, conn);
  const [query, setQuery] = useState("");
  const now = new Date();
  const searching = query.trim() !== "";
  const sections = list ? sectionsOf(list.epics) : [];

  return (
    <Box>
      {!connected && <Strip tone="waiting">Offline, reconnecting…</Strip>}
      <PageContainer>
        <ConnectedTopBar active="epics" />
        <Box display="flex" alignItems="center" gap="4" mt="4">
          <Input
            type="search"
            aria-label="Search epics"
            placeholder="Search epics"
            maxW="240px"
            size="sm"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
          {list && <FollowUps counts={list.follow_ups} />}
        </Box>
        {failed ? (
          <Box mt="6" display="flex" alignItems="center" gap="3">
            <Text color="text.muted">Could not load epics</Text>
            <Button variant="ghost" onClick={retry}>
              Retry
            </Button>
          </Box>
        ) : !list ? (
          <Spinner label="Loading epics" />
        ) : searching ? (
          <Box mt="4">
            {searchEpics(list.epics, query).map((r) => (
              <EpicRow key={r.address} row={r} now={now} withBadge onQueued={retry} />
            ))}
            {searchEpics(list.epics, query).length === 0 && <EmptyState title="No match" />}
          </Box>
        ) : sections.length === 0 ? (
          <EmptyState title="No epics yet.">Run /gx-design to start one.</EmptyState>
        ) : (
          sections.map((s) => (
            <Box key={s.key} mt="6" aria-labelledby={`epics-${s.key}`} role="region">
              <Heading as="h2" id={`epics-${s.key}`} mb="2">
                <Badge tone={s.tone}>{s.title}</Badge>
              </Heading>
              {s.rows.map((r) => (
                <EpicRow key={r.address} row={r} now={now} onQueued={retry} />
              ))}
            </Box>
          ))
        )}
      </PageContainer>
    </Box>
  );
}

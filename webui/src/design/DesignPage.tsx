import { Box, Text } from "@chakra-ui/react";
import { Navigate } from "react-router";
import { useConnectionContext, useConnectionState } from "../connection";
import type { EpicListRow } from "../generated/server";
import { designHref } from "../hrefs";
import { useEpicList, type FetchEpicList } from "../epics/list";
import { DetailPage } from "../page/DetailPage";
import { EmptyState } from "../ui/empty-state/EmptyState";
import { Sidebar } from "../ui/sidebar/Sidebar";
import { SidebarItem } from "../ui/sidebar/SidebarItem";
import { SidebarSection } from "../ui/sidebar/SidebarSection";
import { Spinner } from "../ui/spinner/Spinner";
import { IN_PROGRESS, pickerGroups, rowHref, type PickerRow } from "./view";

function Marks({ section, row }: { section: string; row: PickerRow }) {
  if (section === "ready-to-queue") return <Mark color="tone.done.fg" label="Ready to queue" glyph="▶" />;
  if (section === "recently-finished") return <Mark color="tone.done.fg" label="Finished" glyph="✓" />;
  return (
    <Box display="flex" gap="1.5" alignItems="center">
      {row.failed && <Mark color="tone.deadlocked.fg" label="Needs attention" glyph="!" />}
      {row.question && <Mark color="tone.waiting.fg" label="Waiting for your answer" glyph="?" />}
      {row.live && <LiveMark />}
    </Box>
  );
}

const Mark = ({ color, label, glyph }: { color: string; label: string; glyph: string }) => (
  <Text as="span" role="img" aria-label={label} title={label} color={color} fontWeight="700" fontSize="sm">
    {glyph}
  </Text>
);

function LiveMark() {
  return (
    <Box as="span" role="img" aria-label="Live" title="Live session" position="relative" display="inline-flex" w="2.5" h="2.5">
      <Box position="absolute" inset="0" borderRadius="full" bg="tone.running.fg" opacity={0.5} animation="ping 1.6s infinite" />
      <Box position="relative" w="2.5" h="2.5" borderRadius="full" bg="tone.running.fg" />
    </Box>
  );
}

function Picker({ list, current }: { list: EpicListRow[]; current?: string }) {
  const sessions = useConnectionState(useConnectionContext()).snapshot?.sessions ?? [];
  return (
    <Sidebar title="Designs">
      {pickerGroups(list, sessions).map((g) => (
        <SidebarSection key={g.key} title={g.title} count={g.rows.length}>
          {g.rows.map((r) => (
            <SidebarItem
              key={r.row.address}
              label={r.row.name}
              href={rowHref(g.key, r.row)}
              current={r.row.address === current}
              marks={<Marks section={g.key} row={r} />}
            />
          ))}
        </SidebarSection>
      ))}
    </Sidebar>
  );
}

// `address` is the Design in the route; the bare route has none and opens the first In progress Design.
export function DesignPage({ address, fetchList }: { address?: string; fetchList?: FetchEpicList }) {
  const conn = useConnectionContext();
  const sessions = useConnectionState(conn).snapshot?.sessions ?? [];
  const { list, failed } = useEpicList(fetchList, conn);

  const first = list && !address ? pickerGroups(list.epics, sessions).find((g) => g.key === IN_PROGRESS)?.rows[0] : undefined;
  if (first) return <Navigate to={designHref(first.row.address).slice(1)} replace />;

  return (
    <DetailPage active="designs">
      {failed ? (
        <Text color="text.muted">Could not load Designs</Text>
      ) : !list ? (
        <Spinner label="Loading Designs" />
      ) : (
        <Box display="grid" gridTemplateColumns={{ base: "1fr", lg: "260px 1fr 300px" }} gap="4" minH="60vh">
          <Picker list={list.epics} current={address} />
          <Box aria-label="Chat" role="region">
            {!address && <EmptyState title="No Design in progress.">Run /gx-design to start one.</EmptyState>}
          </Box>
          <Box aria-label="Progress" role="region" />
        </Box>
      )}
    </DetailPage>
  );
}

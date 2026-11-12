import { Box, Text } from "@chakra-ui/react";
import { formatDate } from "../formatDate";
import { HeaderLink } from "../page/HeaderLink";
import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router";
import { useConnectionContext } from "../connection";
import { TargetDone, type ArchivedEpicRow, type TargetResult } from "../generated/server";
import { EPICS_HREF } from "../hrefs";
import { DetailPage } from "../page/DetailPage";
import { PageHeader } from "../page/PageHeader";
import { unarchiveWithToasts } from "../select/bulkWrites";
import { PAGE_ATTR, usePageKeys } from "../select/pageKeys";
import { rowOfArchived } from "../select/selectModel";
import { SidebarBulkActions } from "../shell/SidebarBulkActions";
import { SelectBar, SelectBox, useSelectApi } from "../shell/SidebarSelect";
import { Button } from "../ui/button/Button";
import { EmptyState } from "../ui/empty-state/EmptyState";
import { GxMenu } from "../ui/menu/Menu";
import { MoreActionsButton } from "../ui/menu/MoreActionsButton";
import { ProgressRing } from "../ui/progress-ring/ProgressRing";
import { FullScreenLoader } from "../ui/loader/FullScreenLoader";
import { useFetched } from "../useFetched";
import { fetchArchived, type FetchArchived } from "./archive";

const ONLY_UNARCHIVE = ["unarchive" as const];

function ArchiveRow({
  row,
  select,
  onOpen,
  onUnarchive,
}: {
  row: ArchivedEpicRow;
  select?: { checked: boolean; onToggle: () => void };
  onOpen: () => void;
  onUnarchive: () => void;
}) {
  const act = select ? select.onToggle : onOpen;
  return (
    <Box
      as="li"
      listStyleType="none"
      {...{ [PAGE_ATTR]: row.address }}
      data-focus-ring="inset"
      aria-label={row.title}
      onClick={act}
      position="relative"
      display="flex"
      alignItems="center"
      gap="4"
      py="18px"
      px="20px"
      pl={select ? "46px" : "20px"}
      cursor="pointer"
      bg="surface.raised"
      borderWidth="1px"
      borderColor={select?.checked ? "tone.running.fg" : "border"}
      borderRadius="12px"
    >
      {select && (
        <Box position="absolute" top="12px" left="12px" zIndex="2" display="flex" onClick={(e) => e.stopPropagation()}>
          <SelectBox checked={select.checked} label={`Select ${row.title}`} onToggle={select.onToggle} size="20px" />
        </Box>
      )}
      <ProgressRing progress={1} tone="done" kind={row.kind === "map" ? "design" : "implementation"} size="sm" label="" />
      <Box minW="0" flex="1">
        <Text fontFamily="condensed" fontSize="24px" fontWeight="600" lineHeight="1.15">
          {row.title}
        </Text>
        <Text color="text.muted" fontSize="14px">
          {[row.project, row.kind === "map" ? "Design" : "Epic", row.archived_at && formatDate(row.archived_at), row.workspace].filter(Boolean).join(" · ")}
        </Text>
      </Box>
      {!select && (
        <Box flex="none" onClick={(e) => e.stopPropagation()}>
          <GxMenu trigger={<MoreActionsButton />} items={[{ value: "unarchive", label: "Unarchive", onSelect: onUnarchive }]} />
        </Box>
      )}
    </Box>
  );
}

/** The archived epics and Designs, in the server's order. */
export function ArchivePage({ fetchList = fetchArchived }: { fetchList?: FetchArchived }) {
  const conn = useConnectionContext();
  const navigate = useNavigate();
  const { data, failed } = useFetched("archived", fetchList, conn);
  // Rows an action's response removed; a fresh list replaces them.
  const [gone, setGone] = useState<Set<string>>(new Set());
  // A page-cache read returns a new object each render, so keep the same Set when nothing is gone.
  useEffect(() => setGone((g) => (g.size === 0 ? g : new Set())), [data]);
  const epics = useMemo(() => (data?.epics ?? []).filter((r) => !gone.has(r.address)), [data, gone]);
  const rows = useMemo(() => epics.map(rowOfArchived), [epics]);
  const names = useMemo(() => new Map(epics.map((r) => [r.address, r.title] as const)), [epics]);
  const select = useSelectApi(rows, names, !!data);
  const area = useRef<HTMLDivElement>(null);
  usePageKeys(area, select);

  const left = (results: TargetResult[]) => {
    const done = results.filter((r) => r.status === TargetDone).map((r) => r.target);
    if (done.length > 0) setGone((g) => new Set([...g, ...done]));
  };
  const unarchiveOne = async (row: ArchivedEpicRow) => {
    const results = await unarchiveWithToasts([row.address], () => row.title);
    if (results) left(results);
  };

  return (
    <DetailPage title={{ kind: "Epics", name: "Archive" }}>
      <Box ref={area}>
      <PageHeader
        title="Archive"
        actions={
          <>
            <HeaderLink href={EPICS_HREF}>← Epics</HeaderLink>
            {epics.length > 0 && !select.state.on && (
              <Button emphasis="secondary" flexShrink="0" whiteSpace="nowrap" onClick={select.enter}>
                Select
              </Button>
            )}
          </>
        }
      />
      {select.state.on && (
        <SelectBar sel={select} sticky>
          <SidebarBulkActions sel={select} shown={ONLY_UNARCHIVE} onResults={left} />
        </SelectBar>
      )}
      {failed ? (
        <Text mt="6" color="text.muted">
          Could not load the archive
        </Text>
      ) : !data ? (
        <FullScreenLoader label="Loading archive" />
      ) : epics.length === 0 ? (
        <EmptyState title="Nothing is archived" />
      ) : (
        <>
          <Text mt="6" mb="3" color="text.muted" fontSize="13px" letterSpacing="0.08em" textTransform="uppercase">
            Newest first
          </Text>
          <Box as="ul" m="0" p="0" display="grid" gap="16px" gridTemplateColumns="repeat(auto-fill, minmax(min(100%, 360px), 1fr))">
            {epics.map((r) => (
              <ArchiveRow
                key={r.address}
                row={r}
                select={select.state.on ? { checked: select.state.keys.includes(r.address), onToggle: () => select.toggle(r.address) } : undefined}
                onOpen={() => navigate(`/archive/${r.address}`)}
                onUnarchive={() => void unarchiveOne(r)}
              />
            ))}
          </Box>
        </>
      )}
      </Box>
    </DetailPage>
  );
}

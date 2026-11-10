import { Box, Link, Text } from "@chakra-ui/react";
import { useEffect, useMemo, useState, type KeyboardEvent } from "react";
import { useNavigate } from "react-router";
import { useConnectionContext } from "../connection";
import { TargetDone, type ArchivedEpicRow, type TargetResult } from "../generated/server";
import { EPICS_HREF } from "../hrefs";
import { DetailPage } from "../page/DetailPage";
import { PageHeader } from "../page/PageHeader";
import { bulkUnarchive } from "../select/bulkWrites";
import { rowOfArchived } from "../select/selectModel";
import { SidebarBulkActions } from "../shell/SidebarBulkActions";
import { SelectBar, SelectBox, useSelectApi } from "../shell/SidebarSelect";
import { Button } from "../ui/button/Button";
import { EmptyState } from "../ui/empty-state/EmptyState";
import { GxMenu } from "../ui/menu/Menu";
import { MoreActionsButton } from "../ui/menu/MoreActionsButton";
import { Spinner } from "../ui/spinner/Spinner";
import { showToast } from "../ui/toaster";
import { useFetched } from "../useFetched";
import { WriteError } from "../useWrite";
import { fetchArchived, type FetchArchived } from "./archive";

const ONLY_UNARCHIVE = ["unarchive" as const];

const dateOf = (iso: string) => new Date(iso).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });

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
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.target === e.currentTarget && e.key === "Enter") act();
  };
  return (
    <Box
      as="li"
      listStyleType="none"
      tabIndex={0}
      aria-label={row.title}
      onClick={act}
      onKeyDown={onKeyDown}
      display="flex"
      alignItems="center"
      gap="3"
      p="4"
      cursor="pointer"
      bg="surface.raised"
      borderWidth="1px"
      borderColor={select?.checked ? "tone.running.fg" : "border"}
      borderRadius="12px"
    >
      {select && (
        <Box display="flex" onClick={(e) => e.stopPropagation()}>
          <SelectBox checked={select.checked} label={`Select ${row.title}`} onToggle={select.onToggle} size="20px" />
        </Box>
      )}
      <Box minW="0" flex="1">
        <Text fontFamily="condensed" fontSize="20px" fontWeight="600" lineHeight="1.2">
          {row.title}
        </Text>
        <Text color="text.muted" fontSize="14px">
          {[row.project, row.kind === "map" ? "Design" : "Epic", row.archived_at && dateOf(row.archived_at), row.workspace].filter(Boolean).join(" · ")}
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
  useEffect(() => setGone(new Set()), [data]);
  const epics = useMemo(() => (data?.epics ?? []).filter((r) => !gone.has(r.address)), [data, gone]);
  const rows = useMemo(() => epics.map(rowOfArchived), [epics]);
  const names = useMemo(() => new Map(epics.map((r) => [r.address, r.title] as const)), [epics]);
  const select = useSelectApi(rows, names, !!data);

  const left = (results: TargetResult[]) => {
    const done = results.filter((r) => r.status === TargetDone).map((r) => r.target);
    if (done.length > 0) setGone((g) => new Set([...g, ...done]));
  };
  const unarchiveOne = async (row: ArchivedEpicRow) => {
    try {
      const results = await bulkUnarchive([row.address]);
      left(results);
      for (const r of results) {
        if (r.status !== TargetDone) showToast({ type: "error", title: `${row.title}: ${r.message || r.reason || "refused"}` });
        else if (r.new_address) showToast({ type: "info", title: `Unarchived as ${r.new_address.split(":").pop()}` });
      }
    } catch (err) {
      showToast({ type: "error", title: err instanceof WriteError ? err.message : "Could not reach gx. Try again." });
    }
  };

  return (
    <DetailPage title={{ kind: "Epics", name: "Archive" }}>
      <Box mt="4">
        <PageHeader
          title="Archive"
          actions={
            epics.length > 0 && (
              <Button emphasis="secondary" flexShrink="0" whiteSpace="nowrap" onClick={select.state.on ? select.leave : select.enter}>
                {select.state.on ? "Cancel" : "Select"}
              </Button>
            )
          }
        />
      </Box>
      <Link href={EPICS_HREF} display="inline-block" mt="3" color="text.muted" textDecoration="none" _hover={{ color: "text", textDecoration: "none" }}>
        ← Epics
      </Link>
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
        <Spinner label="Loading archive" />
      ) : epics.length === 0 ? (
        <EmptyState title="Nothing is archived" />
      ) : (
        <Box as="ul" mt="6" p="0" display="flex" flexDirection="column" gap="3">
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
      )}
    </DetailPage>
  );
}

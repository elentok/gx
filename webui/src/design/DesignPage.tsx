import { Box, Input, Text } from "@chakra-ui/react";
import { useEffect, useState } from "react";
import { Navigate } from "react-router";
import { useConnectionContext, useConnectionState } from "../connection";
import { designHref } from "../hrefs";
import { useEpicDetail, type FetchEpicDetail } from "../epic/detail";
import { useEpicList, type FetchEpicList } from "../epics/list";
import { DetailPage } from "../page/DetailPage";
import { Button } from "../ui/button/Button";
import { EmptyState } from "../ui/empty-state/EmptyState";
import { Sidebar } from "../ui/sidebar/Sidebar";
import { SidebarItem } from "../ui/sidebar/SidebarItem";
import { SidebarSection } from "../ui/sidebar/SidebarSection";
import { Spinner } from "../ui/spinner/Spinner";
import type { FetchChat } from "./chat";
import { DesignChat } from "./ChatLog";
import { ImplLink, Progress } from "./Progress";
import { designSession, filterGroups, IN_PROGRESS, needsYouCount, pickerGroups, rowHref, type PickerGroup, type PickerRow } from "./view";

const NOT_STARTED = "not-started";

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

// Chakra's `lg` breakpoint; below it the picker is a dropdown. Without matchMedia (tests) it is wide.
const WIDE = "(min-width: 62em)";
const useWide = () => {
  const mq = () => (typeof window.matchMedia === "function" ? window.matchMedia(WIDE) : undefined);
  const [wide, setWide] = useState(() => mq()?.matches ?? true);
  useEffect(() => {
    const m = mq();
    if (!m) return;
    const on = () => setWide(m.matches);
    m.addEventListener("change", on);
    return () => m.removeEventListener("change", on);
  }, []);
  return wide;
};

function Picker({ groups, projects, current }: { groups: PickerGroup[]; projects: number; current?: string }) {
  const [query, setQuery] = useState("");
  const filtering = query.trim() !== "";
  return (
    <Sidebar title="Designs">
      <Input
        type="search"
        aria-label="Filter Designs"
        placeholder="Filter"
        size="sm"
        mb="3"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
      />
      {filterGroups(groups, query).map((g) => (
        // The key remounts the section so a filter match opens Not started and clearing it collapses it again.
        <SidebarSection
          key={`${g.key}-${filtering}`}
          title={g.title}
          count={g.rows.length}
          defaultCollapsed={g.key === NOT_STARTED && !filtering}
        >
          {g.rows.map((r) => (
            <SidebarItem
              key={r.row.address}
              label={r.row.name}
              href={rowHref(g.key, r.row)}
              current={r.row.address === current}
              marks={
                <>
                  {projects > 1 && (
                    <Text as="span" fontSize="xs" color="text.dim">
                      {r.row.project}
                    </Text>
                  )}
                  <Marks section={g.key} row={r} />
                </>
              }
            />
          ))}
        </SidebarSection>
      ))}
    </Sidebar>
  );
}

function MobilePicker({ groups, projects, current }: { groups: PickerGroup[]; projects: number; current?: string }) {
  const [open, setOpen] = useState(false);
  const name = groups.flatMap((g) => g.rows).find((r) => r.row.address === current)?.row.name ?? "Designs";
  const more = needsYouCount(groups, current);
  return (
    <Box>
      <Box display="flex" alignItems="center" gap="3">
        <Button size="sm" variant="outline" aria-expanded={open} onClick={() => setOpen((o) => !o)}>
          {name} {open ? "▴" : "▾"}
        </Button>
        {more > 0 && (
          <Text as="span" fontSize="sm" color="tone.waiting.fg">
            ? {more} more
          </Text>
        )}
      </Box>
      {open && <Picker groups={groups} projects={projects} current={current} />}
    </Box>
  );
}

// The Chat and Progress columns of one Design. Progress reads the Design's epic detail.
function DesignColumns({
  address,
  wide,
  fetchDetail,
  fetchChat,
}: {
  address: string;
  wide: boolean;
  fetchDetail?: FetchEpicDetail;
  fetchChat?: FetchChat;
}) {
  const conn = useConnectionContext();
  const { detail } = useEpicDetail(address, conn, fetchDetail);
  const sessions = useConnectionState(conn).snapshot?.sessions ?? [];
  return (
    <>
      <Box aria-label="Chat" role="region" display="flex" flexDirection="column" gap="3">
        {wide && detail?.impl_address && <ImplLink address={detail.impl_address} />}
        <DesignChat session={designSession(address, sessions)} fetchChat={fetchChat} />
      </Box>
      {/* On a phone Progress sits right under the picker, above the chat. */}
      <Box aria-label="Progress" role="region" gridRow={{ base: 2, lg: "auto" }}>
        {detail && <Progress detail={detail} wide={wide} />}
      </Box>
    </>
  );
}

// `address` is the Design in the route; the bare route has none and opens the first In progress Design.
export function DesignPage({
  address,
  fetchList,
  fetchDetail,
  fetchChat,
}: {
  address?: string;
  fetchList?: FetchEpicList;
  fetchDetail?: FetchEpicDetail;
  fetchChat?: FetchChat;
}) {
  const conn = useConnectionContext();
  const sessions = useConnectionState(conn).snapshot?.sessions ?? [];
  const { list, failed } = useEpicList(fetchList, conn);

  const wide = useWide();
  const groups = list ? pickerGroups(list.epics, sessions) : [];
  const projects = new Set(list?.epics.filter((e) => e.kind === "map").map((e) => e.project)).size;

  const first = list && !address ? groups.find((g) => g.key === IN_PROGRESS)?.rows[0] : undefined;
  if (first) return <Navigate to={designHref(first.row.address).slice(1)} replace />;

  return (
    <DetailPage active="designs">
      {failed ? (
        <Text color="text.muted">Could not load Designs</Text>
      ) : !list ? (
        <Spinner label="Loading Designs" />
      ) : (
        <Box display="grid" gridTemplateColumns={{ base: "1fr", lg: "260px 1fr 300px" }} gap="4" minH="60vh">
          {wide ? (
            <Picker groups={groups} projects={projects} current={address} />
          ) : (
            <MobilePicker groups={groups} projects={projects} current={address} />
          )}
          {address ? (
            <DesignColumns address={address} wide={wide} fetchDetail={fetchDetail} fetchChat={fetchChat} />
          ) : (
            <>
              <Box aria-label="Chat" role="region">
                <EmptyState title="No Design in progress.">Run /gx-design to start one.</EmptyState>
              </Box>
              <Box aria-label="Progress" role="region" />
            </>
          )}
        </Box>
      )}
    </DetailPage>
  );
}

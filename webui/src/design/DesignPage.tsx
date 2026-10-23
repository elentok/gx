import { Box, Input, Text } from "@chakra-ui/react";
import { useEffect, useState } from "react";
import { Navigate, useNavigate } from "react-router";
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
import { useFocusReport, type PostFocus } from "./focus";
import { DesignChat } from "./ChatLog";
import { NewDesignForm, type NewDesignDraft } from "./NewDesignForm";
import type { OpenDesign, ResumeSession, StopSession } from "./open";
import type { PostPrompt } from "./prompt";
import { ImplLink, Progress } from "./Progress";
import type { FetchScreen } from "./screen";
import { SessionHeader, StartDesignButton } from "./SessionControls";
import { designSession, liveSessions,filterGroups, IN_PROGRESS, needsYouCount, pickerGroups, rowHref, type PickerGroup, type PickerRow } from "./view";

const NOT_STARTED = "not-started";
// Sections that start collapsed, unless a filter matches in them or they hold the open Design.
const COLLAPSED = [NOT_STARTED, "recently-finished"];

function Marks({ section, row }: { section: string; row: PickerRow }) {
  if (section === "ready-to-queue") return <Mark color="tone.done.fg" label="Ready to queue" glyph="▶" />;
  if (section === "recently-finished") return <Mark color="tone.done.fg" label="Finished" glyph="✓" />;
  return (
    <Box display="flex" gap="1.5" alignItems="center">
      {row.chart && (
        <Text as="span" fontSize="xs" color="text.dim">
          Chart
        </Text>
      )}
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

interface PickerProps {
  groups: PickerGroup[];
  projects: number;
  current?: string;
  onNew: () => void;
}

function Picker({ groups, projects, current, onNew }: PickerProps) {
  const [query, setQuery] = useState("");
  const filtering = query.trim() !== "";
  return (
    <Sidebar
      title="Designs"
      action={
        <Button variant="ghost" onClick={onNew}>
          + New
        </Button>
      }
    >
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
          defaultCollapsed={COLLAPSED.includes(g.key) && !filtering && !g.rows.some((r) => r.row.address === current)}
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

function MobilePicker({ groups, projects, current, onNew }: PickerProps) {
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
      {open && <Picker groups={groups} projects={projects} current={current} onNew={onNew} />}
    </Box>
  );
}

// The Chat and Progress columns of one Design. Progress reads the Design's epic detail.
function DesignColumns({
  address,
  wide,
  fetchDetail,
  fetchChat,
  post,
  open,
  stop,
  resume,
  fetchScreen,
  focus,
  chart,
  onNewSession,
}: {
  address: string;
  wide: boolean;
  fetchDetail?: FetchEpicDetail;
  fetchChat?: FetchChat;
  post?: PostPrompt;
  open?: OpenDesign;
  stop?: StopSession;
  resume?: ResumeSession;
  fetchScreen?: FetchScreen;
  focus?: PostFocus;
  /** The Design has no map yet, so only its session can be started again, from the New Design form. */
  chart?: boolean;
  onNewSession: () => void;
}) {
  const conn = useConnectionContext();
  const { detail } = useEpicDetail(address, conn, fetchDetail);
  const connState = useConnectionState(conn);
  const sessions = connState.snapshot?.sessions ?? [];
  const [selected, setSelected] = useState<string>();
  const done = detail?.steps?.some((s) => s.step === "done" && s.state === "done");
  const nextTitle = detail?.rows.find((r) => r.current)?.title;
  const live = liveSessions(address, sessions);
  // Switching shows an older live session; once it is gone the newest drives the page again.
  const session = live.find((s) => s.id === selected) ?? designSession(address, sessions);
  useFocusReport(conn, session?.id, connState.streams, focus);
  return (
    <>
      <Box aria-label="Chat" role="region" minW="0" display="flex" flexDirection="column" gap="3">
        {wide && detail?.impl_address && <ImplLink address={detail.impl_address} />}
        {session && chart ? (
          session.status.state === "exited" && (
            <Box display="flex" justifyContent="flex-end">
              <Button size="sm" variant="outline" onClick={onNewSession}>
                New session
              </Button>
            </Box>
          )
        ) : session ? (
          <SessionHeader address={address} shown={session} live={live} onSelect={setSelected} open={open} stop={stop} />
        ) : (
          <Box display="flex" justifyContent="center" py="4">
            <StartDesignButton address={address} open={open} />
          </Box>
        )}
        <DesignChat session={session} fetchChat={fetchChat} post={post} done={done} nextTitle={nextTitle} actions={{ onNewSession, resume, fetchScreen }} />
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
  post,
  open,
  stop,
  resume,
  fetchScreen,
  focus,
}: {
  address?: string;
  fetchList?: FetchEpicList;
  fetchDetail?: FetchEpicDetail;
  fetchChat?: FetchChat;
  post?: PostPrompt;
  open?: OpenDesign;
  stop?: StopSession;
  resume?: ResumeSession;
  fetchScreen?: FetchScreen;
  focus?: PostFocus;
}) {
  const conn = useConnectionContext();
  const sessions = useConnectionState(conn).snapshot?.sessions ?? [];
  const { list, failed } = useEpicList(fetchList, conn);

  const navigate = useNavigate();
  const [draft, setDraft] = useState<NewDesignDraft>();

  const wide = useWide();
  const groups = list ? pickerGroups(list.epics, sessions) : [];
  const projectNames = [...new Set([...(list?.epics ?? []).map((e) => e.project), ...groups.flatMap((g) => g.rows.map((r) => r.row.project))])];
  const projects = new Set(list?.epics.filter((e) => e.kind === "map").map((e) => e.project)).size;
  // The form's project select starts on the Design on screen.
  const currentProject = address?.split(":")[0];
  const formProjects = currentProject ? [currentProject, ...projectNames.filter((p) => p !== currentProject)] : projectNames;
  const chartRow = groups.flatMap((g) => g.rows).find((r) => r.row.address === address)?.chart;
  const onNew = () => setDraft({});
  const onNewSession = () => {
    if (address) setDraft({ slug: address.slice(address.indexOf(":") + 1), project: currentProject });
  };

  const first = list && !address ? groups.find((g) => g.key === IN_PROGRESS)?.rows[0] : undefined;
  if (first) return <Navigate to={designHref(first.row.address).slice(1)} replace />;

  return (
    <DetailPage active="designs">
      {draft && (
        <NewDesignForm
          key={`${draft.slug}|${draft.project}`}
          projects={formProjects}
          initial={draft}
          open={open}
          onStarted={(started) => {
            setDraft(undefined);
            navigate(designHref(started).slice(1));
          }}
          onCancel={() => setDraft(undefined)}
        />
      )}
      {failed ? (
        <Text color="text.muted">Could not load Designs</Text>
      ) : !list ? (
        <Spinner label="Loading Designs" />
      ) : (
        <Box display="grid" gridTemplateColumns={{ base: "1fr", lg: "260px minmax(0, 1fr) 300px" }} gap="4" minH="60vh">
          {wide ? (
            <Picker groups={groups} projects={projects} current={address} onNew={onNew} />
          ) : (
            <MobilePicker groups={groups} projects={projects} current={address} onNew={onNew} />
          )}
          {address ? (
            <DesignColumns
              address={address}
              wide={wide}
              fetchDetail={fetchDetail}
              fetchChat={fetchChat}
              post={post}
              open={open}
              stop={stop}
              resume={resume}
              fetchScreen={fetchScreen}
              focus={focus}
              chart={chartRow}
              onNewSession={onNewSession}
            />
          ) : (
            <>
              <Box aria-label="Chat" role="region">
                <EmptyState title="No Design in progress.">
                  <Button tone="running" onClick={onNew}>
                    New Design
                  </Button>
                </EmptyState>
              </Box>
              <Box aria-label="Progress" role="region" />
            </>
          )}
        </Box>
      )}
    </DetailPage>
  );
}

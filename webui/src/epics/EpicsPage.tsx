import { Box, chakra, Input, Text } from "@chakra-ui/react";
import { useEffect, useRef, useState } from "react";
import { useLocation } from "react-router";
import { MissingBanner } from "../checks/MissingBanner";
import { SetupDialog } from "../checks/SetupDialog";
import { postSaveChecks, type SaveChecks } from "../checks/setup";
import { fetchChecksStatus, type FetchChecksStatus } from "../checks/status";
import { useConnectionContext } from "../connection";
import { CardKindFollowUps, type Workspace } from "../generated/server";
import { openNewDesign, setPageProject } from "../shell/newItemOpen";
import { followUpsFocus } from "../workspaces/followUpsBadge";
import { DetailPage } from "../page/DetailPage";
import { PageHeader } from "../page/PageHeader";
import { PageIcon } from "../page/PageIcon";
import { Button } from "../ui/button/Button";
import { EmptyState } from "../ui/empty-state/EmptyState";
import { Spinner } from "../ui/spinner/Spinner";
import { useFetched } from "../useFetched";
import { useWorkspaces } from "../workspaces/useActiveWorkspace";
import { EpicCard } from "./EpicCard";
import { useEpicList, type FetchEpicList } from "./list";
import { ProjectFilter } from "./ProjectFilter";
import { sectionsOf, type Section } from "./view";

const PROJECT_KEY = "gx.epics.project";

/** The stored choice: a JSON array, or the plain project name an older version wrote. */
function readStoredProjects(): string[] {
  try {
    const raw = localStorage.getItem(PROJECT_KEY);
    if (!raw) return [];
    if (raw.startsWith("[")) {
      const parsed: unknown = JSON.parse(raw);
      return Array.isArray(parsed) ? parsed.filter((p): p is string => typeof p === "string") : [];
    }
    return [raw];
  } catch {
    return [];
  }
}

const TICK_MS = 30_000;

/** One page clock for every card's time label; it runs only while `active`. */
function usePageClock(active: boolean): number {
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    if (!active) return;
    setNow(Date.now());
    const id = setInterval(() => setNow(Date.now()), TICK_MS);
    return () => clearInterval(id);
  }, [active]);
  return now;
}

function SectionBlock({
  section,
  now,
  open,
  workspaces,
  onToggle,
  onChanged,
}: {
  section: Section;
  now: number;
  open: boolean;
  workspaces: Workspace[];
  onToggle: () => void;
  onChanged: () => void;
}) {
  const id = `epics-${section.key}`;
  return (
    <Box role="region" aria-labelledby={id} mt="6">
      <Box as="h2" id={id} m="0" mb="3">
        <Box
          as="button"
          display="flex"
          alignItems="center"
          gap="2"
          cursor="pointer"
          fontFamily="condensed"
          fontSize="24px"
          fontWeight="600"
          aria-expanded={open}
          onClick={onToggle}
        >
          <Box aria-hidden>{open ? "▾" : "▸"}</Box>
          <Box aria-hidden w="10px" h="10px" borderRadius="full" bg={`tone.${section.tone}.fg`} />
          {section.title}
          <Text as="span" color="text.muted">
            {section.rows.length}
          </Text>
        </Box>
      </Box>
      {open && (
        <Box display="grid" gap="20px" gridTemplateColumns="repeat(auto-fill, minmax(min(100%, 360px), 1fr))">
          {section.rows.map((r) => (
            <EpicCard key={r.address} row={r} now={now} workspaces={workspaces} onChanged={onChanged} />
          ))}
        </Box>
      )}
    </Box>
  );
}

export function EpicsPage({
  fetchList,
  fetchChecks = fetchChecksStatus,
  saveChecks = postSaveChecks,
}: {
  fetchList?: FetchEpicList;
  fetchChecks?: FetchChecksStatus;
  saveChecks?: SaveChecks;
}) {
  const conn = useConnectionContext();
  const { list, failed, retry } = useEpicList(fetchList, conn);
  // A failed read shows no banner: it is not worth an error state.
  const { data: checks, retry: rereadChecks } = useFetched("checks", fetchChecks, conn);
  const [bannerHidden, setBannerHidden] = useState(false);
  const [settingUp, setSettingUp] = useState<string>();
  const [query, setQuery] = useState("");
  const [toggled, setToggled] = useState<Set<string>>(new Set());
  const [workspace, setWorkspace] = useState<string>();
  const { workspaces } = useWorkspaces(conn);
  const [storedProjects, setStoredProjects] = useState(readStoredProjects);
  const now = usePageClock((list?.epics ?? []).some((e) => e.time_kind && e.time_start));
  const searching = query.trim() !== "";
  // Every project, even one with no epics yet; the epics' own projects cover an older server.
  const projects = [...new Set([...(list?.projects ?? []), ...(list?.epics ?? []).map((e) => e.project)])].sort();
  // Stored projects that are gone are ignored without rewriting them; none left acts as "All projects".
  const known = storedProjects.filter((p) => projects.includes(p));
  const filterProjects = projects.length > 1 && known.length > 0 ? known : undefined;
  const missing = (checks?.projects ?? []).filter((p) => p.missing && (filterProjects === undefined || filterProjects.includes(p.project)));
  const setupProject = checks?.projects.find((p) => p.project === settingUp);
  // The choice lives in this page's state only; a workspace that is gone acts as "All workspaces".
  const filterWorkspace = workspaces.some((w) => w.name === workspace) ? workspace : undefined;
  const sections = list ? sectionsOf(list.epics, query, list.recently_finished, filterProjects, filterWorkspace, list.follow_ups) : [];
  const shown = sections.filter((s) => s.rows.length > 0);
  const chooseProjects = (chosen: string[]) => {
    setStoredProjects(chosen);
    try {
      localStorage.setItem(PROJECT_KEY, JSON.stringify(chosen));
    } catch {
      // Storage is a convenience; the choice still holds for this page.
    }
  };
  // A filter match opens a collapsed section; without one, the toggle flips the default.
  const isOpen = (s: Section) => searching || s.collapsedByDefault === toggled.has(s.key);
  const toggle = (key: string) =>
    setToggled((t) => {
      const next = new Set(t);
      if (!next.delete(key)) next.add(key);
      return next;
    });
  // Route state `focus` names a card (the sidebar's follow-ups popover): open its section once, then scroll to it.
  const focus = (useLocation().state as { focus?: string } | null)?.focus;
  const focused = useRef<string | undefined>(undefined);
  const [scrollTo, setScrollTo] = useState<string>();
  useEffect(() => {
    if (!focus || !list || focused.current === focus) return;
    const row = list.epics.find((e) => e.card_kind === CardKindFollowUps && followUpsFocus(e.project) === focus);
    if (!row) return;
    focused.current = focus;
    const section = sections.find((s) => s.key === row.section);
    if (section?.collapsedByDefault) setToggled((t) => new Set(t).add(section.key));
    setScrollTo(focus);
  }, [focus, list, sections]);
  useEffect(() => {
    if (!scrollTo) return;
    const card = [...document.querySelectorAll("[data-focus]")].find((el) => el.getAttribute("data-focus") === scrollTo);
    if (!card) return;
    card.scrollIntoView?.({ block: "center" });
    setScrollTo(undefined);
  }, [scrollTo, toggled, list]);
  const filterProject = filterProjects?.length === 1 ? filterProjects[0] : undefined;
  useEffect(() => {
    setPageProject(filterProject);
    return () => setPageProject(undefined);
  }, [filterProject]);
  const newDesign = (
    <Button emphasis="main" flexShrink="0" whiteSpace="nowrap" onClick={() => openNewDesign()}>
      New Design
    </Button>
  );

  return (
    <DetailPage title={{ kind: "Epics" }}>
      <Box mt="4">
        <PageHeader
          title="Epics"
          icon={<PageIcon kind="epics" />}
          subtitle={list ? `${list.epics.length} ${list.epics.length === 1 ? "epic" : "epics"}` : undefined}
          actions={list && list.epics.length > 0 ? newDesign : undefined}
        />
      </Box>
      <Box display="flex" alignItems="center" gap="4" mt="4">
        <Input
          type="search"
          aria-label="Filter epics"
          placeholder="Filter epics"
          h="40px"
          fontSize="16px"
          maxW="320px"
          flex="1"
          minW="0"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        {projects.length > 1 && <ProjectFilter projects={projects} selected={filterProjects ?? []} onChange={chooseProjects} />}
        {workspaces.length > 1 && (
          <chakra.select
            aria-label="Workspace"
            value={filterWorkspace ?? ""}
            onChange={(e) => setWorkspace(e.target.value)}
            bg="surface"
            borderWidth="1px"
            borderColor="border"
            borderRadius="sm"
            px="3"
            h="40px"
            fontSize="16px"
            flexShrink="0"
            cursor="pointer"
          >
            <option value="">All workspaces</option>
            {workspaces.map((w) => (
              <option key={w.name} value={w.name}>
                {w.name}
              </option>
            ))}
          </chakra.select>
        )}
      </Box>
      {!bannerHidden && <MissingBanner missing={missing} onSetUp={setSettingUp} onDismiss={() => setBannerHidden(true)} />}
      {failed ? (
        <Box mt="6" display="flex" alignItems="center" gap="3">
          <Text color="text.muted">Could not load epics</Text>
          <Button emphasis="ghost" onClick={retry}>
            Retry
          </Button>
        </Box>
      ) : !list ? (
        <Spinner label="Loading epics" />
      ) : list.epics.length === 0 ? (
        <EmptyState title="No epics yet.">{newDesign}</EmptyState>
      ) : shown.length === 0 ? (
        <EmptyState title="No match" />
      ) : (
        shown.map((s) => (
          <SectionBlock key={s.key} section={s} now={now} open={isOpen(s)} workspaces={workspaces} onToggle={() => toggle(s.key)} onChanged={retry} />
        ))
      )}
      {setupProject && <SetupDialog project={setupProject} save={saveChecks} onSaved={rereadChecks} onClose={() => setSettingUp(undefined)} />}
    </DetailPage>
  );
}

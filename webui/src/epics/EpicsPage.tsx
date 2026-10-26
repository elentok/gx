import { Box, chakra, Input, Text } from "@chakra-ui/react";
import { useState } from "react";
import { useNavigate } from "react-router";
import { useConnectionContext } from "../connection";
import type { OpenDesign } from "../design/open";
import { NewDesignForm } from "../design/NewDesignForm";
import { designHref } from "../hrefs";
import { DetailPage } from "../page/DetailPage";
import { Button } from "../ui/button/Button";
import { EmptyState } from "../ui/empty-state/EmptyState";
import { Spinner } from "../ui/spinner/Spinner";
import { EpicCard } from "./EpicCard";
import { useEpicList, type FetchEpicList } from "./list";
import { sectionsOf, type Section } from "./view";

const PROJECT_KEY = "gx.epics.project";

function readStoredProject(): string {
  try {
    return localStorage.getItem(PROJECT_KEY) ?? "";
  } catch {
    return "";
  }
}

function SectionBlock({ section, open, onToggle, onChanged }: { section: Section; open: boolean; onToggle: () => void; onChanged: () => void }) {
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
            <EpicCard key={r.address} row={r} onChanged={onChanged} />
          ))}
        </Box>
      )}
    </Box>
  );
}

export function EpicsPage({ fetchList, openDesign }: { fetchList?: FetchEpicList; openDesign?: OpenDesign }) {
  const conn = useConnectionContext();
  const navigate = useNavigate();
  const { list, failed, retry } = useEpicList(fetchList, conn);
  const [query, setQuery] = useState("");
  const [toggled, setToggled] = useState<Set<string>>(new Set());
  const [designing, setDesigning] = useState(false);
  const [storedProject, setStoredProject] = useState(readStoredProject);
  const searching = query.trim() !== "";
  const projects = [...new Set((list?.epics ?? []).map((e) => e.project))];
  // A stored project that is gone, or a single project, acts as "All projects" without rewriting it.
  const filterProject = projects.length > 1 && projects.includes(storedProject) ? storedProject : undefined;
  const sections = list ? sectionsOf(list.epics, query, list.recently_finished, filterProject) : [];
  const shown = searching ? sections.filter((s) => s.rows.length > 0) : sections;
  const chooseProject = (project: string) => {
    setStoredProject(project);
    try {
      localStorage.setItem(PROJECT_KEY, project);
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
  const newDesign = (
    <Button emphasis="main" flexShrink="0" whiteSpace="nowrap" onClick={() => setDesigning(true)}>
      New Design
    </Button>
  );

  return (
    <DetailPage active="epics">
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
        {projects.length > 1 && (
          <chakra.select
            aria-label="Project"
            value={filterProject ?? ""}
            onChange={(e) => chooseProject(e.target.value)}
            bg="surface"
            borderWidth="1px"
            borderColor="border"
            borderRadius="sm"
            px="2"
            h="40px"
            fontSize="16px"
            flexShrink="0"
          >
            <option value="">All projects</option>
            {projects.map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
          </chakra.select>
        )}
        {list && list.epics.length > 0 && newDesign}
      </Box>
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
      ) : searching && shown.length === 0 ? (
        <EmptyState title="No match" />
      ) : (
        shown.map((s) => <SectionBlock key={s.key} section={s} open={isOpen(s)} onToggle={() => toggle(s.key)} onChanged={retry} />)
      )}
      {designing && (
        <NewDesignForm
          projects={projects}
          filterProject={filterProject}
          open={openDesign}
          onStarted={(address) => navigate(designHref(address).slice(1))}
          onClose={() => setDesigning(false)}
        />
      )}
    </DetailPage>
  );
}

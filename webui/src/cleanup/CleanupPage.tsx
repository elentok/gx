import { Box, chakra, Text } from "@chakra-ui/react";
import { useSearchParams } from "react-router";
import { useConnectionContext } from "../connection";
import { useEpicList, type FetchEpicList } from "../epics/list";
import { DetailPage } from "../page/DetailPage";
import { PageHeader } from "../page/PageHeader";
import { PageIcon } from "../page/PageIcon";
import { SectionHeading } from "../page/SectionHeading";
import { Button } from "../ui/button/Button";
import { Spinner } from "../ui/spinner/Spinner";
import { formatBytes, groupReport, useCleanup, type CleanupGroup, type FetchCleanup } from "./cleanup";

function Group({ group }: { group: CleanupGroup }) {
  const id = `cleanup-${group.id}`;
  return (
    <Box mt="6" role="region" aria-labelledby={id}>
      <SectionHeading id={id} tone={group.id === "agent" ? "waiting" : "running"}>
        {group.title}
      </SectionHeading>
      <Text color="text.muted" fontSize="sm" mb="2">
        {group.hint}
      </Text>
      <Box as="ul" listStyleType="none" p="0" display="flex" flexDirection="column" gap="1">
        {group.worktrees.map((w) => (
          <Box as="li" key={w.branch} display="flex" justifyContent="space-between" gap="3" wordBreak="break-all">
            <Text>{w.branch}</Text>
            {w.size_bytes !== undefined && (
              <Text color="text.muted" flexShrink="0" data-testid="cleanup-size">
                {formatBytes(w.size_bytes)}
              </Text>
            )}
          </Box>
        ))}
        {group.epics.map((e) => (
          <Box as="li" key={`epic:${e.name}`} wordBreak="break-all">
            <Text>{e.name}</Text>
          </Box>
        ))}
      </Box>
    </Box>
  );
}

export function CleanupPage({ fetchCleanup, fetchEpics }: { fetchCleanup?: FetchCleanup; fetchEpics?: FetchEpicList }) {
  const conn = useConnectionContext();
  const [params, setParams] = useSearchParams();
  const { list } = useEpicList(fetchEpics, conn);
  const projects = [...new Set(list?.epics.map((e) => e.project) ?? [])].sort();
  const project = params.get("project") ?? projects[0] ?? "";
  const { report, failed, retry } = useCleanup(project, fetchCleanup, conn);
  const groups = report ? groupReport(report) : [];

  return (
    <DetailPage title={{ kind: "Cleanup" }}>
      <Box mt="4">
        <PageHeader title="Cleanup" icon={<PageIcon kind="cleanup" />} subtitle={project} />
      </Box>
      <Box mt="4">
        <chakra.select
          aria-label="Project"
          value={project}
          onChange={(e) => setParams({ project: e.target.value })}
          fontSize="sm"
          px="2"
          py="1"
          borderWidth="1px"
          borderRadius="md"
          bg="transparent"
        >
          {projects.map((p) => (
            <option key={p} value={p}>
              {p}
            </option>
          ))}
        </chakra.select>
      </Box>
      {failed ? (
        <Box mt="6" display="flex" alignItems="center" gap="3">
          <Text color="text.muted">Could not load cleanup</Text>
          <Button emphasis="ghost" onClick={retry}>
            Retry
          </Button>
        </Box>
      ) : !report ? (
        <Spinner label="Loading cleanup" />
      ) : groups.length === 0 ? (
        <Text mt="6" color="text.muted">
          Nothing to clean up
        </Text>
      ) : (
        groups.map((g) => <Group key={g.id} group={g} />)
      )}
    </DetailPage>
  );
}

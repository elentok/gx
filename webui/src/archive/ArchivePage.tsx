import { Box, Link, Text } from "@chakra-ui/react";
import { useConnectionContext } from "../connection";
import { EPICS_HREF } from "../hrefs";
import { DetailPage } from "../page/DetailPage";
import { PageHeader } from "../page/PageHeader";
import { EmptyState } from "../ui/empty-state/EmptyState";
import { Spinner } from "../ui/spinner/Spinner";
import { useFetched } from "../useFetched";
import { fetchArchived, type FetchArchived } from "./archive";

/** The archived epics. The shell shows no rows yet. */
export function ArchivePage({ fetchList = fetchArchived }: { fetchList?: FetchArchived }) {
  const conn = useConnectionContext();
  const { data, failed } = useFetched("archived", fetchList, conn);
  return (
    <DetailPage title={{ kind: "Epics", name: "Archive" }}>
      <Box mt="4">
        <PageHeader title="Archive" />
      </Box>
      <Link href={EPICS_HREF} display="inline-block" mt="3" color="text.muted" textDecoration="none" _hover={{ color: "text", textDecoration: "none" }}>
        ← Epics
      </Link>
      {failed ? (
        <Text mt="6" color="text.muted">
          Could not load the archive
        </Text>
      ) : !data ? (
        <Spinner label="Loading archive" />
      ) : data.epics.length === 0 ? (
        <EmptyState title="Nothing is archived" />
      ) : null}
    </DetailPage>
  );
}

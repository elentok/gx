import { Box, Text } from "@chakra-ui/react";
import { useParams } from "react-router";
import { useConnectionContext, useConnectionState } from "../connection";
import { DetailPage } from "../page/DetailPage";
import { Spinner } from "../ui/spinner/Spinner";
import { useWide } from "../useWide";
import { AgentSidebar } from "./AgentSidebar";

// The snapshot is cached by snapshotCache, so a reload repaints from it before the stream lands.
export function AgentsPage() {
  const selected = useParams()["*"] || undefined;
  const wide = useWide();
  const { snapshot } = useConnectionState(useConnectionContext());
  const sessions = snapshot?.sessions;
  return (
    <DetailPage active="agents" fill mode="wide">
      {!sessions ? (
        <Spinner label="Loading agents" />
      ) : (
        <Box flex="1" minH="0" display="grid" gridTemplateColumns={wide ? "300px 1fr" : "1fr"}>
          {(wide || !selected) && <AgentSidebar sessions={sessions} selected={selected} />}
          {(wide || selected) && (
            <Box as="main" p="4" minW="0">
              {selected ? null : (
                <Text color="text.muted">{sessions.length === 0 ? "No sessions yet" : "Pick a session"}</Text>
              )}
            </Box>
          )}
        </Box>
      )}
    </DetailPage>
  );
}

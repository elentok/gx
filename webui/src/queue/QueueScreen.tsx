import { Box, Heading } from "@chakra-ui/react";
import type { ConnectionState } from "../connection";
import { EmptyState } from "../ui/empty-state/EmptyState";
import { PageContainer } from "../ui/PageContainer";
import { Strip } from "../ui/strip/Strip";
import { TopBar } from "../ui/top-bar/TopBar";
import { snapshotView } from "../view";
import { EpicCard } from "./EpicCard";
import { RecentlyFinishedRow } from "./RecentlyFinishedRow";

export function QueueScreen({ state }: { state: ConnectionState }) {
  const { snapshot, connected, loadedOnce } = state;
  // state.tick is not read: a new render per tick is what refreshes the relative times.
  const view = snapshot ? snapshotView(snapshot, new Date()) : null;
  const mode = snapshot?.mode;

  return (
    <Box>
      {!connected && <Strip tone="waiting">Offline, reconnecting…</Strip>}
      <PageContainer>
        <TopBar active="queue" build={snapshot?.build} mode={mode} connected={connected} />
        {view && (
          <Box display="flex" flexDirection="column" gap="3">
            {view.epics.map((e) => (
              <EpicCard key={e.address} epic={e} />
            ))}
          </Box>
        )}
        {loadedOnce && view && view.epics.length === 0 && (
          <EmptyState title="All quiet">Nothing is queued. Add an epic in the TUI and it will show up here.</EmptyState>
        )}
        {view && view.recentlyFinished.length > 0 && (
          <Box mt="6">
            <Heading as="h2" fontFamily="condensed" size="md" color="text.muted" mb="2">
              Recently finished
            </Heading>
            {view.recentlyFinished.map((f) => (
              <RecentlyFinishedRow key={f.address} row={f} />
            ))}
          </Box>
        )}
      </PageContainer>
    </Box>
  );
}

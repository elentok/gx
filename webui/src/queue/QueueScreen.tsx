import { Box, Heading } from "@chakra-ui/react";
import { toggleAfk } from "../afk";
import type { ConnectionState } from "../connection";
import { EmptyState } from "../ui/empty-state/EmptyState";
import { PageContainer } from "../ui/PageContainer";
import { Strip } from "../ui/strip/Strip";
import { TopBar } from "../ui/top-bar/TopBar";
import { snapshotView } from "../view";
import { EpicCard } from "./EpicCard";
import { RecentlyFinishedRow } from "./RecentlyFinishedRow";

export function QueueScreen({ state, onChanged = () => {} }: { state: ConnectionState; onChanged?: () => void }) {
  const { snapshot, connected, loadedOnce } = state;
  // state.tick is not read: a new render per tick is what refreshes the relative times.
  const view = snapshot ? snapshotView(snapshot, new Date()) : null;
  const mode = snapshot?.mode;

  return (
    <Box>
      {!connected && <Strip tone="waiting">Offline, reconnecting…</Strip>}
      <PageContainer>
        <TopBar
          active="queue"
          build={snapshot?.build}
          mode={mode}
          connected={connected}
          afk={snapshot?.afk}
          onToggleAfk={() => toggleAfk(snapshot?.afk)}
        />
        {view && (
          // Cards wrap into as many columns as fit, each at least 420px wide.
          <Box display="grid" gridTemplateColumns="repeat(auto-fill, minmax(min(100%, 420px), 1fr))" gap="3" alignItems="start">
            {view.epics.map((e) => (
              <EpicCard key={e.address} epic={e} onMerged={onChanged} />
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

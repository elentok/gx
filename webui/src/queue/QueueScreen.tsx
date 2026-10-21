import { Box, Heading, Text } from "@chakra-ui/react";
import type { ConnectionState } from "../connection";
import { Chip } from "../ui/chip/Chip";
import { EmptyState } from "../ui/empty-state/EmptyState";
import { LiveDot } from "../ui/live-dot/LiveDot";
import { Strip } from "../ui/strip/Strip";
import { snapshotView } from "../view";
import { EpicCard } from "./EpicCard";
import { RecentlyFinishedRow } from "./RecentlyFinishedRow";

export function QueueScreen({ state }: { state: ConnectionState }) {
  const { snapshot, connected, loadedOnce } = state;
  // state.tick is not read: a new render per tick is what refreshes the relative times.
  const view = snapshot ? snapshotView(snapshot, new Date()) : null;
  const mode = snapshot?.mode;
  const queueChip = mode === "paused" ? "Paused" : mode === "draining" ? "Draining" : null;

  return (
    <Box>
      {!connected && <Strip tone="waiting">Offline, reconnecting…</Strip>}
      <Box p="4" maxW="1000px" mx="auto">
        <Box display="flex" alignItems="center" justifyContent="space-between" mb="4">
          <Heading as="h1" fontFamily="condensed" textTransform="uppercase" size="lg">
            Queue
          </Heading>
          <Box display="flex" alignItems="center" gap="2">
            {snapshot?.build && (
              <Text fontSize="xs" color="text.muted" data-testid="server-build">
                {snapshot.build}
              </Text>
            )}
            {queueChip && <Chip tone="waiting">{queueChip}</Chip>}
            <LiveDot tone={connected ? "running" : "muted"} label={connected ? "Live" : "Offline"} />
          </Box>
        </Box>
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
      </Box>
    </Box>
  );
}

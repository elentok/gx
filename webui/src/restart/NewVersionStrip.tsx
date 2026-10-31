import { Box, Text } from "@chakra-ui/react";
import { Button } from "../ui/button/Button";

// Module level: each page mounts its own top bar, so a per-mount baseline
// would reset on navigation and hide the strip.
let firstBuild: string | undefined;

export function resetBuildBaseline() {
  firstBuild = undefined;
}

/** True once the server's build differs from the first one this page load saw. */
export function useBuildChanged(build?: string): boolean {
  if (build && firstBuild === undefined) firstBuild = build;
  return !!build && firstBuild !== undefined && build !== firstBuild;
}

export function NewVersionStrip({ reload = () => window.location.reload() }: { reload?: () => void }) {
  return (
    <Box role="status" display="flex" alignItems="center" justifyContent="space-between" gap="2" bg="surface.raised" borderWidth="1px" borderColor="border" borderRadius="md" px="3" py="2" mb="3">
      <Text>New version, reload</Text>
      <Button tone="running" size="sm" onClick={reload}>
        Reload
      </Button>
    </Box>
  );
}

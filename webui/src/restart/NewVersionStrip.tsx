import { Box, Text } from "@chakra-ui/react";
import { useRef } from "react";
import { Button } from "../ui/button/Button";

/** True once the server's build differs from the first one this page saw. */
export function useBuildChanged(build?: string): boolean {
  const first = useRef<string | undefined>(undefined);
  if (build && first.current === undefined) first.current = build;
  return !!build && first.current !== undefined && build !== first.current;
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

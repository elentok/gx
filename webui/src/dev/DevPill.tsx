import { Box, Spinner } from "@chakra-ui/react";
import type { DevState } from "../generated/server";
import type { Tone } from "../ui/tone";

const LOOK: Partial<Record<DevState, { text: string; tone: Tone; title: string; busy?: boolean }>> = {
  installing: { text: "Dev · installing", tone: "muted", title: "Dev mode: Installing dependencies", busy: true },
  starting: { text: "Dev · starting", tone: "muted", title: "Dev mode: Starting Vite", busy: true },
  running: { text: "Dev", tone: "running", title: "Dev mode: Vite is running" },
  down: { text: "Dev · down", tone: "deadlocked", title: "Dev mode: Vite is down" },
};

/** The sidebar-footer pill for Dev mode; renders nothing while Dev mode is off. */
export function DevPill({ state }: { state: DevState }) {
  const look = LOOK[state];
  if (!look) return null;
  return (
    <Box
      as="span"
      data-part="dev-pill"
      data-tone={look.tone}
      title={look.title}
      display="inline-flex"
      alignItems="center"
      gap="6px"
      h="22px"
      px="8px"
      flex="none"
      whiteSpace="nowrap"
      borderRadius="full"
      fontSize="12px"
      fontWeight="600"
      letterSpacing="0.04em"
      color={`tone.${look.tone}.fg`}
      bg={`tone.${look.tone}.bg`}
    >
      {look.busy && <Spinner data-testid="dev-pill-spinner" boxSize="10px" borderWidth="2px" />}
      {look.text}
    </Box>
  );
}

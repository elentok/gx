import { Box, chakra, Popover, Portal } from "@chakra-ui/react";
import type { Health, MeterHealth } from "../../generated/server";
import { HealthPopover } from "../../health/HealthPopover";
import { STATE_COLOR } from "./stateColor";

const METERS = [
  { key: "cpu", letter: "C", name: "CPU" },
  { key: "memory", letter: "M", name: "memory" },
  { key: "disk", letter: "D", name: "disk" },
] as const;

// Levels 0 to 4 fill a fifth more each, so level 0 still shows a sliver and level 4 fills the bar.
const fillPercent = (level: number) => (Math.min(Math.max(level, 0), 4) + 1) * 20;

export function trayLabel(health: Health): string {
  return `System health: ${METERS.map(({ key, name }) => `${name} ${health[key].state}`).join("; ")}`;
}

// Chakra's `sm` breakpoint. Below it the popover ends 2px from the screen edge, as in the mock.
const PHONE_QUERY = "(max-width: 39.99em)";
const PHONE_EDGE_GAP = 2;

// Stretches the anchor's right edge to the screen edge, so "bottom-end" ends the popover there.
const phoneAnchorRect: NonNullable<NonNullable<Popover.RootProps["positioning"]>["getAnchorRect"]> = (el) => {
  if (!el || typeof window.matchMedia !== "function" || !window.matchMedia(PHONE_QUERY).matches) return null;
  const { x, y, width, height } = el.getBoundingClientRect();
  return { x, y, width: window.innerWidth - PHONE_EDGE_GAP - x, height };
};

export function Tray({ health, connected = true }: { health: Health; connected?: boolean }) {
  // The body mounts only while open, so its polling starts and stops with the popover.
  return (
    <Popover.Root positioning={{ placement: "bottom-end", getAnchorRect: phoneAnchorRect, overflowPadding: PHONE_EDGE_GAP }} lazyMount unmountOnExit>
      <Popover.Trigger asChild>
        <chakra.button
          type="button"
          aria-label={trayLabel(health)}
          h="30px"
          display="flex"
          alignItems="flex-start"
          gap="3px"
          pt="2px"
          px="6px"
          borderWidth="1px"
          borderColor="transparent"
          borderRadius="md"
          cursor="pointer"
          opacity={connected ? 1 : 0.45}
          data-stale={connected ? undefined : ""}
          _hover={{ bg: "tone.muted.bg", borderColor: "border" }}
        >
          {METERS.map(({ key, letter }) => (
            <Meter key={key} letter={letter} meter={health[key]} />
          ))}
        </chakra.button>
      </Popover.Trigger>
      <Portal>
        <Popover.Positioner>
          <Popover.Content
            role="dialog"
            aria-label="System health"
            bg="surface.raised"
            borderWidth="1px"
            borderColor="border"
            borderRadius="8px"
            boxShadow="lg"
            color="text"
            w={{ base: "346px", sm: "min(320px, calc(100vw - 32px))" }}
            maxW="calc(100vw - 4px)"
          >
            <HealthPopover health={health} connected={connected} />
          </Popover.Content>
        </Popover.Positioner>
      </Portal>
    </Popover.Root>
  );
}

function Meter({ letter, meter }: { letter: string; meter: MeterHealth }) {
  const color = STATE_COLOR[meter.state];
  const unknown = meter.state === "unknown";
  return (
    <Box aria-hidden display="flex" flexDirection="column" alignItems="center" gap="1px" data-meter={letter} data-state={meter.state}>
      <Box
        data-part="bar"
        w="5px"
        h="15px"
        borderRadius="2px"
        overflow="hidden"
        position="relative"
        bg={unknown ? "transparent" : "hairline"}
        boxShadow={unknown ? "inset 0 0 0 1px var(--chakra-colors-text-faint)" : undefined}
      >
        {!unknown && <Box data-part="fill" position="absolute" left="0" right="0" bottom="0" h={`${fillPercent(meter.level)}%`} bg={color} />}
      </Box>
      <Box as="span" fontFamily="mono" fontWeight="600" fontSize="8px" lineHeight="1" color={color}>
        {letter}
      </Box>
    </Box>
  );
}

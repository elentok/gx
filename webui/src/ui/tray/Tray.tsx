import { Box, chakra } from "@chakra-ui/react";
import type { Health, HealthState, MeterHealth } from "../../generated/server";

const METERS = [
  { key: "cpu", letter: "C", name: "CPU" },
  { key: "memory", letter: "M", name: "memory" },
  { key: "disk", letter: "D", name: "disk" },
] as const;

// Normal is the faint grey; high and critical borrow the waiting and paused tones.
const STATE_COLOR: Record<HealthState, string> = {
  normal: "text.faint",
  high: "tone.waiting.fg",
  critical: "tone.paused.fg",
  unknown: "text.faint",
};

// Levels 0 to 4 fill a fifth more each, so level 0 still shows a sliver and level 4 fills the bar.
const fillPercent = (level: number) => (Math.min(Math.max(level, 0), 4) + 1) * 20;

export function trayLabel(health: Health): string {
  return `System health: ${METERS.map(({ key, name }) => `${name} ${health[key].state}`).join("; ")}`;
}

// The popover opens from this button in a later ticket.
export function Tray({ health }: { health: Health }) {
  return (
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
      _hover={{ bg: "tone.muted.bg", borderColor: "border" }}
    >
      {METERS.map(({ key, letter }) => (
        <Meter key={key} letter={letter} meter={health[key]} />
      ))}
    </chakra.button>
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

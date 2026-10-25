import { Box, chakra, Text } from "@chakra-ui/react";
import { STATS_HREF } from "../../hrefs";
import type { QueueMode } from "../../snapshot";
import { Chip } from "../chip/Chip";
import { LiveDot } from "../live-dot/LiveDot";
import type { Tone } from "../tone";

export type TopBarTab = "epics" | "stats";

const TABS: { key: TopBarTab; label: string; href: string }[] = [
  { key: "epics", label: "Epics", href: "#/epics" },
  { key: "stats", label: "Stats", href: STATS_HREF },
];

interface Props {
  // No tab is active on the epic and ticket pages.
  active?: TopBarTab;
  tone?: Tone;
  build?: string;
  mode?: QueueMode;
  connected: boolean;
  afk?: boolean;
  // The toggle shows only when the page can switch AFK mode.
  onToggleAfk?: () => void;
}

// Plain anchors, not router links, so the gallery can render it without a router.
export function TopBar({ active, tone = "running", build, mode, connected, afk = false, onToggleAfk }: Props) {
  const queueChip = mode === "paused" ? "Paused" : null;
  return (
    <Box as="nav" display="flex" alignItems="center" justifyContent="space-between" mb="4">
      <Box display="flex" gap="5">
        {TABS.map(({ key, label, href }) => (
          <chakra.a
            key={key}
            href={href}
            aria-current={key === active ? "page" : undefined}
            fontFamily="condensed"
            fontSize="xl"
            fontWeight="600"
            textTransform="uppercase"
            color={key === active ? "text" : "text.muted"}
            borderBottomWidth="2px"
            borderBottomColor={key === active ? `tone.${tone}.fg` : "transparent"}
          >
            {label}
          </chakra.a>
        ))}
      </Box>
      <Box display="flex" alignItems="center" gap="2">
        {build && (
          <Text fontSize="xs" color="text.muted" data-testid="server-build">
            {build}
          </Text>
        )}
        {queueChip && <Chip tone="waiting">{queueChip}</Chip>}
        {onToggleAfk && (
          <chakra.button
            type="button"
            aria-pressed={afk}
            onClick={onToggleAfk}
            fontFamily="condensed"
            fontSize="sm"
            fontWeight="600"
            textTransform="uppercase"
            letterSpacing="wide"
            cursor="pointer"
            color={afk ? `tone.${tone}.fg` : "text.muted"}
          >
            AFK
          </chakra.button>
        )}
        <LiveDot tone={connected ? "running" : "muted"} label={connected ? "Live" : "Offline"} />
      </Box>
    </Box>
  );
}

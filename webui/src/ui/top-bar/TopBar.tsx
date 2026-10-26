import { Box, chakra, Menu, Text } from "@chakra-ui/react";
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
        {queueChip && <Chip tone="waiting">{queueChip}</Chip>}
        <LiveDot tone={connected ? "running" : "muted"} label={connected ? "Live" : "Offline"} />
        {(onToggleAfk || build) && (
          <Menu.Root closeOnSelect={false} lazyMount unmountOnExit positioning={{ placement: "bottom-end" }}>
            <Menu.Trigger asChild>
              <chakra.button type="button" aria-label="Menu" cursor="pointer" color="text.muted" display="flex" p="1">
                <svg width="18" height="18" viewBox="0 0 18 18" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true">
                  <path d="M3 5h12M3 9h12M3 13h12" />
                </svg>
              </chakra.button>
            </Menu.Trigger>
            <Menu.Positioner>
              <Menu.Content bg="surface.raised" borderWidth="1px" borderColor="border" minW="40">
                {onToggleAfk && (
                  <Menu.CheckboxItem value="afk" checked={afk} onCheckedChange={onToggleAfk} cursor="pointer">
                    <Menu.ItemIndicator color={`tone.${tone}.fg`} />
                    AFK
                  </Menu.CheckboxItem>
                )}
                {build && (
                  <Text px="3" py="1.5" fontSize="xs" color="text.muted" data-testid="server-build">
                    {build}
                  </Text>
                )}
              </Menu.Content>
            </Menu.Positioner>
          </Menu.Root>
        )}
      </Box>
    </Box>
  );
}

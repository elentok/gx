import { Box, chakra, Menu, Text } from "@chakra-ui/react";
import { useEffect, useRef } from "react";
import type { Health } from "../../generated/server";
import { STATS_HREF } from "../../hrefs";
import type { QueueMode } from "../../snapshot";
import { Chip } from "../chip/Chip";
import { LiveDot } from "../live-dot/LiveDot";
import { TOP_BAR_HEIGHT_VAR } from "../modal/useVisualViewportHeight";
import type { Tone } from "../tone";
import { Tray } from "../tray/Tray";

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
  /** Epics in Needs you other than the one on screen; it reads on the Epics tab and is left out at zero. */
  waiting?: number;
  afk?: boolean;
  // Absent until the first snapshot lands, so the Tray is left out then.
  health?: Health;
  // The toggle shows only when the page can switch AFK mode.
  onToggleAfk?: () => void;
}

// Plain anchors, not router links, so the gallery can render it without a router.
export function TopBar({ active, tone = "running", build, mode, connected, waiting = 0, afk = false, health, onToggleAfk }: Props) {
  const queueChip = mode === "paused" ? "Paused" : null;
  const nav = useRef<HTMLElement>(null);
  // Sheets that sit below the bar read its real height from the root.
  useEffect(() => {
    const el = nav.current;
    if (!el) return;
    const publish = () => document.documentElement.style.setProperty(TOP_BAR_HEIGHT_VAR, `${el.offsetHeight}px`);
    publish();
    if (typeof ResizeObserver === "undefined") return () => document.documentElement.style.removeProperty(TOP_BAR_HEIGHT_VAR);
    const observer = new ResizeObserver(publish);
    observer.observe(el);
    return () => {
      observer.disconnect();
      document.documentElement.style.removeProperty(TOP_BAR_HEIGHT_VAR);
    };
  }, []);
  return (
    <Box as="nav" ref={nav} display="flex" alignItems="center" justifyContent="space-between" mb="4">
      <Box display="flex" gap="5">
        {TABS.map(({ key, label, href }) => (
          <chakra.a
            key={key}
            href={href}
            aria-current={key === active ? "page" : undefined}
            aria-label={key === "epics" && waiting > 0 ? `${label} · ${waiting} ${waiting === 1 ? "needs" : "need"} you` : undefined}
            fontFamily="condensed"
            fontSize="xl"
            fontWeight="600"
            textTransform="uppercase"
            color={key === active ? "text" : "text.muted"}
            borderBottomWidth="2px"
            borderBottomColor={key === active ? `tone.${tone}.fg` : "transparent"}
          >
            {label}
            {key === "epics" && waiting > 0 && (
              <Text as="span" aria-hidden ml="2" color="tone.waiting.fg">
                {waiting}
              </Text>
            )}
          </chakra.a>
        ))}
      </Box>
      <Box display="flex" alignItems="center" gap="2">
        {queueChip && <Chip tone="waiting">{queueChip}</Chip>}
        {!connected && <LiveDot tone="muted" label="Offline" />}
        {health && <Tray health={health} connected={connected} />}
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
                  <Menu.CheckboxItem
                    value="afk"
                    checked={afk}
                    onCheckedChange={onToggleAfk}
                    cursor="pointer"
                    color="text"
                    ps="3"
                    justifyContent="space-between"
                    _highlighted={{ bg: "surface" }}
                  >
                    AFK
                    <AfkSwitch on={afk} tone={tone} />
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

// A drawn switch: the menu item carries the checkbox role and state, so this is decoration only.
function AfkSwitch({ on, tone }: { on: boolean; tone: Tone }) {
  return (
    <Box
      aria-hidden
      w="32px"
      h="18px"
      p="2px"
      borderRadius="full"
      borderWidth="1px"
      borderColor={on ? `tone.${tone}.fg` : "border"}
      bg={on ? `tone.${tone}.fg` : "surface"}
      display="flex"
      justifyContent={on ? "flex-end" : "flex-start"}
      transition="background 0.1s"
    >
      <Box w="12px" h="12px" borderRadius="full" bg={on ? "surface" : "text.dim"} />
    </Box>
  );
}

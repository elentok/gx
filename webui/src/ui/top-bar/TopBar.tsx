import { Box, Button, chakra, Image, Menu, Text } from "@chakra-ui/react";
import type { ReactNode } from "react";
import type { Health } from "../../generated/server";
import { AGENTS_HREF, SETTINGS_HREF, STATS_HREF } from "../../hrefs";
import type { QueueMode } from "../../snapshot";
import { Chip } from "../chip/Chip";
import { LiveDot } from "../live-dot/LiveDot";
import type { Tone } from "../tone";
import { Tray } from "../tray/Tray";

export type TopBarTab = "epics" | "agents" | "stats";

const TABS: { key: TopBarTab; label: string; href: string }[] = [
  { key: "epics", label: "Epics", href: "#/epics" },
  { key: "agents", label: "Agents", href: AGENTS_HREF },
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
  /** Sessions that are Actionable; it reads on the Agents tab and is left out at zero. */
  actionable?: number;
  afk?: boolean;
  // Absent until the first snapshot lands, so the Tray is left out then.
  health?: Health;
  // The toggle shows only when the page can switch AFK mode.
  onToggleAfk?: () => void;
  /** Sits between the AFK item and the build line. It draws its own two separators, so an empty block adds none. */
  menuBlock?: ReactNode;
  // Whether menuBlock renders anything; the menu is wide only then. Defaults to whether menuBlock is set.
  menuBlockShown?: boolean;
  viMode?: boolean;
  // The Vi mode item shows only when the page can switch it.
  onToggleViMode?: () => void;
  // The item shows only when the page can host the restart dialog.
  onRestart?: () => void;
}

// Plain anchors, not router links, so the gallery can render it without a router.
export function TopBar({ active, tone = "running", build, mode, connected, waiting = 0, actionable = 0, afk = false, health, onToggleAfk, viMode = false, onToggleViMode, onRestart, menuBlock, menuBlockShown }: Props) {
  const queueChip = mode === "paused" ? "Paused" : null;
  return (
    <Box as="nav" display="flex" alignItems="center" justifyContent="space-between" px="4" pt="4" pb="4">
      <Box display="flex" gap="5">
        {TABS.map(({ key, label, href }) => {
          const count = key === "epics" ? waiting : key === "agents" ? actionable : 0;
          return (
          <chakra.a
            key={key}
            href={href}
            aria-current={key === active ? "page" : undefined}
            aria-label={count > 0 ? `${label} · ${count} ${count === 1 ? "needs" : "need"} you` : undefined}
            display="inline-flex"
            alignItems="center"
            gap="2"
            fontFamily="condensed"
            fontSize="xl"
            fontWeight="600"
            textTransform="uppercase"
            color={key === active ? "text" : "text.muted"}
            borderBottomWidth="2px"
            borderBottomColor={key === active ? `tone.${tone}.fg` : "transparent"}
          >
            {label}
            {count > 0 && (
              <Text as="span" aria-hidden px="2" lineHeight="1.4" borderRadius="full" bg="tone.waiting.fg" color="surface" fontFamily="text" fontSize="xs" fontWeight="600">
                {count}
              </Text>
            )}
          </chakra.a>
          );
        })}
      </Box>
      <Box display="flex" alignItems="center" gap="2">
        {queueChip && <Chip tone="waiting">{queueChip}</Chip>}
        {!connected && <LiveDot tone="muted" label="Offline" />}
        {health && <Tray health={health} connected={connected} />}
        <Menu.Root closeOnSelect={false} lazyMount unmountOnExit positioning={{ placement: "bottom-end" }}>
          <Menu.Trigger asChild>
            <Button
              type="button"
              aria-label="Menu"
              variant="ghost"
              cursor="pointer"
              color="text.dim"
              h={{ base: "11", md: "12" }}
              minW={{ base: "15", md: "16" }}
              px="2"
              gap="1"
              bg="surface.raised"
              borderWidth="1px"
              borderColor="hairline"
              borderRadius="lg"
              _hover={{ borderColor: "tone.running.fg" }}
              _focusVisible={{ outlineWidth: "2px", outlineColor: "tone.running.fg" }}
              _open={{ borderColor: "tone.running.fg" }}
            >
              <Image src="/gx-menu-mascot.png" alt="" draggable={false} h={{ base: "7", md: "9" }} w="auto" />
              <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                <path d="m3 4.5 3 3 3-3" />
              </svg>
            </Button>
          </Menu.Trigger>
          <Menu.Positioner>
            <Menu.Content bg="surface.raised" borderWidth="1px" borderColor="border" minW="40" {...((menuBlockShown ?? !!menuBlock) ? { w: "280px", maxW: "calc(100vw - 2rem)" } : {})}>
              <Menu.Item value="settings" asChild cursor="pointer" color="text" ps="3" _highlighted={{ bg: "surface" }}>
                <a href={SETTINGS_HREF}>Settings</a>
              </Menu.Item>
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
                  <MenuSwitch on={afk} tone={tone} />
                </Menu.CheckboxItem>
              )}
              {onToggleViMode && (
                <Menu.CheckboxItem
                  value="vimode"
                  checked={viMode}
                  onCheckedChange={onToggleViMode}
                  cursor="pointer"
                  color="text"
                  ps="3"
                  justifyContent="space-between"
                  _highlighted={{ bg: "surface" }}
                >
                  Vi mode
                  <MenuSwitch on={viMode} tone={tone} />
                </Menu.CheckboxItem>
              )}
              {onRestart && (
                <Menu.Item value="restart" closeOnSelect onClick={onRestart} cursor="pointer" color="text" ps="3" _highlighted={{ bg: "surface" }}>
                  Restart server
                </Menu.Item>
              )}
              {menuBlock}
              {build && (
                <Text px="3" py="1.5" fontSize="xs" color="text.muted" data-testid="server-build">
                  {build}
                </Text>
              )}
            </Menu.Content>
          </Menu.Positioner>
        </Menu.Root>
      </Box>
    </Box>
  );
}

// A drawn switch: the menu item carries the checkbox role and state, so this is decoration only.
function MenuSwitch({ on, tone }: { on: boolean; tone: Tone }) {
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

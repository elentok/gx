import { Box, Button, Menu, Portal, Text } from "@chakra-ui/react";
import { useState } from "react";
import { postAfk as defaultPostAfk } from "../afk";
import { useConnectionContext, useConnectionState } from "../connection";
import { SETTINGS_HREF } from "../hrefs";
import { RestartDialog } from "../restart/RestartDialog";
import { showToast } from "../ui/toaster";
import { UsageMenuBlock } from "../usage/UsageMenuBlock";
import type { FetchSummary } from "../usage/useUsageSummary";
import { postViMode as defaultPostViMode } from "../vimode";
import { WriteError } from "../useWrite";

interface Props {
  postAfk?: (on: boolean) => Promise<void>;
  postViMode?: (on: boolean) => Promise<void>;
  usageFetcher?: FetchSummary;
}

const itemStyle = { cursor: "pointer", color: "text", ps: "3", _highlighted: { bg: "surface" } } as const;

// A failed write leaves the switch as the last snapshot showed it.
function flip(post: (on: boolean) => Promise<void>, current: boolean | undefined) {
  void post(!current).catch((err) =>
    showToast({ type: "error", title: err instanceof WriteError ? err.message : "Could not reach gx. Try again." }),
  );
}

function GearIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <circle cx="10" cy="10" r="2.5" />
      <path d="M10 2.5v2M10 15.5v2M2.5 10h2M15.5 10h2M4.7 4.7l1.4 1.4M13.9 13.9l1.4 1.4M4.7 15.3l1.4-1.4M13.9 6.1l1.4-1.4" />
    </svg>
  );
}

// The app menu: the old top bar's menu, opened from the sidebar footer.
export function GearMenu({ postAfk = defaultPostAfk, postViMode = defaultPostViMode, usageFetcher }: Props) {
  const conn = useConnectionContext();
  const { snapshot } = useConnectionState(conn);
  const [usageShown, setUsageShown] = useState(false);
  const [restartOpen, setRestartOpen] = useState(false);
  const afk = snapshot?.afk ?? false;
  const viMode = snapshot?.vi_mode ?? false;
  return (
    <>
      <Menu.Root closeOnSelect={false} lazyMount unmountOnExit positioning={{ placement: "top-end" }}>
        <Menu.Trigger asChild>
          <Button
            type="button"
            aria-label="Menu"
            title="Menu"
            variant="ghost"
            cursor="pointer"
            color="text.dim"
            boxSize="34px"
            minW="0"
            p="0"
            borderRadius="8px"
            _hover={{ bg: "surface.raised" }}
            _open={{ bg: "surface.raised" }}
          >
            <GearIcon />
          </Button>
        </Menu.Trigger>
        <Portal>
          <Menu.Positioner>
            <Menu.Content bg="surface.raised" borderWidth="1px" borderColor="border" minW="40" {...(usageShown ? { w: "280px", maxW: "calc(100vw - 2rem)" } : {})}>
              <Menu.CheckboxItem value="afk" checked={afk} onCheckedChange={() => flip(postAfk, afk)} justifyContent="space-between" {...itemStyle}>
                AFK
                <MenuSwitch on={afk} />
              </Menu.CheckboxItem>
              <Menu.CheckboxItem value="vimode" checked={viMode} onCheckedChange={() => flip(postViMode, viMode)} justifyContent="space-between" {...itemStyle}>
                Vi mode
                <MenuSwitch on={viMode} />
              </Menu.CheckboxItem>
              <Menu.Separator />
              <Menu.Item value="settings" asChild closeOnSelect {...itemStyle}>
                <a href={SETTINGS_HREF}>Settings</a>
              </Menu.Item>
              <Menu.Item value="restart" closeOnSelect onClick={() => setRestartOpen(true)} {...itemStyle}>
                Restart server
              </Menu.Item>
              <UsageMenuBlock conn={conn} fetcher={usageFetcher} onShownChange={setUsageShown} />
              {snapshot?.build && (
                <Text px="3" py="1.5" fontSize="xs" color="text.muted" truncate title={snapshot.build} data-testid="server-build">
                  {snapshot.build}
                </Text>
              )}
            </Menu.Content>
          </Menu.Positioner>
        </Portal>
      </Menu.Root>
      {restartOpen && <RestartDialog liveInstance={snapshot?.instance} onClose={() => setRestartOpen(false)} />}
    </>
  );
}

// A drawn switch: the menu item carries the checkbox role and state, so this is decoration only.
function MenuSwitch({ on }: { on: boolean }) {
  return (
    <Box
      aria-hidden
      w="32px"
      h="18px"
      p="2px"
      borderRadius="full"
      borderWidth="1px"
      borderColor={on ? "tone.waiting.fg" : "border"}
      bg={on ? "tone.waiting.fg" : "surface"}
      display="flex"
      justifyContent={on ? "flex-end" : "flex-start"}
      transition="background 0.1s"
    >
      <Box w="12px" h="12px" borderRadius="full" bg={on ? "surface" : "text.dim"} />
    </Box>
  );
}

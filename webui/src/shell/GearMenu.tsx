import { Box, Button } from "@chakra-ui/react";
import { useState } from "react";
import { postAfk as defaultPostAfk, toggleAfk } from "../afk";
import { useConnectionContext, useConnectionState } from "../connection";
import { SortActivity, SortDefault, SortName, SortNeedsYou, type Workspace } from "../generated/server";
import { CLEANUP_HREF, SETTINGS_HREF, TIPS_HREF } from "../hrefs";
import { RestartDialog } from "../restart/RestartDialog";
import { GxMenu, type MenuEntry } from "../ui/menu/Menu";
import { showToast } from "../ui/toaster";
import { postViMode as defaultPostViMode } from "../vimode";
import { WriteError } from "../useWrite";
import { postWorkspace } from "../workspaces/postWorkspace";
import { useShell } from "./ShellContext";

interface Props {
  postAfk?: (on: boolean) => Promise<void>;
  postViMode?: (on: boolean) => Promise<void>;
  postSort?: (workspace: string, sort: string) => Promise<unknown>;
  /** The active workspace once it resolves; Sort stays disabled until then. */
  activeWorkspace?: Workspace;
}

const SORT_OPTIONS = [
  { value: SortNeedsYou, label: "Needs you first, then by name" },
  { value: SortActivity, label: "By last activity, newest first" },
  { value: SortName, label: "Alphabetically by name" },
];

const defaultPostSort = (workspace: string, sort: string) => postWorkspace("sort", { name: workspace, sort });

// A failed write leaves the control as the last snapshot showed it.
function guarded(write: Promise<unknown>) {
  void write.catch((err) =>
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
export function GearMenu({ postAfk = defaultPostAfk, postViMode = defaultPostViMode, postSort = defaultPostSort, activeWorkspace }: Props) {
  const { snapshot } = useConnectionState(useConnectionContext());
  const shell = useShell();
  const [restartOpen, setRestartOpen] = useState(false);
  const afk = snapshot?.afk ?? false;
  const viMode = snapshot?.vi_mode ?? false;
  const toggle = (label: string, value: string, on: boolean, flip: () => void): MenuEntry => ({
    kind: "checkbox",
    value,
    label,
    checked: on,
    closeOnSelect: false,
    hint: <MenuSwitch on={on} />,
    inlineHint: true,
    onCheckedChange: flip,
  });
  const sort: MenuEntry = activeWorkspace
    ? {
        kind: "submenu",
        value: "sort",
        label: `Sort · ${activeWorkspace.name}`,
        inlineHint: true,
        items: [
          {
            kind: "radio-group",
            value: "sort-order",
            current: activeWorkspace.sort || SortDefault,
            onValueChange: (v) => guarded(postSort(activeWorkspace.name, v)),
            options: SORT_OPTIONS,
          },
        ],
      }
    : { value: "sort", label: "Sort", disabled: true, onSelect: () => {} };
  return (
    <>
      <GxMenu
        placement="top-end"
        trigger={
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
        }
        items={[
          toggle("AFK", "afk", afk, () => toggleAfk(afk, postAfk)),
          toggle("Vi mode", "vimode", viMode, () => guarded(postViMode(!viMode))),
          sort,
          { kind: "separator" },
          { value: "restart", label: "Restart server", onSelect: () => setRestartOpen(true) },
          {
            value: "settings",
            label: "More settings...",
            onSelect: () => {
              window.location.hash = SETTINGS_HREF;
              shell?.closeSliding();
            },
          },
          {
            value: "cleanup",
            label: "Cleanup",
            onSelect: () => {
              window.location.hash = CLEANUP_HREF;
              shell?.closeSliding();
            },
          },
          {
            value: "tips",
            label: "Tips",
            onSelect: () => {
              window.location.hash = TIPS_HREF;
              shell?.closeSliding();
            },
          },
        ]}
      />
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

import { Box } from "@chakra-ui/react";
import { useState } from "react";
import { postAfk as defaultPostAfk, toggleAfk } from "../afk";
import { useConnectionContext, useConnectionState } from "../connection";
import {
  SortActivity,
  SortDefault,
  SortName,
  SortNeedsYou,
  SortProject,
  SortStatus,
  type Workspace,
} from "../generated/server";
import { CLEANUP_HREF, SETTINGS_HREF, TIPS_HREF } from "../hrefs";
import { forceRefresh } from "../pwa/forceRefresh";
import { CertLink, useCert } from "../auth/cert";
import { useInstall } from "../pwa/install";
import { useWorkerRegistration } from "../pwa/registerWorker";
import { RestartDialog } from "../restart/RestartDialog";
import { GxMenu, type MenuEntry } from "../ui/menu/Menu";
import { showToast } from "../ui/toaster";
import { postDevFrontend as defaultPostDevFrontend, useDev } from "../dev";
import { DevDetails } from "../dev/DevPopover";
import { DEV_LOOK } from "../dev/DevPill";
import { postViMode as defaultPostViMode } from "../vimode";
import { WriteError } from "../useWrite";
import { postWorkspace } from "../workspaces/postWorkspace";
import { FooterButton, interactiveStyle } from "./FooterButton";
import { InstallSheet } from "./InstallSheet";
import { useShell } from "./ShellContext";

interface Props {
  postAfk?: (on: boolean) => Promise<void>;
  postViMode?: (on: boolean) => Promise<void>;
  postDevFrontend?: (on: boolean) => void;
  postSort?: (workspace: string, sort: string) => Promise<unknown>;
  /** The active workspace once it resolves; Sort stays disabled until then. */
  activeWorkspace?: Workspace;
}

const SORT_OPTIONS = [
  { value: SortNeedsYou, label: "Needs you first, then by name" },
  { value: SortActivity, label: "By last activity, newest first" },
  { value: SortName, label: "Alphabetically by name" },
  { value: SortProject, label: "By project, then by name" },
  { value: SortStatus, label: "By status, then by name" },
];

const defaultPostSort = (workspace: string, sort: string) =>
  postWorkspace("sort", { name: workspace, sort });

// A failed write leaves the control as the last snapshot showed it.
function guarded(write: Promise<unknown>) {
  void write.catch((err) =>
    showToast({
      type: "error",
      title:
        err instanceof WriteError
          ? err.message
          : "Could not reach gx. Try again.",
    }),
  );
}

function DotsIcon() {
  return (
    <svg
      width="20"
      height="20"
      viewBox="0 0 20 20"
      fill="currentColor"
      aria-hidden
    >
      <circle cx="10" cy="4.5" r="1.7" />
      <circle cx="10" cy="10" r="1.7" />
      <circle cx="10" cy="15.5" r="1.7" />
    </svg>
  );
}

// The app menu: the old top bar's menu, opened from the sidebar footer.
export function GearMenu({
  postAfk = defaultPostAfk,
  postViMode = defaultPostViMode,
  postDevFrontend = defaultPostDevFrontend,
  postSort = defaultPostSort,
  activeWorkspace,
}: Props) {
  const dev = useDev();
  const { snapshot, connected } = useConnectionState(useConnectionContext());
  const shell = useShell();
  const [restartOpen, setRestartOpen] = useState(false);
  const [sheetOpen, setSheetOpen] = useState(false);
  const install = useInstall();
  const registration = useWorkerRegistration();
  const { cert } = useCert();
  const afk = snapshot?.afk ?? false;
  const viMode = snapshot?.vi_mode ?? false;
  const devOn = dev.state !== "off";
  const toggle = (
    label: string,
    value: string,
    on: boolean,
    flip: () => void,
  ): MenuEntry => ({
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
  const devDetails: MenuEntry = {
    kind: "submenu",
    value: "dev-details",
    label: "Dev details",
    hint: DEV_LOOK[dev.state]?.status,
    inlineHint: true,
    items: [
      {
        kind: "content",
        value: "dev-details-body",
        content: <DevDetails dev={dev} showOff={false} showHeader={false} />,
      },
    ],
  };
  const trustHint =
    registration.state === "failed" &&
    registration.error === "SecurityError" &&
    cert;
  const installEntries: MenuEntry[] = trustHint
    ? [
        {
          kind: "content",
          value: "root-trust-hint",
          content: (
            <Box px="3" py="1.5" fontSize="sm" color="text.muted">
              Can't install: this browser doesn't trust gx.{" "}
              <CertLink href={cert.cer_path}>Install the certificate</CertLink>
            </Box>
          ),
        },
      ]
    : install.mode === "none"
      ? []
      : [
          {
            value: "install",
            label: "Install app",
            hint:
              install.mode === "prompt" && install.desktopCaption
                ? "Chrome / Edge"
                : undefined,
            inlineHint: true,
            onSelect: () =>
              install.mode === "prompt"
                ? void install.promptInstall()
                : setSheetOpen(true),
          },
        ];
  return (
    <>
      <GxMenu
        placement="top-end"
        trigger={
          <FooterButton aria-label="Menu" title="Menu" {...interactiveStyle}>
            <DotsIcon />
          </FooterButton>
        }
        items={[
          toggle("AFK", "afk", afk, () => toggleAfk(afk, postAfk)),
          toggle("Vi mode", "vimode", viMode, () =>
            guarded(postViMode(!viMode)),
          ),
          ...(dev.has_checkout
            ? [
                toggle("Dev mode", "devmode", devOn, () =>
                  postDevFrontend(!devOn),
                ),
              ]
            : []),
          ...(devOn ? [devDetails] : []),
          sort,
          { kind: "separator" },
          {
            value: "restart",
            label: "Restart server",
            onSelect: () => setRestartOpen(true),
          },
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
          { kind: "separator" },
          ...installEntries,
          {
            value: "force-refresh",
            label: "Force refresh",
            onSelect: () => void forceRefresh(connected),
          },
        ]}
      />
      {sheetOpen &&
        (install.mode === "ios" || install.mode === "mac-safari") && (
          <InstallSheet
            mode={install.mode}
            onClose={() => setSheetOpen(false)}
          />
        )}
      {restartOpen && (
        <RestartDialog
          liveInstance={snapshot?.instance}
          onClose={() => setRestartOpen(false)}
        />
      )}
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
      <Box
        w="12px"
        h="12px"
        borderRadius="full"
        bg={on ? "surface" : "text.dim"}
      />
    </Box>
  );
}

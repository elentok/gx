import { Box, chakra, Image } from "@chakra-ui/react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Outlet } from "react-router";
import { useConnectionContext, useConnectionState } from "../connection";
import type { FetchEpicList } from "../epics/list";
import { DiskStrips } from "../page/DiskStrips";
import { FailedMigrationStrip } from "../restart/FailedMigrationStrip";
import { NewVersionStrip, useBuildChanged } from "../restart/NewVersionStrip";
import { MigratingStrip } from "../restart/MigratingStrip";
import { Strip } from "../ui/strip/Strip";
import { useDevice, type Device } from "../useWide";
import { MascotToggle } from "./MascotToggle";
import { ShellContext, type Shell } from "./ShellContext";
import { SidebarFooter } from "./SidebarFooter";
import { SidebarRows } from "./SidebarRows";
import {
  useActiveWorkspace,
  useWorkspaces,
} from "../workspaces/useActiveWorkspace";
import { ManageWorkspaces } from "./ManageWorkspaces";
import {
  NewWorkspaceDialog,
  WorkspaceSwitcher,
  type CreateWorkspace,
} from "./WorkspaceSwitcher";

/** The top bar's sticky offset reads this, so it sits below the strips. */
export const SHELL_STRIPS_HEIGHT_VAR = "--shell-strips-height";

const WIDTH: Record<Device, number> = { desktop: 280, tablet: 330, phone: 344 };

function PanelIcon() {
  return (
    <svg
      width="20"
      height="20"
      viewBox="0 0 20 20"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <rect x="2.5" y="3.5" width="15" height="13" rx="2.5" />
      <path d="M7.5 3.5v13" />
    </svg>
  );
}

function Strips() {
  const { snapshot, connected } = useConnectionState(useConnectionContext());
  const buildChanged = useBuildChanged(snapshot?.build);
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = box.current;
    if (!el) return;
    const publish = () =>
      document.documentElement.style.setProperty(
        SHELL_STRIPS_HEIGHT_VAR,
        `${el.offsetHeight}px`,
      );
    publish();
    if (typeof ResizeObserver === "undefined")
      return () =>
        document.documentElement.style.removeProperty(SHELL_STRIPS_HEIGHT_VAR);
    const observer = new ResizeObserver(publish);
    observer.observe(el);
    return () => {
      observer.disconnect();
      document.documentElement.style.removeProperty(SHELL_STRIPS_HEIGHT_VAR);
    };
  }, []);
  return (
    <Box ref={box} position="sticky" top="0" zIndex="sticky" bg="surface">
      {!connected && <Strip tone="waiting">Offline, reconnecting…</Strip>}
      <MigratingStrip />
      <DiskStrips />
      <FailedMigrationStrip />
      {buildChanged && <NewVersionStrip />}
    </Box>
  );
}

// Mounted once above every page route. The sidebar is docked on desktop and slides over on tablet and phone.
export function AppShell({
  children,
  fetchEpicList,
  createWorkspace,
  onManageWorkspaces = () => {},
}: {
  children?: React.ReactNode;
  fetchEpicList?: FetchEpicList;
  createWorkspace?: CreateWorkspace;
  /** Opens the manage-workspaces dialog. */
  onManageWorkspaces?: () => void;
}) {
  const device = useDevice();
  const docked = device === "desktop";
  // An explicit choice holds only for the device it was made on; a resize back to the default follows the device.
  const [choice, setChoice] = useState<{
    device: Device;
    open: boolean;
  } | null>(null);
  const open = choice?.device === device ? choice.open : docked;
  const [headers, setHeaders] = useState(0);
  const claimHeader = useCallback(() => {
    setHeaders((n) => n + 1);
    return () => setHeaders((n) => n - 1);
  }, []);
  const shell = useMemo<Shell>(
    () => ({
      sidebarOpen: open,
      toggle: () => setChoice({ device, open: !open }),
      close: () => setChoice({ device, open: false }),
      claimHeader,
    }),
    [open, device, claimHeader],
  );
  const conn = useConnectionContext();
  const { snapshot } = useConnectionState(conn);
  const { workspaces } = useWorkspaces(conn);
  const { active, setActive } = useActiveWorkspace(conn, fetchEpicList);
  const [newWorkspace, setNewWorkspace] = useState(false);
  const [manageWorkspaces, setManageWorkspaces] = useState(false);
  // The snapshot learns of a new workspace from the server's event; activating it earlier would resolve straight back to Default.
  const [created, setCreated] = useState<string>();
  useEffect(() => {
    if (created && workspaces.some((w) => w.name === created)) {
      setActive(created);
      setCreated(undefined);
    }
  }, [created, workspaces, setActive]);
  const width = WIDTH[device];
  return (
    <ShellContext.Provider value={shell}>
      <Box display="flex" alignItems="flex-start" minH="100dvh">
        {!docked && open && (
          <Box
            data-testid="scrim"
            position="fixed"
            inset="0"
            bg="blackAlpha.700"
            zIndex="overlay"
            onClick={shell.close}
          />
        )}
        <Box
          as="aside"
          aria-label="Sidebar"
          data-device={device}
          data-open={open}
          bg="surface"
          borderRightWidth="1px"
          borderColor="border"
          display="flex"
          flexDirection="column"
          w={`${width}px`}
          maxW="100vw"
          h="100dvh"
          flex="none"
          overflowY="auto"
          zIndex="modal"
          {...(docked
            ? {
                position: "sticky",
                top: "0",
                ...(open ? {} : { display: "none" }),
              }
            : {
                position: "fixed",
                top: "0",
                left: "0",
                transform: open ? "none" : "translateX(-100%)",
                visibility: open ? "visible" : "hidden",
                transition: "transform 0.2s, visibility 0.2s",
              })}
          // Picking an item on a sliding sidebar closes it.
          onClick={(e) => {
            if (!docked && (e.target as HTMLElement).closest("a"))
              shell.close();
          }}
        >
          <Box
            display="flex"
            alignItems="center"
            gap="2"
            pt="10px"
            pr="2"
            pb="6px"
            pl="3"
          >
            <Image
              src="/gx-logo-wide.png"
              alt="gx"
              h="26px"
              w="auto"
              draggable={false}
            />
            {snapshot?.build && (
              <Box
                as="span"
                color="text.faint"
                fontSize="12px"
                minW="0"
                whiteSpace="nowrap"
                overflow="hidden"
                textOverflow="ellipsis"
                title={snapshot.build}
              >
                {snapshot.build}
              </Box>
            )}
            <Box flex="1" />
            <chakra.button
              type="button"
              aria-label="Hide sidebar"
              onClick={shell.close}
              boxSize="34px"
              borderRadius="8px"
              display="grid"
              placeItems="center"
              color="text.dim"
              cursor="pointer"
              _hover={{ bg: "surface.raised" }}
            >
              <PanelIcon />
            </chakra.button>
          </Box>
          <WorkspaceSwitcher
            workspaces={workspaces}
            active={active}
            onPick={setActive}
            onNew={() => setNewWorkspace(true)}
            onManage={() => {
              setManageWorkspaces(true);
              onManageWorkspaces();
            }}
          />
          <Box
            as="ul"
            listStyleType="none"
            m="0"
            p="0"
            flex="1"
            data-testid="sidebar-body"
          >
            <SidebarRows fetcher={fetchEpicList} active={active} />
          </Box>
          <SidebarFooter onNewWorkspace={() => setNewWorkspace(true)} />
          {manageWorkspaces && (
            <ManageWorkspaces
              workspaces={workspaces}
              onClose={() => setManageWorkspaces(false)}
            />
          )}
          {newWorkspace && (
            <NewWorkspaceDialog
              create={createWorkspace}
              onClose={() => setNewWorkspace(false)}
              onCreated={(name) => {
                setCreated(name);
                setNewWorkspace(false);
              }}
            />
          )}
        </Box>
        <Box flex="1" minW="0">
          <Strips />
          {headers === 0 && (
            <Box px="4" pt="2">
              <MascotToggle />
            </Box>
          )}
          {children ?? <Outlet />}
        </Box>
      </Box>
    </ShellContext.Provider>
  );
}

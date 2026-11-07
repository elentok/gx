import { Box } from "@chakra-ui/react";
import { useEffect, useRef, type ReactNode } from "react";
import { MemoryRouter } from "react-router";
import { ConnectionContext, type Connection } from "../connection";
import { CardKindDesign, CardKindImplementation, SectionImplementing, SectionOnHold, type EpicListRow, type SessionInfo, type Workspace } from "../generated/server";
import { AppShell } from "../shell/AppShell";
import { ManageWorkspaces } from "../shell/ManageWorkspaces";
import { useShell } from "../shell/ShellContext";
import { WorkspaceSwitcher } from "../shell/WorkspaceSwitcher";
import { DeviceContext, WideContext, type Device } from "../useWide";
import type { GalleryEntry } from "./registry";

const workspaces: Workspace[] = [
  { name: "Default", default: true, members: 3 },
  { name: "Work", members: 2, needs_you: 1 },
  { name: "Home", members: 0 },
];

const row = (address: string, over: Partial<EpicListRow>): EpicListRow =>
  ({
    address,
    project: "p",
    name: address,
    kind: "loop",
    state: "idle",
    done: 1,
    total: 4,
    queued: false,
    queueable: false,
    blocked_by_epics: [],
    workspace: "Default",
    card_kind: CardKindImplementation,
    section: SectionImplementing,
    status_words: "Idle",
    ...over,
  }) as EpicListRow;

const rows = [
  row("p:blog", { name: "Blog", status_words: "Waiting on review" }),
  row("p:shop", { name: "Shop", card_kind: CardKindDesign }),
  row("p:old", { name: "Old", section: SectionOnHold, status_words: "Held" }),
];

const session = (id: string, name: string): SessionInfo =>
  ({ id, project: "p", name, dir: "/x", harness: "claude", status: { state: "idle" }, status_changed_at: "2026-01-01T00:00:00Z", run_started_at: "2026-01-01T00:00:00Z", unread: {}, actionable: {}, auto_advance: true }) as SessionInfo;

// getState must return the same object each call, or useSyncExternalStore loops.
const state = {
  snapshot: {
    needs_you_total: 1,
    workspaces,
    sessions: [session("s1", "builder"), session("s2", "scratch")],
    placements: [
      { session: "s1", kind: "epic", epic: "p:blog" },
      { session: "s2", kind: "other" },
    ],
  },
  connected: true,
  loadedOnce: true,
  tick: 0,
};
const connection = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {}, refresh: () => {} } as unknown as Connection;

/** Runs `step` on the frame's DOM once it returns true; rows load after mount, so it retries on every change. */
const Drive = ({ step, children }: { step: (root: HTMLElement) => boolean; children: ReactNode }) => {
  const root = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = root.current;
    if (!el) return;
    const body = el.ownerDocument.body;
    if (step(body)) return;
    const watch = new MutationObserver(() => step(body) && watch.disconnect());
    watch.observe(body, { childList: true, subtree: true });
    return () => watch.disconnect();
  }, [step]);
  return <div ref={root}>{children}</div>;
};

const clickButton = (root: HTMLElement, match: (b: HTMLButtonElement) => boolean) => {
  const button = [...root.querySelectorAll("button")].find(match);
  button?.click();
  return !!button;
};
const byLabel = (label: string) => (b: HTMLButtonElement) => b.getAttribute("aria-label") === label;

const clickLabel = (label: string) => (root: HTMLElement) => clickButton(root, byLabel(label));
const clickSections = (...titles: string[]) => (root: HTMLElement) => {
  const buttons = titles.map((t) => [...root.querySelectorAll("button")].find((b) => b.textContent?.startsWith(t)));
  if (buttons.some((b) => !b)) return false;
  buttons.forEach((b) => b?.click());
  return true;
};
const noop = () => true;

// A transform makes fixed children (the slide-over, the dialogs) fill this frame instead of the page.
const Frame = ({ w, h, children }: { w: string; h: string; children: ReactNode }) => (
  <Box position="relative" w={w} h={h} transform="translateZ(0)" overflow="hidden" borderWidth="1px" borderColor="border">
    {children}
  </Box>
);

// Slides the sidebar open once mounted; the docked desktop sidebar is already open.
const OpenSidebar = () => {
  const shell = useShell();
  useEffect(() => {
    if (!shell?.sidebarOpen) shell?.toggle();
    // Once on mount; toggling again would close it.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  return null;
};

const fetchEpicList = async () => ({ epics: rows, follow_ups: [] });

const FRAME: Record<Device, { w: string; h: string }> = {
  desktop: { w: "1000px", h: "640px" },
  tablet: { w: "760px", h: "640px" },
  phone: { w: "390px", h: "640px" },
};

const sidebarCase = (label: string, device: Device, step: (root: HTMLElement) => boolean) => ({
  label,
  node: (
    <DeviceContext.Provider value={device}>
      <WideContext.Provider value={device !== "phone"}>
        <ConnectionContext.Provider value={connection}>
          <MemoryRouter initialEntries={["/e/p:blog"]}>
            <Frame {...FRAME[device]}>
              <Drive step={step}>
                <AppShell fetchEpicList={fetchEpicList}>{device === "desktop" ? null : <OpenSidebar />}</AppShell>
              </Drive>
            </Frame>
          </MemoryRouter>
        </ConnectionContext.Provider>
      </WideContext.Provider>
    </DeviceContext.Provider>
  ),
});

// Types into a field the way a user does, so React sees the change.
const typeInto = (root: HTMLElement, label: string, value: string) => {
  const input = root.querySelector<HTMLInputElement>(`input[aria-label="${label}"]`);
  if (!input) return false;
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set?.call(input, value);
  input.dispatchEvent(new Event("input", { bubbles: true }));
  return true;
};

const manageCase = (label: string, step: (root: HTMLElement) => boolean) => ({
  label,
  node: (
    <ConnectionContext.Provider value={connection}>
      <Frame w="800px" h="460px">
        <Drive step={step}>
          <ManageWorkspaces workspaces={workspaces} onClose={() => {}} />
        </Drive>
      </Frame>
    </ConnectionContext.Provider>
  ),
});

export const shellEntries: GalleryEntry[] = [
  {
    name: "Sidebar",
    cases: [
      sidebarCase("desktop, docked", "desktop", noop),
      sidebarCase("tablet, slide-over open", "tablet", noop),
      sidebarCase("phone, slide-over open", "phone", noop),
      sidebarCase("desktop, Other open", "desktop", clickSections("Other")),
      sidebarCase("phone, Other open", "phone", clickSections("Other")),
      sidebarCase("desktop, On hold collapsed", "desktop", noop),
      sidebarCase("desktop, On hold expanded", "desktop", clickSections("On hold")),
    ],
  },
  {
    name: "WorkspaceSwitcher",
    cases: [
      {
        label: "menu open",
        node: (
          <Frame w="344px" h="260px">
            <Drive step={clickLabel("Workspace: Default")}>
              <WorkspaceSwitcher workspaces={workspaces} active="Default" onPick={() => {}} onNew={() => {}} onManage={() => {}} />
            </Drive>
          </Frame>
        ),
      },
    ],
  },
  {
    name: "ManageWorkspaces",
    cases: [
      manageCase("default", noop),
      manageCase("rename in progress", (root) => typeInto(root, "Rename Work", "Client work")),
      manageCase("delete confirmation", (root) => clickButton(root, byLabel("Delete Work"))),
    ],
  },
];

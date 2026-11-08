import { Box } from "@chakra-ui/react";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { ConnectionContext, type Connection } from "../connection";
import { Composer } from "../design/Composer";
import type { Answers } from "../design/round";
import type { SessionInfo, SessionRoundCard } from "../generated/server";
import { DeviceContext, WideContext, type Device } from "../useWide";
import type { GalleryEntry } from "./registry";

const connection = { refresh: () => {} } as Connection;

const round: SessionRoundCard[] = [
  { label: "8", title: "Which store?", body: "SQLite or files?", kind: "options", options: [{ id: "sqlite", label: "SQLite" }, { id: "files", label: "Files" }], recommended: "sqlite" },
  { label: "9", title: "Keep history?", body: "", kind: "yes-no" },
];
const answered = (n: number): Answers => Object.fromEntries(round.slice(0, n).map((c) => [c.label, { choice: "sqlite" }]));

const session = (over: Partial<SessionInfo> = {}): SessionInfo =>
  ({
    id: "gallery-footer",
    project: "p",
    name: "design-epic",
    dir: "/x",
    harness: "claude",
    status: { state: "idle" },
    status_changed_at: "2026-01-01T00:00:00Z",
    run_started_at: "2026-01-01T00:00:00Z",
    unread: {},
    actionable: {},
    auto_advance: true,
    auto_queue: false,
    ...over,
  }) as SessionInfo;

/** Clicks a button of the footer once mounted: the states the mock shows open. */
const Click = ({ label, children }: { label: string; children: ReactNode }) => {
  const root = useRef<HTMLDivElement>(null);
  useEffect(() => root.current?.querySelector<HTMLElement>(`[aria-label="${label}"]`)?.click(), [label]);
  return <div ref={root}>{children}</div>;
};

function Footer({ device, answers, open = false, countdown = false }: { device: Device; answers: number; open?: boolean; countdown?: boolean }) {
  const [menuOpen, setMenuOpen] = useState(open);
  const [deadline] = useState(() => new Date(Date.now() + 5000).toISOString());
  return (
    <Composer
      session={session(countdown ? { auto_advance_at: deadline } : {})}
      connected
      round={round}
      answers={answered(answers)}
      autoMenu={{ open: menuOpen, setOpen: setMenuOpen }}
    />
  );
}

const WIDTH: Record<Device, string> = { phone: "390px", tablet: "744px", desktop: "1000px" };

const footerCase = (label: string, device: Device, answers: number, opts: { open?: boolean; countdown?: boolean; note?: boolean; width?: string } = {}) => {
  const footer = <Footer device={device} answers={answers} open={opts.open} countdown={opts.countdown} />;
  return {
    label,
    node: (
      <DeviceContext.Provider value={device}>
        <WideContext.Provider value={device !== "phone"}>
          <ConnectionContext.Provider value={connection}>
            <Box w={opts.width ?? WIDTH[device]} minH="220px" display="flex" flexDirection="column" justifyContent="flex-end" borderWidth="1px" borderColor="border">
              {opts.note ? <Click label="Note">{footer}</Click> : footer}
            </Box>
          </ConnectionContext.Provider>
        </WideContext.Provider>
      </DeviceContext.Provider>
    ),
  };
};

/** One case per state of mock 09-design-phone-footer, named as the mock names it. */
export const designFooterEntries: GalleryEntry[] = [
  {
    name: "DesignFooter",
    cases: [
      footerCase("design-phone-footer-empty", "phone", 0),
      footerCase("design-phone-footer-answered", "phone", 1),
      footerCase("design-phone-footer-note", "phone", 1, { note: true }),
      footerCase("design-phone-footer-auto", "phone", 1, { open: true }),
      footerCase("design-phone-footer-countdown", "phone", 2, { countdown: true }),
      footerCase("design-phone-footer-320", "phone", 1, { width: "320px" }),
      footerCase("design-phone-footer-320-countdown", "phone", 2, { countdown: true, width: "320px" }),
      footerCase("design-tablet-footer", "tablet", 1),
      footerCase("design-tablet-footer-countdown", "tablet", 2, { countdown: true }),
    ],
  },
];

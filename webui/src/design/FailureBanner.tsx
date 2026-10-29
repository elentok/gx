import { Box, Spinner, Text } from "@chakra-ui/react";
import { useState } from "react";
import type { SessionFailure, SessionInfo } from "../generated/server";
import { ReasonStale } from "../ticket/answer";
import { Button } from "../ui/button/Button";
import { postResumeSession, stateToken, type OpenDesign, type ResumeSession } from "./open";
import { postPrompt, type PostPrompt } from "./prompt";
import type { FetchScreen } from "./screen";
import { ScreenModal } from "./ScreenModal";
import { startNew, useDesignWrite } from "./SessionControls";

// The generated file is imported for types only: its tygo constants are not runtime values.
const CRASHED = "crashed";
const ENDED = "ended";
const AUTH = "auth";
const STALLED = "stalled";
const BLOCKED = "blocked";
const ReasonSessionDirGone = "dir-gone";
const ReasonSessionNoResume = "no-resume";

/** Crashed and ended sessions are gone: the banner takes the composer's place. */
export const replacesComposer = (f?: SessionFailure) => f?.kind === CRASHED || f?.kind === ENDED;

/** A finished Design ends its session on purpose, so only the other failures still show. */
export const visibleFailure = (f?: SessionFailure, done?: boolean) => (done && f?.kind === ENDED ? undefined : f);

/** Auth and stalled are read from the transcript, which only Claude sessions have. */
export const needsTranscript = (f: SessionFailure) => f.kind === AUTH || f.kind === STALLED;

type ButtonName = "Restore" | "New session" | "Screen" | "Retry";

const BUTTONS: Record<string, ButtonName[]> = {
  [CRASHED]: ["Restore", "New session"],
  [ENDED]: ["Restore", "New session"],
  [AUTH]: ["Retry"],
  [STALLED]: ["Screen", "New session"],
  [BLOCKED]: ["Screen"],
};

export interface FailureActions {
  /** The Design's epic address, for New session. */
  address: string;
  open?: OpenDesign;
  resume?: ResumeSession;
  fetchScreen?: FetchScreen;
}

/** Why the session needs the person, with a button named after what they do about it. */
export function FailureBanner({
  failure,
  session,
  lastMessage,
  post = postPrompt,
  onStale,
  resume = postResumeSession,
  fetchScreen,
  address,
  open,
  restore: canRestore = true,
}: {
  failure: SessionFailure;
  session: SessionInfo;
  /** Off drops Restore and New session. */
  restore?: boolean;
  /** The id of the last chat message the page saw, for Retry's stale guard. */
  lastMessage?: string;
  post?: PostPrompt;
  onStale?: () => void;
} & FailureActions) {
  const { run, pending } = useDesignWrite();
  const [clicked, setClicked] = useState<ButtonName>();
  const [screen, setScreen] = useState(false);
  // A refusal that no retry can fix: the page stops offering Restore, as it does after a quick re-crash.
  const [cannotRestore, setCannotRestore] = useState(false);
  const sessionId = session.id;
  const restoreHidden = failure.recrashed || cannotRestore;

  const restore = () =>
    run(async () => {
      const res = await resume({ id: sessionId, state: stateToken(session) });
      if (res.refused && (res.reason === ReasonSessionDirGone || res.reason === ReasonSessionNoResume)) setCannotRestore(true);
      return res;
    });

  const retry = () =>
    run(async () => {
      const res = await post({ id: sessionId, text: "continue", last_message: lastMessage });
      if (res.refused && res.reason === ReasonStale) onStale?.();
      return res;
    });

  const onClick: Record<ButtonName, () => void> = {
    Restore: () => (setClicked("Restore"), restore()),
    "New session": () => (setClicked("New session"), startNew(run, address, session, open)),
    Screen: () => setScreen(true),
    Retry: () => (setClicked("Retry"), retry()),
  };
  const buttons = (BUTTONS[failure.kind] ?? []).filter(
    (name) => (canRestore || (name !== "Restore" && name !== "New session")) && !(name === "Restore" && restoreHidden),
  );

  return (
    <Box
      role="region"
      aria-label="Session failure"
      data-kind={failure.kind}
      borderWidth="1px"
      borderColor="tone.deadlocked.fg"
      borderRadius="md"
      p="3"
      display="flex"
      flexDirection="column"
      gap="2"
    >
      <Text fontWeight="700" color="tone.deadlocked.fg">
        {failure.label}
      </Text>
      {failure.text && <Text fontSize="sm">{failure.text}</Text>}
      {failure.kind === BLOCKED && (
        <Text fontSize="sm" color="text.muted">
          Open it in gx, Agents tab.
        </Text>
      )}
      {buttons.length > 0 && (
        <Box display="flex" gap="2">
          {buttons.map((name) => (
            <Button
              key={name}
              emphasis={name === "New session" && restoreHidden ? "main" : "secondary"}
              tone={name === "New session" && restoreHidden ? "running" : undefined}
              disabled={pending}
              onClick={onClick[name]}
            >
              {pending && clicked === name && <Spinner size="sm" />}
              {name}
            </Button>
          ))}
        </Box>
      )}
      {screen && <ScreenModal target={sessionId} fetcher={fetchScreen} onClose={() => setScreen(false)} />}
    </Box>
  );
}

/** A step gx could not finish, such as a Continue that never started: its reason, so it shows outside the log. */
export function GxStopped({ reason }: { reason: string }) {
  return (
    <Box
      role="region"
      aria-label="gx stopped"
      borderWidth="1px"
      borderColor="tone.waiting.fg"
      borderRadius="md"
      p="3"
      display="flex"
      flexDirection="column"
      gap="1"
    >
      <Text fontWeight="700" color="tone.waiting.fg">
        gx stopped
      </Text>
      <Text fontSize="sm">{reason}</Text>
      <Text fontSize="sm" color="text.muted">
        Continue tries again.
      </Text>
    </Box>
  );
}

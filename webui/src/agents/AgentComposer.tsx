import { Box, Spinner } from "@chakra-ui/react";
import { useRef, useState } from "react";
import { codexSkillHint, hasChat } from "../design/chat";
import { FooterLine } from "../design/FooterLine";
import { ComposerImages } from "../design/ComposerImages";
import { InterruptPopover } from "../design/InterruptPopover";
import { postInterruptSession, stateToken, type InterruptSession } from "../design/open";
import { promptable } from "../design/view";
import { useComposerImages } from "../design/useComposerImages";
import { ExpandableTextField } from "../ui/expandable-text/ExpandableTextField";
import type { PostPrompt } from "../design/prompt";
import { usePromptSend } from "../design/usePromptSend";
import { useComposerDraft } from "../drafts";
import type { SessionInfo } from "../generated/server";
import { refusalText } from "../useWrite";
import { useWide } from "../useWide";
import { Button } from "../ui/button/Button";
import { usePromptFocus } from "./promptFocus";

// Under the Agents page's chat: the box is always open with Send beside it. A refusal shows inline only, as in the mock.
export function AgentComposer({
  session,
  lastMessage,
  connected,
  post,
  interrupt = postInterruptSession,
  onStale,
  reason,
  onSent,
  ceiling,
}: {
  /** The smart-zone ceiling from the snapshot; the badge needs it. */
  ceiling?: number;
  session: SessionInfo;
  lastMessage?: string;
  connected: boolean;
  post?: PostPrompt;
  interrupt?: InterruptSession;
  onStale?: () => void;
  /** The line under the box; the caller decides the text. */
  reason?: string;
  /** Called with the text once the server took the prompt, so the chat can show it before the transcript catches up. */
  onSent?: (text: string, images: string[]) => void;
}) {
  const wide = useWide();
  const box = useRef<HTMLDivElement>(null);
  usePromptFocus(box);
  const [text, setText] = useComposerDraft(session.id);
  const images = useComposerImages(session.id);
  const { send, write, enabled, pending, refusal, error } = usePromptSend({
    session,
    lastMessage,
    connected,
    post,
    onStale,
    toastRefusal: false,
  });
  // A Working session takes no prompt: Interrupt ends its turn and keeps the session.
  const working = session.status.state === "working" && !promptable(session);
  const [interruptOpen, setInterruptOpen] = useState(false);
  const interruptRef = useRef<HTMLButtonElement>(null);
  const onInterrupt = () => {
    setInterruptOpen(false);
    return connected ? write(() => interrupt({ id: session.id, state: stateToken(session) })) : Promise.resolve();
  };
  const canSend = images.canSend(text);
  const sendText = () =>
    send({ text, ...images.payload }, () => {
      onSent?.(text, images.readyIds);
      setText("");
      images.clear();
    });
  // A phone has no key send.
  const onSubmit = wide
    ? () => {
        if (enabled && canSend) sendText();
      }
    : undefined;

  // Messages sit inside the box: the caller's reason first, then the last refusal in red, then the Codex `$` hint.
  const failure = refusal ? refusalText(refusal) : error;
  const hint = codexSkillHint(session.harness, text);
  const line = reason
    ? { text: reason, red: false }
    : failure
      ? { text: failure, red: true }
      : hint
        ? { text: hint, red: false }
        : undefined;

  return (
    <Box
      ref={box}
      position="sticky"
      bottom="0"
      bg="surface"
      pt="3"
      pb="0"
      display="flex"
      flexDirection="column"
      gap="2"
      role="form"
      aria-label="Composer"
    >
      <ComposerImages
        state={images}
        send={
          working ? (
            <>
              <Button
                ref={interruptRef}
                emphasis="secondary"
                disabled={!connected || pending}
                aria-expanded={interruptOpen}
                onClick={() => setInterruptOpen(!interruptOpen)}
              >
                {pending && <Spinner size="sm" />}
                Interrupt
              </Button>
              <InterruptPopover
                open={interruptOpen}
                disabled={!connected || pending}
                anchorRef={interruptRef}
                onConfirm={onInterrupt}
                onClose={() => setInterruptOpen(false)}
              />
            </>
          ) : (
            <Button disabled={!enabled || !canSend} onClick={sendText}>
              {pending && <Spinner size="sm" />}
              Send
            </Button>
          )
        }
      >
        <ExpandableTextField
          rows={3}
          label="Message"
          name="chat-message"
          title={`Message to ${session.name || session.id}`}
          placeholder={`Message ${session.name || session.id}`}
          value={text}
          onChange={setText}
          onSubmit={onSubmit}
          note={line ?? null}
          footer={<FooterLine session={session} ceiling={ceiling} modelOnly={!hasChat(session.harness)} />}
          completion={{ target: { sessionId: session.id }, harness: session.harness, off: !enabled }}
        />
      </ComposerImages>
    </Box>
  );
}

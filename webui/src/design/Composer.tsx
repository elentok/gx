import { Box, Text, Textarea } from "@chakra-ui/react";
import { useState } from "react";
import type { SessionInfo, SessionNext, SessionPromptRequest } from "../generated/server";
import { ReasonStale } from "../ticket/answer";
import { Button } from "../ui/button/Button";
import { postPrompt, type PostPrompt } from "./prompt";

const CONTINUE_PHASES = ["map", "review"];

// Under the chat: the notes box behind a note icon, and Continue after a run-end line. Nothing sends unless the session is Idle.
export function Composer({
  session,
  lastMessage,
  next,
  connected,
  post = postPrompt,
  onStale,
}: {
  session: SessionInfo;
  /** The id of the last chat message the page saw. */
  lastMessage?: string;
  next?: SessionNext;
  connected: boolean;
  post?: PostPrompt;
  /** The conversation moved under the person: refetch the chat, keep the draft. */
  onStale?: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [text, setText] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const enabled = connected && session.status.state === "idle" && !pending;

  const send = async (body: Partial<SessionPromptRequest>, onSent?: () => void) => {
    if (!enabled) return;
    setPending(true);
    setError("");
    try {
      const res = await post({ id: session.id, text: "", last_message: lastMessage, ...body });
      if (res.refused) {
        setError(res.message ?? "The message was refused.");
        if (res.reason === ReasonStale) onStale?.();
      } else {
        onSent?.();
      }
    } catch {
      setError("Could not send. Try again.");
    } finally {
      setPending(false);
    }
  };

  const phase = next?.phase ?? "";
  return (    <Box position="sticky" bottom="0" bg="surface" borderTopWidth="1px" py="3" display="flex" flexDirection="column" gap="2" role="form" aria-label="Composer">
      {CONTINUE_PHASES.includes(phase) && (
        <Button disabled={!enabled} onClick={() => send({ continue: true })} flexDirection="column" height="auto" py="2">
          <Text as="span">Continue</Text>
          <Text as="span" fontSize="xs" fontWeight="normal">
            Continue · {phase}
          </Text>
        </Button>
      )}
      {open && (
        <Textarea aria-label="Message" value={text} disabled={!enabled} onChange={(e) => setText(e.target.value)} />
      )}
      {error && <Text role="alert">{error}</Text>}
      <Box display="flex" gap="2">
        <Button variant="outline" aria-label="Note" aria-pressed={open} onClick={() => setOpen((o) => !o)}>
          📝
        </Button>
        {open && (
          <Button disabled={!enabled || text.trim() === ""} onClick={() => send({ text }, () => setText(""))}>
            Send
          </Button>
        )}
      </Box>
    </Box>
  );
}

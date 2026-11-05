import { Box, Spinner, Text } from "@chakra-ui/react";
import type { PostPrompt } from "../design/prompt";
import { usePromptSend } from "../design/usePromptSend";
import type { SessionInfo } from "../generated/server";
import { Button } from "../ui/button/Button";
import { refusalText } from "../useWrite";

/** Sends the gx-promote-to-design command for a plain session. It never touches the composer draft. */
export function PromoteButton({
  session,
  lastMessage,
  hasMessages,
  connected,
  reason,
  post,
  onStale,
}: {
  session: SessionInfo;
  lastMessage?: string;
  hasMessages: boolean;
  connected: boolean;
  /** Why the composer cannot send; the caller decides the text. */
  reason?: string;
  post?: PostPrompt;
  onStale?: () => void;
}) {
  const { send, enabled, pending, refusal, error } = usePromptSend({ session, lastMessage, connected, post, onStale, toastRefusal: false });
  const off = reason ?? (hasMessages ? undefined : "No messages yet. Promote turns on after the first message.");
  const failure = refusal ? refusalText(refusal) : error;
  // The composer already prints its own reason under the box, so only the no-message reason is printed here.
  const line = !reason && off ? { text: off, red: false } : failure ? { text: failure, red: true } : undefined;
  return (
    <Box display="flex" alignItems="center" gap="2" flexWrap="wrap">
      <Button emphasis="secondary" disabled={!enabled || off !== undefined} title={off} onClick={() => send({ promote: true })}>
        {pending && <Spinner size="sm" />}
        Promote to Design
      </Button>
      {line && (
        <Text as="span" fontSize="sm" color={line.red ? "tone.deadlocked.fg" : "text.dim"}>
          {line.text}
        </Text>
      )}
    </Box>
  );
}

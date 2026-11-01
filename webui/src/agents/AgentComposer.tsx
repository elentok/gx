import { Box, Spinner, Text } from "@chakra-ui/react";
import { codexSkillHint } from "../design/chat";
import { ExpandableTextField } from "../ui/expandable-text/ExpandableTextField";
import type { PostPrompt } from "../design/prompt";
import { usePromptSend } from "../design/usePromptSend";
import { useComposerDraft } from "../drafts";
import type { SessionInfo } from "../generated/server";
import { refusalText } from "../useWrite";
import { useWide } from "../useWide";
import { Button } from "../ui/button/Button";

// Under the Agents page's chat: the box is always open with Send beside it. A refusal shows inline only, as in the mock.
export function AgentComposer({
  session,
  lastMessage,
  connected,
  post,
  onStale,
  reason,
}: {
  session: SessionInfo;
  lastMessage?: string;
  connected: boolean;
  post?: PostPrompt;
  onStale?: () => void;
  /** The line under the box; the caller decides the text. */
  reason?: string;
}) {
  const wide = useWide();
  const [text, setText] = useComposerDraft(session.id);
  const { send, enabled, pending, refusal, error } = usePromptSend({
    session,
    lastMessage,
    connected,
    post,
    onStale,
    toastRefusal: false,
  });
  const sendText = () => send({ text }, () => setText(""));
  // A phone has no key send.
  const onSubmit = wide
    ? () => {
        if (enabled && text.trim() !== "") sendText();
      }
    : undefined;

  // The caller's reason comes first, then the last refusal in red, then the Codex `$` hint, else the draft note.
  const failure = refusal ? refusalText(refusal) : error;
  const hint = codexSkillHint(session.harness, text);
  const line = reason
    ? { text: reason, red: false }
    : failure
      ? { text: failure, red: true }
      : hint
        ? { text: hint, red: false }
        : text !== ""
          ? { text: "Draft saved in this browser", red: false }
          : undefined;

  return (
    <Box
      position="sticky"
      bottom="0"
      bg="surface"
      py="3"
      display="flex"
      flexDirection="column"
      gap="2"
      role="form"
      aria-label="Composer"
    >
      <Box display="flex" gap="2" alignItems="flex-end">
        <Box flex="1">
          <ExpandableTextField
            rows={3}
            label="Message"
            title={`Message to ${session.name || session.id}`}
            placeholder={`Message ${session.name || session.id}`}
            value={text}
            onChange={setText}
            onSubmit={onSubmit}
            completion={{ target: { sessionId: session.id }, harness: session.harness, off: !enabled }}
          />
        </Box>
        <Button disabled={!enabled || text.trim() === ""} onClick={sendText}>
          {pending && <Spinner size="sm" />}
          Send
        </Button>
      </Box>
      {line && (
        <Text fontSize="xs" color={line.red ? "tone.deadlocked.fg" : "text.dim"}>
          {line.text}
        </Text>
      )}
    </Box>
  );
}

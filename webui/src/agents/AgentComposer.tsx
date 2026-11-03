import { Box, Spinner } from "@chakra-ui/react";
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
  onSent,
}: {
  session: SessionInfo;
  lastMessage?: string;
  connected: boolean;
  post?: PostPrompt;
  onStale?: () => void;
  /** The line under the box; the caller decides the text. */
  reason?: string;
  /** Called with the text once the server took the prompt, so the chat can show it before the transcript catches up. */
  onSent?: (text: string) => void;
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
  const sendText = () =>
    send({ text }, () => {
      onSent?.(text);
      setText("");
    });
  // A phone has no key send.
  const onSubmit = wide
    ? () => {
        if (enabled && text.trim() !== "") sendText();
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
      <Box display="flex" gap="2" alignItems="stretch">
        <Box flex="1">
          <ExpandableTextField
            rows={3}
            label="Message"
            title={`Message to ${session.name || session.id}`}
            placeholder={`Message ${session.name || session.id}`}
            value={text}
            onChange={setText}
            onSubmit={onSubmit}
            note={line ?? null}
            completion={{ target: { sessionId: session.id }, harness: session.harness, off: !enabled }}
          />
        </Box>
        <Button h="auto" disabled={!enabled || text.trim() === ""} onClick={sendText}>
          {pending && <Spinner size="sm" />}
          Send
        </Button>
      </Box>
    </Box>
  );
}

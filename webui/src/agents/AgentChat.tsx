import { Box, Text } from "@chakra-ui/react";
import { useState } from "react";
import { ChatMessages, WorkingRow } from "../design/ChatCore";
import { sheetCardsOf } from "../design/chat";
import { FailureBanner, visibleFailure } from "../design/FailureBanner";
import { postAnswer, type PostAnswer } from "../design/prompt";
import { QuestionSheet } from "../design/QuestionSheet";
import { useRoundDraft } from "../design/roundDraft";
import {
  CardSourceClaudeDialog,
  CardSourceCodex,
  CardSourcePermission,
  CardSourceRound,
  ReasonCodexNotReady,
  ReasonCodexStopped,
  type QuestionCard,
  type SessionAnswer,
  type SessionChatResponse,
  type SessionInfo,
} from "../generated/server";
import { ReasonStale } from "../ticket/answer";
import { Button } from "../ui/button/Button";
import { allAnswered, type QuestionMode } from "../ui/question-sheet/round";

/** The first open card, docked above the banner. The caller keys it by card id, so a new card starts a fresh draft. */
function CardSheet({
  session,
  card,
  lastMessage,
  post,
  refetch,
}: {
  session: SessionInfo;
  card: QuestionCard;
  lastMessage?: string;
  post: PostAnswer;
  refetch: () => void;
}) {
  const { draft, setAnswers, setIndex, clear } = useRoundDraft(
    session.id,
    card.id,
  );
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  // Codex typed some keys and stopped: another Submit would type the rest on top of them.
  const [stopped, setStopped] = useState(false);
  const cards = sheetCardsOf(card);
  const ready = allAnswered(cards, draft.answers);
  const claude = card.source === CardSourceClaudeDialog;
  const hook = claude && !card.fallback;
  const dismissable = claude || card.source === CardSourcePermission;
  const mode: QuestionMode =
    card.source === CardSourceRound
      ? "round"
      : hook || card.source === CardSourceCodex
        ? "notes"
        : "plain";

  const send = async (answers: SessionAnswer[], release?: boolean) => {
    if (pending) return;
    setPending(true);
    setError("");
    try {
      const res = await post({
        id: session.id,
        source: card.source,
        card_id: card.id,
        answers,
        release,
        last_message: lastMessage,
      });
      if (res.refused) {
        if (res.reason === ReasonCodexNotReady)
          setError("Codex is not ready for the answer yet. Retry in a moment.");
        else if (res.reason === ReasonCodexStopped) {
          setStopped(true);
          setError(
            "Codex took part of the answer and stopped. Open Screen to finish it.",
          );
        } else setError(res.message ?? "The answer was refused.");
        if (res.reason === ReasonStale) refetch();
      } else {
        clear();
        refetch();
      }
    } catch {
      setError("Could not send the answer. Try again.");
    } finally {
      setPending(false);
    }
  };

  return (
    <Box
      display="flex"
      flexDirection="column"
      gap="2"
      maxH="50dvh"
      minH="0"
      flex="none"
    >
      <QuestionSheet
        round={cards}
        answers={draft.answers}
        onAnswers={setAnswers}
        initialIndex={draft.index}
        onIndex={setIndex}
        mode={mode}
      />
      {error && <Text role="alert">{error}</Text>}
      <Box display="flex" gap="2" justifyContent="flex-end" flexWrap="wrap">
        {hook && (
          <Button
            emphasis="ghost"
            tone="muted"
            disabled={pending}
            onClick={() => send([], true)}
          >
            Answer in the terminal
          </Button>
        )}
        {dismissable && (
          <Button
            emphasis="ghost"
            tone="muted"
            disabled={pending}
            onClick={() => send([])}
          >
            Dismiss
          </Button>
        )}
        <Button
          emphasis="main"
          disabled={!ready || pending || stopped}
          onClick={() =>
            ready && send(cards.map((c) => draft.answers[c.label] ?? {}))
          }
        >
          Submit
        </Button>
      </Box>
    </Box>
  );
}

/** The chat of a Claude or Codex session on the Agents page: the messages, a plain Working row, the question sheet and the failure notice. */
export function AgentChat({
  session,
  chat,
  refetch,
  post = postAnswer,
}: {
  session: SessionInfo;
  chat: SessionChatResponse;
  refetch: () => void;
  post?: PostAnswer;
}) {
  const busy = session.status.state === "working" && !session.status.background;
  const failure = visibleFailure(session.failure);
  const card = chat.questions[0];
  return (
    // Sized by its content first, so the "No messages yet" row beside it cannot squeeze a card and banner under the composer.
    <Box display="flex" flexDirection="column" gap="3" flex="1 1 auto" minH="0">
      <ChatMessages
        chat={chat}
        busy={busy}
        sessionId={session.id}
        showTools
        working={<WorkingRow current={chat.current} others={chat.others} />}
      />
      {card && (
        <CardSheet
          key={card.id}
          session={session}
          card={card}
          lastMessage={chat.messages.at(-1)?.id}
          post={post}
          refetch={refetch}
        />
      )}
      {failure && (
        <FailureBanner
          failure={failure}
          session={session}
          lastMessage={chat.messages.at(-1)?.id}
          onStale={refetch}
          restore={false}
          address=""
        />
      )}
    </Box>
  );
}

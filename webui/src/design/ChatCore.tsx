import { Box, Spinner as ChakraSpinner, Text } from "@chakra-ui/react";
import { useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { Button } from "../ui/button/Button";
import { fetchArchive } from "./chat";
import type {
  SessionBackgroundTask,
  SessionChatImage,
  SessionChatMessage,
  SessionChatResponse,
} from "../generated/server";
import { sessionImageUrl } from "./sessionImages";
import { Markdown } from "../markdown/Markdown";

// The part of the chat both the Design page and the Agents page show: the scrolling messages and the working row.

const KIND_NAMES: Record<string, string> = {
  shell: "shell",
  monitor: "Monitor",
  agent: "subagent",
};

const MAX_THUMBS = 3;

// Up to three thumbnails; the rest fold into a "+N" tile on the third. The full-size view is a later ticket.
function ChatImageGrid({ images, sessionId }: { images: SessionChatImage[]; sessionId: string }) {
  const shown = images.length > MAX_THUMBS ? images.slice(0, MAX_THUMBS) : images;
  const extra = images.length - shown.length;
  const tile = { w: "100%", h: "100%", minH: "0", borderRadius: "md", overflow: "hidden", display: "block", p: "0", border: "0" };
  const cols = shown.length === 1 ? "1fr" : shown.length === 2 ? "1fr 1fr" : "2fr 1fr";
  return (
    <Box
      data-mock-part="chat-thumbs"
      display="grid"
      gap="1"
      gridTemplateColumns={cols}
      gridTemplateRows={shown.length >= 3 ? "1fr 1fr" : undefined}
      maxW={shown.length === 1 ? "240px" : undefined}
      aspectRatio={shown.length === 1 ? "4/3" : shown.length === 2 ? "2/1" : "3/2"}
    >
      {shown.map((img, i) => {
        const label = `Image ${i + 1} of ${images.length}`;
        const last = i === shown.length - 1 && extra > 0;
        return (
          <Box
            as="button"
            key={img.id ?? `ph-${i}`}
            aria-label={last ? `${label}, ${extra} more` : label}
            bg="surface"
            cursor="pointer"
            position="relative"
            gridRow={shown.length >= 3 && i === 0 ? "1 / 3" : undefined}
            {...tile}
          >
            {img.id && !img.placeholder ? (
              <img
                src={sessionImageUrl(sessionId, img.id)}
                alt=""
                loading="lazy"
                decoding="async"
                style={{ width: "100%", height: "100%", objectFit: "cover", display: "block" }}
              />
            ) : (
              <Box data-testid="chat-image-placeholder" w="100%" h="100%" bg="border" />
            )}
            {last && (
              <Box
                position="absolute"
                inset="0"
                display="flex"
                alignItems="center"
                justifyContent="center"
                bg="surface.raised"
                opacity="0.9"
                fontWeight="semibold"
                fontSize="lg"
              >
                {`+${extra}`}
              </Box>
            )}
          </Box>
        );
      })}
    </Box>
  );
}

function Entry({ m, showTools, sessionId }: { m: SessionChatMessage; showTools: boolean; sessionId: string }) {
  if (m.kind === "compacted") {
    return (
      <Text
        role="separator"
        textAlign="center"
        fontSize="xs"
        color="text.dim"
        borderBlockStart="1px solid"
        borderColor="border"
        pt="1"
      >
        Context compacted
      </Text>
    );
  }
  if (m.kind === "tools") return showTools ? <ToolCalls m={m} /> : null;
  if (m.kind === "prompt") {
    const images = m.images ?? [];
    if (images.length === 0) {
      return (
        <Box
          alignSelf="flex-end"
          maxW="85%"
          bg="surface.raised"
          borderRadius="lg"
          px="3"
          py="2"
          fontSize="base"
          whiteSpace="pre-wrap"
        >
          {m.text}
        </Box>
      );
    }
    return (
      <Box
        alignSelf="flex-end"
        maxW="85%"
        bg="surface.raised"
        border="1px solid"
        borderColor="border"
        borderRadius="lg"
        px="3"
        py="2"
        fontSize="base"
        display="flex"
        flexDirection="column"
        gap="2"
      >
        <ChatImageGrid images={images} sessionId={sessionId} />
        {m.text && (
          <Box as="span" data-mock-part="chat-text" whiteSpace="pre-wrap">
            {m.text}
          </Box>
        )}
      </Box>
    );
  }
  if (m.kind === "command") {
    return (
      <Text
        as="span"
        alignSelf="flex-end"
        fontFamily="mono"
        fontSize="xs"
        color="text"
        bg="surface.raised"
        border="1px solid"
        borderColor="border"
        borderRadius="md"
        px="2"
      >
        {m.text}
      </Text>
    );
  }
  if (m.kind === "output") return <CommandOutput text={m.text} />;
  return <Markdown fontSize="base">{m.text}</Markdown>;
}

// A run of tool calls, folded: the count in the summary, one line per call (shell commands, files) when opened.
function ToolCalls({ m }: { m: SessionChatMessage }) {
  const calls = m.calls ?? [];
  const n = m.tool_count ?? calls.length;
  return (
    <Box as="details" fontSize="xs" color="text.dim" data-testid="tool-calls">
      <Box as="summary" cursor="pointer">
        {n === 1 ? "1 tool call" : `${n} tool calls`}
        {m.last_tool ? ` · last: ${m.last_tool}` : ""}
      </Box>
      <Box as="ul" listStyleType="none" m="0" mt="1" p="0" fontFamily="mono" display="flex" flexDirection="column" gap="0.5">
        {calls.map((c, i) => (
          <Box as="li" key={i} whiteSpace="pre-wrap" wordBreak="break-all" borderInlineStart="2px solid" borderColor="border" px="2.5">
            {c}
          </Box>
        ))}
      </Box>
    </Box>
  );
}

const OUTPUT_LINE_LIMIT = 12;

// Plain text on purpose: command output is not Markdown, so `*` and `#` must stay literal.
function CommandOutput({ text }: { text: string }) {
  const [expanded, setExpanded] = useState(false);
  const lines = text.split("\n");
  const cut = !expanded && lines.length > OUTPUT_LINE_LIMIT;
  return (
    <Box alignSelf="flex-end" maxW="85%" display="flex" flexDirection="column" alignItems="flex-end" gap="1.5">
      <Text
        as="pre"
        data-testid="command-output"
        w="max-content"
        maxW="100%"
        borderInlineStart="2px solid"
        borderColor="border"
        px="2.5"
        py="0.5"
        fontFamily="mono"
        fontSize="xs"
        lineHeight="1.65"
        color="text.dim"
        textAlign="left"
        whiteSpace="pre"
        overflow="hidden"
      >
        {cut ? `${lines.slice(0, OUTPUT_LINE_LIMIT).join("\n")}\n…` : text}
      </Text>
      {cut && (
        <Button size="sm" emphasis="ghost" tone="running" onClick={() => setExpanded(true)}>
          {`Show all (${lines.length} lines)`}
        </Button>
      )}
    </Box>
  );
}

function BackgroundStrip({ tasks }: { tasks: SessionBackgroundTask[] }) {
  return (
    <Box
      role="list"
      aria-label="Running in background"
      display="flex"
      flexWrap="wrap"
      gap="2"
      fontSize="xs"
      color="text.muted"
    >
      {tasks.map((t, i) => (
        <Box
          key={i}
          role="listitem"
          border="1px solid"
          borderColor="border"
          borderRadius="md"
          px="2"
          py="0.5"
        >
          {KIND_NAMES[t.kind] ?? t.kind}: {t.label}
        </Box>
      ))}
    </Box>
  );
}

const KIND_PLURALS: Record<string, string> = { shell: "shells", monitor: "monitors", agent: "subagents" };

/** "2 subagents, 1 shell": what the turn waits on besides the model, by task kind. */
export function backgroundSummary(tasks: SessionBackgroundTask[]): string {
  const counts = new Map<string, number>();
  for (const t of tasks) counts.set(t.kind, (counts.get(t.kind) ?? 0) + 1);
  return [...counts]
    .map(([kind, n]) => `${n} ${n === 1 ? (KIND_NAMES[kind] ?? kind) : (KIND_PLURALS[kind] ?? `${kind}s`)}`)
    .join(", ");
}

export interface WorkingStep {
  step?: string;
  title?: string;
}

// Inline, not the page-centered Spinner: this one sits at the bottom of the chat.
export function WorkingRow({
  step,
  title,
  background = [],
}: WorkingStep & { background?: SessionBackgroundTask[] }) {
  return (
    <Box
      role="status"
      display="flex"
      alignItems="center"
      gap="2"
      fontSize="sm"
      color="text.muted"
    >
      <ChakraSpinner size="sm" />
      <Text as="span">
        Working
        {step && `: ${step}`}
        {title && ` · ${title}`}
        {background.length > 0 && ` · waiting on ${backgroundSummary(background)}`}
      </Text>
    </Box>
  );
}

// The messages scroll alone. They open at the newest one and follow new ones until the reader scrolls up.
function ChatScroller({
  children,
  signal,
}: {
  children: ReactNode;
  /** Changes whenever the content grows. */
  signal: unknown;
}) {
  const el = useRef<HTMLDivElement>(null);
  const stuck = useRef(true);
  useLayoutEffect(() => {
    if (el.current && stuck.current)
      el.current.scrollTop = el.current.scrollHeight;
  }, [signal]);
  return (
    <Box
      ref={el}
      flex="1"
      minH="0"
      overflowY="auto"
      lineHeight="1.65"
      display="flex"
      flexDirection="column"
      gap="3"
      onScroll={(e) => {
        const t = e.currentTarget;
        stuck.current = t.scrollHeight - t.scrollTop - t.clientHeight < 80;
      }}
    >
      {/* A short chat sits at the bottom, next to the composer; the spacer shrinks away once the chat scrolls. */}
      <Box flex="1" aria-hidden />
      {children}
    </Box>
  );
}

// The Codex notices. An archived rollout is decoded only when the person asks; the page load never does.
function CodexNotices({
  chat,
  sessionId,
  onDecoded,
}: {
  chat: SessionChatResponse;
  sessionId: string;
  onDecoded: (messages: SessionChatMessage[]) => void;
}) {
  const [error, setError] = useState<string>();
  const decode = () => {
    setError(undefined);
    fetchArchive(sessionId).then(
      (r) => (r.kind === "ok" ? onDecoded(r.detail.messages) : setError("No archived rollout for this session.")),
      (e: Error) => setError(e.message),
    );
  };
  return (
    <>
      {chat.archived && (
        <Box role="status" display="flex" alignItems="center" gap="3" flexWrap="wrap" fontSize="sm" color="text.muted">
          <Text as="span">Codex archived this conversation</Text>
          <Button emphasis="secondary" onClick={decode}>
            Decode
          </Button>
          {error && <Text as="span" color="tone.deadlocked.fg">{error}</Text>}
        </Box>
      )}
      {chat.unreadable && (
        <Text role="status" fontSize="sm" color="text.muted">
          gx cannot read this Codex conversation
          {chat.codex_version && ` (Codex ${chat.codex_version})`}. The terminal and the agent are unaffected. Run `gx
          codex doctor`.
        </Text>
      )}
    </>
  );
}

/** The scrolling messages, the background strip and, while the agent works, the caller's working row. */
export function ChatMessages({
  chat,
  busy,
  working,
  sessionId,
  showTools = false,
}: {
  chat: SessionChatResponse;
  /** Shows each folded run of tool calls; the Design page leaves them out. */
  showTools?: boolean;
  busy: boolean;
  /** Shown at the bottom while `busy`. */
  working?: ReactNode;
  /** The session the chat belongs to: the archive route needs it. */
  sessionId: string;
}) {
  // /clear keeps the session and changes the conversation: the decoded archive is dropped and the scroller
  // remounts, so it sticks to the bottom again. The draft lives with the session and is untouched.
  const conversationId = chat.conversation_id;
  const [decoded, setDecoded] = useState<{ sessionId: string; conversationId: string; messages: SessionChatMessage[] }>();
  const messages =
    decoded?.sessionId === sessionId && decoded.conversationId === conversationId ? decoded.messages : chat.messages;
  return (
    <ChatScroller
      key={conversationId}
      signal={`${messages.length}|${chat.background.length}|${busy}`}
    >
      {messages.length === 0 && (
        <CodexNotices
          chat={chat}
          sessionId={sessionId}
          onDecoded={(m) => setDecoded({ sessionId, conversationId, messages: m })}
        />
      )}
      {chat.truncated && (
        <Text fontSize="xs" color="text.dim" textAlign="center">
          Earlier messages hidden
        </Text>
      )}
      {messages.map((m, i) => (
        <Entry key={m.id || i} m={m} showTools={showTools} sessionId={sessionId} />
      ))}
      {chat.background.length > 0 && <BackgroundStrip tasks={chat.background} />}
      {busy && working}
    </ChatScroller>
  );
}

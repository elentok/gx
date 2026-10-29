import { Box, Spinner as ChakraSpinner, Text } from "@chakra-ui/react";
import { useLayoutEffect, useRef, type ReactNode } from "react";
import type {
  SessionBackgroundTask,
  SessionChatMessage,
  SessionChatResponse,
} from "../generated/server";
import { Markdown } from "../markdown/Markdown";

// The part of the chat both the Design page and the Agents page show: the scrolling messages and the working row.

const KIND_NAMES: Record<string, string> = {
  shell: "shell",
  monitor: "Monitor",
  agent: "subagent",
};

function Entry({ m }: { m: SessionChatMessage }) {
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
  if (m.kind === "tools") return null;
  if (m.kind === "prompt") {
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
  if (m.kind === "command") {
    return (
      <Text
        as="span"
        alignSelf="flex-end"
        fontSize="xs"
        color="text.muted"
        border="1px solid"
        borderColor="border"
        borderRadius="full"
        px="2"
      >
        {m.text}
      </Text>
    );
  }
  return <Markdown fontSize="base">{m.text}</Markdown>;
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

export interface WorkingStep {
  step?: string;
  title?: string;
}

// Inline, not the page-centered Spinner: this one sits at the bottom of the chat.
export function WorkingRow({ step, title }: WorkingStep) {
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

/** The scrolling messages, the background strip and, while the agent works, the caller's working row. */
export function ChatMessages({
  chat,
  busy,
  working,
}: {
  chat: SessionChatResponse;
  busy: boolean;
  /** Shown at the bottom while `busy`. */
  working?: ReactNode;
}) {
  return (
    <ChatScroller
      signal={`${chat.messages.length}|${chat.background.length}|${busy}`}
    >
      {chat.truncated && (
        <Text fontSize="xs" color="text.dim" textAlign="center">
          Earlier messages hidden
        </Text>
      )}
      {chat.messages.map((m, i) => (
        <Entry key={m.id || i} m={m} />
      ))}
      {chat.background.length > 0 && <BackgroundStrip tasks={chat.background} />}
      {busy && working}
    </ChatScroller>
  );
}

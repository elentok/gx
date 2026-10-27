import { Box, chakra, Popover, Portal, Text } from "@chakra-ui/react";
import { useEffect, useRef, type RefObject } from "react";
import type { SessionRoundCard } from "../generated/server";
import { Button } from "../ui/button/Button";
import { nameOf } from "./round";

// Send with unanswered questions: a list of the gaps anchored on Send. A row jumps to its question; Send anyway sends.
export function SendPopover({
  open,
  unanswered,
  total,
  sendDisabled,
  anchorRef,
  onSendAnyway,
  onAnswer,
  onClose,
}: {
  open: boolean;
  unanswered: SessionRoundCard[];
  total: number;
  sendDisabled: boolean;
  /** The Send button the popover anchors on. */
  anchorRef: RefObject<HTMLElement | null>;
  onSendAnyway: () => void;
  onAnswer: (c: SessionRoundCard) => void;
  onClose: () => void;
}) {
  const sendAnyway = useRef<HTMLButtonElement>(null);
  // A row hands focus to the question, so the popover must not give it back to Send.
  const restoreFocus = useRef(true);
  useEffect(() => {
    if (open) restoreFocus.current = true;
  }, [open]);
  const n = unanswered.length;
  return (
    <Popover.Root
      open={open}
      onOpenChange={(e) => !e.open && onClose()}
      initialFocusEl={() => sendAnyway.current}
      finalFocusEl={() => anchorRef.current}
      restoreFocus={restoreFocus.current}
      // Send is not "outside": its own click toggles, so a second click can't reopen what the first closed.
      persistentElements={[() => anchorRef.current]}
      positioning={{
        placement: "top-end",
        gutter: 10,
        getAnchorRect: () => anchorRef.current?.getBoundingClientRect() ?? null,
      }}
      lazyMount
      unmountOnExit
    >
      <Portal>
        <Popover.Positioner>
          <Popover.Content
            role="alertdialog"
            aria-label="Unanswered questions"
            bg="surface.raised"
            borderWidth="1px"
            borderColor="border"
            borderRadius="12px"
            boxShadow="lg"
            color="text"
            w="min(360px, calc(100vw - 32px))"
            css={{ "--popover-bg": "colors.surface.raised" }}
          >
            <Popover.Arrow>
              <Popover.ArrowTip borderColor="border" />
            </Popover.Arrow>
            <Box
              px="4"
              pt="3"
              pb="2"
              display="flex"
              justifyContent="space-between"
              alignItems="baseline"
            >
              <Text fontWeight="600">
                {n} {n === 1 ? "question is" : "questions are"} unanswered
              </Text>
              <Text fontSize="xs" color="text.dim">
                {total - n} of {total} answered
              </Text>
            </Box>
            <Box
              as="ul"
              listStyleType="none"
              m="0"
              p="0"
              borderTopWidth="1px"
              borderColor="border"
            >
              {unanswered.map((c) => (
                <Box as="li" key={c.label}>
                  <chakra.button
                    type="button"
                    onClick={() => {
                      restoreFocus.current = false;
                      onAnswer(c);
                      onClose();
                    }}
                    w="100%"
                    textAlign="left"
                    display="flex"
                    gap="2"
                    px="4"
                    py="2"
                    cursor="pointer"
                    _hover={{ bg: "tone.done.bg" }}
                    _focusVisible={{
                      outline: "2px solid",
                      outlineColor: "tone.done.fg",
                      outlineOffset: "-2px",
                    }}
                  >
                    <Text
                      as="span"
                      fontWeight="700"
                      color="tone.waiting.fg"
                      flexShrink="0"
                    >
                      {nameOf(c)}
                    </Text>
                    <Text as="span" truncate>
                      {c.title}
                    </Text>
                    <Text as="span" ml="auto" color="text.dim" flexShrink="0">
                      ›
                    </Text>
                  </chakra.button>
                </Box>
              ))}
            </Box>
            <Box
              px="4"
              py="3"
              borderTopWidth="1px"
              borderColor="border"
              display="flex"
              justifyContent="flex-end"
            >
              <Button
                ref={sendAnyway}
                size="sm"
                disabled={sendDisabled}
                onClick={onSendAnyway}
              >
                Send anyway
              </Button>
            </Box>
          </Popover.Content>
        </Popover.Positioner>
      </Portal>
    </Popover.Root>
  );
}

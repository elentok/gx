import { Box, Popover, Portal, Text } from "@chakra-ui/react";
import { useRef, type RefObject } from "react";
import { Button } from "../ui/button/Button";

// Interrupt asks first: a small confirm anchored on the button, so a stray click can't end the agent's turn.
export function InterruptPopover({
  open,
  disabled,
  anchorRef,
  onConfirm,
  onClose,
}: {
  open: boolean;
  disabled: boolean;
  /** The Interrupt button the popover anchors on. */
  anchorRef: RefObject<HTMLElement | null>;
  onConfirm: () => void;
  onClose: () => void;
}) {
  const cancel = useRef<HTMLButtonElement>(null);
  return (
    <Popover.Root
      open={open}
      onOpenChange={(e) => !e.open && onClose()}
      initialFocusEl={() => cancel.current}
      finalFocusEl={() => anchorRef.current}
      // Interrupt is not "outside": its own click toggles, so a second click can't reopen what the first closed.
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
            aria-label="Interrupt the agent?"
            bg="surface.raised"
            borderWidth="1px"
            borderColor="border"
            borderRadius="12px"
            boxShadow="lg"
            color="text"
            w="min(320px, calc(100vw - 32px))"
            css={{ "--popover-bg": "colors.surface.raised" }}
          >
            <Popover.Arrow>
              <Popover.ArrowTip borderColor="border" />
            </Popover.Arrow>
            <Box px="4" pt="3" pb="2">
              <Text fontWeight="600">Interrupt the agent?</Text>
              <Text fontSize="sm" color="text.dim">
                This ends its current turn. The session stays open.
              </Text>
            </Box>
            <Box
              px="4"
              py="3"
              borderTopWidth="1px"
              borderColor="border"
              display="flex"
              justifyContent="flex-end"
              gap="2"
            >
              <Button
                ref={cancel}
                size="sm"
                emphasis="secondary"
                onClick={onClose}
              >
                Cancel
              </Button>
              <Button
                size="sm"
                tone="deadlocked"
                disabled={disabled}
                onClick={onConfirm}
              >
                Interrupt
              </Button>
            </Box>
          </Popover.Content>
        </Popover.Positioner>
      </Portal>
    </Popover.Root>
  );
}

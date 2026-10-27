import { Box, chakra, Popover, Portal, Text } from "@chakra-ui/react";
import { useRef, useState } from "react";

// A switch's look: a track with a knob, amber when on. `mini` is the one inside the Auto button.
function Track({ on, mini }: { on: boolean; mini?: boolean }) {
  const [w, h, knob] = mini ? [20, 11, 7] : [36, 20, 16];
  const pad = (h - knob) / 2;
  return (
    <Box
      aria-hidden
      w={`${w}px`}
      h={`${h}px`}
      borderRadius="full"
      bg={on ? "tone.waiting.fg" : "border"}
      position="relative"
      flexShrink={0}
    >
      <Box
        position="absolute"
        top={`${pad}px`}
        left={`${on ? w - knob - pad : pad}px`}
        w={`${knob}px`}
        h={`${knob}px`}
        borderRadius="full"
        bg={on ? "surface" : "text.dim"}
      />
    </Box>
  );
}

export type AutoSwitch = {
  label: string;
  description: string;
  on: boolean;
  disabled?: boolean;
  onFlip?: () => void;
};

// The Auto button: one mini switch per row, named as in the popover it opens.
export function AutoMenu({ switches }: { switches: AutoSwitch[] }) {
  const [open, setOpen] = useState(false);
  const anchor = useRef<HTMLButtonElement>(null);
  return (
    <>
      <chakra.button
        ref={anchor}
        aria-label={`Auto: ${switches.map((s) => `${s.label} ${s.on ? "on" : "off"}`).join(", ")}`}
        aria-expanded={open}
        aria-haspopup="dialog"
        onClick={() => setOpen(!open)}
        display="flex"
        flexDirection="column"
        justifyContent="center"
        gap="1"
        h="40px"
        px="2"
        borderRadius="8px"
        borderWidth="1px"
        borderColor={open ? "border" : "transparent"}
        bg="transparent"
        cursor="pointer"
        _hover={{ bg: "tone.muted.bg" }}
        _focusVisible={{
          outline: "2px solid",
          outlineColor: "tone.muted.fg",
          outlineOffset: "2px",
        }}
      >
        {switches.map((s) => (
          <Box
            key={s.label}
            display="flex"
            alignItems="center"
            gap="6px"
            fontSize="12px"
            lineHeight="1"
            color={s.on ? "text" : "text.dim"}
          >
            <Track on={s.on} mini />
            {s.label}
          </Box>
        ))}
      </chakra.button>
      <Popover.Root
        open={open}
        onOpenChange={(e) => !e.open && setOpen(false)}
        finalFocusEl={() => anchor.current}
        // The button's own click toggles, so a second click can't reopen what the first closed.
        persistentElements={[() => anchor.current]}
        positioning={{
          placement: "top-start",
          gutter: 10,
          getAnchorRect: () => anchor.current?.getBoundingClientRect() ?? null,
        }}
        lazyMount
        unmountOnExit
      >
        <Portal>
          <Popover.Positioner>
            <Popover.Content
              aria-label="Auto"
              bg="surface.raised"
              borderWidth="1px"
              borderColor="border"
              borderRadius="12px"
              boxShadow="lg"
              color="text"
              w="min(320px, calc(100vw - 32px))"
              overflow="hidden"
              css={{ "--popover-bg": "colors.surface.raised" }}
            >
              {switches.map((s, i) => (
                <chakra.button
                  key={s.label}
                  role="switch"
                  aria-checked={s.on}
                  aria-label={`${s.label} ${s.on ? "on" : "off"}`}
                  disabled={s.disabled}
                  onClick={s.onFlip}
                  display="flex"
                  alignItems="center"
                  gap="3"
                  w="full"
                  minH="60px"
                  px="4"
                  py="2"
                  textAlign="left"
                  bg="transparent"
                  cursor="pointer"
                  borderTopWidth={i > 0 ? "1px" : "0"}
                  borderColor="hairline"
                  _hover={{ bg: "tone.muted.bg" }}
                  _disabled={{
                    opacity: 0.5,
                    cursor: "default",
                    pointerEvents: "none",
                  }}
                >
                  <Box flex="1">
                    <Text fontWeight="500">{s.label}</Text>
                    <Text fontSize="xs" color="text.dim">
                      {s.description}
                    </Text>
                  </Box>
                  <Track on={s.on} />
                </chakra.button>
              ))}
            </Popover.Content>
          </Popover.Positioner>
        </Portal>
      </Popover.Root>
    </>
  );
}

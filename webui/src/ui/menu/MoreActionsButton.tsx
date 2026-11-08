import { chakra } from "@chakra-ui/react";
import type { ComponentProps } from "react";
import { useWide } from "../../useWide";

const dots = (
  <svg viewBox="0 0 20 20" aria-hidden width="20" height="20" fill="currentColor">
    <circle cx="4.5" cy="10" r="1.7" />
    <circle cx="10" cy="10" r="1.7" />
    <circle cx="15.5" cy="10" r="1.7" />
  </svg>
);

const hovered = { bg: "surface.raised", color: "text" } as const;
const opened = { ...hovered, boxShadow: "inset 0 0 0 1px var(--chakra-colors-border)" } as const;

/**
 * The page header's round "More actions" trigger for a `GxMenu`. `pinned` holds it in a state, so the
 * gallery can show hover, focus and open side by side.
 */
export function MoreActionsButton({ pinned, ...rest }: { pinned?: "hover" | "focus" | "open" } & ComponentProps<typeof chakra.button>) {
  const phone = !useWide();
  return (
    <chakra.button
      type="button"
      aria-label="More actions"
      cursor="pointer"
      display="grid"
      placeItems="center"
      boxSizing="border-box"
      w={phone ? "40px" : "36px"}
      h={phone ? "40px" : "36px"}
      borderRadius="full"
      color="text.dim"
      position="relative"
      transition="background .12s, color .12s"
      // A phone's hit area is 44px around the 40px circle.
      _before={phone ? { content: '""', position: "absolute", inset: "-2px" } : undefined}
      {...(pinned === "hover" && hovered)}
      {...(pinned === "focus" && { boxShadow: "0 0 0 2px {colors.surface}, 0 0 0 4px {colors.tone.running.fg}" })}
      {...(pinned === "open" && opened)}
      _hover={hovered}
      _open={opened}
      _expanded={opened}
      {...rest}
    >
      {dots}
    </chakra.button>
  );
}

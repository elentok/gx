import { chakra } from "@chakra-ui/react";
import type { ComponentProps, ReactNode } from "react";
import type { Tone } from "../tone";

export type ButtonVariant = "solid" | "tinted" | "outline" | "ghost";
export type ButtonSize = "sm" | "md";
export type ButtonEmphasis = "main" | "secondary" | "ghost";

// The old variants render as the role they become when call sites move to `emphasis`.
const VARIANT_EMPHASIS: Record<ButtonVariant, ButtonEmphasis> = {
  solid: "main",
  tinted: "main",
  outline: "secondary",
  ghost: "ghost",
};

type Props = Omit<ComponentProps<typeof chakra.button>, "color" | "variant" | "size"> & {
  /** @deprecated use `emphasis`. */
  variant?: ButtonVariant;
  emphasis?: ButtonEmphasis;
  tone?: Tone;
  size?: ButtonSize;
  children: ReactNode;
};

// Action button. Tone picks the color, emphasis picks how much of it fills the button.
export function Button({ variant, emphasis, tone = "done", size = "md", children, ...rest }: Props) {
  const fg = `tone.${tone}.fg`;
  const role = emphasis ?? VARIANT_EMPHASIS[variant ?? "tinted"];
  const style = {
    main: { bg: fg, color: "surface", borderColor: fg, _hover: { filter: "brightness(1.12)" } },
    secondary: { bg: "surface.raised", color: "text", borderColor: "border", _hover: { borderColor: "text.dim" } },
    ghost: {
      bg: "transparent",
      color: tone === "muted" ? "text.muted" : fg,
      borderColor: "transparent",
      _hover: { bg: `tone.${tone}.bg` },
    },
  }[role];
  return (
    <chakra.button
      data-variant={emphasis ? undefined : (variant ?? "tinted")}
      data-emphasis={role}
      data-size={size}
      data-tone={tone}
      display="inline-flex"
      alignItems="center"
      gap="6px"
      px={size === "sm" ? "3" : "4"}
      h={size === "sm" ? "32px" : "40px"}
      borderRadius="8px"
      borderWidth="1px"
      fontFamily="text"
      fontWeight="600"
      fontSize={size === "sm" ? "sm" : "md"}
      cursor="pointer"
      transition="background 0.1s, filter 0.1s"
      _active={{ transform: "translateY(1px)" }}
      _focusVisible={{ outline: "2px solid", outlineColor: fg, outlineOffset: "2px" }}
      _disabled={{ opacity: 0.5, cursor: "default", pointerEvents: "none" }}
      {...style}
      {...rest}
    >
      {children}
    </chakra.button>
  );
}

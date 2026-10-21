import { chakra } from "@chakra-ui/react";
import type { ComponentProps, ReactNode } from "react";
import type { Tone } from "../tone";

export type ButtonVariant = "solid" | "tinted" | "outline" | "ghost";
export type ButtonSize = "sm" | "md";

type Props = Omit<ComponentProps<typeof chakra.button>, "color" | "variant" | "size"> & {
  variant?: ButtonVariant;
  tone?: Tone;
  size?: ButtonSize;
  children: ReactNode;
};

// Action button. Tone picks the color, variant picks how much of it fills the button.
export function Button({ variant = "tinted", tone = "done", size = "sm", children, ...rest }: Props) {
  const fg = `tone.${tone}.fg`;
  const style = {
    solid: { bg: fg, color: "surface", borderColor: fg, _hover: { opacity: 0.85 } },
    tinted: { bg: `tone.${tone}.bg`, color: fg, borderColor: fg, _hover: { bg: fg, color: "surface" } },
    outline: { bg: "transparent", color: "text", borderColor: "border", _hover: { bg: "surface.raised", borderColor: fg } },
    ghost: { bg: "transparent", color: fg, borderColor: "transparent", _hover: { bg: `tone.${tone}.bg` } },
  }[variant];
  return (
    <chakra.button
      data-variant={variant}
      data-tone={tone}
      display="inline-flex"
      alignItems="center"
      px={size === "sm" ? "2" : "3"}
      h={size === "sm" ? "26px" : "34px"}
      borderRadius="sm"
      borderWidth="1px"
      fontFamily="condensed"
      fontWeight="600"
      fontSize={size === "sm" ? "sm" : "md"}
      textTransform="uppercase"
      letterSpacing="wide"
      cursor="pointer"
      transition="background 0.1s"
      _disabled={{ opacity: 0.5, cursor: "default", pointerEvents: "none" }}
      {...style}
      {...rest}
    >
      {children}
    </chakra.button>
  );
}

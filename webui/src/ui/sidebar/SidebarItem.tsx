import { chakra, Text } from "@chakra-ui/react";
import type { ReactNode } from "react";
import type { Tone } from "../tone";

interface SidebarItemProps {
  label: string;
  hint?: string;
  tone?: Tone;
  // Slot on the right for marks such as a question or live dot.
  marks?: ReactNode;
  current?: boolean;
  // Renders an anchor instead of a button.
  href?: string;
  onClick?: () => void;
}

// chakra.button's props have no href, so the anchor variant is typed loosely.
const anchorProps = (href: string): object => ({ as: "a", href });

// The current item uses the page background, so it reads as cut out of the raised sidebar.
export function SidebarItem({ label, hint, tone = "muted", marks, current, href, onClick }: SidebarItemProps) {
  return (
    <chakra.button
      {...(href ? anchorProps(href) : {})}
      data-tone={tone}
      display="flex"
      w="100%"
      alignItems="center"
      gap="2"
      py="1.5"
      px="2"
      borderRadius="sm"
      cursor="pointer"
      bg={current ? "surface" : undefined}
      _hover={{ bg: current ? "surface" : `tone.${tone}.bg` }}
      aria-current={current ? "page" : undefined}
      onClick={onClick}
      title={hint}
    >
      <Text
        flex="1"
        textAlign="start"
        fontSize="sm"
        truncate
        color={current ? `tone.${tone}.fg` : "text.muted"}
        fontWeight={current ? "600" : "400"}
      >
        {label}
      </Text>
      {marks}
    </chakra.button>
  );
}

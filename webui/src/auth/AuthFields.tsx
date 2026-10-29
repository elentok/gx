import { Box, chakra, Text } from "@chakra-ui/react";
import type { ReactNode } from "react";

export function Field({
  label,
  part,
  error,
  ...input
}: {
  label: string;
  part?: string;
  error?: boolean;
} & Omit<React.ComponentProps<typeof chakra.input>, "label">) {
  return (
    <chakra.label display="block" data-mock-part={part}>
      <Text fontSize="sm" color="text.muted" mb="1">
        {label}
      </Text>
      <chakra.input
        w="100%"
        h="44px"
        px="3"
        bg="surface"
        color="text"
        borderWidth="1px"
        borderColor={error ? "tone.paused.fg" : "border"}
        borderRadius="md"
        fontSize="base"
        _focus={{ outline: "none", borderColor: "tone.running.fg" }}
        {...input}
      />
    </chakra.label>
  );
}

export function ErrorLine({ children }: { children: ReactNode }) {
  return (
    <Box role="alert" data-mock-part="error" bg="tone.paused.bg" color="tone.paused.text" borderRadius="md" px="3" py="2" fontSize="sm">
      {children}
    </Box>
  );
}

import { Text } from "@chakra-ui/react";
import type { SessionInfo } from "../generated/server";
import { agentDetails } from "./agentDetails";

/** The header's agent part: `<Harness> <Model> · <effort>`, without the effort on a phone. */
export function AgentLine({ session, wide }: { session: SessionInfo; wide: boolean }) {
  const { model, effort } = agentDetails(session);
  if (!model) return null;
  return (
    <Text fontFamily="mono" fontSize="12px" color="text.dim">
      <Text as="span" color="text">
        {model}
      </Text>
      {wide && effort && ` · ${effort}`}
    </Text>
  );
}

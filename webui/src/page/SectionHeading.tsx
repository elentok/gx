import { Heading } from "@chakra-ui/react";
import { Badge } from "../ui/badge/Badge";
import type { Tone } from "../ui/tone";

export function SectionHeading({ id, tone, children }: { id: string; tone: Tone; children: string }) {
  return (
    <Heading as="h2" id={id} mb="2">
      <Badge tone={tone}>{children}</Badge>
    </Heading>
  );
}

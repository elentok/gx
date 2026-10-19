import { Box, Heading, List } from "@chakra-ui/react";
import type { Snapshot } from "./snapshot";

export function App({ snapshot }: { snapshot: Snapshot }) {
  return (
    <Box p="4">
      <Heading as="h1" size="lg">
        gx
      </Heading>
      <List.Root>
        {snapshot.tickets.map((t) => (
          <List.Item key={t.address}>
            {t.address} {t.title} ({t.rendered_status})
          </List.Item>
        ))}
      </List.Root>
    </Box>
  );
}

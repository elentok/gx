import { Box, ChakraProvider, Heading, Text } from "@chakra-ui/react";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "../ui/fonts.css";
import { system } from "../ui/theme";
import { galleryEntries } from "./registry";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ChakraProvider value={system}>
      <Box p="4">
        {galleryEntries.map((entry) => (
          <Box as="section" key={entry.name} mb="6">
            <Heading as="h2" size="lg" mb="3">
              {entry.name}
            </Heading>
            {entry.cases.map((c) => (
              <Box key={c.label} mb="3" maxW="360px">
                <Text fontSize="xs" color="text.dim" mb="1">
                  {c.label}
                </Text>
                {c.node}
              </Box>
            ))}
          </Box>
        ))}
      </Box>
    </ChakraProvider>
  </StrictMode>,
);

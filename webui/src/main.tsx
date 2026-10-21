import { ChakraProvider } from "@chakra-ui/react";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import { sessionSnapshotCache } from "./snapshotCache";
import "./ui/fonts.css";
import { system } from "./ui/theme";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ChakraProvider value={system}>
      <App deps={{ cache: sessionSnapshotCache }} />
    </ChakraProvider>
  </StrictMode>,
);

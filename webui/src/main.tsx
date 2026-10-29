import { ChakraProvider } from "@chakra-ui/react";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import { installClientErrorReporting } from "./clientErrors";
import { ErrorBoundary } from "./ErrorBoundary";
import { sessionSnapshotCache } from "./snapshotCache";
import "./ui/fonts.css";
import { system } from "./ui/theme";
import { AppToaster } from "./ui/toaster";

installClientErrorReporting();

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ChakraProvider value={system}>
      <ErrorBoundary>
        <App deps={{ cache: sessionSnapshotCache }} />
        <AppToaster />
      </ErrorBoundary>
    </ChakraProvider>
  </StrictMode>,
);

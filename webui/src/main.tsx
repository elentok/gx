import { ChakraProvider } from "@chakra-ui/react";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import { AuthGate } from "./auth/AuthGate";
import { installClientErrorReporting } from "./clientErrors";
import { ErrorBoundary } from "./ErrorBoundary";
import { registerWorker } from "./pwa/registerWorker";
import { sessionSnapshotCache } from "./snapshotCache";
import "./ui/fonts.css";
import { system } from "./ui/theme";
import { AppToaster } from "./ui/toaster";

installClientErrorReporting();
void registerWorker();

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ChakraProvider value={system}>
      <ErrorBoundary>
        <AuthGate>
          <App deps={{ cache: sessionSnapshotCache }} />
        </AuthGate>
        <AppToaster />
      </ErrorBoundary>
    </ChakraProvider>
  </StrictMode>,
);

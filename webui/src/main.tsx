import { ChakraProvider } from "@chakra-ui/react"
import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { App } from "./App"
import { AuthGate } from "./auth/AuthGate"
import { installClientErrorReporting } from "./clientErrors"
import { ErrorBoundary } from "./ErrorBoundary"
import { listenForNotificationTaps } from "./pwa/notificationTap"
import { registerWorker } from "./pwa/registerWorker"
import { sessionSnapshotCache } from "./snapshotCache"
import "./ui/fonts.css"
import { installButtonTabStops } from "./ui/buttonTabStops"
import { system } from "./ui/theme"
import { AppToaster } from "./ui/toaster"

installClientErrorReporting()
void registerWorker()
listenForNotificationTaps()
installButtonTabStops()

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
)

// The vitals chunk loads after first paint so it never delays the app.
const loadWebVitals = () => void import("./webVitals").then((m) => m.startWebVitals())
if ("requestIdleCallback" in window) requestIdleCallback(loadWebVitals, { timeout: 5000 })
else setTimeout(loadWebVitals, 2000)

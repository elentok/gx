import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { StrictMode, useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import { fetchSnapshot, type Snapshot } from "./snapshot";

function Root() {
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    fetchSnapshot().then(setSnapshot, (e: Error) => setError(e.message));
  }, []);
  if (error) return <p>{error}</p>;
  if (!snapshot) return null;
  return <App snapshot={snapshot} />;
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ChakraProvider value={defaultSystem}>
      <Root />
    </ChakraProvider>
  </StrictMode>,
);

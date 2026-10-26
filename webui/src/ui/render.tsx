import { ChakraProvider } from "@chakra-ui/react";
import { render } from "@testing-library/react";
import type { ReactElement } from "react";
import { system } from "./theme";
import { AppToaster } from "./toaster";

// Render tests use the real theme so token names are resolved as in production.
export function renderThemed(ui: ReactElement) {
  return render(
    <ChakraProvider value={system}>
      {ui}
      <AppToaster />
    </ChakraProvider>,
  );
}

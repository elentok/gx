import { Text } from "@chakra-ui/react";
import { Component, type ReactNode } from "react";
import { reportClientError } from "./clientErrors";
import { AuthCard } from "./auth/AuthCard";
import { Button } from "./ui/button/Button";

export function ErrorCard() {
  return (
    <AuthCard>
      <Text>Something went wrong</Text>
      <Button onClick={() => window.location.reload()}>Reload</Button>
    </AuthCard>
  );
}

export class ErrorBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  componentDidCatch(error: Error) {
    reportClientError(error);
  }

  render() {
    return this.state.failed ? <ErrorCard /> : this.props.children;
  }
}

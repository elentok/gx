import { Box, chakra, Heading, Text } from "@chakra-ui/react";
import { useEffect, useState, type FormEvent } from "react";
import { Button } from "../ui/button/Button";
import { ProgressBar } from "../ui/progress-bar/ProgressBar";
import { AuthCard } from "./AuthCard";
import { ErrorLine, Field } from "./AuthFields";
import { postAuth, type AuthState } from "./authState";
import { PlainSetupForm } from "./PlainAuthForms";
import { TrustHelp } from "./LoginPage";

const SETUP_HASH = "#/setup";
const WRONG_CODE = "That code is wrong or expired. Run gx server status for a new one.";

export interface SetupCodeCardProps {
  totpOn: boolean;
  error?: string;
  /** Echoed back into the field; the page passes the code from the QR link. */
  code?: string;
  onSubmit?: (e: FormEvent<HTMLFormElement>) => void;
}

export function SetupCodeCard({ totpOn, error, code, onSubmit }: SetupCodeCardProps) {
  const steps = totpOn ? 3 : 2;
  return (
    <AuthCard below={<TrustHelp />}>
      <chakra.form onSubmit={onSubmit} display="flex" flexDirection="column" gap="4">
        <Box data-mock-part="step-header">
          <Text fontSize="xs" color="text.dim" mb="2" textTransform="uppercase" letterSpacing="0.06em">
            Set up the web UI · step 1 of {steps}
          </Text>
          <ProgressBar value={1} max={steps} />
        </Box>
        <Heading as="h1" data-mock-part="heading" fontFamily="condensed" fontSize="xl">
          Enter the setup code
        </Heading>
        <Text data-mock-part="intro" fontSize="sm" color="text.dim">
          Run <code>gx server status</code> or <code>gx server web-url --qr</code> on the server to see it.
        </Text>
        {error && <ErrorLine>{error}</ErrorLine>}
        <Field
          part="setup-code"
          label="Setup code"
          name="code"
          placeholder="XXXX-XXXX"
          defaultValue={code}
          key={code}
          autoFocus
        />
        <Button type="submit" data-mock-part="submit">
          Continue
        </Button>
      </chakra.form>
    </AuthCard>
  );
}

// Reads the code from `#/setup?code=…` once, then drops it from the URL.
function useCodeFromHash(): string {
  const [code] = useState(() => new URLSearchParams(window.location.hash.split("?")[1] ?? "").get("code") ?? "");
  useEffect(() => {
    if (window.location.hash.includes("?")) history.replaceState(null, "", SETUP_HASH);
  }, []);
  return code;
}

export function SetupPage({ auth, onDone }: { auth: AuthState; onDone: () => void }) {
  const initial = useCodeFromHash();
  const [code, setCode] = useState(initial);
  const [error, setError] = useState("");
  const [accepted, setAccepted] = useState(false);

  const onSubmit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const typed = (e.currentTarget.elements.namedItem("code") as HTMLInputElement).value;
    setCode(typed);
    try {
      const res = await postAuth("setup-start", { code: typed });
      setError(res.refused ? WRONG_CODE : "");
      setAccepted(!res.refused);
    } catch (err) {
      setError(String(err));
    }
  };

  // Step 2 is a later ticket; the plain form stands in and receives the accepted code.
  if (accepted) {
    return (
      <AuthCard>
        <PlainSetupForm auth={auth} code={code} onDone={onDone} />
      </AuthCard>
    );
  }
  return <SetupCodeCard totpOn={auth.totp_on} code={code} error={error} onSubmit={(e) => void onSubmit(e)} />;
}

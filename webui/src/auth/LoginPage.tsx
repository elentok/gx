import { Box, chakra, Heading, Text } from "@chakra-ui/react";
import { useState, type FormEvent } from "react";
import { Button } from "../ui/button/Button";
import { AuthCard } from "./AuthCard";
import { ErrorLine, Field } from "./AuthFields";
import { postAuth, type AuthResult, type AuthState } from "./authState";

const WRONG_CREDENTIALS = "wrong-credentials";

export interface LoginCardProps {
  totpOn: boolean;
  setupCodeRequired?: boolean;
  error?: string;
  /** Echoed back into the fields; only the gallery sets it. */
  password?: string;
  secret?: string;
  onSubmit?: (e: FormEvent<HTMLFormElement>) => void;
}

// The trust help's unfolded content is a later ticket.
function TrustHelp() {
  return (
    <chakra.details data-mock-part="trust-help" mt="6" color="text.dim" fontSize="sm">
      <Box as="summary" cursor="pointer" textAlign="center">
        Browser warns about this page?
      </Box>
    </chakra.details>
  );
}

export function LoginCard({ totpOn, setupCodeRequired, error, password, secret, onSubmit }: LoginCardProps) {
  return (
    <AuthCard below={<TrustHelp />}>
      <chakra.form onSubmit={onSubmit} display="flex" flexDirection="column" gap="4">
        <Heading as="h1" data-mock-part="heading" fontFamily="condensed" fontSize="xl">
          Sign in
        </Heading>
        {error && <ErrorLine>{error}</ErrorLine>}
        <Field part="password" label="Password" name="password" type="password" defaultValue={password} autoFocus />
        {totpOn && (
          <Field
            part="code"
            label="Code from your authenticator app"
            name="totp_code"
            placeholder="123456"
            inputMode="numeric"
            autoComplete="one-time-code"
          />
        )}
        {setupCodeRequired && <Field label="Setup code" name="setup_code" placeholder="XXXX-XXXX" />}
        <Button type="submit" data-mock-part="submit" style={{ justifyContent: "center" }}>
          Sign in
        </Button>
        {secret && <Text>{secret}</Text>}
        <Text data-mock-part="reset-hint" fontSize="xs" color="text.dim">
          Forgot the password or lost the authenticator? Run <code>gx server web-reset</code> on the server.
        </Text>
      </chakra.form>
    </AuthCard>
  );
}

function refusalText(res: AuthResult): string {
  return res.reason === WRONG_CREDENTIALS ? "Wrong password or code" : (res.reason ?? "Sign in refused");
}

function value(form: HTMLFormElement, name: string): string {
  return (form.elements.namedItem(name) as HTMLInputElement | null)?.value ?? "";
}

export function LoginPage({ auth, onDone }: { auth: AuthState; onDone: () => void }) {
  const [error, setError] = useState("");
  const [secret, setSecret] = useState("");
  const onSubmit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const f = e.currentTarget;
    const body: Record<string, string> = { password: value(f, "password") };
    if (auth.totp_on) body.totp_code = value(f, "totp_code");
    if (auth.setup_code_required) body.setup_code = value(f, "setup_code");
    try {
      const res = await postAuth("login", body);
      setError(res.refused ? refusalText(res) : "");
      setSecret(res.authenticator_needed ? (res.secret ?? "") : "");
      if (!res.refused && !res.authenticator_needed) onDone();
    } catch (err) {
      setError(String(err));
    }
  };
  return (
    <LoginCard
      totpOn={auth.totp_on}
      setupCodeRequired={auth.setup_code_required}
      error={error}
      secret={secret}
      onSubmit={(e) => void onSubmit(e)}
    />
  );
}

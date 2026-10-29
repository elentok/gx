import { useCallback, useEffect, useState, type ReactNode } from "react";
import { onSignedOut } from "../apiFetch";
import { Spinner } from "../ui/spinner/Spinner";
import { AuthCard } from "./AuthCard";
import { fetchAuthState, type AuthState } from "./authState";
import { PlainLoginForm, PlainSetupForm } from "./PlainAuthForms";

/**
 * Shows setup, login or the app by the auth state. It never touches the hash, and the app mounts
 * only while signed in, so after login the router starts from the hash the person was on.
 */
export function AuthGate({
  children,
  fetchState = fetchAuthState,
}: {
  children: ReactNode;
  fetchState?: () => Promise<AuthState>;
}) {
  const [auth, setAuth] = useState<AuthState | null>(null);
  const [failed, setFailed] = useState(false);

  const refresh = useCallback(() => {
    fetchState().then(setAuth, () => setFailed(true));
  }, [fetchState]);

  useEffect(refresh, [refresh]);
  // A 401 is the signed-out signal; keep the flags so the login page still knows what to ask for.
  useEffect(
    () => onSignedOut(() => setAuth((a) => (a ? { ...a, state: "signed-out" } : a))),
    [],
  );

  if (failed) return <AuthCard>Could not reach the server.</AuthCard>;
  if (!auth) return <Spinner label="Loading" />;
  if (auth.state === "signed-in") return <>{children}</>;
  return (
    <AuthCard>
      {auth.state === "setup" ? (
        <PlainSetupForm auth={auth} onDone={refresh} />
      ) : (
        <PlainLoginForm auth={auth} onDone={refresh} />
      )}
    </AuthCard>
  );
}

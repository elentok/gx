import { useCallback, useEffect, useState, type ReactNode } from "react";
import { onSignedOut } from "../apiFetch";
import { Spinner } from "../ui/spinner/Spinner";
import { AuthCard } from "./AuthCard";
import { CertContext } from "./cert";
import { certOf, fetchAuthState, type AuthState } from "./authState";
import { LoginPage } from "./LoginPage";
import { SetupPage } from "./SetupPage";

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
  // A 401 is the signed-out signal. Re-read the state: a `gx server web-reset` leaves nothing to sign in to, so the page has to move to setup.
  useEffect(() => onSignedOut(refresh), [refresh]);

  if (failed) return <AuthCard>Could not reach the server.</AuthCard>;
  if (!auth) return <Spinner label="Loading" />;
  if (auth.state === "signed-in") return <>{children}</>;
  return (
    <CertContext.Provider value={{ cert: certOf(auth) }}>
      {auth.state === "setup" ? <SetupPage auth={auth} onDone={refresh} /> : <LoginPage auth={auth} onDone={refresh} />}
    </CertContext.Provider>
  );
}

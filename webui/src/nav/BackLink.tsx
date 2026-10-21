import { Link, useLocation, useNavigate } from "react-router";

// "default" is the key of the first entry: the page was opened by a direct link or a reload.
const reachedInApp = (key: string) => key !== "default";

// Goes back in router history when the page was reached inside the app, else to the fixed parent.
export function BackLink({ to, label }: { to: string; label: string }) {
  const navigate = useNavigate();
  const inApp = reachedInApp(useLocation().key);
  return (
    <Link
      to={to}
      onClick={
        inApp
          ? (e) => {
              e.preventDefault();
              navigate(-1);
            }
          : undefined
      }
    >
      {inApp ? "← Back" : label}
    </Link>
  );
}

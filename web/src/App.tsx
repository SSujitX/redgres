import { useEffect, useState } from "react";
import { errorMessage, fetchSession, logout } from "./api/auth";
import AppShell from "./components/shell/AppShell";
import LoginPage from "./features/auth/LoginPage";

type View =
  | { kind: "loading" }
  | { kind: "login" }
  | { kind: "shell"; username: string; csrf: string };

export default function App() {
  const [view, setView] = useState<View>({ kind: "loading" });
  const [loggingOut, setLoggingOut] = useState(false);
  const [sessionError, setSessionError] = useState("");

  useEffect(() => {
    const controller = new AbortController();
    fetchSession()
      .then((result) => {
        if (controller.signal.aborted) {
          return;
        }
        if (result.status === 200 && result.body.owner?.username && result.body.csrf_token) {
          setView({
            kind: "shell",
            username: result.body.owner.username,
            csrf: result.body.csrf_token,
          });
          return;
        }
        setView({ kind: "login" });
      })
      .catch(() => {
        if (!controller.signal.aborted) {
          setView({ kind: "login" });
        }
      });
    return () => controller.abort();
  }, []);

  async function handleLogout() {
    if (view.kind !== "shell") {
      return;
    }
    setLoggingOut(true);
    setSessionError("");
    try {
      const result = await logout(view.csrf);
      if (result.status === 200) {
        setView({ kind: "login" });
        return;
      }
      if (result.status === 401) {
        setView({ kind: "login" });
        return;
      }
      setSessionError(errorMessage(result.body, "Sign-out failed"));
    } finally {
      setLoggingOut(false);
    }
  }

  if (view.kind === "loading") {
    return (
      <div className="boot">
        <p>Checking session.</p>
      </div>
    );
  }

  if (view.kind === "login") {
    return (
      <LoginPage
        onSuccess={(username, csrf) => {
          setSessionError("");
          setView({ kind: "shell", username, csrf });
        }}
      />
    );
  }

  return (
    <>
      {sessionError ? (
        <p className="form-error shell-banner" role="alert">
          {sessionError}
        </p>
      ) : null}
      <AppShell username={view.username} onLogout={() => void handleLogout()} loggingOut={loggingOut} />
    </>
  );
}

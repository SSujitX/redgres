import { useEffect, useState } from "react";
import { errorMessage, fetchSession, logout, parseToolLinks, type ToolLinks } from "./api/auth";
import AppShell from "./components/shell/AppShell";
import LoginPage from "./features/auth/LoginPage";

type View =
  | { kind: "loading" }
  | { kind: "login" }
  | { kind: "shell"; username: string; csrf: string; toolLinks: ToolLinks; version?: string };

function shellFromSession(body: {
  owner?: { username?: string };
  csrf_token?: string;
  tool_links?: unknown;
  version?: unknown;
}): View | null {
  if (!body.owner?.username || !body.csrf_token) {
    return null;
  }
  return {
    kind: "shell",
    username: body.owner.username,
    csrf: body.csrf_token,
    toolLinks: parseToolLinks(body.tool_links),
    version: typeof body.version === "string" && body.version !== "" ? body.version : undefined,
  };
}

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
        if (result.status === 200) {
          const next = shellFromSession(result.body);
          if (next) {
            setView(next);
            return;
          }
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
        onSuccess={() => {
          setSessionError("");
          setView({ kind: "loading" });
          void fetchSession()
            .then((result) => {
              if (result.status === 200) {
                const next = shellFromSession(result.body);
                if (next) {
                  setView(next);
                  return;
                }
              }
              setView({ kind: "login" });
            })
            .catch(() => {
              setView({ kind: "login" });
            });
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
      <AppShell
        username={view.username}
        csrf={view.csrf}
        toolLinks={view.toolLinks}
        version={view.version}
        onLogout={() => void handleLogout()}
        onPasswordChanged={() => setView({ kind: "login" })}
        loggingOut={loggingOut}
      />
    </>
  );
}

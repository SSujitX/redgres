import { useEffect, useState } from "react";

type HealthState = "loading" | "ok" | "unavailable";

export default function App() {
  const [state, setState] = useState<HealthState>("loading");

  useEffect(() => {
    const controller = new AbortController();
    fetch("/api/v1/healthz", { signal: controller.signal })
      .then(async (response) => {
        if (!response.ok) {
          setState("unavailable");
          return;
        }
        const body = (await response.json()) as { status?: string };
        setState(body.status === "ok" ? "ok" : "unavailable");
      })
      .catch(() => {
        if (!controller.signal.aborted) {
          setState("unavailable");
        }
      });
    return () => controller.abort();
  }, []);

  return (
    <main>
      <h1>Redgres</h1>
      {state === "loading" ? <p>Checking control-plane storage.</p> : null}
      {state === "ok" ? <p>Control-plane storage is reachable.</p> : null}
      {state === "unavailable" ? <p>Control-plane storage is unavailable.</p> : null}
    </main>
  );
}

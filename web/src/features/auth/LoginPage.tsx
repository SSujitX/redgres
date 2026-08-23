import { FormEvent, useEffect, useId, useState } from "react";
import { errorMessage, login } from "../../api/auth";

type LoginPageProps = {
  onSuccess: (username: string, csrf: string) => void;
};

export default function LoginPage({ onSuccess }: LoginPageProps) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState("");
  const [retryAfter, setRetryAfter] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const errorId = useId();
  const retryId = useId();

  useEffect(() => {
    return () => {
      setPassword("");
    };
  }, []);

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    setError("");
    setRetryAfter(null);
    setSubmitting(true);
    try {
      const result = await login(username, password);
      if (result.status === 200 && result.body.owner?.username && result.body.csrf_token) {
        setPassword("");
        onSuccess(result.body.owner.username, result.body.csrf_token);
        return;
      }
      setError(errorMessage(result.body, "Invalid username or password."));
      if (result.status === 429) {
        setRetryAfter(result.retryAfter);
      }
    } catch {
      setError("Sign-in is unavailable. Try again.");
    } finally {
      setSubmitting(false);
    }
  }

  const retry = formatRetryAfter(retryAfter);
  const describedBy = [error ? errorId : null, retry ? retryId : null].filter(Boolean).join(" ") || undefined;

  return (
    <div className="login-page">
      <section className="login-identity" aria-label="Redgres">
        <div className="service-rail" aria-hidden="true">
          <span className="service-rail-postgres" />
          <span className="service-rail-redis" />
        </div>
        <div>
          <h1>Redgres</h1>
          <p className="login-tagline">One secure control plane for PostgreSQL and Redis.</p>
        </div>
      </section>
      <section className="login-panel">
        <form className="login-form" onSubmit={handleSubmit} aria-busy={submitting}>
          <label htmlFor="username">Username</label>
          <input
            id="username"
            name="username"
            autoComplete="username"
            value={username}
            onChange={(event) => setUsername(event.target.value)}
            required
            aria-invalid={error ? true : undefined}
            aria-describedby={describedBy}
          />
          <label htmlFor="password">Password</label>
          <div className="password-row">
            <input
              id="password"
              name="password"
              type={showPassword ? "text" : "password"}
              autoComplete="current-password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              required
              aria-invalid={error ? true : undefined}
              aria-describedby={describedBy}
            />
            <button
              type="button"
              className="text-button"
              aria-pressed={showPassword}
              onClick={() => setShowPassword((value) => !value)}
            >
              {showPassword ? "Hide password" : "Show password"}
            </button>
          </div>
          {error ? (
            <p id={errorId} className="form-error" role="alert">
              {error}
            </p>
          ) : null}
          {retry ? (
            <p id={retryId} className="form-warning" role="status">
              {retry.text}
            </p>
          ) : null}
          <button type="submit" className="primary-button" disabled={submitting || Boolean(retry?.disable)}>
            Log in
          </button>
        </form>
      </section>
    </div>
  );
}

export function formatRetryAfter(value: string | null): { text: string; disable: boolean } | null {
  if (!value) {
    return null;
  }
  const trimmed = value.trim();
  if (/^\d+$/.test(trimmed)) {
    return { text: `Try again in ${trimmed} seconds.`, disable: true };
  }
  const when = Date.parse(trimmed);
  if (!Number.isNaN(when)) {
    const seconds = Math.max(1, Math.ceil((when - Date.now()) / 1000));
    return { text: `Try again in ${seconds} seconds.`, disable: true };
  }
  return { text: "Try again later.", disable: false };
}

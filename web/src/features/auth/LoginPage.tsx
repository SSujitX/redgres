import { FormEvent, useEffect, useId, useRef, useState } from "react";
import { errorMessage, login } from "../../api/auth";
import BrandLogo from "../../components/BrandLogo";
import ThemeToggle from "../../components/ThemeToggle";

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
  const pageRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    document.title = "Sign in — Redgres";
    return () => {
      setPassword("");
    };
  }, []);
  useEffect(() => {
    const page = pageRef.current;
    const canvas = canvasRef.current;
    if (!page || !canvas) {
      return;
    }
    const canvasNode = canvas;
    const ctx = canvasNode.getContext("2d");
    if (!ctx) {
      return;
    }
    const ctxNode = ctx;
    const reduceMotion =
      typeof window.matchMedia === "function" &&
      window.matchMedia("(prefers-reduced-motion: reduce)").matches;

    let width = 0;
    let height = 0;
    let dpr = 1;
    type Cell = { x: number; y: number };
    let cells: Cell[] = [];
    let mouseX = -9999;
    let mouseY = -9999;
    let active = false;
    let raf = 0;

    function inkRgb(): [number, number, number] {
      const hex = (getComputedStyle(document.documentElement).getPropertyValue("--ink") || "#0f172a").trim().replace("#", "");
      return [
        parseInt(hex.slice(0, 2), 16) || 15,
        parseInt(hex.slice(2, 4), 16) || 23,
        parseInt(hex.slice(4, 6), 16) || 42,
      ];
    }

    function fillRound(c: CanvasRenderingContext2D, x: number, y: number, w: number, h: number, r: number) {
      c.beginPath();
      c.moveTo(x + r, y);
      c.arcTo(x + w, y, x + w, y + h, r);
      c.arcTo(x + w, y + h, x, y + h, r);
      c.arcTo(x, y + h, x, y, r);
      c.arcTo(x, y, x + w, y, r);
      c.closePath();
      c.fill();
    }

    function resize() {
      dpr = Math.min(window.devicePixelRatio || 1, 2);
      width = window.innerWidth;
      height = window.innerHeight;
      canvasNode.width = Math.round(width * dpr);
      canvasNode.height = Math.round(height * dpr);
      ctxNode.setTransform(dpr, 0, 0, dpr, 0, 0);
      const spacing = 38;
      const margin = 40;
      cells = [];
      for (let x = margin; x <= width + margin; x += spacing) {
        for (let y = margin; y <= height + margin; y += spacing) {
          cells.push({ x, y });
        }
      }
    }

    function draw(animate: boolean) {
      ctxNode.clearRect(0, 0, width, height);
      const [r, g, b] = inkRgb();
      const base = 5;
      const radius = 180;
      const ox = active ? -(mouseX - width / 2) * 0.012 : 0;
      const oy = active ? -(mouseY - height / 2) * 0.012 : 0;
      for (const cell of cells) {
        let size = base;
        let alpha = 0.12;
        if (animate && active) {
          const dx = cell.x - mouseX;
          const dy = cell.y - mouseY;
          const dist = Math.sqrt(dx * dx + dy * dy);
          if (dist < radius) {
            const t = 1 - dist / radius;
            const e = t * t;
            size = base * (1 + e * 1.9);
            alpha = 0.12 + e * 0.2;
          }
        }
        ctxNode.fillStyle = "rgba(" + r + ", " + g + ", " + b + ", " + alpha + ")";
        fillRound(ctxNode, cell.x + ox - size / 2, cell.y + oy - size / 2, size, size, Math.max(1, size / 5));
      }
    }

    function loop() {
      draw(true);
      if (active) {
        raf = requestAnimationFrame(loop);
      }
    }

    function onEnter(event: PointerEvent) {
      active = true;
      mouseX = event.clientX;
      mouseY = event.clientY;
      if (reduceMotion) {
        return;
      }
      if (!raf) {
        raf = requestAnimationFrame(loop);
      }
    }

    function onMove(event: PointerEvent) {
      mouseX = event.clientX;
      mouseY = event.clientY;
    }

    function onLeave() {
      active = false;
      if (raf) {
        cancelAnimationFrame(raf);
        raf = 0;
      }
      draw(false);
    }

    resize();
    draw(false);
    window.addEventListener("resize", resize);
    page.addEventListener("pointerenter", onEnter);
    page.addEventListener("pointermove", onMove, { passive: true });
    page.addEventListener("pointerleave", onLeave);
    return () => {
      window.removeEventListener("resize", resize);
      page.removeEventListener("pointerenter", onEnter);
      page.removeEventListener("pointermove", onMove);
      page.removeEventListener("pointerleave", onLeave);
      if (raf) {
        cancelAnimationFrame(raf);
      }
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
    <div className="login-page" ref={pageRef}>
      <canvas className="term-canvas" ref={canvasRef} aria-hidden="true" />
      <ThemeToggle className="login-theme-toggle" />
      <main className="login-wrap">
        <header className="login-brand">
          <a className="brand-logo-login-link" href="/" aria-label="Redgres home">
            <BrandLogo className="brand-logo-login" />
          </a>
          <h1 className="visually-hidden">Redgres</h1>
          <p className="login-tagline">One secure control plane for PostgreSQL and Redis.</p>
          <ul className="login-engines" aria-hidden="true">
            <li>
              <span className="engine-dot engine-dot-postgres" />
              PostgreSQL
            </li>
            <li>
              <span className="engine-dot engine-dot-redis" />
              Redis
            </li>
          </ul>
        </header>
        <section className="login-panel" aria-labelledby="login-heading">
          <div className="term-titlebar" aria-hidden="true">
            <span className="term-dots">
              <span className="term-dot term-dot-red" />
              <span className="term-dot term-dot-yellow" />
              <span className="term-dot term-dot-green" />
            </span>
            <span className="term-title">redgres — control plane</span>
          </div>
          <div className="login-panel-body">
            <h2 id="login-heading" className="login-panel-heading">
              Sign in
            </h2>
            <p className="login-panel-sub">Sign in to your Redgres console.</p>
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
          </div>
        </section>
        <p className="login-foot">Self-hosted control plane</p>
      </main>
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

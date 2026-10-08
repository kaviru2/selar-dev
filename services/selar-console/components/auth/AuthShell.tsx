// AuthShell.tsx — shared frame for /login and /register.
// Left: the form. Right: a friendly brand panel (illustration + the SELAR
// principle). Stacks on small screens with the form first.

import Link from "next/link";
import type { ReactNode } from "react";
import { Icon, type IconName } from "@/components/ui/Icon";
import { Illustration, type IllustrationName } from "@/components/ui/Illustration";
import { ThemeToggle } from "@/components/ui/ThemeToggle";
import { Wordmark } from "@/components/ui/Wordmark";

const PRINCIPLE: { icon: IconName; text: string }[] = [
  { icon: "sparkles", text: "AI suggests a possible link" },
  { icon: "doc", text: "Both source passages are shown" },
  { icon: "pen", text: "You explain and decide" },
];

export function AuthShell({
  title,
  lede,
  illustration = "hero",
  aside,
  children,
}: {
  title: string;
  lede: ReactNode;
  illustration?: IllustrationName;
  aside: { heading: string; body: ReactNode };
  children: ReactNode;
}) {
  return (
    <div className="auth">
      <a className="ui-skip" href="#auth-form">Skip to form</a>
      <header className="auth-top">
        <Wordmark href="/" subtitle />
        <ThemeToggle />
      </header>

      <main className="auth-main">
        <section className="auth-card" aria-labelledby="auth-title">
          <h1 id="auth-title">{title}</h1>
          <p className="auth-lede">{lede}</p>
          <div id="auth-form">{children}</div>
        </section>

        <aside className="auth-aside" aria-label="About SELAR">
          <div className="auth-aside-art">
            <Illustration name={illustration} width={420} />
          </div>
          <h2>{aside.heading}</h2>
          <div className="auth-aside-body">{aside.body}</div>
          <ul className="auth-principle">
            {PRINCIPLE.map((p) => (
              <li key={p.text}>
                <span aria-hidden="true"><Icon name={p.icon} size={14} /></span>
                {p.text}
              </li>
            ))}
          </ul>
        </aside>
      </main>

      <footer className="auth-foot">
        <span>A UCSC research prototype. Use non-sensitive readings while testing.</span>
        <Link href="/about">About SELAR</Link>
      </footer>
    </div>
  );
}

export function AuthField({
  id,
  label,
  hint,
  ...input
}: { id: string; label: string; hint?: ReactNode } & React.InputHTMLAttributes<HTMLInputElement>) {
  const hintId = hint ? `${id}-hint` : undefined;
  return (
    <div className="auth-field">
      <label htmlFor={id}>{label}</label>
      <input id={id} name={id} aria-describedby={hintId} {...input} />
      {hint && <p id={hintId} className="auth-hint">{hint}</p>}
    </div>
  );
}

export function AuthError({ message }: { message: string }) {
  if (!message) return null;
  return (
    <div className="auth-error" role="alert">
      <Icon name="info" size={15} />
      <span>{message}</span>
    </div>
  );
}

// Card.tsx — shared surface, Badge pill and PageHeader.

import type { HTMLAttributes, ReactNode } from "react";

type Tint = "none" | "green" | "sky" | "warm";

interface CardProps extends HTMLAttributes<HTMLElement> {
  tint?: Tint;
  interactive?: boolean;
  as?: "div" | "section" | "article" | "li";
}

export function Card({ tint = "none", interactive, as: Tag = "div", className, ...rest }: CardProps) {
  const cls = [
    "ui-card",
    tint !== "none" ? `ui-card--tint-${tint}` : "",
    interactive ? "ui-card--interactive" : "",
    className ?? "",
  ]
    .filter(Boolean)
    .join(" ");
  return <Tag className={cls} {...rest} />;
}

type BadgeTone = "neutral" | "green" | "warm" | "amber" | "sky";

export function Badge({ tone = "neutral", dot, children, className }: { tone?: BadgeTone; dot?: boolean; children: ReactNode; className?: string }) {
  const cls = ["ui-badge", tone !== "neutral" ? `ui-badge--${tone}` : "", className ?? ""].filter(Boolean).join(" ");
  return (
    <span className={cls}>
      {dot && <span className="ui-dot" aria-hidden="true" />}
      {children}
    </span>
  );
}

export function PageHeader({ eyebrow, title, description, actions }: { eyebrow?: string; title: string; description?: ReactNode; actions?: ReactNode }) {
  return (
    <header className="ui-page-head">
      <div>
        {eyebrow && <span className="ui-eyebrow">{eyebrow}</span>}
        <h1>{title}</h1>
        {description && <p>{description}</p>}
      </div>
      {actions && <div className="ui-page-actions">{actions}</div>}
    </header>
  );
}

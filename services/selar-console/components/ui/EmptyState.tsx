// EmptyState.tsx — friendly empty/zero state with a spot illustration.

import type { ReactNode } from "react";
import { Illustration, type IllustrationName } from "./Illustration";

interface EmptyStateProps {
  illustration?: IllustrationName;
  title: string;
  children?: ReactNode;
  actions?: ReactNode;
  compact?: boolean;
  headingLevel?: 2 | 3;
}

export function EmptyState({ illustration = "reading", title, children, actions, compact, headingLevel = 2 }: EmptyStateProps) {
  const Heading = headingLevel === 2 ? "h2" : "h3";
  return (
    <div className={`ui-empty${compact ? " ui-empty--compact" : ""}`}>
      <Illustration name={illustration} width={compact ? 120 : 180} />
      <Heading>{title}</Heading>
      {children && <p>{children}</p>}
      {actions && <div className="ui-empty-actions">{actions}</div>}
    </div>
  );
}

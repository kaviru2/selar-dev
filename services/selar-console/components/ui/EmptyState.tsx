// EmptyState.tsx — friendly empty/zero state.
// Shows Linny (the SELAR mascot) in a pose, or a spot illustration.

import type { ReactNode } from "react";
import { Linny, type LinnyPose } from "@/components/brand";
import { Illustration, type IllustrationName } from "./Illustration";

interface EmptyStateProps {
  /** Mascot pose (preferred). */
  mascot?: LinnyPose;
  /** Spot illustration, used when no mascot is given. */
  illustration?: IllustrationName;
  title: string;
  children?: ReactNode;
  actions?: ReactNode;
  compact?: boolean;
  headingLevel?: 2 | 3;
}

export function EmptyState({ mascot, illustration = "reading", title, children, actions, compact, headingLevel = 2 }: EmptyStateProps) {
  const Heading = headingLevel === 2 ? "h2" : "h3";
  return (
    <div className={`ui-empty${compact ? " ui-empty--compact" : ""}`}>
      {mascot ? (
        <span className="ui-empty-mascot">
          <Linny pose={mascot} size={compact ? 96 : 150} title="" />
        </span>
      ) : (
        <Illustration name={illustration} width={compact ? 120 : 180} />
      )}
      <Heading>{title}</Heading>
      {children && <p>{children}</p>}
      {actions && <div className="ui-empty-actions">{actions}</div>}
    </div>
  );
}

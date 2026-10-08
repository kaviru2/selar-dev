// Button.tsx — shared button and button-styled link.
// Variants: primary (deep green), secondary (outline), warm (rust), ghost.

import Link from "next/link";
import type { ButtonHTMLAttributes, ComponentProps, ReactNode } from "react";

export type ButtonVariant = "primary" | "secondary" | "warm" | "ghost";
export type ButtonSize = "sm" | "md" | "lg";

interface StyleProps {
  variant?: ButtonVariant;
  size?: ButtonSize;
  block?: boolean;
  className?: string;
}

export function buttonClass({ variant = "secondary", size = "md", block, className }: StyleProps = {}) {
  return [
    "ui-btn",
    variant !== "secondary" ? `ui-btn--${variant}` : "",
    size !== "md" ? `ui-btn--${size}` : "",
    block ? "ui-btn--block" : "",
    className ?? "",
  ]
    .filter(Boolean)
    .join(" ");
}

type ButtonProps = StyleProps & ButtonHTMLAttributes<HTMLButtonElement> & { children: ReactNode };

export function Button({ variant, size, block, className, type = "button", ...rest }: ButtonProps) {
  return <button type={type} className={buttonClass({ variant, size, block, className })} {...rest} />;
}

type ButtonLinkProps = StyleProps & ComponentProps<typeof Link> & { children: ReactNode };

export function ButtonLink({ variant, size, block, className, ...rest }: ButtonLinkProps) {
  return <Link className={buttonClass({ variant, size, block, className })} {...rest} />;
}

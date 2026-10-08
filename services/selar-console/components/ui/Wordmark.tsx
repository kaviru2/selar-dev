// Wordmark.tsx — SELAR brand lockup used across the console and landing page.
// The mark is LogoMark (Linny the linnet holding a page) from components/brand;
// see brand/DESIGN.md for usage rules. Every page renders the brand through
// this one component. Both the light and on-dark marks are rendered and CSS
// shows the one that matches [data-theme] (see .ui-mark-* in app/globals.css),
// so there is no flash when the theme script runs before paint.

import Link from "next/link";
import { LogoMark } from "@/components/brand";

interface WordmarkProps {
  href?: string;
  size?: number;
  subtitle?: boolean;
}

export function WordmarkMark({ size = 26 }: { size?: number }) {
  // Linny the linnet holding a page: bringing back something you read earlier.
  return (
    <>
      <LogoMark size={size} title="" focusable="false" className="ui-mark-light" />
      <LogoMark size={size} title="" focusable="false" variant="dark" className="ui-mark-dark" />
    </>
  );
}

export function Wordmark({ href = "/", size = 26, subtitle }: WordmarkProps) {
  const body = (
    <>
      <WordmarkMark size={size} />
      <span>
        <span className="ui-wordmark-text" style={{ fontSize: size * 0.7 }}>SELAR</span>
        {subtitle && <span className="ui-wordmark-sub" style={{ display: "block", marginTop: 3 }}>Semantic Linking for Active Retention</span>}
      </span>
    </>
  );
  return href ? (
    <Link href={href} className="ui-wordmark" aria-label="SELAR home">
      {body}
    </Link>
  ) : (
    <span className="ui-wordmark">{body}</span>
  );
}

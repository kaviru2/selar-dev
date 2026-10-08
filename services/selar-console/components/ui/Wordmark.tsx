// Wordmark.tsx — placeholder SELAR wordmark.
// TEMPORARY: replace the mark with components/brand/Logo once the brand PR
// lands. Every page renders the brand through this one component, so the
// swap is a single-file change.

import Link from "next/link";

interface WordmarkProps {
  href?: string;
  size?: number;
  subtitle?: boolean;
}

export function WordmarkMark({ size = 26 }: { size?: number }) {
  // Two pages joined by a thread: "this reading links to that one".
  return (
    <svg width={size} height={size} viewBox="0 0 32 32" aria-hidden="true" focusable="false">
      <rect x="2" y="5" width="13" height="18" rx="3" fill="var(--selar-green-800)" />
      <rect x="17" y="9" width="13" height="18" rx="3" fill="var(--selar-rust)" />
      <path d="M9 14 C 13 20, 19 8, 23 18" fill="none" stroke="#fff" strokeWidth="2.2" strokeLinecap="round" />
      <circle cx="9" cy="14" r="2.2" fill="#fff" />
      <circle cx="23" cy="18" r="2.2" fill="#fff" />
    </svg>
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

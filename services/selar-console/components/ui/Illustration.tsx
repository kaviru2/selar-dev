// Illustration.tsx — SELAR spot illustrations.
// Flat, friendly SVGs drawn for SELAR in the deck palette (no third-party
// artwork). Gentle motion via .float/.pulse/.draw classes in app/ui.css,
// all disabled under prefers-reduced-motion.
// Illustrations are decorative by default (aria-hidden). Pass `title` to
// expose one to assistive technology.

import type { ReactNode } from "react";

export type IllustrationName =
  | "reading"
  | "connect"
  | "explain"
  | "compare"
  | "decide"
  | "library"
  | "graph"
  | "lost"
  | "hero";

const G = "var(--selar-green-800)";
const GM = "var(--selar-green-500)";
const GL = "var(--selar-green-100)";
const R = "var(--selar-rust)";
const RT = "var(--selar-rust-tint)";
const A = "var(--selar-amber)";
const S = "var(--selar-sky)";
const INK = "var(--selar-ink)";
const W = "#fff";
const LINE = "#cbd5e1";

function Page({ x, y, w = 70, h = 90, fill = W, accent = GM, r = 8, lines = 4 }: { x: number; y: number; w?: number; h?: number; fill?: string; accent?: string; r?: number; lines?: number }) {
  const rows = Array.from({ length: lines }, (_, i) => (
    <rect key={i} x={x + 12} y={y + 28 + i * 13} width={i === lines - 1 ? w * 0.4 : w - 24} height="5" rx="2.5" fill={LINE} />
  ));
  return (
    <g>
      <rect x={x} y={y} width={w} height={h} rx={r} fill={fill} stroke={INK} strokeWidth="2.5" />
      <rect x={x + 12} y={y + 12} width={w * 0.5} height="7" rx="3.5" fill={accent} />
      {rows}
    </g>
  );
}

const ART: Record<IllustrationName, { vb: string; body: ReactNode }> = {
  reading: {
    vb: "0 0 240 160",
    body: (
      <>
        <ellipse cx="120" cy="140" rx="96" ry="10" fill={GL} />
        <path d="M30 60 Q 75 44 120 62 L 120 132 Q 75 116 30 130 Z" fill={W} stroke={INK} strokeWidth="2.5" strokeLinejoin="round" />
        <path d="M210 60 Q 165 44 120 62 L 120 132 Q 165 116 210 130 Z" fill={W} stroke={INK} strokeWidth="2.5" strokeLinejoin="round" />
        <path d="M44 74 Q 78 64 108 76 M44 88 Q 78 78 108 90 M44 102 Q 70 94 92 102" stroke={LINE} strokeWidth="4" fill="none" strokeLinecap="round" />
        <path d="M132 76 Q 162 64 196 74 M132 90 Q 162 78 196 88" stroke={LINE} strokeWidth="4" fill="none" strokeLinecap="round" />
        <path d="M132 104 Q 150 96 170 100" stroke={A} strokeWidth="7" fill="none" strokeLinecap="round" opacity="0.55" />
        <g className="float">
          <circle cx="182" cy="30" r="13" fill={RT} stroke={R} strokeWidth="2.5" />
          <path d="M176 30 l4 4 l8 -8" stroke={R} strokeWidth="2.5" fill="none" strokeLinecap="round" strokeLinejoin="round" />
        </g>
        <g className="float-2">
          <circle cx="54" cy="34" r="6" fill={A} />
          <circle cx="36" cy="46" r="3.5" fill={GM} />
        </g>
      </>
    ),
  },
  connect: {
    vb: "0 0 240 160",
    body: (
      <>
        <ellipse cx="120" cy="146" rx="100" ry="8" fill={GL} />
        <Page x={18} y={30} accent={G} />
        <Page x={152} y={40} accent={R} />
        <path className="draw" d="M70 70 C 110 20, 130 120, 170 82" stroke={G} strokeWidth="3" fill="none" strokeLinecap="round" />
        <circle cx="70" cy="70" r="6" fill={G} />
        <circle cx="170" cy="82" r="6" fill={R} />
        <g className="pulse">
          <circle cx="120" cy="70" r="15" fill={W} stroke={INK} strokeWidth="2.5" />
          <path d="M114 70 h12 M120 64 v12" stroke={G} strokeWidth="3" strokeLinecap="round" />
        </g>
      </>
    ),
  },
  explain: {
    vb: "0 0 240 160",
    body: (
      <>
        <ellipse cx="120" cy="146" rx="96" ry="8" fill={GL} />
        <path d="M40 30 h130 a14 14 0 0 1 14 14 v50 a14 14 0 0 1 -14 14 h-80 l-22 20 v-20 h-28 a14 14 0 0 1 -14 -14 v-50 a14 14 0 0 1 14 -14 z" fill={S} stroke={INK} strokeWidth="2.5" strokeLinejoin="round" />
        <path d="M50 56 h96 M50 72 h110 M50 88 h64" stroke={INK} strokeWidth="4" strokeLinecap="round" opacity="0.25" />
        <g className="float">
          <path d="M176 128 l34 -60 l12 7 l-34 60 l-14 6 z" fill={A} stroke={INK} strokeWidth="2.5" strokeLinejoin="round" />
          <path d="M210 68 l12 7" stroke={INK} strokeWidth="2.5" />
          <path d="M176 128 l2 -8 l8 5 z" fill={INK} />
        </g>
      </>
    ),
  },
  compare: {
    vb: "0 0 240 160",
    body: (
      <>
        <ellipse cx="120" cy="146" rx="100" ry="8" fill={GL} />
        <path d="M120 26 v108 M88 134 h64" stroke={INK} strokeWidth="3" strokeLinecap="round" />
        <path d="M44 48 h152" stroke={INK} strokeWidth="3" strokeLinecap="round" />
        <circle cx="120" cy="26" r="6" fill={A} stroke={INK} strokeWidth="2.5" />
        <g className="float">
          <path d="M44 48 l-22 46 h44 z" fill="none" stroke={INK} strokeWidth="2" opacity="0.4" />
          <rect x="18" y="94" width="52" height="32" rx="6" fill={GL} stroke={G} strokeWidth="2.5" />
          <path d="M28 106 h32 M28 116 h20" stroke={G} strokeWidth="3" strokeLinecap="round" />
        </g>
        <g className="float-2">
          <path d="M196 48 l-22 46 h44 z" fill="none" stroke={INK} strokeWidth="2" opacity="0.4" />
          <rect x="170" y="94" width="52" height="32" rx="6" fill={RT} stroke={R} strokeWidth="2.5" />
          <path d="M180 106 h32 M180 116 h20" stroke={R} strokeWidth="3" strokeLinecap="round" />
        </g>
      </>
    ),
  },
  decide: {
    vb: "0 0 240 160",
    body: (
      <>
        <ellipse cx="120" cy="146" rx="100" ry="8" fill={GL} />
        <g>
          <rect x="22" y="40" width="58" height="70" rx="12" fill={GL} stroke={G} strokeWidth="2.5" />
          <path d="M38 76 l9 9 l17 -19" stroke={G} strokeWidth="4" fill="none" strokeLinecap="round" strokeLinejoin="round" />
        </g>
        <g className="float">
          <rect x="91" y="28" width="58" height="70" rx="12" fill={S} stroke={INK} strokeWidth="2.5" />
          <path d="M106 70 l18 -18 l7 7 l-18 18 h-7 z" fill={A} stroke={INK} strokeWidth="2" strokeLinejoin="round" />
        </g>
        <g>
          <rect x="160" y="40" width="58" height="70" rx="12" fill={RT} stroke={R} strokeWidth="2.5" />
          <path d="M178 64 l22 22 M200 64 l-22 22" stroke={R} strokeWidth="4" strokeLinecap="round" />
        </g>
      </>
    ),
  },
  library: {
    vb: "0 0 240 160",
    body: (
      <>
        <ellipse cx="120" cy="146" rx="100" ry="8" fill={GL} />
        <rect x="40" y="40" width="26" height="100" rx="4" fill={G} stroke={INK} strokeWidth="2.5" />
        <rect x="70" y="56" width="22" height="84" rx="4" fill={R} stroke={INK} strokeWidth="2.5" />
        <rect x="96" y="30" width="28" height="110" rx="4" fill={S} stroke={INK} strokeWidth="2.5" />
        <rect x="128" y="62" width="24" height="78" rx="4" fill={A} stroke={INK} strokeWidth="2.5" transform="rotate(-12 140 101)" />
        <path d="M46 56 h14 M46 64 h14 M102 46 h16 M102 54 h16" stroke={W} strokeWidth="3" strokeLinecap="round" />
        <g className="pulse">
          <circle cx="188" cy="62" r="22" fill={W} stroke={G} strokeWidth="2.5" strokeDasharray="5 5" />
          <path d="M188 52 v20 M178 62 h20" stroke={G} strokeWidth="3.5" strokeLinecap="round" />
        </g>
      </>
    ),
  },
  graph: {
    vb: "0 0 240 160",
    body: (
      <>
        <ellipse cx="120" cy="146" rx="100" ry="8" fill={GL} />
        <path d="M60 50 L120 84 L184 46 M120 84 L90 124 M120 84 L170 118" fill="none" stroke={INK} strokeWidth="2.5" opacity="0.5" />
        <path className="draw" d="M60 50 C 80 100, 150 120, 170 118" stroke={G} strokeWidth="2.5" fill="none" />
        <circle cx="60" cy="50" r="14" fill={GL} stroke={G} strokeWidth="2.5" />
        <circle cx="184" cy="46" r="12" fill={RT} stroke={R} strokeWidth="2.5" />
        <circle cx="90" cy="124" r="10" fill={S} stroke={INK} strokeWidth="2.5" />
        <circle cx="170" cy="118" r="11" fill={GL} stroke={G} strokeWidth="2.5" />
        <g className="pulse">
          <circle cx="120" cy="84" r="17" fill={G} stroke={INK} strokeWidth="2.5" />
        </g>
      </>
    ),
  },
  lost: {
    vb: "0 0 240 160",
    body: (
      <>
        <ellipse cx="120" cy="146" rx="96" ry="8" fill={GL} />
        <g className="float">
          <Page x={84} y={24} w={72} h={96} accent={R} lines={3} />
          <text x="120" y="106" textAnchor="middle" fontFamily="var(--font-display)" fontSize="26" fontWeight="700" fill={INK}>?</text>
        </g>
        <circle cx="54" cy="56" r="7" fill={A} />
        <circle cx="190" cy="40" r="5" fill={GM} />
      </>
    ),
  },
  hero: {
    vb: "0 0 520 420",
    body: (
      <>
        <ellipse cx="260" cy="392" rx="220" ry="16" fill={GL} />
        <circle cx="400" cy="90" r="64" fill={S} />
        <circle cx="64" cy="356" r="40" fill={RT} />
        {/* Earlier reading */}
        <g transform="translate(0 40)">
        <g className="float-2">
          <rect x="44" y="70" width="190" height="240" rx="18" fill={W} stroke={INK} strokeWidth="3" />
          <rect x="66" y="94" width="88" height="12" rx="6" fill={R} />
          <text x="66" y="134" fontFamily="var(--font-sans)" fontSize="13" fontWeight="700" fill="#475569">EARLIER READING</text>
          <rect x="66" y="150" width="146" height="8" rx="4" fill={LINE} />
          <rect x="66" y="168" width="130" height="8" rx="4" fill={LINE} />
          <rect x="60" y="182" width="158" height="26" rx="6" fill={RT} />
          <rect x="66" y="191" width="138" height="8" rx="4" fill={R} opacity="0.7" />
          <rect x="66" y="222" width="146" height="8" rx="4" fill={LINE} />
          <rect x="66" y="240" width="96" height="8" rx="4" fill={LINE} />
        </g>
        </g>
        {/* Current reading */}
        <g className="float">
          <rect x="288" y="150" width="190" height="240" rx="18" fill={W} stroke={INK} strokeWidth="3" />
          <rect x="310" y="174" width="88" height="12" rx="6" fill={G} />
          <text x="310" y="214" fontFamily="var(--font-sans)" fontSize="13" fontWeight="700" fill="#475569">READING NOW</text>
          <rect x="310" y="230" width="146" height="8" rx="4" fill={LINE} />
          <rect x="304" y="246" width="158" height="26" rx="6" fill={GL} />
          <rect x="310" y="255" width="128" height="8" rx="4" fill={G} opacity="0.7" />
          <rect x="310" y="286" width="146" height="8" rx="4" fill={LINE} />
          <rect x="310" y="304" width="112" height="8" rx="4" fill={LINE} />
          <rect x="310" y="322" width="140" height="8" rx="4" fill={LINE} />
        </g>
        {/* The suggested link */}
        <path className="draw" d="M218 235 C 262 200, 252 300, 304 259" stroke={G} strokeWidth="4" fill="none" strokeLinecap="round" />
        <circle cx="218" cy="235" r="8" fill={R} stroke={INK} strokeWidth="2.5" />
        <circle cx="304" cy="259" r="8" fill={G} stroke={INK} strokeWidth="2.5" />
        {/* The learner's question */}
        <g className="float">
          <path d="M262 60 h170 a14 14 0 0 1 14 14 v32 a14 14 0 0 1 -14 14 h-62 l-16 16 v-16 h-92 a14 14 0 0 1 -14 -14 v-32 a14 14 0 0 1 14 -14 z" fill={A} stroke={INK} strokeWidth="3" strokeLinejoin="round" />
          <text x="354" y="96" textAnchor="middle" fontFamily="var(--font-sans)" fontSize="16" fontWeight="800" fill={INK}>How are they linked?</text>
        </g>
      </>
    ),
  },
};

interface IllustrationProps {
  name: IllustrationName;
  width?: number | string;
  title?: string;
  className?: string;
}

export function Illustration({ name, width = 200, title, className }: IllustrationProps) {
  const art = ART[name];
  const labelled = Boolean(title);
  return (
    <svg
      viewBox={art.vb}
      width={width}
      className={`ui-illo${className ? ` ${className}` : ""}`}
      role={labelled ? "img" : undefined}
      aria-hidden={labelled ? undefined : true}
      aria-label={title}
      focusable="false"
    >
      {art.body}
    </svg>
  );
}

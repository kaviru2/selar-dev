// Icon.tsx — SELAR shared SVG icon set.
// Inline SVG icons rendered at configurable size. Stroke-based, 14px default.
// Decorative (aria-hidden); pair with visible text or an aria-label.
// Ported from design_handoff_selar/design_refs/icons.jsx.

import type { CSSProperties } from "react";

export type IconName =
  | "check"
  | "x"
  | "tag"
  | "link"
  | "chevron_down"
  | "chevron_right"
  | "chevron_left"
  | "plus"
  | "upload"
  | "search"
  | "book"
  | "doc"
  | "graph"
  | "quiz"
  | "settings"
  | "flag"
  | "arrow_right"
  | "sparkles"
  | "filter"
  | "bolt"
  | "kbd"
  | "drive"
  | "more"
  | "spinner"
  | "refresh"
  | "clock"
  | "note"
  | "highlight"
  | "zoom_in"
  | "zoom_out"
  | "eye"
  | "trash"
  | "info"
  | "menu"
  | "logout"
  | "chat"
  | "bulb"
  | "scale"
  | "shield"
  | "users"
  | "pen"
  | "history"
  | "external";

interface IconProps {
  name: IconName;
  size?: number;
  style?: CSSProperties;
  className?: string;
}

const p = {
  fill: "none",
  stroke: "currentColor",
  strokeWidth: 1.5,
  strokeLinecap: "round" as const,
  strokeLinejoin: "round" as const,
};

export function Icon({ name, size = 14, style, className }: IconProps) {
  const s: CSSProperties = {
    width: size,
    height: size,
    display: "inline-block",
    verticalAlign: "middle",
    ...style,
  };

  const paths: Record<IconName, React.ReactNode> = {
    check: <path {...p} d="M2.5 8l3.5 3.5L13 4.5" />,
    x: <path {...p} d="M3.5 3.5l9 9M12.5 3.5l-9 9" />,
    tag: (
      <>
        <path {...p} d="M2.5 2.5h5l6 6-5 5-6-6v-5z" />
        <circle cx="5" cy="5" r="1" {...p} />
      </>
    ),
    link: (
      <path
        {...p}
        d="M6 10a3 3 0 003 3l2-2a3 3 0 000-4M10 6a3 3 0 00-3-3L5 5a3 3 0 000 4"
      />
    ),
    chevron_down: <path {...p} d="M3.5 5.5L8 10l4.5-4.5" />,
    chevron_right: <path {...p} d="M5.5 3.5L10 8l-4.5 4.5" />,
    chevron_left: <path {...p} d="M10.5 3.5L6 8l4.5 4.5" />,
    plus: <path {...p} d="M8 3v10M3 8h10" />,
    upload: (
      <>
        <path {...p} d="M8 11V3M5 6l3-3 3 3" />
        <path
          {...p}
          d="M3 11v1.5A1.5 1.5 0 004.5 14h7a1.5 1.5 0 001.5-1.5V11"
        />
      </>
    ),
    search: (
      <>
        <circle cx="7" cy="7" r="4" {...p} />
        <path {...p} d="M10 10l3 3" />
      </>
    ),
    book: <path {...p} d="M3 3.5v10l5-1 5 1v-10L8 4.5z M8 4.5v9" />,
    doc: <path {...p} d="M4 2h5l3 3v9H4zM9 2v3h3" />,
    graph: (
      <>
        <circle cx="4" cy="4" r="1.5" {...p} />
        <circle cx="12" cy="5" r="1.5" {...p} />
        <circle cx="8" cy="12" r="1.5" {...p} />
        <path {...p} d="M5 5l2 6M12 6.5l-3 4.5" />
      </>
    ),
    quiz: <path {...p} d="M5 5a2 2 0 114 0c0 1-2 1.5-2 3M7 11h.01" />,
    settings: (
      <>
        <circle cx="8" cy="8" r="2" {...p} />
        <path
          {...p}
          d="M8 1v2M8 13v2M1 8h2M13 8h2M3 3l1.5 1.5M11.5 11.5L13 13M3 13l1.5-1.5M11.5 4.5L13 3"
        />
      </>
    ),
    flag: <path {...p} d="M3 13V2.5M3 3h9l-2 3 2 3H3" />,
    arrow_right: <path {...p} d="M3 8h10M9 4l4 4-4 4" />,
    sparkles: (
      <path
        {...p}
        d="M5 2v3M3.5 3.5h3M11 8v4M9 10h4M4 10l1 2 2 1-2 1-1 2-1-2-2-1 2-1z"
      />
    ),
    filter: <path {...p} d="M2 3h12l-4.5 6v4l-3 2v-6z" />,
    bolt: <path {...p} d="M9 2L4 9h4l-1 5 5-7H8z" />,
    kbd: (
      <>
        <rect x="2" y="5" width="12" height="7" rx="1" {...p} />
        <path {...p} d="M5 8h.01M8 8h.01M11 8h.01" />
      </>
    ),
    drive: (
      <path
        {...p}
        d="M5.5 3h5l3.5 6-2.5 4H3.5L1 9z M5.5 3L1 9M10.5 3l-5 6h8"
      />
    ),
    more: <path {...p} d="M4 8h.01M8 8h.01M12 8h.01" />,
    spinner: (
      <path
        {...p}
        d="M8 2v2M8 12v2M3 8H1M15 8h-2M3.5 3.5l1.5 1.5M11 11l1.5 1.5M3.5 12.5L5 11M11 5l1.5-1.5"
      />
    ),
    refresh: <path {...p} d="M13 5V2.5l-2 2A5.5 5.5 0 104 12M13 2.5h-2.5" />,
    clock: (
      <>
        <circle cx="8" cy="8" r="6" {...p} />
        <path {...p} d="M8 5v3l2 2" />
      </>
    ),
    note: <path {...p} d="M3 3h10v7l-3 3H3zM10 13v-3h3" />,
    highlight: <path {...p} d="M4 10l5-7 3 3-5 7-4 1z M3 14h8" />,
    zoom_in: (
      <>
        <circle cx="7" cy="7" r="4" {...p} />
        <path {...p} d="M10 10l3 3M7 5v4M5 7h4" />
      </>
    ),
    zoom_out: (
      <>
        <circle cx="7" cy="7" r="4" {...p} />
        <path {...p} d="M10 10l3 3M5 7h4" />
      </>
    ),
    eye: (
      <>
        <path
          {...p}
          d="M1 8s2.5-5 7-5 7 5 7 5-2.5 5-7 5-7-5-7-5z"
        />
        <circle cx="8" cy="8" r="2" {...p} />
      </>
    ),
    trash: (
      <>
        <path {...p} d="M4.5 4v10a1 1 0 001 1h5a1 1 0 001-1V4" />
        <path {...p} d="M3 4h10 M6 4v-1.5a1 1 0 011-1h2a1 1 0 011 1V4" />
        <path {...p} d="M7 6v6 M9 6v6" />
      </>
    ),
    info: (
      <>
        <circle cx="8" cy="8" r="6" {...p} />
        <path {...p} d="M8 7.5v3.5M8 5h.01" />
      </>
    ),
    menu: <path {...p} d="M2.5 4h11M2.5 8h11M2.5 12h11" />,
    logout: <path {...p} d="M6 2.5H3.5v11H6M10 5l3 3-3 3M13 8H6.5" />,
    chat: <path {...p} d="M2.5 3.5h11v7h-6l-3 2.5v-2.5h-2z" />,
    bulb: <path {...p} d="M6 12h4M6.5 14h3M8 2a4 4 0 00-2.4 7.2c.5.4.9 1 .9 1.8h3c0-.8.4-1.4.9-1.8A4 4 0 008 2z" />,
    scale: <path {...p} d="M8 2v12M5 14h6M3 4.5h10M3 4.5L1.5 8.5a1.5 1.5 0 003 0zM13 4.5l-1.5 4a1.5 1.5 0 003 0z" />,
    shield: <path {...p} d="M8 1.8l5 2v4c0 3.2-2.2 5.3-5 6.4-2.8-1.1-5-3.2-5-6.4v-4z M5.8 8l1.6 1.6L10.5 6.5" />,
    users: (
      <>
        <circle cx="6" cy="5.5" r="2.2" {...p} />
        <path {...p} d="M1.8 13.5c.4-2.4 2.1-3.7 4.2-3.7s3.8 1.3 4.2 3.7M10.5 3.6a2.2 2.2 0 010 4.1M12 9.9c1.2.5 2 1.7 2.2 3.6" />
      </>
    ),
    pen: <path {...p} d="M10.5 2.5l3 3-8 8H2.5v-3z M9 4l3 3" />,
    history: <path {...p} d="M2.5 8a5.5 5.5 0 101.6-3.9M2.5 2.5v2.5H5M8 5v3l2 1.5" />,
    external: <path {...p} d="M9 2.5h4.5V7M13.5 2.5L7.5 8.5M11.5 9.5v4h-9v-9h4" />,
  };

  return (
    <svg viewBox="0 0 16 16" style={s} className={className} aria-hidden="true" focusable="false">
      {paths[name]}
    </svg>
  );
}

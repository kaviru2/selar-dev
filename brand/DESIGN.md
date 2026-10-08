---
version: alpha
name: SELAR
description: Friendly, careful research tool. A small linnet carries a page between what you read now and what you read before; you decide whether the link holds.
colors:
  primary: "#1F5135"
  primary-mid: "#5B8A6F"
  ink: "#0F172A"
  slate: "#475569"
  slate-strong: "#334155"
  mist: "#F1F5F9"
  paper: "#FFFDF8"
  white: "#FFFFFF"
  sky: "#E1EDFB"
  rust: "#B4532A"
  rust-ink: "#9A4322"
  blush: "#FBEDE6"
  amber: "#D97706"
  sun: "#F6C453"
typography:
  display:
    fontFamily: Fraunces
    fontSize: 3rem
    fontWeight: 700
    lineHeight: 1.05
    letterSpacing: "-0.015em"
    fontVariation: "'SOFT' 100, 'opsz' 72"
  h1:
    fontFamily: Fraunces
    fontSize: 2.25rem
    fontWeight: 650
    lineHeight: 1.1
    letterSpacing: "-0.01em"
    fontVariation: "'SOFT' 100"
  h2:
    fontFamily: Fraunces
    fontSize: 1.5rem
    fontWeight: 650
    lineHeight: 1.2
  body-md:
    fontFamily: Figtree
    fontSize: 1rem
    fontWeight: 400
    lineHeight: 1.55
  body-sm:
    fontFamily: Figtree
    fontSize: 0.875rem
    fontWeight: 400
    lineHeight: 1.5
  label:
    fontFamily: Figtree
    fontSize: 0.75rem
    fontWeight: 700
    lineHeight: 1.3
    letterSpacing: "0.08em"
  mono:
    fontFamily: JetBrains Mono
    fontSize: 0.8125rem
    fontWeight: 400
    lineHeight: 1.5
rounded:
  sm: 6px
  md: 12px
  lg: 20px
  pill: 999px
spacing:
  xs: 4px
  sm: 8px
  md: 16px
  lg: 24px
  xl: 40px
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.white}"
    typography: "{typography.body-md}"
    rounded: "{rounded.pill}"
    padding: 12px 20px
  button-primary-hover:
    backgroundColor: "{colors.ink}"
    textColor: "{colors.white}"
  button-secondary:
    backgroundColor: "{colors.white}"
    textColor: "{colors.primary}"
    rounded: "{rounded.pill}"
    padding: 12px 20px
  prototype-pill:
    backgroundColor: "{colors.blush}"
    textColor: "{colors.rust-ink}"
    typography: "{typography.label}"
    rounded: "{rounded.pill}"
    padding: 6px 12px
  link-card:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.ink}"
    rounded: "{rounded.md}"
    padding: 16px
  link-card-meta:
    backgroundColor: "{colors.mist}"
    textColor: "{colors.slate}"
    typography: "{typography.body-sm}"
  callout-info:
    backgroundColor: "{colors.sky}"
    textColor: "{colors.ink}"
    rounded: "{rounded.md}"
    padding: 16px
  thread-link:
    backgroundColor: "{colors.white}"
    textColor: "{colors.rust}"
  illustration-fill:
    backgroundColor: "{colors.primary-mid}"
    textColor: "{colors.ink}"
  callout-warning:
    backgroundColor: "{colors.blush}"
    textColor: "{colors.slate-strong}"
    rounded: "{rounded.md}"
    padding: 16px
  on-dark-surface:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.paper}"
  badge-kept:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.white}"
    rounded: "{rounded.pill}"
  highlight-decorative:
    backgroundColor: "{colors.sun}"
    textColor: "{colors.ink}"
    rounded: "{rounded.sm}"
  badge-amber:
    backgroundColor: "{colors.amber}"
    textColor: "{colors.ink}"
    rounded: "{rounded.pill}"
---

## Overview

SELAR (Semantic Linking for Active Retention) is a research prototype PDF reader from UCSC IS4101 Group 04. While you read, it suggests a possible link to something you read earlier, shows both source passages, and asks you to explain, compare and decide (keep, change or reject).

The brand has one principle: **AI suggests, sources show, you explain and decide.** The visual identity should feel like a well-kept study notebook with a cheerful companion: warm, curious and tidy, never hyped. Students should smile at it; an evaluation panel should trust it.

- **Primary mark:** Linny the linnet, a round green bird holding a small page in its beak. The page is the "earlier reading" Linny is bringing back to you.
- **Wordmark:** lowercase `selar` in Fraunces (SOFT 100, weight 650), outlined to paths. The descriptor "Semantic Linking for Active Retention" is set in Figtree.
- **Alternative directions kept in the kit:** B "Page Bridge" (two pages and a thread) and C "Knot S" (a thread tied into an S). Swap only as a whole system.
- **Status:** always label SELAR a *research prototype*. Never claim it improves memory, retention, grades or learning; the retention study is planned, not run.

## Colors

Continuity with the illustrated presentation deck, plus two additions (`paper`, `sun`) for warmth.

- **Primary / Forest (#1F5135):** Linny's body, primary buttons, dark surfaces. White on forest = 9.2:1.
- **Primary-mid / Moss (#5B8A6F):** illustration fills, chart series, Linny's wing. White on moss is 3.95:1: large text (24 px+) only. Use forest behind small white text.
- **Ink (#0F172A):** body text and outlines. 16.3:1 on mist.
- **Slate (#475569):** secondary text. 7.6:1 on white; 6.9:1 on mist. Use **slate-strong (#334155)** on tints (9.0:1 on blush).
- **Mist (#F1F5F9) and Paper (#FFFDF8):** page backgrounds. Paper is for reading surfaces and print.
- **Sky (#E1EDFB):** info callouts and illustration blobs.
- **Rust (#B4532A):** the thread. Links and focus accents; 5.0:1 on white. On blush it drops to 4.4:1, so small text there uses **rust-ink (#9A4322)**, 5.7:1.
- **Blush (#FBEDE6):** rust tint for the prototype pill and gentle warnings.
- **Amber (#D97706):** Linny's beak and feet; badges with ink text (5.6:1). Never amber text on light backgrounds.
- **Sun (#F6C453):** decorative highlights and celebration sparkles only. Never as text on light backgrounds.

## Typography

All fonts are SIL Open Font License, from Google Fonts.

- **Fraunces** (display): headlines, the wordmark, big numbers. Use the SOFT axis at 100 for the friendly, rounded serif look. Do not use for body text.
- **Figtree** (UI and body): everything people read at length, labels, buttons.
- **JetBrains Mono** (code/metadata): page refs, IDs, similarity scores in debug views.

The console currently loads Inter / Source Serif 4 / JetBrains Mono. The redesign may adopt Fraunces + Figtree via `next/font/google`; until then the logo is outlined SVG and does not depend on installed fonts.

## Layout

8 px base grid. Generous margins: posters use 46 px at A4 (96 dpi), social at 80 px on a 1080 canvas. One idea per surface. Linny takes no more than about a third of any layout and never covers text.

## Elevation & Depth

Flat with ink outlines (3.5 px at the 100-unit mark scale) and soft tinted circles behind illustrations. Shadows, if used, are a single soft drop (`0 8px 24px rgba(15,23,42,.08)`). No gradients on the logo.

## Shapes

Rounded, friendly geometry: pills for actions and status, 12 px cards, circles for illustration backdrops. The "page" motif is a rounded rectangle with a folded corner and two text lines. The "thread" is a rust curve with round caps; dashed when a link is uncertain.

## Components

- `button-primary` is the only high-emphasis action per view.
- `prototype-pill` ("Research prototype · feedback welcome") appears on every marketing surface and the landing page.
- `link-card` shows a suggested link: both passages side by side, each with its source and page.
- Linny mascot poses (React `Linny` component, `pose` prop): `hello` (onboarding), `reading` (loading/processing), `linking` (a suggestion is ready), `thinking` (explain prompt), `celebrating` (a link was kept), `questioning` (weak or rejected link; gentle, not scolding), `empty` (nothing yet).

## Do's and Don'ts

- Do say "possible link", "suggests", "you decide". Don't say "perfect match" or "AI knows".
- Do write "research prototype, feedback welcome". Don't promise study participation, payment or results.
- Do cite the formative survey only as: 38 eligible respondents; 31/36 wanted linking; 28/36 worried about AI accuracy; 24/36 worried about less active thinking.
- Do say the design is grounded in research on depth of processing, self-explanation, comparison and retrieval practice. Don't claim SELAR improves memory, retention, grades or learning.
- Do keep clear space around the mark equal to the page height in Linny's beak (about 25% of the icon). Don't rotate, recolour outside the palette, outline, or add effects to the logo.
- Do use the mono versions on photos or single-colour print. Don't place the colour mark on busy images.
- Minimum size: mark 24 px (use the favicon drawing below that); lockup 120 px wide.
- No testimonials, user counts, partner logos, prices or incentives.

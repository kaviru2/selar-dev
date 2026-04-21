# Handoff: SELAR — Semantic Linking for Active Retention

## Overview

SELAR is a web application that behaves like a lightweight PDF reader with a right-side "matches" panel. When a user uploads a document, the system chunks and embeds it, compares it against everything else the user has read, and surfaces candidate semantic links. The user then confirms, rejects, or relabels each suggestion — every confirmation is a retrieval-practice event and is the pedagogical intervention under study.

The build is a **Next.js 15 (App Router, TypeScript)** frontend + **Go 1.22+ (Chi)** backend + **Postgres 16 w/ pgvector**. This handoff covers the **frontend only** — the schema, pipelines and API surface live in the implementation plan (SELAR-Implementation-Plan.md) that accompanies this project.

## About the design files

The files in `design_refs/` are **design references created in HTML + React (Babel in-browser)**. They are prototypes showing intended look and behaviour — **not production code to lift directly**. Your job is to recreate these designs in the real Next.js app using the patterns already established there (Tailwind + shadcn/ui + react-pdf per the implementation plan). The JSX files are close to the final component shape and can be ported with minimal structural change, but:

- Inline `<script type="text/babel">` → real TSX files with proper imports
- `window.xxx` globals → ES module exports
- Mock `sampleSuggestions` / `LibraryList` → typed fetch from the Go API
- Inline `<span class="hl-suggest">` overlays → real `OverlayLayer` driven by `lib/pdfCoords.ts` (§5.4 of the plan)

## Fidelity

**High-fidelity.** Colors, typography, spacing, hover/active states, iconography and micro-interactions are finalised. Recreate pixel-accurately, using the codebase's existing primitives (shadcn button/dialog/tooltip) where they match.

---

## Views (8 total)

### 1. Topbar (chrome, on every logged-in page)

- 40px tall, `border-bottom: 1px solid var(--rule)`
- Left: geometric brand-mark (16px, two quadrants — solid ink + rust fill) + "SELAR" wordmark (13px, 600)
- Nav: Library · Reader · Graph · Quiz · Settings (11.5px, 4px 10px padding, active = `--bg-2` background)
- Right: keyboard-hint (`J K navigate · Y confirm`), **cohort chip** (mono 10px uppercase, dot + label, border tinted by cohort), avatar (22px circle)
- Cohort colors: Control `#9a938a`, Auto `#7a8c5c` (sage), HITL `#c96442` (rust)

### 2. Library view

Dashboard-style. Header row: title "Library" + subtitle counts + three buttons (Filter, From Drive, Upload PDF primary).
- **Stats grid** — 4 cards, `grid-template-columns: repeat(4, 1fr)`: Documents, Confirmed links, Reading time, Retention (predicted). Each is `padding: 12px 14px`, mono uppercase 10px label, 22px 600 value, 10.5px delta.
- **Doc table** — custom grid (`22px 1fr 120px 80px 90px 120px 60px 32px`), 7px vertical padding, status dot + title + authors + year + pages + `chunks · links` mono + status pill (`ready` = sage mono, `processing` = progress bar + %). Header row has uppercase mono-10 labels.

### 3. Reader — the hero

Three-column layout inside a flex row (`.reader`): **Sidebar (240px) · Doc pane (fill) · Matches panel (360px)**.

**Sidebar** (`.sidebar`):
- Header: "LIBRARY" label + count + `+` upload icon button
- Search field with `⌘K` kbd hint
- Group label ("ML FUNDAMENTALS · CORPUS")
- Item rows: status dot (sage ready / rust pulsing processing / red failed) + title ellipsis + mono meta (`First-author et al. · year · N links`)
- Footer pinned bottom: Drive sync indicator (sage drive icon + "synced 2m ago")

**Doc pane** (`.doc-pane`):
- Sticky top doc toolbar — page nav, zoom −/+, marks toggle, search/note/more
- PDF page mock: **820px wide**, cream `#fdfcf9`, `padding: 72px 96px 96px`, Source Serif 4 body @ 14.5px / 1.55, justify + pretty wrap + hyphens
- Page chrome: running header (mono 10px, journal name + page num), title (21px 600), authors (12px sans), section h2 (15.5px), equations centered italic with `(N)` eq-num floated right
- Figure placeholder: 130px tall, 45° stripe pattern, mono caption
- **Highlight classes**:
  - `.hl` (user wheat highlight) — `rgba(223,193,121,0.42)` background
  - `.hl-suggest` (AI candidate) — `rgba(201,100,66,0.12)` bg + `inset 0 -1px 0 rgba(201,100,66,0.55)` underline; `.active` deepens both + shows inline pin (`.pin` — mono 9px rust pill bottom-right of span)
- Overlay is for demo **only**; production uses absolute-positioned `<div>`s over the pdf.js canvas computed from PDF-point bboxes (see plan §5.4).

**Matches panel** — see next section (5 variants).

### 4. Matches panel (5 variants — the core variation)

All share the header: title (`Matches` or `Applied links`) + pending/linked chips + segmented toggle (cards/diff/feed/keyboard).

**A · Cards** (default, hero, what the plan assumes): `.match-card` with mono row1 (similarity % in rust + sim-bar + doc title ellipsis), serif 12.5px snippet, footer with Confirm (ink-filled) / Reject / Label buttons + page number mono right-aligned. Confirmed state: sage border + faint sage bg + footer replaced with `✓ confirmed · related_to`. Rejected: `opacity: 0.48`.

**B · Diff** — `grid-template-columns: 1fr 1fr` on `.match-diff`, left col has rust left-border ("this doc" + src snippet), right col has rule left-border (source label + tgt snippet). Compact footer: Link / Dismiss.

**C · Feed** — `.feed-card` removes card padding, becomes bottom-bordered row. Tight footer buttons (`✓ ✗ label`) + mono kbd hint `J K next`.

**D · Keyboard** — single active card in `--bg-2`, a progress bar at top (`7/10 reviewed`), active snippet in serif 13.5px, mono arrow to target `→ doc · §`, action row: `Y Yes, link` (ink) / `N No` / `L Label` / `K skip` (ghost, pushed right). Below: "QUEUE" heading + 4 dashed-divided mono rows.

**E · Popover** — inline floating card (340px) anchored below a clicked passage. Same Confirm/Reject/Label actions + `1 of N` mono index + `×` close. Matches panel is hidden or collapsed in this variant.

### 5. Knowledge graph view

`flex-row`: main canvas (`--bg`) + right detail pane (300px).

- Canvas: SVG force-graph-style — circles sized by importance, stroked in concept-origin color (umber = prerequisite, rust = core, sage = extension), labels sans 11.5 below node. Selected node: solid fill + 600 label.
- Edges: thin rules with arrowhead marker; selected node highlights incident edges in rust + shows mono relation label at midpoint.
- Legend bottom-left: small bg card with three color swatches.
- Detail pane: "CONCEPT" uppercase mono label, node title h2 14, description paragraph, key-value grid (`90px 1fr` — Sources / Confirmed / Pending / Last seen), relations list below a rule-top divider — each row has a pill-shaped relation label tinted rust + direction arrow + peer name.

Production: swap the SVG for `react-force-graph-2d`; keep the same data shape.

### 6. Retention quiz

Centered `max-width: 720px`.
- Phase chip top (`Post-test · Day 15`, rust pill)
- Title h1 22/600, description
- Progress segments: 24 × 3px tall bars, sage for done, rust for current
- Question card: mono meta (`Question 08 · Multiple choice · 1 correct`), serif 17px prompt (supports `<cloze>` span — 120px min-width with 1.5px ink underline), options as `.q-option` (10-12px padding, hover `--bg-2`, selected = ink border + `--bg-2` bg, letter A/B/C/D mono), concept tags mono-10 pills
- Foot: Previous · time remaining (mono with clock icon) · Next primary

### 7. Onboarding

Two-column split (50/50).
- Left (`padding: 60px 72px`): big brand, h1 36/600 welcome, 15px lede, 4-step list with mono step numbers — `.step.done` strikes-through label and turns step number sage. CTA button on the active step only.
- Right (`--bg-2`): cohort card (`340px`) — mono uppercase label tinted by cohort, 18px name, serif-ish description, horizontal stats row (reading/day, days active, quizzes). Below: mono disclaimer about random assignment.

### 8. Settings

Centered `max-width: 720px`, sectioned by rule-top. Each row is `grid-template-columns: 200px 1fr`. Account · Study (cohort toggle + day counter bar) · Appearance (theme + density toggles) · Privacy (export/withdraw). Toggle component: mono 10.5 pills in a rounded-12 `--bg-2` container, active pill = `--bg` + `--shadow-1`.

---

## Interactions & state

- **Cohort gates the whole right rail** — the single check `cohort !== 'control'` in `ReaderView` decides whether the MatchesPanel renders. `cohort === 'treatment_auto'` flips every suggestion to `confirmed` at mount and renders the panel read-only (no confirm/reject buttons, just `✓ linked · relation` status rows).
- **Passage click → suggestion focus** — clicking any `.hl-suggest` sets `activeSuggestId`, matches panel re-filters to that chunk's candidates. In `popover` style, also anchor a floating popover to `getBoundingClientRect()` of the passage.
- **Confirm/Reject** — optimistic local state update + `POST /api/suggestions/:id/respond` with `{ action, label?, time_to_respond_ms }`. Log a PostHog `link_confirmed` event with suggestion_id, similarity, doc_id.
- **Keyboard variant** — bind `y / n / l / k / j` globally while matches panel has focus. Advance queue after each action.
- **Zoom** — CSS `transform: scale()` on page wrapper in mock; in production, call `pdfjs` viewport.clone({ scale }) and re-render canvas + recompute every overlay via `pdfToScreen(bbox, viewport)` from `lib/pdfCoords.ts`.
- **Theme** — four themes via body class: default (paper), `theme-warm`, `theme-sage`, `theme-dark`. Persist to `user.preferences` + swap via `data-theme` on `<html>`.
- **Tweaks panel** — demo only. Remove for production.

## State management

Per plan §6, fetch via typed `api.ts`. Minimal global state (Zustand or React Context is fine):
- `user: { id, email, cohort }` — hydrated from Clerk publicMetadata
- `activeDocId`, `activePage`, `activeSuggestId`, `activeChunkId`
- Per-doc: `chunks`, `annotations`, `suggestions` — fetched on page change, cached by `(docId, page)`
- Mutations: `respondToSuggestion(id, action)`, `createAnnotation(...)`, `updateAnnotation(id, patch)`

Don't cache suggestions globally across documents — refetch on doc switch. Page-level fetch (`?page=N`) keeps payloads small.

---

## Design tokens

### Colors (CSS custom properties — see `styles.css`)

| Token | Light default | Dark |
|---|---|---|
| `--bg` | `#faf9f7` | `#17161a` |
| `--bg-2` | `#f3f1ec` | `#1e1d22` |
| `--bg-3` | `#ebe8e1` | `#26252b` |
| `--ink` | `#1a1816` | `#ece8e0` |
| `--ink-2` | `#3a3633` | `#c9c4ba` |
| `--ink-3` | `#6b655e` | `#8a857c` |
| `--ink-4` | `#9a938a` | `#5a554e` |
| `--rule` | `#e4e0d8` | `#2c2a30` |
| `--rule-2` | `#d4cfc4` | `#3a373f` |
| `--accent` (rust) | `#c96442` | `#e07a56` |
| `--accent-2` (sage) | `#7a8c5c` | `#8fa76a` |
| `--accent-3` (umber) | `#8a6a3d` | `#b58a57` |

Annotation colors (same in both): `wheat rgba(223,193,121,0.42)` · `coral rgba(220,140,120,0.40)` · `sage rgba(140,165,120,0.38)` · `yellow rgba(240,215,110,0.48)`.

Two extra themes in `styles.css`: `.theme-warm` (more cream) and `.theme-sage` (green-shifted accent).

### Typography

- Sans: **Inter** 400/500/600/700 — UI
- Serif: **Source Serif 4** 400/500/600 — PDF body + long-form reading (snippets)
- Mono: **JetBrains Mono** 400/500 — labels, metadata, keyboard hints

Sizes: `--t-xs 10.5` · `--t-sm 11.5` · `--t-md 13` · `--t-lg 15`. PDF body 14.5/1.55 serif. Section h2 15.5. Card title 22. Onboarding h1 36/-0.03em.

### Spacing / radius / shadow

- Radius scale: `--r-xs 2` · `--r-sm 3` · `--r-md 5` · `--r-lg 8`
- Shadow: `--shadow-1` subtle chip · `--shadow-2` card lift · `--shadow-3` popover

### Density

Compact by default (read the `:root` values). If implementing balanced/spacious, scale padding on `.match-card`, `.doc-row`, `.sb-item`, `.q-option` by 1.25× / 1.5×.

---

## Assets

No bitmap assets. All icons are inline SVG in `icons.jsx` (23 icons — `check, x, tag, link, chevron_*, plus, upload, search, book, doc, graph, quiz, settings, flag, arrow_right, sparkles, filter, bolt, kbd, drive, more, spinner, clock, note, highlight, zoom_in, zoom_out, eye`). Port these as a single `<Icon name="...">` React component or adopt lucide-react equivalents — both work.

Fonts loaded via Google Fonts CDN (`Inter`, `Source Serif 4`, `JetBrains Mono`). In production self-host via `next/font` for CLS + privacy.

---

## Files in `design_refs/`

| File | Purpose |
|---|---|
| `SELAR.html` | Full clickable prototype entry point — open this first |
| `SELAR Canvas.html` | Design canvas with all variants & dark mode side-by-side |
| `styles.css` | **All design tokens + component CSS** — single source of truth |
| `app.jsx` | `SelarApp` shell — view switching, cohort state, theme, Tweaks |
| `topbar.jsx` | Topbar + brand mark + cohort chip |
| `sidebar.jsx` | Library sidebar + mock `LibraryList` shape |
| `reader.jsx` | PDF page mock + doc toolbar + `sampleSuggestions` shape |
| `matches-panel.jsx` | **All 5 match panel variants** — MatchesCards / MatchesDiff / MatchesFeed / MatchesKeyboard + popover |
| `library-view.jsx` | Library dashboard + doc table |
| `graph-view.jsx` | Knowledge graph SVG (swap for react-force-graph-2d) |
| `quiz-view.jsx` | Retention quiz |
| `other-views.jsx` | OnboardingView + SettingsView |
| `icons.jsx` | Inline SVG icon set |
| `paper-content.jsx` | Fake ML paper content for the reader mock |
| `design-canvas.jsx` | Canvas shell (dev-time only, don't port) |

---

## Port checklist (in recommended order)

1. Drop `styles.css` into `apps/web/app/globals.css`. Rename custom props if your codebase already uses a design-token convention, or expose via Tailwind `@theme`.
2. Port `icons.jsx` → `components/ui/Icon.tsx` OR replace with `lucide-react`.
3. Port `Topbar` + `Sidebar` — simplest, smallest surface.
4. Stub the Reader with a real `react-pdf` canvas and `lib/pdfCoords.ts` (copy from plan §5.4 literally — it is the production code).
5. Port `MatchesPanel` with **Cards** variant first; others are flagged behind a prop and can ship later or gate behind an internal Tweaks drawer for participant-blind A/B sanity checks.
6. Cohort state flows from Clerk `publicMetadata.cohort` → React context at the `(app)/layout.tsx` level → consumed in `ReaderView`.
7. Library / Graph / Quiz / Onboarding / Settings — straight port.
8. Dark mode — toggle `data-theme="dark"` on `<html>`; all tokens already switch.

---

## One thing not to get wrong

**Coordinates.** Every bbox stored and returned by the Go API must be in PDF points with `"unit": "pdf_points"`. Every overlay in the frontend goes through `pdfToScreen(bbox, viewport)` — never compute inline. This is the only reason highlights survive zoom and cross-device sync. Plan §5.4 has the full helper + rationale.
